// Package support implements customer support threads: general inquiries and order or request
// linked help, with attachments, staff assignment, internal notes and unread counts.
package support

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/access"
	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/customers"
	"github.com/kuyamcliff/tailor-website/backend/internal/notifications"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

var Categories = map[string]string{"general": "General inquiry", "order_issue": "Order issue", "measurement_help": "Measurement help",
	"payment_issue": "Payment issue", "appointment_issue": "Appointment issue", "alteration_request": "Alteration request", "other": "Other"}

type Handler struct {
	Pool     *pgxpool.Pool
	Settings *settings.Service
}

type Attachment struct {
	ID    uuid.UUID `json:"id"`
	URL   string    `json:"url"`
	Thumb string    `json:"thumb"`
}

type Message struct {
	ID          uuid.UUID    `json:"id"`
	AuthorType  string       `json:"authorType"`
	Author      *string      `json:"author"`
	Body        string       `json:"body"`
	Internal    bool         `json:"internal"`
	Attachments []Attachment `json:"attachments"`
	CreatedAt   time.Time    `json:"createdAt"`
}

type Thread struct {
	ID            uuid.UUID  `json:"id"`
	Number        string     `json:"number"`
	CustomerID    uuid.UUID  `json:"customerId"`
	CustomerName  string     `json:"customerName"`
	Subject       string     `json:"subject"`
	Category      string     `json:"category"`
	Status        string     `json:"status"`
	Priority      string     `json:"priority"`
	AssignedTo    *uuid.UUID `json:"assignedTo"`
	AssigneeName  *string    `json:"assigneeName"`
	OrderID       *uuid.UUID `json:"orderId"`
	OrderNumber   *string    `json:"orderNumber"`
	RequestID     *uuid.UUID `json:"requestId"`
	RequestNumber *string    `json:"requestNumber"`
	Unread        int        `json:"unread"`
	LastMessageAt time.Time  `json:"lastMessageAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	Messages      []Message  `json:"messages,omitempty"`
	accessHash    []byte
}

type createInput struct {
	Subject     string             `json:"subject"`
	Category    string             `json:"category"`
	Message     string             `json:"message"`
	OrderID     *uuid.UUID         `json:"orderId"`
	Attachments []uuid.UUID        `json:"attachments"`
	Contact     *customers.Contact `json:"contact"`
}

// checkAttachments verifies the uploads belong to the author and are support or reference images.
func checkAttachments(ctx context.Context, tx pgx.Tx, r *http.Request, ids []uuid.UUID, customerID uuid.UUID, staff bool) error {
	if len(ids) > 6 {
		return httpx.Validation(map[string]string{"attachments": "Attach up to 6 images."})
	}
	guest := uploads.GuestToken(r)
	for _, id := range ids {
		var cust *uuid.UUID
		var gh []byte
		var purpose string
		var deleted *time.Time
		err := tx.QueryRow(ctx, `SELECT customer_id, guest_token_hash, purpose, deleted_at FROM uploads WHERE id=$1`, id).Scan(&cust, &gh, &purpose, &deleted)
		ok := err == nil && deleted == nil && (purpose == "support" || purpose == "reference") &&
			(staff || (cust != nil && *cust == customerID) || (guest != "" && gh != nil && tokens.Matches(guest, gh)))
		if !ok {
			return httpx.Validation(map[string]string{"attachments": "An attachment is no longer available. Please upload it again."})
		}
		if _, err := tx.Exec(ctx, `UPDATE uploads SET customer_id=coalesce(customer_id, $2) WHERE id=$1`, id, customerID); err != nil {
			return err
		}
	}
	return nil
}

// Create opens a support thread. Guests provide contact details; signed-in customers use their account.
func (h Handler) Create(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		return err
	}
	var in createInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var existing uuid.UUID
	if err := h.Pool.QueryRow(ctx, `SELECT id FROM support_threads WHERE idempotency_key=$1`, key).Scan(&existing); err == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"id": existing, "replayed": true})
		return nil
	}
	f := httpx.Fields{}
	in.Subject = strings.TrimSpace(in.Subject)
	in.Message = strings.TrimSpace(in.Message)
	if in.Subject == "" || len(in.Subject) > 200 {
		f.Add("subject", "Add a short subject.")
	}
	if _, ok := Categories[in.Category]; !ok {
		f.Add("category", "Choose what this is about.")
	}
	if in.Message == "" || len(in.Message) > 5000 {
		f.Add("message", "Write your message (up to 5000 characters).")
	}
	p := auth.FromContext(ctx)
	if p == nil || p.CustomerID == nil {
		if in.Contact == nil {
			f.Add("contact.name", "Tell us how to reach you.")
		} else {
			biz, err := h.Settings.Business(ctx, h.Pool)
			if err != nil {
				return err
			}
			in.Contact.Normalize(biz.CountryCode, "contact.", f)
		}
	}
	if err := f.Err(); err != nil {
		return err
	}
	var id uuid.UUID
	var number, token string
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var customerID uuid.UUID
		var err error
		if p != nil && p.CustomerID != nil {
			customerID = *p.CustomerID
		} else if customerID, err = customers.Resolve(ctx, tx, *in.Contact); err != nil {
			return err
		}
		if in.OrderID != nil {
			var owner uuid.UUID
			var hash []byte
			if err := tx.QueryRow(ctx, `SELECT customer_id, access_token_hash FROM orders WHERE id=$1`, *in.OrderID).Scan(&owner, &hash); err != nil ||
				!access.Customer(r, owner, hash) {
				return httpx.Validation(map[string]string{"orderId": "We could not find this order."})
			}
		}
		if err := checkAttachments(ctx, tx, r, in.Attachments, customerID, false); err != nil {
			return err
		}
		if number, err = access.Number(ctx, tx, "support_number_seq", "SUP"); err != nil {
			return err
		}
		var hash []byte
		token, hash = access.NewToken()
		if err := tx.QueryRow(ctx, `INSERT INTO support_threads (number, customer_id, subject, category, order_id, unread_for_staff, access_token_hash, idempotency_key)
			VALUES ($1,$2,$3,$4,$5,1,$6,$7) RETURNING id`, number, customerID, in.Subject, in.Category, in.OrderID, hash, key).Scan(&id); err != nil {
			return err
		}
		if in.Attachments == nil {
			in.Attachments = []uuid.UUID{}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (thread_id, author_type, author_id, body, attachment_ids) VALUES ($1,'customer',$2,$3,$4)`,
			id, auth.ActorID(ctx), in.Message, in.Attachments); err != nil {
			return err
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "support_created", DedupeKey: "support_created:" + id.String(),
			Title: "New message: " + in.Subject, Body: truncate(in.Message, 140), Link: "/owner/support/" + id.String(), Channels: []string{"in_app"}}); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "support_created", DedupeKey: "support_created:" + id.String(),
			Title: "We received your message", Body: "Reference " + number + ". We will reply as soon as we can.",
			Link: "/support/" + id.String() + "?token=" + token, Channels: []string{"in_app", "email"}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "number": number, "accessToken": token})
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "..."
}

func load(ctx context.Context, q db.Querier, id uuid.UUID, staff bool) (*Thread, error) {
	t := &Thread{}
	err := q.QueryRow(ctx, `SELECT t.id, t.number, t.customer_id, c.full_name, t.subject, t.category, t.status, t.priority, t.assigned_to, u.full_name,
			t.order_id, o.number, t.request_id, r.number, CASE WHEN $2 THEN t.unread_for_staff ELSE t.unread_for_customer END, t.last_message_at,
			t.created_at, t.access_token_hash
		FROM support_threads t JOIN customers c ON c.id=t.customer_id LEFT JOIN users u ON u.id=t.assigned_to
		LEFT JOIN orders o ON o.id=t.order_id LEFT JOIN quote_requests r ON r.id=t.request_id WHERE t.id=$1`, id, staff).
		Scan(&t.ID, &t.Number, &t.CustomerID, &t.CustomerName, &t.Subject, &t.Category, &t.Status, &t.Priority, &t.AssignedTo, &t.AssigneeName,
			&t.OrderID, &t.OrderNumber, &t.RequestID, &t.RequestNumber, &t.Unread, &t.LastMessageAt, &t.CreatedAt, &t.accessHash)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT m.id, m.author_type, u.full_name, m.body, m.internal, m.attachment_ids, m.created_at
		FROM support_messages m LEFT JOIN users u ON u.id=m.author_id WHERE m.thread_id=$1 AND ($2 OR NOT m.internal) ORDER BY m.created_at`, id, staff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t.Messages = []Message{}
	for rows.Next() {
		var m Message
		var ids []uuid.UUID
		if err := rows.Scan(&m.ID, &m.AuthorType, &m.Author, &m.Body, &m.Internal, &ids, &m.CreatedAt); err != nil {
			return nil, err
		}
		if !staff && m.AuthorType == "staff" {
			m.Author = nil // show "The atelier" to customers
		}
		m.Attachments = []Attachment{}
		for _, a := range ids {
			m.Attachments = append(m.Attachments, Attachment{ID: a, URL: "/api/v1/uploads/" + a.String() + "/preview", Thumb: "/api/v1/uploads/" + a.String() + "/thumb"})
		}
		t.Messages = append(t.Messages, m)
	}
	return t, rows.Err()
}

func (h Handler) CustomerGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	t, err := load(ctx, h.Pool, id, false)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !access.Customer(r, t.CustomerID, t.accessHash)) {
		return httpx.NotFound("We could not find this conversation.")
	}
	if err != nil {
		return err
	}
	if t.Unread > 0 {
		_, _ = h.Pool.Exec(ctx, `UPDATE support_threads SET unread_for_customer=0 WHERE id=$1`, id)
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, t)
	return nil
}

func (h Handler) MyThreads(w http.ResponseWriter, r *http.Request) error {
	p := auth.FromContext(r.Context())
	if p == nil || p.CustomerID == nil {
		return httpx.Unauthorized()
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT id, number, subject, category, status, unread_for_customer, last_message_at, created_at
		FROM support_threads WHERE customer_id=$1 ORDER BY last_message_at DESC LIMIT 100`, *p.CustomerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Thread{}
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ID, &t.Number, &t.Subject, &t.Category, &t.Status, &t.Unread, &t.LastMessageAt, &t.CreatedAt); err != nil {
			return err
		}
		out = append(out, t)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

type replyInput struct {
	Body        string      `json:"body"`
	Attachments []uuid.UUID `json:"attachments"`
	Internal    bool        `json:"internal"`
}

func (h Handler) reply(w http.ResponseWriter, r *http.Request, staff bool) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in replyInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" || len(in.Body) > 5000 {
		return httpx.Validation(map[string]string{"body": "Write a message (up to 5000 characters)."})
	}
	if !staff {
		in.Internal = false
	}
	if in.Attachments == nil {
		in.Attachments = []uuid.UUID{}
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var customerID uuid.UUID
		var hash []byte
		var number, subject, status string
		if err := tx.QueryRow(ctx, `SELECT customer_id, access_token_hash, number, subject, status FROM support_threads WHERE id=$1 FOR UPDATE`, id).
			Scan(&customerID, &hash, &number, &subject, &status); err != nil {
			return err
		}
		if !staff && !access.Customer(r, customerID, hash) {
			return httpx.NotFound("We could not find this conversation.")
		}
		if err := checkAttachments(ctx, tx, r, in.Attachments, customerID, staff); err != nil {
			return err
		}
		author := "customer"
		if staff {
			author = "staff"
		}
		var msgID uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO support_messages (thread_id, author_type, author_id, body, attachment_ids, internal) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			id, author, auth.ActorID(ctx), in.Body, in.Attachments, in.Internal).Scan(&msgID); err != nil {
			return err
		}
		switch {
		case staff && in.Internal:
			// internal notes do not change unread counts or status
		case staff:
			if _, err := tx.Exec(ctx, `UPDATE support_threads SET unread_for_customer=unread_for_customer+1, unread_for_staff=0, last_message_at=now(),
				status=CASE WHEN status='open' THEN 'pending' ELSE status END, first_response_at=coalesce(first_response_at, now()) WHERE id=$1`, id); err != nil {
				return err
			}
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "support_reply", DedupeKey: "support_reply:" + msgID.String(),
				Title: "New reply: " + subject, Body: truncate(in.Body, 300), Link: "/support/" + id.String()}); err != nil {
				return err
			}
		default:
			if _, err := tx.Exec(ctx, `UPDATE support_threads SET unread_for_staff=unread_for_staff+1, last_message_at=now(), status='open' WHERE id=$1`, id); err != nil {
				return err
			}
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "support_reply", DedupeKey: "support_reply:" + msgID.String(),
				Title: "Reply on " + number, Body: truncate(in.Body, 140), Link: "/owner/support/" + id.String(), Channels: []string{"in_app"}}); err != nil {
				return err
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) CustomerReply(w http.ResponseWriter, r *http.Request) error {
	return h.reply(w, r, false)
}
func (h Handler) OwnerReply(w http.ResponseWriter, r *http.Request) error { return h.reply(w, r, true) }

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	qs := r.URL.Query()
	p := auth.FromContext(r.Context())
	assigned := qs.Get("assigned")
	if assigned == "me" {
		assigned = p.UserID.String()
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT t.id, t.number, t.customer_id, c.full_name, t.subject, t.category, t.status, t.priority, t.assigned_to, u.full_name,
			t.order_id, o.number, t.request_id, rq.number, t.unread_for_staff, t.last_message_at, t.created_at, count(*) OVER()
		FROM support_threads t JOIN customers c ON c.id=t.customer_id LEFT JOIN users u ON u.id=t.assigned_to
		LEFT JOIN orders o ON o.id=t.order_id LEFT JOIN quote_requests rq ON rq.id=t.request_id
		WHERE ($1 = '' OR t.status=$1) AND ($2 = '' OR t.priority=$2) AND ($3 = '' OR t.assigned_to::text=$3 OR ($3='none' AND t.assigned_to IS NULL))
		  AND (NOT $4 OR t.unread_for_staff > 0) AND ($5 = '' OR t.customer_id::text=$5)
		ORDER BY CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 ELSE 2 END, t.last_message_at DESC LIMIT $6 OFFSET $7`,
		qs.Get("status"), qs.Get("priority"), assigned, qs.Get("unread") == "true", qs.Get("customerId"), page.Limit, page.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	var out []Thread
	total := 0
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ID, &t.Number, &t.CustomerID, &t.CustomerName, &t.Subject, &t.Category, &t.Status, &t.Priority, &t.AssignedTo, &t.AssigneeName,
			&t.OrderID, &t.OrderNumber, &t.RequestID, &t.RequestNumber, &t.Unread, &t.LastMessageAt, &t.CreatedAt, &total); err != nil {
			return err
		}
		out = append(out, t)
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(out, total, page))
	return rows.Err()
}

func (h Handler) OwnerGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	t, err := load(ctx, h.Pool, id, true)
	if err != nil {
		return err
	}
	if t.Unread > 0 {
		_, _ = h.Pool.Exec(ctx, `UPDATE support_threads SET unread_for_staff=0 WHERE id=$1`, id)
	}
	httpx.JSON(w, http.StatusOK, t)
	return nil
}

func (h Handler) OwnerUpdate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Status     *string    `json:"status"`
		Priority   *string    `json:"priority"`
		AssignedTo *uuid.UUID `json:"assignedTo"`
		Unassign   bool       `json:"unassign"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Status != nil && *in.Status != "open" && *in.Status != "pending" && *in.Status != "resolved" {
		return httpx.Validation(map[string]string{"status": "Choose open, pending or resolved."})
	}
	if in.Priority != nil && *in.Priority != "low" && *in.Priority != "normal" && *in.Priority != "high" && *in.Priority != "urgent" {
		return httpx.Validation(map[string]string{"priority": "Choose a priority."})
	}
	if in.AssignedTo != nil {
		var staff bool
		if err := h.Pool.QueryRow(ctx, `SELECT r.is_staff FROM users u JOIN roles r ON r.key=u.role WHERE u.id=$1 AND u.status='active'`, *in.AssignedTo).Scan(&staff); err != nil || !staff {
			return httpx.Validation(map[string]string{"assignedTo": "Assign an active staff member."})
		}
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var before struct {
			Status     string     `json:"status"`
			Priority   string     `json:"priority"`
			AssignedTo *uuid.UUID `json:"assignedTo"`
		}
		if err := tx.QueryRow(ctx, `SELECT status, priority, assigned_to FROM support_threads WHERE id=$1 FOR UPDATE`, id).
			Scan(&before.Status, &before.Priority, &before.AssignedTo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_threads SET status=coalesce($2, status), priority=coalesce($3, priority),
			assigned_to = CASE WHEN $5 THEN NULL ELSE coalesce($4, assigned_to) END WHERE id=$1`, id, in.Status, in.Priority, in.AssignedTo, in.Unassign); err != nil {
			return err
		}
		action := "support.update"
		if in.AssignedTo != nil || in.Unassign {
			action = "support.assign"
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: action, ObjectType: "support_thread", ObjectID: id.String(), Before: before, After: in}); err != nil {
			return err
		}
		if in.AssignedTo != nil && (before.AssignedTo == nil || *before.AssignedTo != *in.AssignedTo) {
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{UserID: in.AssignedTo, Audience: "staff", Event: "support_assigned",
				DedupeKey: fmt.Sprintf("support_assigned:%s:%s:%d", id, *in.AssignedTo, time.Now().Unix()), Title: "A conversation was assigned to you",
				Body: "Open the support inbox to reply.", Link: "/owner/support/" + id.String(), Channels: []string{"in_app"}}); err != nil {
				return err
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}
