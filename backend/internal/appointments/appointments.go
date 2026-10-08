// Package appointments computes bookable slots from availability rules and blocked periods, and manages
// bookings. Overlapping bookings are rejected by a database exclusion constraint, so two customers
// racing for the same slot cannot both win.
package appointments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
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
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
)

var Types = map[string]string{
	"consultation": "Consultation", "measuring": "Measurement session", "fitting": "Fitting", "final_fitting": "Final fitting",
	"pickup": "Pickup", "alteration": "Alteration", "video_consultation": "Video consultation", "other": "Appointment",
}

type Rule struct {
	ID          uuid.UUID `json:"id"`
	Weekday     int       `json:"weekday"` // 0 = Sunday
	StartMinute int       `json:"startMinute"`
	EndMinute   int       `json:"endMinute"`
}

type Block struct {
	ID       uuid.UUID `json:"id"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
	Kind     string    `json:"kind"`
	Reason   string    `json:"reason"`
}

type busy struct{ start, end time.Time }

// SlotInput is everything needed to compute availability; kept pure for testing.
type SlotInput struct {
	From, To   time.Time // inclusive day range in the business timezone
	Loc        *time.Location
	Now        time.Time
	Duration   time.Duration
	Buffer     time.Duration
	Step       time.Duration
	MinNotice  time.Duration
	MaxAdvance time.Duration
	Rules      []Rule
	Busy       []busy // existing bookings (start to end+buffer) and blocked periods
}

// Slots returns available start times. A slot needs the full duration plus buffer free of any busy period
// and must fit inside one availability window.
func Slots(in SlotInput) []time.Time {
	var out []time.Time
	earliest := in.Now.Add(in.MinNotice)
	latest := in.Now.Add(in.MaxAdvance)
	for day := in.From; !day.After(in.To); day = day.AddDate(0, 0, 1) {
		y, m, d := day.Date()
		for _, r := range in.Rules {
			if int(day.Weekday()) != r.Weekday {
				continue
			}
			winStart := time.Date(y, m, d, 0, 0, 0, 0, in.Loc).Add(time.Duration(r.StartMinute) * time.Minute)
			winEnd := time.Date(y, m, d, 0, 0, 0, 0, in.Loc).Add(time.Duration(r.EndMinute) * time.Minute)
			for s := winStart; !s.Add(in.Duration).After(winEnd); s = s.Add(in.Step) {
				if s.Before(earliest) || s.After(latest) {
					continue
				}
				end := s.Add(in.Duration + in.Buffer)
				free := true
				for _, b := range in.Busy {
					if s.Before(b.end) && b.start.Before(end) {
						free = false
						break
					}
				}
				if free {
					out = append(out, s)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return dedupe(out)
}

func dedupe(ts []time.Time) []time.Time {
	var out []time.Time
	for i, t := range ts {
		if i == 0 || !t.Equal(ts[i-1]) {
			out = append(out, t)
		}
	}
	return out
}

type Handler struct {
	Pool     *pgxpool.Pool
	Settings *settings.Service
}

func (h Handler) rules(ctx context.Context, q db.Querier) ([]Rule, error) {
	rows, err := q.Query(ctx, `SELECT id, weekday, start_minute, end_minute FROM availability_rules ORDER BY weekday, start_minute`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Rule, error) {
		var x Rule
		err := r.Scan(&x.ID, &x.Weekday, &x.StartMinute, &x.EndMinute)
		return x, err
	})
}

func (h Handler) busyBetween(ctx context.Context, q db.Querier, from, to time.Time, exclude *uuid.UUID) ([]busy, error) {
	rows, err := q.Query(ctx, `SELECT starts_at, buffer_until FROM appointments WHERE status='booked' AND starts_at < $2 AND buffer_until > $1
			AND ($3::uuid IS NULL OR id <> $3)
		UNION ALL SELECT starts_at, ends_at FROM blocked_periods WHERE starts_at < $2 AND ends_at > $1`, from, to, exclude)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (busy, error) {
		var b busy
		err := r.Scan(&b.start, &b.end)
		return b, err
	})
}

func (h Handler) slotInput(ctx context.Context, q db.Querier, typ string, from, to time.Time, exclude *uuid.UUID) (SlotInput, settings.Business, error) {
	biz, err := h.Settings.Business(ctx, q)
	if err != nil {
		return SlotInput{}, biz, err
	}
	loc, err := time.LoadLocation(biz.Timezone)
	if err != nil {
		loc = time.UTC
	}
	rules, err := h.rules(ctx, q)
	if err != nil {
		return SlotInput{}, biz, err
	}
	a := biz.Appointment
	dur := a.Durations[typ]
	if dur == 0 {
		dur = a.SlotMinutes
	}
	b, err := h.busyBetween(ctx, q, from.Add(-24*time.Hour), to.Add(48*time.Hour), exclude)
	if err != nil {
		return SlotInput{}, biz, err
	}
	return SlotInput{From: from.In(loc), To: to.In(loc), Loc: loc, Now: time.Now(), Duration: time.Duration(dur) * time.Minute,
		Buffer: time.Duration(a.BufferMinutes) * time.Minute, Step: time.Duration(max(a.SlotMinutes, 5)) * time.Minute,
		MinNotice: time.Duration(a.MinNoticeHours) * time.Hour, MaxAdvance: time.Duration(a.MaxAdvanceDays) * 24 * time.Hour,
		Rules: rules, Busy: b}, biz, nil
}

// PublicSlots exposes only free start times, never the owner's schedule details.
func (h Handler) PublicSlots(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	typ := r.URL.Query().Get("type")
	if _, ok := Types[typ]; !ok {
		return httpx.Validation(map[string]string{"type": "Choose an appointment type."})
	}
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(biz.Timezone)
	if err != nil {
		loc = time.UTC
	}
	from, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("from"), loc)
	if err != nil {
		y, m, d := time.Now().In(loc).Date()
		from = time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	days := 14
	to := from.AddDate(0, 0, days-1)
	in, biz, err := h.slotInput(ctx, h.Pool, typ, from, to, nil)
	if err != nil {
		return err
	}
	slots := Slots(in)
	out := make([]string, len(slots))
	for i, s := range slots {
		out[i] = s.Format(time.RFC3339)
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{"slots": out, "timezone": biz.Timezone, "durationMinutes": int(in.Duration.Minutes()),
		"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})
	return nil
}

func slotAvailable(in SlotInput, start time.Time) bool {
	for _, s := range Slots(in) {
		if s.Equal(start) {
			return true
		}
	}
	return false
}

type bookInput struct {
	Type       string            `json:"type"`
	StartsAt   time.Time         `json:"startsAt"`
	Contact    customers.Contact `json:"contact"`
	Notes      string            `json:"notes"`
	OrderID    *uuid.UUID        `json:"orderId"`
	RequestID  *uuid.UUID        `json:"requestId"`
	OrderToken string            `json:"orderToken"`
}

var errSlotTaken = httpx.Conflict("slot_taken", "That time was just booked by someone else. Please choose another time.")

// Book creates an appointment at an available slot.
func (h Handler) Book(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		return err
	}
	var in bookInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var existingID uuid.UUID
	if err := h.Pool.QueryRow(ctx, `SELECT id FROM appointments WHERE idempotency_key=$1`, key).Scan(&existingID); err == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"id": existingID, "replayed": true})
		return nil
	}
	if _, ok := Types[in.Type]; !ok {
		return httpx.Validation(map[string]string{"type": "Choose an appointment type."})
	}
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	f := httpx.Fields{}
	in.Contact.Normalize(biz.CountryCode, "contact.", f)
	if len(in.Notes) > 1000 {
		f.Add("notes", "Notes are too long.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	var id uuid.UUID
	var number, token string
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		sin, _, err := h.slotInput(ctx, tx, in.Type, in.StartsAt.Add(-24*time.Hour), in.StartsAt.Add(24*time.Hour), nil)
		if err != nil {
			return err
		}
		if !slotAvailable(sin, in.StartsAt.In(sin.Loc)) {
			return errSlotTaken
		}
		customerID, err := customers.Resolve(ctx, tx, in.Contact)
		if err != nil {
			return err
		}
		// Linking to an order or request requires access to it.
		if in.OrderID != nil {
			var owner uuid.UUID
			var hash []byte
			if err := tx.QueryRow(ctx, `SELECT customer_id, access_token_hash FROM orders WHERE id=$1`, *in.OrderID).Scan(&owner, &hash); err != nil ||
				!access.Customer(r, owner, hash) {
				in.OrderID = nil
			}
		}
		if in.RequestID != nil {
			var owner uuid.UUID
			var hash []byte
			if err := tx.QueryRow(ctx, `SELECT customer_id, access_token_hash FROM quote_requests WHERE id=$1`, *in.RequestID).Scan(&owner, &hash); err != nil ||
				!access.Customer(r, owner, hash) {
				in.RequestID = nil
			}
		}
		number, err = access.Number(ctx, tx, "appointment_number_seq", "APT")
		if err != nil {
			return err
		}
		var hash []byte
		token, hash = access.NewToken()
		end := in.StartsAt.Add(sin.Duration)
		err = tx.QueryRow(ctx, `INSERT INTO appointments (number, customer_id, type, starts_at, ends_at, buffer_until, order_id, request_id,
				customer_notes, contact, access_token_hash, idempotency_key)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`, number, customerID, in.Type, in.StartsAt, end, end.Add(sin.Buffer),
			in.OrderID, in.RequestID, strings.TrimSpace(in.Notes), in.Contact, hash, key).Scan(&id)
		if db.IsExclusionViolation(err) {
			return errSlotTaken
		}
		if err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "appointment", id, "", "booked", "", true); err != nil {
			return err
		}
		when := in.StartsAt.In(sin.Loc).Format("Monday 2 January at 15:04")
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "appointment_booked", DedupeKey: "appt_booked:" + id.String(),
			Title: Types[in.Type] + " booked", Body: in.Contact.Name + " booked " + when + ".", Link: "/owner/appointments?focus=" + id.String(),
			Channels: []string{"in_app"}}); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "appointment_booked", DedupeKey: "appt_booked:" + id.String(),
			Title: "Your " + strings.ToLower(Types[in.Type]) + " is booked", Body: "See you " + when + ".",
			Link: "/appointments/" + id.String() + "?token=" + token})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "number": number, "accessToken": token})
	return nil
}

type View struct {
	ID            uuid.UUID         `json:"id"`
	Number        string            `json:"number"`
	CustomerID    uuid.UUID         `json:"customerId"`
	CustomerName  string            `json:"customerName"`
	Type          string            `json:"type"`
	TypeLabel     string            `json:"typeLabel"`
	Status        string            `json:"status"`
	StartsAt      time.Time         `json:"startsAt"`
	EndsAt        time.Time         `json:"endsAt"`
	StaffID       *uuid.UUID        `json:"staffId"`
	StaffName     *string           `json:"staffName"`
	Location      string            `json:"location"`
	OrderID       *uuid.UUID        `json:"orderId"`
	OrderNumber   *string           `json:"orderNumber"`
	RequestID     *uuid.UUID        `json:"requestId"`
	CustomerNotes string            `json:"customerNotes"`
	InternalNotes string            `json:"internalNotes,omitempty"`
	Contact       customers.Contact `json:"contact"`
	CanChange     bool              `json:"canChange"`
	Version       int               `json:"version"`
	accessHash    []byte
}

func (h Handler) load(ctx context.Context, q db.Querier, id uuid.UUID) (*View, error) {
	v := &View{}
	err := q.QueryRow(ctx, `SELECT a.id, a.number, a.customer_id, c.full_name, a.type, a.status, a.starts_at, a.ends_at, a.staff_id, u.full_name, a.location,
			a.order_id, o.number, a.request_id, a.customer_notes, a.internal_notes, a.contact, a.version, a.access_token_hash
		FROM appointments a JOIN customers c ON c.id=a.customer_id LEFT JOIN users u ON u.id=a.staff_id LEFT JOIN orders o ON o.id=a.order_id
		WHERE a.id=$1`, id).Scan(&v.ID, &v.Number, &v.CustomerID, &v.CustomerName, &v.Type, &v.Status, &v.StartsAt, &v.EndsAt, &v.StaffID,
		&v.StaffName, &v.Location, &v.OrderID, &v.OrderNumber, &v.RequestID, &v.CustomerNotes, &v.InternalNotes, &v.Contact, &v.Version, &v.accessHash)
	v.TypeLabel = Types[v.Type]
	return v, err
}

func (h Handler) CustomerGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	v, err := h.load(ctx, h.Pool, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !access.Customer(r, v.CustomerID, v.accessHash)) {
		return httpx.NotFound("We could not find this appointment.")
	}
	if err != nil {
		return err
	}
	biz, _ := h.Settings.Business(ctx, h.Pool)
	v.InternalNotes = ""
	v.CanChange = v.Status == "booked" && time.Until(v.StartsAt) > time.Duration(biz.Appointment.CancelNoticeHours)*time.Hour
	httpx.JSON(w, http.StatusOK, map[string]any{"appointment": v, "timezone": biz.Timezone, "cancelNoticeHours": biz.Appointment.CancelNoticeHours})
	return nil
}

func (h Handler) MyAppointments(w http.ResponseWriter, r *http.Request) error {
	p := auth.FromContext(r.Context())
	if p == nil || p.CustomerID == nil {
		return httpx.Unauthorized()
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT id FROM appointments WHERE customer_id=$1 ORDER BY starts_at DESC LIMIT 100`, *p.CustomerID)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	out := []View{}
	for _, id := range ids {
		v, err := h.load(r.Context(), h.Pool, id)
		if err != nil {
			return err
		}
		v.InternalNotes = ""
		out = append(out, *v)
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// change reschedules or cancels. Customers must respect the cancellation notice; staff can always change.
func (h Handler) change(w http.ResponseWriter, r *http.Request, staff bool) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Action   string     `json:"action"` // reschedule or cancel
		StartsAt *time.Time `json:"startsAt"`
		Reason   string     `json:"reason"`
		Override bool       `json:"override"` // staff may book outside published availability
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM appointments WHERE id=$1 FOR UPDATE`, id); err != nil {
			return err
		}
		v, err := h.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if !staff && !access.Customer(r, v.CustomerID, v.accessHash) {
			return httpx.NotFound("We could not find this appointment.")
		}
		if v.Status != "booked" {
			return httpx.Conflict("not_booked", "This appointment is no longer active.")
		}
		biz, err := h.Settings.Business(ctx, tx)
		if err != nil {
			return err
		}
		if !staff && time.Until(v.StartsAt) < time.Duration(biz.Appointment.CancelNoticeHours)*time.Hour {
			return httpx.Conflict("too_late", fmt.Sprintf("Changes need at least %d hours notice. Please contact the atelier directly.", biz.Appointment.CancelNoticeHours))
		}
		var title, body string
		switch in.Action {
		case "cancel":
			if _, err := tx.Exec(ctx, `UPDATE appointments SET status='cancelled', version=version+1 WHERE id=$1`, id); err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "appointment", id, "booked", "cancelled", strings.TrimSpace(in.Reason), true); err != nil {
				return err
			}
			title, body = "Appointment cancelled", "Your "+strings.ToLower(v.TypeLabel)+" "+v.Number+" has been cancelled."
		case "reschedule":
			if in.StartsAt == nil {
				return httpx.Validation(map[string]string{"startsAt": "Choose a new time."})
			}
			sin, _, err := h.slotInput(ctx, tx, v.Type, in.StartsAt.Add(-24*time.Hour), in.StartsAt.Add(24*time.Hour), &id)
			if err != nil {
				return err
			}
			if !(staff && in.Override) && !slotAvailable(sin, in.StartsAt.In(sin.Loc)) {
				return errSlotTaken
			}
			end := in.StartsAt.Add(v.EndsAt.Sub(v.StartsAt))
			_, err = tx.Exec(ctx, `UPDATE appointments SET starts_at=$2, ends_at=$3, buffer_until=$4, reminder_sent_at=NULL, version=version+1 WHERE id=$1`,
				id, *in.StartsAt, end, end.Add(sin.Buffer))
			if db.IsExclusionViolation(err) {
				return errSlotTaken
			}
			if err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "appointment", id, "booked", "booked", "Rescheduled to "+in.StartsAt.In(sin.Loc).Format("2 Jan 15:04"), true); err != nil {
				return err
			}
			title, body = "Appointment moved", "Your "+strings.ToLower(v.TypeLabel)+" is now "+in.StartsAt.In(sin.Loc).Format("Monday 2 January at 15:04")+"."
		default:
			return httpx.Validation(map[string]string{"action": "Choose reschedule or cancel."})
		}
		if staff {
			if err := audit.Write(ctx, tx, audit.Entry{Action: "appointments." + in.Action, ObjectType: "appointment", ObjectID: id.String(), After: in}); err != nil {
				return err
			}
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &v.CustomerID, Event: "appointment_changed",
				DedupeKey: fmt.Sprintf("appt_changed:%s:%d", id, v.Version), Title: title, Body: body, Link: "/appointments/" + id.String()}); err != nil {
				return err
			}
		} else {
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "appointment_changed",
				DedupeKey: fmt.Sprintf("appt_changed:%s:%d", id, v.Version), Title: title + ": " + v.CustomerName, Body: body,
				Link: "/owner/appointments?focus=" + id.String(), Channels: []string{"in_app"}}); err != nil {
				return err
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) CustomerChange(w http.ResponseWriter, r *http.Request) error {
	return h.change(w, r, false)
}
func (h Handler) OwnerChange(w http.ResponseWriter, r *http.Request) error {
	return h.change(w, r, true)
}

// ---- Owner ----

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		from = time.Now().Add(-24 * time.Hour)
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil || to.Sub(from) > 100*24*time.Hour {
		to = from.Add(35 * 24 * time.Hour)
	}
	rows, err := h.Pool.Query(ctx, `SELECT id FROM appointments WHERE starts_at >= $1 AND starts_at < $2 AND ($3 = '' OR status=$3) ORDER BY starts_at`,
		from, to, r.URL.Query().Get("status"))
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	out := []View{}
	for _, id := range ids {
		v, err := h.load(ctx, h.Pool, id)
		if err != nil {
			return err
		}
		out = append(out, *v)
	}
	blocks, err := h.blocks(ctx, from, to)
	if err != nil {
		return err
	}
	biz, _ := h.Settings.Business(ctx, h.Pool)
	httpx.JSON(w, http.StatusOK, map[string]any{"appointments": out, "blocks": blocks, "timezone": biz.Timezone})
	return nil
}

func (h Handler) blocks(ctx context.Context, from, to time.Time) ([]Block, error) {
	rows, err := h.Pool.Query(ctx, `SELECT id, starts_at, ends_at, kind, reason FROM blocked_periods WHERE ends_at > $1 AND starts_at < $2 ORDER BY starts_at`, from, to)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Block, error) {
		var b Block
		err := r.Scan(&b.ID, &b.StartsAt, &b.EndsAt, &b.Kind, &b.Reason)
		return b, err
	})
	if out == nil {
		out = []Block{}
	}
	return out, err
}

// OwnerCreate books on behalf of a customer (e.g. a fitting for an order). Staff can override availability.
func (h Handler) OwnerCreate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var in struct {
		CustomerID    uuid.UUID  `json:"customerId"`
		Type          string     `json:"type"`
		StartsAt      time.Time  `json:"startsAt"`
		DurationMin   int        `json:"durationMinutes"`
		OrderID       *uuid.UUID `json:"orderId"`
		RequestID     *uuid.UUID `json:"requestId"`
		StaffID       *uuid.UUID `json:"staffId"`
		Location      string     `json:"location"`
		CustomerNotes string     `json:"customerNotes"`
		InternalNotes string     `json:"internalNotes"`
		Override      bool       `json:"override"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if _, ok := Types[in.Type]; !ok {
		return httpx.Validation(map[string]string{"type": "Choose an appointment type."})
	}
	var id uuid.UUID
	var number string
	err := db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var c customers.Contact
		if err := tx.QueryRow(ctx, `SELECT full_name, coalesce(phone,''), email, preferred_contact FROM customers WHERE id=$1 AND deleted_at IS NULL`, in.CustomerID).
			Scan(&c.Name, &c.Phone, &c.Email, &c.PreferredContact); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.Validation(map[string]string{"customerId": "Choose a customer."})
			}
			return err
		}
		sin, _, err := h.slotInput(ctx, tx, in.Type, in.StartsAt.Add(-24*time.Hour), in.StartsAt.Add(24*time.Hour), nil)
		if err != nil {
			return err
		}
		if in.DurationMin > 0 {
			sin.Duration = time.Duration(in.DurationMin) * time.Minute
		}
		if !in.Override && !slotAvailable(sin, in.StartsAt.In(sin.Loc)) {
			return httpx.Conflict("slot_unavailable", "This time is outside availability or overlaps another appointment. Choose override to book it anyway.")
		}
		if number, err = access.Number(ctx, tx, "appointment_number_seq", "APT"); err != nil {
			return err
		}
		var hash []byte
		if in.OrderID != nil {
			_ = tx.QueryRow(ctx, `SELECT access_token_hash FROM orders WHERE id=$1 AND customer_id=$2`, *in.OrderID, in.CustomerID).Scan(&hash)
		}
		if hash == nil {
			_, hash = access.NewToken()
		}
		end := in.StartsAt.Add(sin.Duration)
		loc := in.Location
		if loc == "" {
			loc = "studio"
			if in.Type == "video_consultation" {
				loc = "video"
			}
		}
		err = tx.QueryRow(ctx, `INSERT INTO appointments (number, customer_id, type, starts_at, ends_at, buffer_until, staff_id, location, order_id, request_id,
				customer_notes, internal_notes, contact, access_token_hash, idempotency_key)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`, number, in.CustomerID, in.Type, in.StartsAt, end, end.Add(sin.Buffer),
			in.StaffID, loc, in.OrderID, in.RequestID, strings.TrimSpace(in.CustomerNotes), strings.TrimSpace(in.InternalNotes), c, hash, "staff:"+uuid.NewString()).Scan(&id)
		if db.IsExclusionViolation(err) {
			return httpx.Conflict("slot_taken", "Another appointment already uses this time.")
		}
		if err != nil {
			return err
		}
		if in.OrderID != nil && (in.Type == "fitting" || in.Type == "final_fitting") {
			var status string
			if err := tx.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1 FOR UPDATE`, *in.OrderID).Scan(&status); err == nil &&
				status != "fitting_scheduled" && status != "fitting" && status != "completed" && status != "cancelled" && status != "refunded" &&
				status != "delivered" && status != "ready" && status != "dispatched" {
				if _, err := tx.Exec(ctx, `UPDATE orders SET status='fitting_scheduled', version=version+1 WHERE id=$1`, *in.OrderID); err != nil {
					return err
				}
				if err := audit.Status(ctx, tx, "order", *in.OrderID, status, "fitting_scheduled", "Fitting booked for "+in.StartsAt.In(sin.Loc).Format("2 Jan 15:04"), true); err != nil {
					return err
				}
			}
		}
		if err := audit.Status(ctx, tx, "appointment", id, "", "booked", "Booked by the atelier", true); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "appointments.create", ObjectType: "appointment", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &in.CustomerID, Event: "appointment_booked", DedupeKey: "appt_booked:" + id.String(),
			Title: "Your " + strings.ToLower(Types[in.Type]) + " is booked", Body: "See you " + in.StartsAt.In(sin.Loc).Format("Monday 2 January at 15:04") + ".",
			Link: "/appointments/" + id.String()})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "number": number})
	return nil
}

func (h Handler) OwnerUpdate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Status        *string    `json:"status"`
		StaffID       *uuid.UUID `json:"staffId"`
		InternalNotes *string    `json:"internalNotes"`
		Location      *string    `json:"location"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var cur string
		if err := tx.QueryRow(ctx, `SELECT status FROM appointments WHERE id=$1 FOR UPDATE`, id).Scan(&cur); err != nil {
			return err
		}
		if in.Status != nil && *in.Status != cur {
			if cur != "booked" || (*in.Status != "completed" && *in.Status != "no_show") {
				return httpx.Conflict("invalid_transition", "Only booked appointments can be marked completed or no show.")
			}
			if _, err := tx.Exec(ctx, `UPDATE appointments SET status=$2 WHERE id=$1`, id, *in.Status); err != nil {
				return err
			}
			if err := audit.Status(ctx, tx, "appointment", id, cur, *in.Status, "", true); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE appointments SET staff_id=coalesce($2, staff_id), internal_notes=coalesce($3, internal_notes),
			location=coalesce($4, location), version=version+1 WHERE id=$1`, id, in.StaffID, in.InternalNotes, in.Location); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "appointments.update", ObjectType: "appointment", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) OwnerAvailability(w http.ResponseWriter, r *http.Request) error {
	rules, err := h.rules(r.Context(), h.Pool)
	if err != nil {
		return err
	}
	if rules == nil {
		rules = []Rule{}
	}
	blocks, err := h.blocks(r.Context(), time.Now().Add(-24*time.Hour), time.Now().AddDate(1, 0, 0))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rules": rules, "blocks": blocks})
	return nil
}

func (h Handler) OwnerPutRules(w http.ResponseWriter, r *http.Request) error {
	var rules []Rule
	if err := httpx.Decode(r, &rules); err != nil {
		return err
	}
	for i, x := range rules {
		if x.Weekday < 0 || x.Weekday > 6 || x.StartMinute < 0 || x.EndMinute > 1440 || x.EndMinute <= x.StartMinute {
			return httpx.Validation(map[string]string{fmt.Sprintf("rules.%d", i): "Each window needs a day and a start before its end."})
		}
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM availability_rules`); err != nil {
			return err
		}
		for _, x := range rules {
			if _, err := tx.Exec(ctx, `INSERT INTO availability_rules (weekday, start_minute, end_minute) VALUES ($1,$2,$3)`, x.Weekday, x.StartMinute, x.EndMinute); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "appointments.availability", ObjectType: "availability", After: rules}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) OwnerAddBlock(w http.ResponseWriter, r *http.Request) error {
	var b Block
	if err := httpx.Decode(r, &b); err != nil {
		return err
	}
	if !b.EndsAt.After(b.StartsAt) {
		return httpx.Validation(map[string]string{"endsAt": "The end must be after the start."})
	}
	if b.Kind != "holiday" {
		b.Kind = "blocked"
	}
	ctx := r.Context()
	if err := h.Pool.QueryRow(ctx, `INSERT INTO blocked_periods (starts_at, ends_at, kind, reason, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		b.StartsAt, b.EndsAt, b.Kind, strings.TrimSpace(b.Reason), auth.ActorID(ctx)).Scan(&b.ID); err != nil {
		return err
	}
	_ = audit.Write(ctx, h.Pool, audit.Entry{Action: "appointments.block.add", ObjectType: "blocked_period", ObjectID: b.ID.String(), After: b})
	httpx.JSON(w, http.StatusCreated, b)
	return nil
}

func (h Handler) OwnerDeleteBlock(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	if _, err := h.Pool.Exec(r.Context(), `DELETE FROM blocked_periods WHERE id=$1`, id); err != nil {
		return err
	}
	_ = audit.Write(r.Context(), h.Pool, audit.Entry{Action: "appointments.block.delete", ObjectType: "blocked_period", ObjectID: id.String()})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// SendReminders queues reminders for appointments in the next 24 hours, once per booking time.
func SendReminders(ctx context.Context, pool *pgxpool.Pool) error {
	return db.InTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, customer_id, type, starts_at, number FROM appointments
			WHERE status='booked' AND reminder_sent_at IS NULL AND starts_at BETWEEN now() AND now() + interval '24 hours' FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return err
		}
		type a struct {
			id, customer uuid.UUID
			typ, number  string
			starts       time.Time
		}
		var list []a
		for rows.Next() {
			var x a
			if err := rows.Scan(&x.id, &x.customer, &x.typ, &x.starts, &x.number); err != nil {
				rows.Close()
				return err
			}
			list = append(list, x)
		}
		rows.Close()
		var tz string
		_ = tx.QueryRow(ctx, `SELECT coalesce(value->>'timezone','Africa/Douala') FROM business_settings WHERE key='business'`).Scan(&tz)
		loc, err := time.LoadLocation(tz)
		if err != nil {
			loc, _ = time.LoadLocation("Africa/Douala")
		}
		for _, x := range list {
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &x.customer, Event: "appointment_reminder",
				DedupeKey: "appt_reminder:" + x.id.String() + ":" + x.starts.UTC().Format(time.RFC3339), Title: "Reminder: " + strings.ToLower(Types[x.typ]),
				Body: "Your appointment is " + x.starts.In(loc).Format("Monday 2 January at 15:04") + ".", Link: "/appointments/" + x.id.String()}); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE appointments SET reminder_sent_at=now() WHERE id=$1`, x.id); err != nil {
				return err
			}
		}
		return nil
	})
}
