// Package quotes implements bespoke custom requests (leads) and quotes, from submission through
// review, quoting, acceptance and conversion into an order.
package quotes

import (
	"context"
	"encoding/json"
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
	"github.com/kuyamcliff/tailor-website/backend/internal/fabrics"
	"github.com/kuyamcliff/tailor-website/backend/internal/garments"
	"github.com/kuyamcliff/tailor-website/backend/internal/measurements"
	"github.com/kuyamcliff/tailor-website/backend/internal/notifications"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

var Occasions = map[string]bool{"wedding": true, "work": true, "ceremony": true, "casual": true, "everyday": true,
	"photoshoot": true, "graduation": true, "other": true}

var ReferenceTags = map[string]bool{"overall": true, "color": true, "silhouette": true, "collar": true, "sleeve": true, "pocket": true,
	"fabric": true, "embroidery": true, "embellishment": true, "back": true, "front": true, "other": true}

type Handler struct {
	Pool     *pgxpool.Pool
	Settings *settings.Service
}

type ReferenceInput struct {
	UploadID uuid.UUID `json:"uploadId"`
	Tag      string    `json:"tag"`
	Note     string    `json:"note"`
}

type SubmitRequest struct {
	Garment              string             `json:"garment"`
	Occasion             string             `json:"occasion"`
	OccasionNote         string             `json:"occasionNote"`
	MeasurementMode      string             `json:"measurementMode"`
	Measurements         *measurementsInput `json:"measurements"`
	MeasurementVersionID *uuid.UUID         `json:"measurementVersionId"`
	BodyModel            string             `json:"bodyModel"`
	FitPreference        string             `json:"fitPreference"`
	FabricMode           string             `json:"fabricMode"`
	FabricKey            string             `json:"fabricKey"`
	ColorKey             string             `json:"colorKey"`
	DesignVersionID      *uuid.UUID         `json:"designVersionId"`
	References           []ReferenceInput   `json:"references"`
	Notes                string             `json:"notes"`
	DesiredDate          string             `json:"desiredDate"`
	DateFlexibility      string             `json:"dateFlexibility"`
	Urgency              string             `json:"urgency"`
	Contact              customers.Contact  `json:"contact"`
}

type measurementsInput struct {
	Unit   string             `json:"unit"`
	Height *float64           `json:"height"`
	Values map[string]float64 `json:"values"`
}

// Submit creates a bespoke request. No payment is required before the tailor reviews it.
func (h Handler) Submit(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		return err
	}
	var req SubmitRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if res, ok, err := h.replay(ctx, key); err != nil {
		return err
	} else if ok {
		httpx.JSON(w, http.StatusOK, res)
		return nil
	}
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	p := auth.FromContext(ctx)
	var signedIn *uuid.UUID
	if p != nil {
		signedIn = p.CustomerID
	}
	guest := uploads.GuestToken(r)
	f := httpx.Fields{}

	garment, err := garments.GetType(ctx, h.Pool, req.Garment)
	if err != nil || !garment.Active {
		f.Add("garment", "Choose a garment.")
	}
	if !Occasions[req.Occasion] {
		f.Add("occasion", "Choose an occasion.")
	}
	if len(req.OccasionNote) > 200 {
		f.Add("occasionNote", "Keep the occasion note short.")
	}
	if req.BodyModel == "" {
		req.BodyModel = "masculine"
	}
	if req.FitPreference == "" {
		req.FitPreference = "regular"
	}
	if req.BodyModel != "masculine" && req.BodyModel != "feminine" {
		f.Add("bodyModel", "Choose a body model.")
	}
	if req.FitPreference != "slim" && req.FitPreference != "regular" && req.FitPreference != "relaxed" {
		f.Add("fitPreference", "Choose a fit preference.")
	}
	var validated *measurements.Validated
	switch req.MeasurementMode {
	case "entered":
		if req.Measurements == nil {
			f.Add("measurements", "Enter your measurements or choose another option.")
			break
		}
		fields, err := measurements.Fields(ctx, h.Pool, req.Garment)
		if err != nil {
			return err
		}
		v := measurements.Validate(fields, req.Measurements.Values, req.Measurements.Unit, req.Measurements.Height, true)
		for k, msg := range v.Errors {
			f.Add("measurements."+k, msg)
		}
		for _, k := range v.Missing {
			for _, fl := range fields {
				if fl.Key == k {
					f.Add("measurements."+k, fl.Label+" is required.")
				}
			}
		}
		validated = &v
	case "saved_profile":
		if req.MeasurementVersionID == nil || signedIn == nil {
			f.Add("measurementVersionId", "Sign in to use saved measurements.")
			break
		}
		var owner *uuid.UUID
		err := h.Pool.QueryRow(ctx, `SELECT coalesce(v.customer_id, p.customer_id) FROM measurement_versions v
			LEFT JOIN measurement_profiles p ON p.id=v.profile_id WHERE v.id=$1`, *req.MeasurementVersionID).Scan(&owner)
		if err != nil || owner == nil || *owner != *signedIn {
			f.Add("measurementVersionId", "Saved measurements not found.")
		}
	case "in_store":
	default:
		f.Add("measurementMode", "Choose how we get your measurements.")
	}
	switch req.FabricMode {
	case "catalog":
		fab, err := fabrics.Get(ctx, h.Pool, req.FabricKey)
		if err != nil || !fab.Active {
			f.Add("fabricKey", "Choose a fabric from the collection.")
		}
	case "recommend", "reference":
		req.FabricKey, req.ColorKey = "", ""
	default:
		f.Add("fabricMode", "Choose a fabric option.")
	}
	if req.FabricMode == "reference" && len(req.References) == 0 {
		f.Add("references", "Add a photo of the fabric you have in mind.")
	}
	if len(req.References) > 20 {
		f.Add("references", "You can attach up to 20 images.")
	}
	for i, ref := range req.References {
		if !ReferenceTags[ref.Tag] {
			f.Add(fmt.Sprintf("references.%d", i), "Choose what this image shows.")
		}
		if len(ref.Note) > 300 {
			f.Add(fmt.Sprintf("references.%d", i), "Keep image notes short.")
		}
	}
	req.Notes = strings.TrimSpace(req.Notes)
	if len(req.Notes) > 4000 {
		f.Add("notes", "Notes are too long.")
	}
	var desired *time.Time
	if req.DesiredDate != "" {
		t, err := time.Parse("2006-01-02", req.DesiredDate)
		if err != nil || t.Before(time.Now().AddDate(0, 0, -1)) {
			f.Add("desiredDate", "Choose a date in the future.")
		}
		desired = &t
	}
	if req.DateFlexibility == "" {
		req.DateFlexibility = "flexible"
	}
	if req.DateFlexibility != "fixed" && req.DateFlexibility != "flexible" && req.DateFlexibility != "very_flexible" {
		f.Add("dateFlexibility", "Choose how flexible the date is.")
	}
	if req.Urgency == "" {
		req.Urgency = "standard"
	}
	if req.Urgency != "standard" && req.Urgency != "soon" && req.Urgency != "urgent" {
		f.Add("urgency", "Choose an urgency.")
	}
	req.Contact.Normalize(biz.CountryCode, "contact.", f)
	if err := f.Err(); err != nil {
		return err
	}

	var res SubmitResult
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		customerID, err := customers.Resolve(ctx, tx, req.Contact)
		if err != nil {
			return err
		}
		// References must belong to the submitter and be live customer reference uploads.
		for i, ref := range req.References {
			var cust *uuid.UUID
			var gh []byte
			var purpose string
			var deleted *time.Time
			err := tx.QueryRow(ctx, `SELECT customer_id, guest_token_hash, purpose, deleted_at FROM uploads WHERE id=$1 FOR UPDATE`, ref.UploadID).
				Scan(&cust, &gh, &purpose, &deleted)
			owned := err == nil && deleted == nil && purpose == "reference" &&
				((signedIn != nil && cust != nil && *cust == *signedIn) || (guest != "" && gh != nil && tokens.Matches(guest, gh)))
			if !owned {
				return httpx.Validation(map[string]string{fmt.Sprintf("references.%d", i): "This image is no longer available. Please upload it again."})
			}
		}
		var designVersion *uuid.UUID
		if req.DesignVersionID != nil {
			var owner *uuid.UUID
			var gh []byte
			err := tx.QueryRow(ctx, `SELECT d.customer_id, d.guest_token_hash FROM design_versions v JOIN designs d ON d.id=v.design_id WHERE v.id=$1`,
				*req.DesignVersionID).Scan(&owner, &gh)
			ok := err == nil && ((signedIn != nil && owner != nil && *owner == *signedIn) || (guest != "" && gh != nil && tokens.Matches(guest, gh)))
			if !ok {
				return httpx.Validation(map[string]string{"designVersionId": "Saved design not found."})
			}
			designVersion = req.DesignVersionID
		}
		mvID := req.MeasurementVersionID
		if validated != nil {
			id, err := measurements.CreateVersion(ctx, tx, measurements.NewVersion{CustomerID: &customerID, Source: "customer_entered",
				BodyModel: req.BodyModel, FitPreference: req.FitPreference, Unit: req.Measurements.Unit, Validated: *validated})
			if err != nil {
				return err
			}
			mvID = &id
		}
		if req.MeasurementMode == "in_store" {
			mvID = nil
		}
		number, err := access.Number(ctx, tx, "request_number_seq", "REQ")
		if err != nil {
			return err
		}
		token, tokenHash := access.NewToken()
		var fabricKey, colorKey *string
		if req.FabricKey != "" {
			fabricKey = &req.FabricKey
		}
		if req.ColorKey != "" {
			colorKey = &req.ColorKey
		}
		var reqID uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO quote_requests (number, customer_id, garment_type_key, occasion, occasion_note, measurement_mode,
				measurement_version_id, body_model, fit_preference, fabric_mode, fabric_key, color_key, design_version_id, notes, desired_date,
				date_flexibility, urgency, contact_name, contact_phone, contact_email, preferred_contact, access_token_hash, idempotency_key)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23) RETURNING id`,
			number, customerID, req.Garment, req.Occasion, strings.TrimSpace(req.OccasionNote), req.MeasurementMode, mvID, req.BodyModel,
			req.FitPreference, req.FabricMode, fabricKey, colorKey, designVersion, req.Notes, desired, req.DateFlexibility, req.Urgency,
			req.Contact.Name, req.Contact.Phone, req.Contact.Email, req.Contact.PreferredContact, tokenHash, key).Scan(&reqID)
		if db.IsUniqueViolation(err, "quote_requests_idempotency_key_key") {
			return errReplay
		}
		if err != nil {
			return err
		}
		for i, ref := range req.References {
			if _, err := tx.Exec(ctx, `INSERT INTO request_references (request_id, upload_id, tag, note, sort_order) VALUES ($1,$2,$3,$4,$5)`,
				reqID, ref.UploadID, ref.Tag, strings.TrimSpace(ref.Note), i); err != nil {
				if db.IsUniqueViolation(err, "") {
					return httpx.Validation(map[string]string{"references": "The same image was attached twice."})
				}
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE uploads SET customer_id=$2 WHERE id=$1 AND customer_id IS NULL`, ref.UploadID, customerID); err != nil {
				return err
			}
		}
		if err := audit.Status(ctx, tx, "request", reqID, "", "new", "", true); err != nil {
			return err
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "request_created", DedupeKey: "request_created:" + reqID.String(),
			Title: "New request " + number, Body: req.Contact.Name + " asked about a " + strings.ToLower(garment.Name) + " for " + req.Occasion + ".",
			Link: "/owner/requests/" + reqID.String(), Channels: []string{"in_app"}}); err != nil {
			return err
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "request_created", DedupeKey: "request_created:" + reqID.String(),
			Title: "We received your request " + number, Body: "Thank you. Your tailor will review your request and reply with a quote or questions.",
			Link: "/requests/" + reqID.String() + "?token=" + token}); err != nil {
			return err
		}
		res = SubmitResult{ID: reqID, Number: number, AccessToken: token}
		return nil
	})
	if errors.Is(err, errReplay) {
		existing, _, err := h.replay(ctx, key)
		if err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, existing)
		return nil
	}
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, res)
	return nil
}

var errReplay = errors.New("replay")

type SubmitResult struct {
	ID          uuid.UUID `json:"id"`
	Number      string    `json:"number"`
	AccessToken string    `json:"accessToken,omitempty"`
	Replayed    bool      `json:"replayed"`
}

func (h Handler) replay(ctx context.Context, key string) (SubmitResult, bool, error) {
	var res SubmitResult
	err := h.Pool.QueryRow(ctx, `SELECT id, number FROM quote_requests WHERE idempotency_key=$1`, key).Scan(&res.ID, &res.Number)
	if errors.Is(err, pgx.ErrNoRows) {
		return res, false, nil
	}
	res.Replayed = true
	return res, err == nil, err
}

// ---- Views ----

type Reference struct {
	UploadID uuid.UUID `json:"uploadId"`
	Tag      string    `json:"tag"`
	Note     string    `json:"note"`
	URL      string    `json:"url"`
	Thumb    string    `json:"thumb"`
	Removed  bool      `json:"removed"`
	Width    *int      `json:"width"`
	Height   *int      `json:"height"`
}

type RequestView struct {
	ID                   uuid.UUID             `json:"id"`
	Number               string                `json:"number"`
	CustomerID           uuid.UUID             `json:"customerId"`
	Status               string                `json:"status"`
	Garment              string                `json:"garment"`
	GarmentName          string                `json:"garmentName"`
	Occasion             string                `json:"occasion"`
	OccasionNote         string                `json:"occasionNote"`
	MeasurementMode      string                `json:"measurementMode"`
	MeasurementVersionID *uuid.UUID            `json:"measurementVersionId"`
	VerifiedVersionID    *uuid.UUID            `json:"verifiedMeasurementVersionId"`
	Measurements         *measurements.Version `json:"measurements"`
	VerifiedMeasurements *measurements.Version `json:"verifiedMeasurements"`
	BodyModel            string                `json:"bodyModel"`
	FitPreference        string                `json:"fitPreference"`
	FabricMode           string                `json:"fabricMode"`
	FabricKey            *string               `json:"fabricKey"`
	FabricName           *string               `json:"fabricName"`
	ColorKey             *string               `json:"colorKey"`
	DesignVersionID      *uuid.UUID            `json:"designVersionId"`
	Design               json.RawMessage       `json:"design"`
	Notes                string                `json:"notes"`
	DesiredDate          *time.Time            `json:"desiredDate"`
	DateFlexibility      string                `json:"dateFlexibility"`
	Urgency              string                `json:"urgency"`
	Contact              customers.Contact     `json:"contact"`
	InfoRequested        *string               `json:"infoRequested"`
	InternalNotes        string                `json:"internalNotes,omitempty"`
	SupportThreadID      *uuid.UUID            `json:"supportThreadId"`
	References           []Reference           `json:"references"`
	Quotes               []QuoteSummary        `json:"quotes"`
	History              []audit.HistoryItem   `json:"history"`
	OrderID              *uuid.UUID            `json:"orderId"`
	Version              int                   `json:"version"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
	accessHash           []byte
}

type QuoteSummary struct {
	ID         uuid.UUID  `json:"id"`
	Number     string     `json:"number"`
	Status     string     `json:"status"`
	TotalMinor *int64     `json:"totalMinor"`
	Currency   *string    `json:"currency"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

func loadRequest(ctx context.Context, q db.Querier, id uuid.UUID, staff bool) (*RequestView, error) {
	v := &RequestView{}
	err := q.QueryRow(ctx, `SELECT r.id, r.number, r.customer_id, r.status, r.garment_type_key, coalesce(g.name, r.garment_type_key), r.occasion, r.occasion_note,
			r.measurement_mode, r.measurement_version_id, r.verified_measurement_version_id, r.body_model, r.fit_preference, r.fabric_mode, r.fabric_key,
			f.name, r.color_key, r.design_version_id, dv.snapshot, r.notes, r.desired_date, r.date_flexibility, r.urgency, r.contact_name, r.contact_phone,
			r.contact_email, r.preferred_contact, r.info_requested, r.internal_notes, r.support_thread_id, r.version, r.created_at, r.updated_at,
			r.access_token_hash, (SELECT id FROM orders o WHERE o.request_id=r.id LIMIT 1)
		FROM quote_requests r LEFT JOIN garment_types g ON g.key=r.garment_type_key LEFT JOIN fabrics f ON f.key=r.fabric_key
		LEFT JOIN design_versions dv ON dv.id=r.design_version_id WHERE r.id=$1`, id).
		Scan(&v.ID, &v.Number, &v.CustomerID, &v.Status, &v.Garment, &v.GarmentName, &v.Occasion, &v.OccasionNote, &v.MeasurementMode,
			&v.MeasurementVersionID, &v.VerifiedVersionID, &v.BodyModel, &v.FitPreference, &v.FabricMode, &v.FabricKey, &v.FabricName, &v.ColorKey,
			&v.DesignVersionID, &v.Design, &v.Notes, &v.DesiredDate, &v.DateFlexibility, &v.Urgency, &v.Contact.Name, &v.Contact.Phone,
			&v.Contact.Email, &v.Contact.PreferredContact, &v.InfoRequested, &v.InternalNotes, &v.SupportThreadID, &v.Version, &v.CreatedAt,
			&v.UpdatedAt, &v.accessHash, &v.OrderID)
	if err != nil {
		return nil, err
	}
	if !staff {
		v.InternalNotes = ""
	}
	if v.MeasurementVersionID != nil {
		if v.Measurements, err = measurements.GetVersion(ctx, q, *v.MeasurementVersionID); err != nil {
			return nil, err
		}
	}
	if v.VerifiedVersionID != nil {
		if v.VerifiedMeasurements, err = measurements.GetVersion(ctx, q, *v.VerifiedVersionID); err != nil {
			return nil, err
		}
	}
	rows, err := q.Query(ctx, `SELECT rr.upload_id, rr.tag, rr.note, u.deleted_at IS NOT NULL, u.width, u.height
		FROM request_references rr JOIN uploads u ON u.id=rr.upload_id WHERE rr.request_id=$1 ORDER BY rr.sort_order`, id)
	if err != nil {
		return nil, err
	}
	v.References = []Reference{}
	for rows.Next() {
		var ref Reference
		if err := rows.Scan(&ref.UploadID, &ref.Tag, &ref.Note, &ref.Removed, &ref.Width, &ref.Height); err != nil {
			rows.Close()
			return nil, err
		}
		if !ref.Removed {
			ref.URL = "/api/v1/uploads/" + ref.UploadID.String() + "/preview"
			ref.Thumb = "/api/v1/uploads/" + ref.UploadID.String() + "/thumb"
		}
		v.References = append(v.References, ref)
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT q.id, q.number, q.status, rv.total_minor, rv.currency, rv.expires_at, q.updated_at
		FROM quotes q LEFT JOIN quote_revisions rv ON rv.id=q.current_revision_id
		WHERE q.request_id=$1 AND ($2 OR q.status <> 'draft') ORDER BY q.created_at DESC`, id, staff)
	if err != nil {
		return nil, err
	}
	v.Quotes = []QuoteSummary{}
	for rows.Next() {
		var s QuoteSummary
		if err := rows.Scan(&s.ID, &s.Number, &s.Status, &s.TotalMinor, &s.Currency, &s.ExpiresAt, &s.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		v.Quotes = append(v.Quotes, s)
	}
	rows.Close()
	v.History, err = audit.History(ctx, q, "request", id, !staff)
	return v, err
}

func (h Handler) CustomerGet(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	v, err := loadRequest(r.Context(), h.Pool, id, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("We could not find this request.")
	}
	if err != nil {
		return err
	}
	if !access.Customer(r, v.CustomerID, v.accessHash) {
		return httpx.NotFound("We could not find this request.")
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, v)
	return nil
}

func (h Handler) MyRequests(w http.ResponseWriter, r *http.Request) error {
	p := auth.FromContext(r.Context())
	if p == nil || p.CustomerID == nil {
		return httpx.Unauthorized()
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT r.id, r.number, r.status, coalesce(g.name, r.garment_type_key), r.occasion, r.created_at
		FROM quote_requests r LEFT JOIN garment_types g ON g.key=r.garment_type_key WHERE r.customer_id=$1 ORDER BY r.created_at DESC LIMIT 100`, *p.CustomerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		ID        uuid.UUID `json:"id"`
		Number    string    `json:"number"`
		Status    string    `json:"status"`
		Garment   string    `json:"garment"`
		Occasion  string    `json:"occasion"`
		CreatedAt time.Time `json:"createdAt"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Number, &x.Status, &x.Garment, &x.Occasion, &x.CreatedAt); err != nil {
			return err
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

// CustomerReply lets the customer answer questions about their request. Messages go into a support
// thread linked to the request so the whole conversation is in one place.
func (h Handler) CustomerReply(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" || len(in.Message) > 5000 {
		return httpx.Validation(map[string]string{"message": "Write a message (up to 5000 characters)."})
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var customerID uuid.UUID
		var hash []byte
		var status, number string
		var threadID *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT customer_id, access_token_hash, status, number, support_thread_id FROM quote_requests WHERE id=$1 FOR UPDATE`, id).
			Scan(&customerID, &hash, &status, &number, &threadID); err != nil {
			return err
		}
		if !access.Customer(r, customerID, hash) {
			return httpx.NotFound("We could not find this request.")
		}
		tid, err := ensureThread(ctx, tx, threadID, customerID, id, number, hash)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (thread_id, author_type, author_id, body) VALUES ($1,'customer',$2,$3)`,
			tid, auth.ActorID(ctx), in.Message); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_threads SET unread_for_staff=unread_for_staff+1, last_message_at=now(), status='open' WHERE id=$1`, tid); err != nil {
			return err
		}
		if status == "need_information" {
			if _, err := tx.Exec(ctx, `UPDATE quote_requests SET status='reviewing', version=version+1 WHERE id=$1`, id); err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "request", id, status, "reviewing", "Customer replied", true); err != nil {
				return err
			}
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "request_reply",
			DedupeKey: "request_reply:" + id.String() + ":" + time.Now().UTC().Format(time.RFC3339Nano), Title: "Reply on " + number,
			Body: truncate(in.Message, 140), Link: "/owner/requests/" + id.String(), Channels: []string{"in_app"}}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

// ensureThread returns the request's support thread, creating it on first use. The thread shares the
// request's access token so a guest's request link also opens the conversation.
func ensureThread(ctx context.Context, tx pgx.Tx, existing *uuid.UUID, customerID, requestID uuid.UUID, number string, tokenHash []byte) (uuid.UUID, error) {
	if existing != nil {
		return *existing, nil
	}
	tnum, err := access.Number(ctx, tx, "support_number_seq", "SUP")
	if err != nil {
		return uuid.Nil, err
	}
	var tid uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO support_threads (number, customer_id, subject, category, request_id, access_token_hash)
		VALUES ($1,$2,$3,'general',$4,$5) RETURNING id`, tnum, customerID, "Request "+number, requestID, tokenHash).Scan(&tid); err != nil {
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE quote_requests SET support_thread_id=$2 WHERE id=$1`, requestID, tid)
	return tid, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "..."
}

// ---- Owner ----

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	qs := r.URL.Query()
	rows, err := h.Pool.Query(r.Context(), `SELECT r.id, r.number, r.status, coalesce(g.name, r.garment_type_key), r.occasion, r.urgency, r.desired_date,
			r.contact_name, r.measurement_mode, (SELECT count(*) FROM request_references rr WHERE rr.request_id=r.id), r.created_at, r.updated_at, count(*) OVER()
		FROM quote_requests r LEFT JOIN garment_types g ON g.key=r.garment_type_key
		WHERE ($1 = '' OR r.status=$1) AND ($2 = '' OR r.garment_type_key=$2) AND ($3 = '' OR r.urgency=$3)
		  AND ($4 = '' OR r.number ILIKE '%'||$4||'%' OR r.contact_name ILIKE '%'||$4||'%' OR r.contact_phone LIKE '%'||$4||'%')
		  AND ($5 = '' OR r.customer_id::text=$5)
		ORDER BY CASE r.status WHEN 'new' THEN 0 WHEN 'reviewing' THEN 1 WHEN 'need_information' THEN 2 ELSE 3 END, r.created_at DESC
		LIMIT $6 OFFSET $7`, qs.Get("status"), qs.Get("garment"), qs.Get("urgency"), strings.TrimSpace(qs.Get("q")), qs.Get("customerId"), page.Limit, page.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		ID              uuid.UUID  `json:"id"`
		Number          string     `json:"number"`
		Status          string     `json:"status"`
		Garment         string     `json:"garment"`
		Occasion        string     `json:"occasion"`
		Urgency         string     `json:"urgency"`
		DesiredDate     *time.Time `json:"desiredDate"`
		Customer        string     `json:"customer"`
		MeasurementMode string     `json:"measurementMode"`
		References      int        `json:"references"`
		CreatedAt       time.Time  `json:"createdAt"`
		UpdatedAt       time.Time  `json:"updatedAt"`
	}
	var out []row
	total := 0
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Number, &x.Status, &x.Garment, &x.Occasion, &x.Urgency, &x.DesiredDate, &x.Customer, &x.MeasurementMode,
			&x.References, &x.CreatedAt, &x.UpdatedAt, &total); err != nil {
			return err
		}
		out = append(out, x)
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
	v, err := loadRequest(ctx, h.Pool, id, true)
	if err != nil {
		return err
	}
	var messages []byte
	if err := h.Pool.QueryRow(ctx, `SELECT coalesce(json_agg(x ORDER BY x.created_at), '[]') FROM (SELECT m.id, m.author_type, u.full_name AS author, m.body,
			m.internal, m.created_at FROM support_messages m LEFT JOIN users u ON u.id=m.author_id WHERE m.thread_id=$1) x`, v.SupportThreadID).Scan(&messages); err != nil {
		return err
	}
	fields, err := measurements.Fields(ctx, h.Pool, v.Garment)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"request": v, "messages": json.RawMessage(messages), "measurementFields": fields})
	return nil
}

var requestTransitions = map[string][]string{
	"new":              {"reviewing", "need_information", "closed"},
	"reviewing":        {"need_information", "closed", "new"},
	"need_information": {"reviewing", "closed"},
	"quote_sent":       {"reviewing", "closed"},
	"accepted":         {"converted"},
	"closed":           {"reviewing"},
}

func (h Handler) OwnerUpdate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Status        *string `json:"status"`
		Message       string  `json:"message"`
		InternalNotes *string `json:"internalNotes"`
		Version       int     `json:"version"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Message = strings.TrimSpace(in.Message)
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var cur, number string
		var version int
		var customerID uuid.UUID
		var hash []byte
		var threadID *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT status, version, number, customer_id, access_token_hash, support_thread_id FROM quote_requests WHERE id=$1 FOR UPDATE`, id).
			Scan(&cur, &version, &number, &customerID, &hash, &threadID); err != nil {
			return err
		}
		if in.Version != version {
			return httpx.Conflict("stale_version", "This request was updated elsewhere. Reload to see the latest.")
		}
		if in.InternalNotes != nil {
			if len(*in.InternalNotes) > 10000 {
				return httpx.Validation(map[string]string{"internalNotes": "Notes are too long."})
			}
			if _, err := tx.Exec(ctx, `UPDATE quote_requests SET internal_notes=$2, version=version+1 WHERE id=$1`, id, *in.InternalNotes); err != nil {
				return err
			}
		}
		if in.Status != nil && *in.Status != cur {
			allowed := false
			for _, s := range requestTransitions[cur] {
				allowed = allowed || s == *in.Status
			}
			if !allowed {
				return httpx.Conflict("invalid_transition", fmt.Sprintf("A request cannot move from %s to %s.", cur, *in.Status))
			}
			if *in.Status == "need_information" && in.Message == "" {
				return httpx.Validation(map[string]string{"message": "Tell the customer what information you need."})
			}
			var info *string
			if *in.Status == "need_information" {
				info = &in.Message
			}
			if _, err := tx.Exec(ctx, `UPDATE quote_requests SET status=$2, info_requested=coalesce($3, info_requested), version=version+1 WHERE id=$1`,
				id, *in.Status, info); err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "request", id, cur, *in.Status, in.Message, true); err != nil {
				return err
			}
			if *in.Status == "need_information" {
				tid, err := ensureThread(ctx, tx, threadID, customerID, id, number, hash)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO support_messages (thread_id, author_type, author_id, body) VALUES ($1,'staff',$2,$3)`,
					tid, auth.ActorID(ctx), in.Message); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE support_threads SET unread_for_customer=unread_for_customer+1, last_message_at=now(), status='pending',
					first_response_at=coalesce(first_response_at, now()) WHERE id=$1`, tid); err != nil {
					return err
				}
				if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "request_info_needed",
					DedupeKey: fmt.Sprintf("request_info:%s:%d", id, version), Title: "A question about your request " + number,
					Body: in.Message, Link: "/requests/" + id.String()}); err != nil {
					return err
				}
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "requests.update", ObjectType: "request", ObjectID: id.String(),
			Before: map[string]string{"status": cur}, After: in}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

// OwnerVerifyMeasurements records tailor-verified measurements for a request. The verified version is
// saved to the customer's measurement profile so it can be reused for future orders.
func (h Handler) OwnerVerifyMeasurements(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Unit   string             `json:"unit"`
		Height *float64           `json:"height"`
		Values map[string]float64 `json:"values"`
		Notes  string             `json:"notes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var customerID uuid.UUID
	var garment, number, bodyModel, fitPref string
	if err := h.Pool.QueryRow(ctx, `SELECT customer_id, garment_type_key, number, body_model, fit_preference FROM quote_requests WHERE id=$1`, id).
		Scan(&customerID, &garment, &number, &bodyModel, &fitPref); err != nil {
		return err
	}
	fields, err := measurements.Fields(ctx, h.Pool, garment)
	if err != nil {
		return err
	}
	v := measurements.Validate(fields, in.Values, in.Unit, in.Height, false)
	if len(v.Errors) > 0 {
		return httpx.Validation(v.Errors)
	}
	if len(v.ValuesMM) == 0 {
		return httpx.Validation(map[string]string{"values": "Enter the verified measurements."})
	}
	var versionID uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var profileID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM measurement_profiles WHERE customer_id=$1 AND deleted_at IS NULL ORDER BY is_default DESC, created_at LIMIT 1`, customerID).Scan(&profileID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `INSERT INTO measurement_profiles (customer_id, name, is_default, body_model, unit, fit_preference)
				VALUES ($1,'Personal',true,$2,$3,$4) RETURNING id`, customerID, bodyModel, in.Unit, fitPref).Scan(&profileID)
		}
		if err != nil {
			return err
		}
		versionID, err = measurements.CreateVersion(ctx, tx, measurements.NewVersion{ProfileID: &profileID, CustomerID: &customerID,
			Source: "tailor_verified", BodyModel: bodyModel, FitPreference: fitPref, Unit: in.Unit, Validated: v,
			Notes: strings.TrimSpace(in.Notes), Verified: true})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE quote_requests SET verified_measurement_version_id=$2, version=version+1 WHERE id=$1`, id, versionID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "measurements.verify", ObjectType: "request", ObjectID: id.String(),
			After: map[string]any{"versionId": versionID, "valuesMm": v.ValuesMM, "heightMm": v.HeightMM}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"versionId": versionID, "reviewFlags": v.Flags})
	return nil
}
