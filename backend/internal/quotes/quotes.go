package quotes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kuyamcliff/tailor-website/backend/internal/access"
	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/customers"
	"github.com/kuyamcliff/tailor-website/backend/internal/notifications"
	"github.com/kuyamcliff/tailor-website/backend/internal/orders"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/money"
	"github.com/kuyamcliff/tailor-website/backend/internal/pricing"
)

type Revision struct {
	ID                   uuid.UUID      `json:"id"`
	RevisionNo           int            `json:"revisionNo"`
	Currency             string         `json:"currency"`
	Lines                []pricing.Line `json:"lines"`
	SubtotalMinor        int64          `json:"subtotalMinor"`
	DiscountMinor        int64          `json:"discountMinor"`
	DeliveryMinor        int64          `json:"deliveryMinor"`
	TaxMinor             int64          `json:"taxMinor"`
	TaxRateBP            int            `json:"taxRateBp"`
	TotalMinor           int64          `json:"totalMinor"`
	DepositMinor         int64          `json:"depositMinor"`
	BalanceMinor         int64          `json:"balanceMinor"`
	ExpiresAt            time.Time      `json:"expiresAt"`
	CustomerNotes        string         `json:"customerNotes"`
	Terms                string         `json:"terms"`
	EstimatedReadyDate   *time.Time     `json:"estimatedReadyDate"`
	MeasurementVersionID *uuid.UUID     `json:"measurementVersionId"`
	DesignVersionID      *uuid.UUID     `json:"designVersionId"`
	SentAt               *time.Time     `json:"sentAt"`
	CreatedAt            time.Time      `json:"createdAt"`
}

type Quote struct {
	ID                 uuid.UUID           `json:"id"`
	Number             string              `json:"number"`
	RequestID          *uuid.UUID          `json:"requestId"`
	RequestNumber      *string             `json:"requestNumber"`
	CustomerID         uuid.UUID           `json:"customerId"`
	CustomerName       string              `json:"customerName"`
	Status             string              `json:"status"`
	Current            *Revision           `json:"current"`
	AcceptedRevisionID *uuid.UUID          `json:"acceptedRevisionId"`
	DecisionNote       *string             `json:"decisionNote"`
	DecidedAt          *time.Time          `json:"decidedAt"`
	OrderID            *uuid.UUID          `json:"orderId"`
	Revisions          []Revision          `json:"revisions,omitempty"`
	History            []audit.HistoryItem `json:"history"`
	Expired            bool                `json:"expired"`
	Version            int                 `json:"version"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
	accessHash         []byte
}

const revisionCols = `id, revision_no, currency, lines, subtotal_minor, discount_minor, delivery_minor, tax_minor, tax_rate_bp, total_minor,
	deposit_minor, balance_minor, expires_at, customer_notes, terms, estimated_ready_date, measurement_version_id, design_version_id, sent_at, created_at`

func scanRevision(row pgx.Row) (*Revision, error) {
	r := &Revision{}
	err := row.Scan(&r.ID, &r.RevisionNo, &r.Currency, &r.Lines, &r.SubtotalMinor, &r.DiscountMinor, &r.DeliveryMinor, &r.TaxMinor, &r.TaxRateBP,
		&r.TotalMinor, &r.DepositMinor, &r.BalanceMinor, &r.ExpiresAt, &r.CustomerNotes, &r.Terms, &r.EstimatedReadyDate, &r.MeasurementVersionID,
		&r.DesignVersionID, &r.SentAt, &r.CreatedAt)
	return r, err
}

func loadQuote(ctx context.Context, q db.Querier, id uuid.UUID, staff bool) (*Quote, error) {
	v := &Quote{}
	var cur *uuid.UUID
	err := q.QueryRow(ctx, `SELECT q.id, q.number, q.request_id, r.number, q.customer_id, c.full_name, q.status, q.current_revision_id,
			q.accepted_revision_id, q.decision_note, q.decided_at, q.version, q.created_at, q.updated_at, q.access_token_hash,
			(SELECT id FROM orders o WHERE o.quote_id=q.id)
		FROM quotes q JOIN customers c ON c.id=q.customer_id LEFT JOIN quote_requests r ON r.id=q.request_id WHERE q.id=$1`, id).
		Scan(&v.ID, &v.Number, &v.RequestID, &v.RequestNumber, &v.CustomerID, &v.CustomerName, &v.Status, &cur, &v.AcceptedRevisionID,
			&v.DecisionNote, &v.DecidedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.accessHash, &v.OrderID)
	if err != nil {
		return nil, err
	}
	if cur != nil {
		if v.Current, err = scanRevision(q.QueryRow(ctx, `SELECT `+revisionCols+` FROM quote_revisions WHERE id=$1`, *cur)); err != nil {
			return nil, err
		}
		v.Expired = v.Status == "expired" || (v.Status == "sent" && v.Current.ExpiresAt.Before(time.Now()))
	}
	if staff {
		rows, err := q.Query(ctx, `SELECT `+revisionCols+` FROM quote_revisions WHERE quote_id=$1 ORDER BY revision_no DESC`, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			rv, err := scanRevision(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			v.Revisions = append(v.Revisions, *rv)
		}
		rows.Close()
	}
	v.History, err = audit.History(ctx, q, "quote", id, !staff)
	return v, err
}

type revisionInput struct {
	Lines                []pricing.Line `json:"lines"`
	TaxRateBP            *int64         `json:"taxRateBp"`
	DepositPercentBP     *int64         `json:"depositPercentBp"`
	DepositMinor         *int64         `json:"depositMinor"`
	ValidDays            int            `json:"validDays"`
	CustomerNotes        string         `json:"customerNotes"`
	Terms                string         `json:"terms"`
	EstimatedReadyDate   *string        `json:"estimatedReadyDate"`
	MeasurementVersionID *uuid.UUID     `json:"measurementVersionId"`
	DesignVersionID      *uuid.UUID     `json:"designVersionId"`
	Version              int            `json:"version"`
}

// insertRevision computes totals with the pricing engine and stores an immutable revision snapshot.
func (h Handler) insertRevision(ctx context.Context, tx pgx.Tx, quoteID uuid.UUID, in revisionInput) (*Revision, error) {
	biz, err := h.Settings.Business(ctx, tx)
	if err != nil {
		return nil, err
	}
	tax := int64(biz.TaxRateBP)
	if in.TaxRateBP != nil {
		tax = *in.TaxRateBP
	}
	pin := pricing.Input{Lines: in.Lines, TaxRateBP: tax, PricesIncludeTax: biz.PricesIncludeTax}
	switch {
	case in.DepositMinor != nil:
		pin.DepositMinor = in.DepositMinor
	case in.DepositPercentBP != nil:
		pin.DepositBP = in.DepositPercentBP
	default:
		d := int64(biz.DepositPercentBP)
		pin.DepositBP = &d
	}
	totals, err := pricing.Calculate(pin)
	var fe *pricing.FieldError
	if errors.As(err, &fe) {
		return nil, httpx.Validation(map[string]string{fe.Field: fe.Message})
	}
	if err != nil {
		return nil, err
	}
	days := in.ValidDays
	if days == 0 {
		days = biz.QuoteValidityDays
	}
	if days < 1 || days > 180 {
		return nil, httpx.Validation(map[string]string{"validDays": "A quote can be valid for 1 to 180 days."})
	}
	var ready *time.Time
	if in.EstimatedReadyDate != nil && *in.EstimatedReadyDate != "" {
		t, err := time.Parse("2006-01-02", *in.EstimatedReadyDate)
		if err != nil {
			return nil, httpx.Validation(map[string]string{"estimatedReadyDate": "Enter a valid date."})
		}
		ready = &t
	}
	if len(in.CustomerNotes) > 4000 || len(in.Terms) > 8000 {
		return nil, httpx.Validation(map[string]string{"customerNotes": "Notes or terms are too long."})
	}
	var no int
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(revision_no),0)+1 FROM quote_revisions WHERE quote_id=$1`, quoteID).Scan(&no); err != nil {
		return nil, err
	}
	rev, err := scanRevision(tx.QueryRow(ctx, `INSERT INTO quote_revisions (quote_id, revision_no, currency, lines, subtotal_minor, discount_minor,
			delivery_minor, tax_minor, tax_rate_bp, total_minor, deposit_minor, balance_minor, expires_at, customer_notes, terms, estimated_ready_date,
			measurement_version_id, design_version_id, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, now() + make_interval(days => $13), $14,$15,$16,$17,$18,$19) RETURNING `+revisionCols,
		quoteID, no, biz.Currency, totals.Lines, totals.SubtotalMinor, totals.DiscountMinor, totals.DeliveryMinor, totals.TaxMinor, tax,
		totals.TotalMinor, totals.DepositMinor, totals.BalanceMinor, days, strings.TrimSpace(in.CustomerNotes), strings.TrimSpace(in.Terms), ready,
		in.MeasurementVersionID, in.DesignVersionID, auth.ActorID(ctx)))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE quotes SET current_revision_id=$2, status='draft', version=version+1 WHERE id=$1`, quoteID, rev.ID); err != nil {
		return nil, err
	}
	return rev, nil
}

// OwnerCreateQuote drafts a quote for a request.
func (h Handler) OwnerCreateQuote(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	reqID, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in revisionInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var quoteID uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var customerID uuid.UUID
		var status string
		var hash []byte
		var mv, verified, dv *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT customer_id, status, access_token_hash, measurement_version_id, verified_measurement_version_id, design_version_id
			FROM quote_requests WHERE id=$1 FOR UPDATE`, reqID).Scan(&customerID, &status, &hash, &mv, &verified, &dv); err != nil {
			return err
		}
		if status == "converted" || status == "closed" {
			return httpx.Conflict("request_closed", "This request is closed. Reopen it before quoting.")
		}
		if in.MeasurementVersionID == nil {
			in.MeasurementVersionID = verified
			if in.MeasurementVersionID == nil {
				in.MeasurementVersionID = mv
			}
		}
		if in.DesignVersionID == nil {
			in.DesignVersionID = dv
		}
		number, err := access.Number(ctx, tx, "quote_number_seq", "QUO")
		if err != nil {
			return err
		}
		// The quote shares the request's access token so the guest's request link can open its quotes.
		if err := tx.QueryRow(ctx, `INSERT INTO quotes (number, request_id, customer_id, access_token_hash) VALUES ($1,$2,$3,$4) RETURNING id`,
			number, reqID, customerID, hash).Scan(&quoteID); err != nil {
			return err
		}
		rev, err := h.insertRevision(ctx, tx, quoteID, in)
		if err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "quote", quoteID, "", "draft", "", false); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "quotes.create", ObjectType: "quote", ObjectID: quoteID.String(), After: rev})
	})
	if err != nil {
		return err
	}
	q, err := loadQuote(ctx, h.Pool, quoteID, true)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, q)
	return nil
}

// OwnerReviseQuote adds a new revision. Accepted, declined and withdrawn quotes cannot be revised.
func (h Handler) OwnerReviseQuote(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in revisionInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var status string
		var version int
		if err := tx.QueryRow(ctx, `SELECT status, version FROM quotes WHERE id=$1 FOR UPDATE`, id).Scan(&status, &version); err != nil {
			return err
		}
		if in.Version != version {
			return httpx.Conflict("stale_version", "This quote was changed elsewhere. Reload to see the latest revision.")
		}
		if !canDo(status, actRevise) {
			return httpx.Conflict("quote_closed", "This quote can no longer be revised. Create a new quote instead.")
		}
		if in.MeasurementVersionID == nil || in.DesignVersionID == nil {
			var mv, dv *uuid.UUID
			_ = tx.QueryRow(ctx, `SELECT rv.measurement_version_id, rv.design_version_id FROM quotes q JOIN quote_revisions rv ON rv.id=q.current_revision_id WHERE q.id=$1`, id).Scan(&mv, &dv)
			if in.MeasurementVersionID == nil {
				in.MeasurementVersionID = mv
			}
			if in.DesignVersionID == nil {
				in.DesignVersionID = dv
			}
		}
		rev, err := h.insertRevision(ctx, tx, id, in)
		if err != nil {
			return err
		}
		if status != "draft" {
			if err := audit.Status(ctx, tx, "quote", id, status, "draft", fmt.Sprintf("Revision %d drafted", rev.RevisionNo), false); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "quotes.revise", ObjectType: "quote", ObjectID: id.String(), After: rev})
	})
	if err != nil {
		return err
	}
	q, err := loadQuote(ctx, h.Pool, id, true)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, q)
	return nil
}

// OwnerSendQuote sends (or resends) the current revision to the customer.
func (h Handler) OwnerSendQuote(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var status, number string
		var customerID uuid.UUID
		var reqID, revID *uuid.UUID
		var expires time.Time
		var total int64
		var currency string
		if err := tx.QueryRow(ctx, `SELECT q.status, q.number, q.customer_id, q.request_id, q.current_revision_id, rv.expires_at, rv.total_minor, rv.currency
			FROM quotes q JOIN quote_revisions rv ON rv.id=q.current_revision_id WHERE q.id=$1 FOR UPDATE OF q`, id).
			Scan(&status, &number, &customerID, &reqID, &revID, &expires, &total, &currency); err != nil {
			return err
		}
		if !canDo(status, actSend) {
			return httpx.Conflict("quote_closed", "This quote can no longer be sent.")
		}
		if expires.Before(time.Now()) {
			return httpx.Conflict("quote_expired", "This revision has expired. Create a new revision with a fresh validity date.")
		}
		var sends int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM status_history WHERE object_type='quote' AND object_id=$1 AND new_status='sent'`, id).Scan(&sends); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE quote_revisions SET sent_at=coalesce(sent_at, now()) WHERE id=$1`, *revID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE quotes SET status='sent', version=version+1 WHERE id=$1`, id); err != nil {
			return err
		}
		note := ""
		if status == "sent" {
			note = "Resent"
		}
		if err := audit.Status(ctx, tx, "quote", id, status, "sent", note, true); err != nil {
			return err
		}
		if reqID != nil {
			var rs string
			if err := tx.QueryRow(ctx, `SELECT status FROM quote_requests WHERE id=$1 FOR UPDATE`, *reqID).Scan(&rs); err != nil {
				return err
			}
			if rs != "quote_sent" && rs != "accepted" && rs != "converted" {
				if _, err := tx.Exec(ctx, `UPDATE quote_requests SET status='quote_sent', version=version+1 WHERE id=$1`, *reqID); err != nil {
					return err
				}
				if err := audit.Status(ctx, tx, "request", *reqID, rs, "quote_sent", "", true); err != nil {
					return err
				}
			}
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "quote_created",
			DedupeKey: fmt.Sprintf("quote_sent:%s:%d", id, sends), Title: "Your quote " + number,
			Body: fmt.Sprintf("Your quote is ready: %s. It is valid until %s.", money.Format(total, currency), expires.Format("2 January 2006")),
			Link: "/quotes/" + id.String()}); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "quotes.send", ObjectType: "quote", ObjectID: id.String(), After: map[string]any{"revisionId": revID}}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) OwnerWithdrawQuote(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM quotes WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
			return err
		}
		if !canDo(status, actWithdraw) {
			return httpx.Conflict("quote_closed", "This quote cannot be withdrawn.")
		}
		if _, err := tx.Exec(ctx, `UPDATE quotes SET status='withdrawn', version=version+1 WHERE id=$1`, id); err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "quote", id, status, "withdrawn", "", true); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "quotes.withdraw", ObjectType: "quote", ObjectID: id.String()}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) OwnerGetQuote(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	q, err := loadQuote(r.Context(), h.Pool, id, true)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, q)
	return nil
}

func (h Handler) OwnerListQuotes(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	status := r.URL.Query().Get("status")
	rows, err := h.Pool.Query(r.Context(), `SELECT q.id, q.number, q.status, c.full_name, r.number, rv.total_minor, rv.currency, rv.expires_at, q.updated_at, count(*) OVER()
		FROM quotes q JOIN customers c ON c.id=q.customer_id LEFT JOIN quote_requests r ON r.id=q.request_id
		LEFT JOIN quote_revisions rv ON rv.id=q.current_revision_id
		WHERE ($1 = '' OR q.status=$1) ORDER BY q.updated_at DESC LIMIT $2 OFFSET $3`, status, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		ID            uuid.UUID  `json:"id"`
		Number        string     `json:"number"`
		Status        string     `json:"status"`
		Customer      string     `json:"customer"`
		RequestNumber *string    `json:"requestNumber"`
		TotalMinor    *int64     `json:"totalMinor"`
		Currency      *string    `json:"currency"`
		ExpiresAt     *time.Time `json:"expiresAt"`
		UpdatedAt     time.Time  `json:"updatedAt"`
	}
	var out []row
	total := 0
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Number, &x.Status, &x.Customer, &x.RequestNumber, &x.TotalMinor, &x.Currency, &x.ExpiresAt, &x.UpdatedAt, &total); err != nil {
			return err
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(out, total, page))
	return rows.Err()
}

// ---- Customer ----

func (h Handler) CustomerGetQuote(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	q, err := loadQuote(ctx, h.Pool, id, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("We could not find this quote.")
	}
	if err != nil {
		return err
	}
	if !access.Customer(r, q.CustomerID, q.accessHash) || q.Status == "draft" {
		return httpx.NotFound("We could not find this quote.")
	}
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{"quote": q, "business": map[string]any{"name": biz.Name, "phone": biz.Phone,
		"email": biz.Email, "address": biz.Address, "taxLabel": biz.TaxLabel, "pricesIncludeTax": biz.PricesIncludeTax}})
	return nil
}

type decisionInput struct {
	RevisionID uuid.UUID `json:"revisionId"`
	Note       string    `json:"note"`
}

// lockForDecision loads and validates a quote for a customer decision on a specific revision.
// It marks an expired quote as expired (committing that change) and reports it to the customer.
func (h Handler) lockForDecision(ctx context.Context, tx pgx.Tx, r *http.Request, id uuid.UUID, in decisionInput) (*Quote, error) {
	q := &Quote{}
	var cur *uuid.UUID
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT q.id, q.number, q.request_id, q.customer_id, q.status, q.current_revision_id, q.access_token_hash, rv.expires_at,
			q.accepted_revision_id
		FROM quotes q JOIN quote_revisions rv ON rv.id=q.current_revision_id WHERE q.id=$1 FOR UPDATE OF q`, id).
		Scan(&q.ID, &q.Number, &q.RequestID, &q.CustomerID, &q.Status, &cur, &q.accessHash, &expires, &q.AcceptedRevisionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound("We could not find this quote.")
	}
	if err != nil {
		return nil, err
	}
	if !access.Customer(r, q.CustomerID, q.accessHash) || q.Status == "draft" {
		return nil, httpx.NotFound("We could not find this quote.")
	}
	if cur == nil || *cur != in.RevisionID {
		return nil, httpx.Conflict("revision_changed", "This quote has been updated by the atelier. Please review the latest version.")
	}
	return &Quote{ID: q.ID, Number: q.Number, RequestID: q.RequestID, CustomerID: q.CustomerID, Status: q.Status, accessHash: q.accessHash,
		AcceptedRevisionID: q.AcceptedRevisionID, Expired: isExpired(q.Status, expires, time.Now())}, nil
}

var errExpired = httpx.Conflict("quote_expired", "This quote has expired. Contact the atelier and we will send you an updated quote.")

// Accept converts the quote into an order with an immutable snapshot. Pressing Accept twice, or from two
// tabs, returns the same order.
func (h Handler) Accept(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in decisionInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var orderID uuid.UUID
	var orderNumber string
	expired := false
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q, err := h.lockForDecision(ctx, tx, r, id, in)
		if err != nil {
			return err
		}
		if q.Status == "accepted" && q.AcceptedRevisionID != nil && *q.AcceptedRevisionID == in.RevisionID {
			return tx.QueryRow(ctx, `SELECT id, number FROM orders WHERE quote_id=$1`, id).Scan(&orderID, &orderNumber)
		}
		if q.Expired {
			if _, err := tx.Exec(ctx, `UPDATE quotes SET status='expired', version=version+1 WHERE id=$1`, id); err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "quote", id, "sent", "expired", "", true); err != nil {
				return err
			}
			expired = true
			return nil
		}
		if !canDo(q.Status, actAccept) {
			return httpx.Conflict("quote_not_open", "This quote is no longer open for acceptance.")
		}
		rev, err := scanRevision(tx.QueryRow(ctx, `SELECT `+revisionCols+` FROM quote_revisions WHERE id=$1`, in.RevisionID))
		if err != nil {
			return err
		}
		var contact customers.Contact
		var urgency string
		var desired *time.Time
		if q.RequestID != nil {
			if err := tx.QueryRow(ctx, `SELECT contact_name, contact_phone, contact_email, preferred_contact, urgency, desired_date FROM quote_requests WHERE id=$1`,
				*q.RequestID).Scan(&contact.Name, &contact.Phone, &contact.Email, &contact.PreferredContact, &urgency, &desired); err != nil {
				return err
			}
		} else {
			if err := tx.QueryRow(ctx, `SELECT full_name, coalesce(phone,''), email, preferred_contact FROM customers WHERE id=$1`, q.CustomerID).
				Scan(&contact.Name, &contact.Phone, &contact.Email, &contact.PreferredContact); err != nil {
				return err
			}
		}
		verified := false
		if rev.MeasurementVersionID != nil {
			var src string
			if err := tx.QueryRow(ctx, `SELECT source FROM measurement_versions WHERE id=$1`, *rev.MeasurementVersionID).Scan(&src); err == nil {
				verified = src == "tailor_verified"
			}
		}
		due := rev.EstimatedReadyDate
		if due == nil {
			due = desired
		}
		orderID, orderNumber, err = orders.CreateFromQuote(ctx, tx, orders.FromQuote{QuoteID: id, RevisionID: rev.ID, RequestID: q.RequestID,
			CustomerID: q.CustomerID, Currency: rev.Currency, Lines: rev.Lines, Totals: pricing.Totals{SubtotalMinor: rev.SubtotalMinor,
				DiscountMinor: rev.DiscountMinor, DeliveryMinor: rev.DeliveryMinor, TaxMinor: rev.TaxMinor, TotalMinor: rev.TotalMinor,
				DepositMinor: rev.DepositMinor, BalanceMinor: rev.BalanceMinor},
			MeasurementVersionID: rev.MeasurementVersionID, DesignVersionID: rev.DesignVersionID, Contact: contact, Urgency: urgency, DueDate: due,
			MeasurementsVerified: verified, AccessTokenHash: q.accessHash})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE quotes SET status='accepted', accepted_revision_id=$2, decided_at=now(), decision_note=$3, version=version+1 WHERE id=$1`,
			id, rev.ID, strings.TrimSpace(in.Note)); err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "quote", id, "sent", "accepted", "", true); err != nil {
			return err
		}
		if q.RequestID != nil {
			var rs string
			if err := tx.QueryRow(ctx, `SELECT status FROM quote_requests WHERE id=$1 FOR UPDATE`, *q.RequestID).Scan(&rs); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE quote_requests SET status='converted', version=version+1 WHERE id=$1`, *q.RequestID); err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "request", *q.RequestID, rs, "accepted", "", true); err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "request", *q.RequestID, "accepted", "converted", "Order "+orderNumber+" created", true); err != nil {
				return err
			}
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "quote_accepted", DedupeKey: "quote_accepted:" + id.String(),
			Title: "Quote " + q.Number + " accepted", Body: contact.Name + " accepted the quote. Order " + orderNumber + " was created.",
			Link: "/owner/orders/" + orderID.String(), Channels: []string{"in_app", "email"}}); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &q.CustomerID, Event: "quote_accepted", DedupeKey: "quote_accepted:" + id.String(),
			Title: "Order " + orderNumber + " confirmed", Body: "Thank you for accepting your quote. Your order " + orderNumber + " has been created.",
			Link: "/orders/" + orderID.String()})
	})
	if err != nil {
		return err
	}
	if expired {
		return errExpired
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"orderId": orderID, "orderNumber": orderNumber})
	return nil
}

func (h Handler) decide(w http.ResponseWriter, r *http.Request, newStatus, event, title string) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in decisionInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Note = strings.TrimSpace(in.Note)
	if newStatus == "changes_requested" && in.Note == "" {
		return httpx.Validation(map[string]string{"note": "Tell us what you would like to change."})
	}
	if len(in.Note) > 2000 {
		return httpx.Validation(map[string]string{"note": "Please keep this under 2000 characters."})
	}
	expired := false
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q, err := h.lockForDecision(ctx, tx, r, id, in)
		if err != nil {
			return err
		}
		if q.Status == newStatus {
			return nil
		}
		if q.Expired && newStatus != "changes_requested" {
			if _, err := tx.Exec(ctx, `UPDATE quotes SET status='expired', version=version+1 WHERE id=$1`, id); err != nil {
				return err
			}
			expired = true
			return audit.Status(ctx, tx, "quote", id, "sent", "expired", "", true)
		}
		act := actDecline
		if newStatus == "changes_requested" {
			act = actRequestChanges
		}
		if !canDo(q.Status, act) {
			return httpx.Conflict("quote_not_open", "This quote is no longer open.")
		}
		if _, err := tx.Exec(ctx, `UPDATE quotes SET status=$2, decision_note=$3, decided_at=now(), version=version+1 WHERE id=$1`, id, newStatus, in.Note); err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "quote", id, q.Status, newStatus, in.Note, true); err != nil {
			return err
		}
		if q.RequestID != nil && newStatus == "changes_requested" {
			var rs string
			if err := tx.QueryRow(ctx, `SELECT status FROM quote_requests WHERE id=$1 FOR UPDATE`, *q.RequestID).Scan(&rs); err == nil && rs == "quote_sent" {
				if _, err := tx.Exec(ctx, `UPDATE quote_requests SET status='reviewing', version=version+1 WHERE id=$1`, *q.RequestID); err != nil {
					return err
				}
				if err := audit.Status(ctx, tx, "request", *q.RequestID, rs, "reviewing", "Customer asked for changes", true); err != nil {
					return err
				}
			}
		}
		return notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: event, DedupeKey: event + ":" + id.String() + ":" + in.RevisionID.String(),
			Title: title + " " + q.Number, Body: in.Note, Link: "/owner/quotes/" + id.String(), Channels: []string{"in_app"}})
	})
	if err != nil {
		return err
	}
	if expired {
		return errExpired
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h Handler) Decline(w http.ResponseWriter, r *http.Request) error {
	return h.decide(w, r, "declined", "quote_declined", "Quote declined:")
}

func (h Handler) RequestChanges(w http.ResponseWriter, r *http.Request) error {
	return h.decide(w, r, "changes_requested", "quote_changes_requested", "Changes requested on")
}

// ExpireDue marks sent quotes past their validity as expired.
func ExpireDue(ctx context.Context, pool db.Querier) (int64, error) {
	tag, err := pool.Exec(ctx, `WITH expired AS (
			UPDATE quotes q SET status='expired', version=version+1 FROM quote_revisions rv
			WHERE rv.id=q.current_revision_id AND q.status='sent' AND rv.expires_at < now() RETURNING q.id)
		INSERT INTO status_history (object_type, object_id, old_status, new_status, actor_label, note)
		SELECT 'quote', id, 'sent', 'expired', 'system', 'Validity period ended' FROM expired`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
