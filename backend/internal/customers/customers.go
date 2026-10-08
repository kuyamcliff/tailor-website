// Package customers manages customer records, addresses, wishlists, privacy actions
// (export and deletion) and the owner's customer views.
package customers

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/phone"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

// Contact is customer-provided contact information on requests, orders and bookings.
type Contact struct {
	Name             string  `json:"name"`
	Phone            string  `json:"phone"`
	Email            *string `json:"email"`
	PreferredContact string  `json:"preferredContact"`
}

// Normalize validates contact details; prefix scopes field error keys (e.g. "contact.").
func (c *Contact) Normalize(countryCode, prefix string, f httpx.Fields) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 160 {
		f.Add(prefix+"name", "Enter your name.")
	}
	n, err := phone.Normalize(c.Phone, countryCode)
	if err != nil {
		f.Add(prefix+"phone", "Enter a valid phone number.")
	} else {
		c.Phone = n
	}
	if c.Email != nil {
		e := strings.ToLower(strings.TrimSpace(*c.Email))
		if e == "" {
			c.Email = nil
		} else if a, err := mail.ParseAddress(e); err != nil || a.Address != e {
			f.Add(prefix+"email", "Enter a valid email address or leave it empty.")
		} else {
			c.Email = &e
		}
	}
	switch c.PreferredContact {
	case "phone", "whatsapp", "email", "sms":
	case "":
		c.PreferredContact = "phone"
	default:
		f.Add(prefix+"preferredContact", "Choose how we should contact you.")
	}
	if c.PreferredContact == "email" && c.Email == nil {
		f.Add(prefix+"email", "Add an email address or choose another contact method.")
	}
}

// Resolve returns the signed-in customer's ID, or finds or creates a guest customer for the contact.
// Guest records are matched by phone only among records without an account, so a guest can never
// attach data to someone else's account.
func Resolve(ctx context.Context, tx pgx.Tx, c Contact) (uuid.UUID, error) {
	if p := auth.FromContext(ctx); p != nil && p.CustomerID != nil {
		return *p.CustomerID, nil
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM customers WHERE phone=$1 AND user_id IS NULL AND deleted_at IS NULL ORDER BY created_at LIMIT 1 FOR UPDATE`, c.Phone).Scan(&id)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE customers SET full_name=$2, email=coalesce($3, email), preferred_contact=$4 WHERE id=$1`, id, c.Name, c.Email, c.PreferredContact)
		return id, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO customers (full_name, phone, email, preferred_contact) VALUES ($1,$2,$3,$4) RETURNING id`,
		c.Name, c.Phone, c.Email, c.PreferredContact).Scan(&id)
	return id, err
}

type Handler struct{ Pool *pgxpool.Pool }

type Profile struct {
	ID                uuid.UUID       `json:"id"`
	Name              string          `json:"name"`
	Phone             *string         `json:"phone"`
	Email             *string         `json:"email"`
	PreferredContact  string          `json:"preferredContact"`
	NotificationPrefs map[string]bool `json:"notificationPrefs"`
	MarketingConsent  bool            `json:"marketingConsent"`
	CreatedAt         time.Time       `json:"createdAt"`
}

func requireCustomer(r *http.Request) (uuid.UUID, error) {
	p := auth.FromContext(r.Context())
	if p == nil || p.CustomerID == nil {
		return uuid.Nil, httpx.Unauthorized()
	}
	return *p.CustomerID, nil
}

func (h Handler) GetProfile(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	var p Profile
	if err := h.Pool.QueryRow(r.Context(), `SELECT id, full_name, phone, email, preferred_contact, notification_prefs, marketing_consent, created_at
		FROM customers WHERE id=$1`, cid).Scan(&p.ID, &p.Name, &p.Phone, &p.Email, &p.PreferredContact, &p.NotificationPrefs, &p.MarketingConsent, &p.CreatedAt); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

func (h Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	var in struct {
		Name              string          `json:"name"`
		Phone             string          `json:"phone"`
		PreferredContact  string          `json:"preferredContact"`
		NotificationPrefs map[string]bool `json:"notificationPrefs"`
		MarketingConsent  bool            `json:"marketingConsent"`
		CountryCode       string          `json:"-"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 160 {
		f.Add("name", "Enter your name.")
	}
	var ph *string
	if strings.TrimSpace(in.Phone) != "" {
		var cc string
		_ = h.Pool.QueryRow(ctx, `SELECT coalesce(value->>'countryCode','237') FROM business_settings WHERE key='business'`).Scan(&cc)
		if cc == "" {
			cc = "237"
		}
		n, err := phone.Normalize(in.Phone, cc)
		if err != nil {
			f.Add("phone", "Enter a valid phone number.")
		}
		ph = &n
	}
	switch in.PreferredContact {
	case "phone", "whatsapp", "email", "sms":
	default:
		f.Add("preferredContact", "Choose how we should contact you.")
	}
	prefs := map[string]bool{"email": in.NotificationPrefs["email"], "sms": in.NotificationPrefs["sms"], "in_app": true}
	if err := f.Err(); err != nil {
		return err
	}
	p := auth.FromContext(ctx)
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE customers SET full_name=$2, phone=$3, preferred_contact=$4, notification_prefs=$5, marketing_consent=$6 WHERE id=$1`,
			cid, in.Name, ph, in.PreferredContact, prefs, in.MarketingConsent); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE users SET full_name=$2, phone=$3 WHERE id=$1`, p.UserID, in.Name, ph)
		if db.IsUniqueViolation(err, "") {
			return httpx.Validation(map[string]string{"phone": "This phone number is used by another account."})
		}
		return err
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ClaimGuestData moves designs and uploads created before sign-in on this device to the account.
func (h Handler) ClaimGuestData(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	g := uploads.GuestToken(r)
	if g == "" {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	ctx := r.Context()
	gh := tokens.Hash(g)
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE designs SET customer_id=$1, guest_token_hash=NULL WHERE guest_token_hash=$2 AND customer_id IS NULL`, cid, gh); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE uploads SET customer_id=$1, guest_token_hash=NULL WHERE guest_token_hash=$2 AND customer_id IS NULL`, cid, gh)
		return err
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- Addresses ----

type Address struct {
	ID        uuid.UUID `json:"id"`
	Label     string    `json:"label"`
	Recipient string    `json:"recipient"`
	Phone     string    `json:"phone"`
	Line1     string    `json:"line1"`
	Line2     string    `json:"line2"`
	City      string    `json:"city"`
	Region    string    `json:"region"`
	Country   string    `json:"country"`
	Notes     string    `json:"notes"`
	IsDefault bool      `json:"isDefault"`
}

func (a *Address) Validate(f httpx.Fields, prefix string) {
	a.Recipient, a.Line1, a.City = strings.TrimSpace(a.Recipient), strings.TrimSpace(a.Line1), strings.TrimSpace(a.City)
	if a.Recipient == "" {
		f.Add(prefix+"recipient", "Enter the recipient's name.")
	}
	if strings.TrimSpace(a.Phone) == "" {
		f.Add(prefix+"phone", "Enter a phone number for delivery.")
	}
	if a.Line1 == "" {
		f.Add(prefix+"line1", "Enter the street or landmark.")
	}
	if a.City == "" {
		f.Add(prefix+"city", "Enter the city.")
	}
	if a.Country == "" {
		a.Country = "CM"
	}
	if a.Label == "" {
		a.Label = "Home"
	}
	if len(a.Notes) > 500 || len(a.Line1) > 200 || len(a.Line2) > 200 {
		f.Add(prefix+"notes", "Some fields are too long.")
	}
}

func (h Handler) ListAddresses(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT id, label, recipient, phone, line1, line2, city, region, country, notes, is_default
		FROM addresses WHERE customer_id=$1 ORDER BY is_default DESC, created_at`, cid)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Address{}
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.ID, &a.Label, &a.Recipient, &a.Phone, &a.Line1, &a.Line2, &a.City, &a.Region, &a.Country, &a.Notes, &a.IsDefault); err != nil {
			return err
		}
		out = append(out, a)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

func (h Handler) SaveAddress(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	var a Address
	if err := httpx.Decode(r, &a); err != nil {
		return err
	}
	f := httpx.Fields{}
	a.Validate(f, "")
	if err := f.Err(); err != nil {
		return err
	}
	id, idErr := httpx.PathUUID(r, "id")
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if a.IsDefault {
			if _, err := tx.Exec(ctx, `UPDATE addresses SET is_default=false WHERE customer_id=$1`, cid); err != nil {
				return err
			}
		}
		if idErr != nil { // no id in the path: create
			return tx.QueryRow(ctx, `INSERT INTO addresses (customer_id, label, recipient, phone, line1, line2, city, region, country, notes, is_default)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, $11 OR NOT EXISTS (SELECT 1 FROM addresses WHERE customer_id=$1)) RETURNING id, is_default`,
				cid, a.Label, a.Recipient, a.Phone, a.Line1, a.Line2, a.City, a.Region, a.Country, a.Notes, a.IsDefault).Scan(&a.ID, &a.IsDefault)
		}
		tag, err := tx.Exec(ctx, `UPDATE addresses SET label=$3, recipient=$4, phone=$5, line1=$6, line2=$7, city=$8, region=$9, country=$10, notes=$11,
			is_default = is_default OR $12 WHERE id=$1 AND customer_id=$2`, id, cid, a.Label, a.Recipient, a.Phone, a.Line1, a.Line2, a.City, a.Region,
			a.Country, a.Notes, a.IsDefault)
		if err == nil && tag.RowsAffected() == 0 {
			return httpx.NotFound("Address not found.")
		}
		a.ID = id
		return err
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, a)
	return nil
}

func (h Handler) DeleteAddress(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	if _, err := h.Pool.Exec(r.Context(), `DELETE FROM addresses WHERE id=$1 AND customer_id=$2`, id, cid); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- Wishlist ----

type WishItem struct {
	ItemType  string    `json:"itemType"`
	ItemID    uuid.UUID `json:"itemId"`
	CreatedAt time.Time `json:"createdAt"`
}

func (h Handler) Wishlist(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT item_type, item_id, created_at FROM wishlist_items WHERE customer_id=$1 ORDER BY created_at DESC`, cid)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (WishItem, error) {
		var it WishItem
		err := row.Scan(&it.ItemType, &it.ItemID, &it.CreatedAt)
		return it, err
	})
	if err != nil {
		return err
	}
	if items == nil {
		items = []WishItem{}
	}
	httpx.JSON(w, http.StatusOK, items)
	return nil
}

// AddWishlist merges items (used to sync a guest's local wishlist after sign-in) or toggles one.
func (h Handler) AddWishlist(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	var in struct {
		Items []WishItem `json:"items"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Items) > 200 {
		return httpx.BadRequest("Too many items.")
	}
	ctx := r.Context()
	for _, it := range in.Items {
		if it.ItemType != "product" && it.ItemType != "fabric" && it.ItemType != "design" {
			return httpx.Validation(map[string]string{"itemType": "Unknown item type."})
		}
		if _, err := h.Pool.Exec(ctx, `INSERT INTO wishlist_items (customer_id, item_type, item_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			cid, it.ItemType, it.ItemID); err != nil {
			return err
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h Handler) RemoveWishlist(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	if _, err := h.Pool.Exec(r.Context(), `DELETE FROM wishlist_items WHERE customer_id=$1 AND item_id=$2 AND item_type=$3`,
		cid, id, r.URL.Query().Get("type")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- Privacy ----

// Export returns all personal data held for the signed-in customer as JSON.
func (h Handler) Export(w http.ResponseWriter, r *http.Request) error {
	cid, err := requireCustomer(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	out := map[string]any{"exportedAt": time.Now().UTC()}
	queries := map[string]string{
		"profile":      `SELECT row_to_json(c) FROM (SELECT full_name, phone, email, preferred_contact, notification_prefs, marketing_consent, created_at FROM customers WHERE id=$1) c`,
		"addresses":    `SELECT coalesce(json_agg(a), '[]') FROM (SELECT label, recipient, phone, line1, line2, city, region, country, notes FROM addresses WHERE customer_id=$1) a`,
		"measurements": `SELECT coalesce(json_agg(m), '[]') FROM (SELECT p.name, v.version_no, v.source, v.height_mm, v.values_mm, v.unit, v.created_at FROM measurement_versions v JOIN measurement_profiles p ON p.id=v.profile_id WHERE p.customer_id=$1) m`,
		"designs":      `SELECT coalesce(json_agg(d), '[]') FROM (SELECT d.name, v.snapshot, d.created_at FROM designs d LEFT JOIN design_versions v ON v.id=d.current_version_id WHERE d.customer_id=$1 AND d.deleted_at IS NULL) d`,
		"requests":     `SELECT coalesce(json_agg(q), '[]') FROM (SELECT number, status, garment_type_key, occasion, notes, desired_date, created_at FROM quote_requests WHERE customer_id=$1) q`,
		"orders":       `SELECT coalesce(json_agg(o), '[]') FROM (SELECT number, status, total_minor, currency, payment_status, created_at FROM orders WHERE customer_id=$1) o`,
		"appointments": `SELECT coalesce(json_agg(a), '[]') FROM (SELECT number, type, status, starts_at FROM appointments WHERE customer_id=$1) a`,
		"support":      `SELECT coalesce(json_agg(s), '[]') FROM (SELECT t.number, t.subject, m.body, m.author_type, m.created_at FROM support_threads t JOIN support_messages m ON m.thread_id=t.id WHERE t.customer_id=$1 AND NOT m.internal) s`,
	}
	for k, q := range queries {
		var raw []byte
		if err := h.Pool.QueryRow(ctx, q, cid).Scan(&raw); err != nil {
			return err
		}
		out[k] = jsonRaw(raw)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="my-atelier-data.json"`)
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

type jsonRaw []byte

func (j jsonRaw) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// DeleteAccount anonymizes the customer. Orders and payments are retained for accounting with
// anonymized contact details; measurements, designs, addresses and private uploads are removed.
func (h Handler) DeleteAccount(store uploads.Storage) http.HandlerFunc {
	return httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		ctx := r.Context()
		p := auth.FromContext(ctx)
		if p == nil || p.CustomerID == nil {
			return httpx.Unauthorized()
		}
		if p.IsStaff {
			return httpx.Forbidden()
		}
		var in struct {
			Password string `json:"password"`
		}
		if err := httpx.Decode(r, &in); err != nil {
			return err
		}
		var hash string
		if err := h.Pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, p.UserID).Scan(&hash); err != nil {
			return err
		}
		if !auth.VerifyPassword(in.Password, hash) {
			return httpx.Validation(map[string]string{"password": "Your password is not correct."})
		}
		cid := *p.CustomerID
		var keys []string
		err := db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
			var active int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM orders WHERE customer_id=$1 AND status NOT IN ('completed','cancelled','refunded','delivered')`, cid).Scan(&active); err != nil {
				return err
			}
			if active > 0 {
				return httpx.Conflict("active_orders", "You have orders in progress. Please contact the atelier so we can finish or cancel them before deleting your account.")
			}
			rows, err := tx.Query(ctx, `SELECT storage_key, derivatives FROM uploads WHERE customer_id=$1 AND deleted_at IS NULL`, cid)
			if err != nil {
				return err
			}
			for rows.Next() {
				var k string
				var d map[string]string
				if err := rows.Scan(&k, &d); err != nil {
					rows.Close()
					return err
				}
				keys = append(keys, k)
				for _, v := range d {
					keys = append(keys, v)
				}
			}
			rows.Close()
			stmts := []string{
				`UPDATE uploads SET status='deleted', deleted_at=now() WHERE customer_id=$1 AND deleted_at IS NULL`,
				`DELETE FROM addresses WHERE customer_id=$1`,
				`DELETE FROM wishlist_items WHERE customer_id=$1`,
				`UPDATE designs SET deleted_at=now() WHERE customer_id=$1`,
				`UPDATE measurement_profiles SET deleted_at=now(), is_default=false WHERE customer_id=$1`,
				`UPDATE customers SET full_name='Deleted customer', phone=NULL, email=NULL, internal_notes='', marketing_consent=false, deleted_at=now() WHERE id=$1`,
				`UPDATE orders SET contact='{"name":"Deleted customer","phone":""}'::jsonb, delivery_address=NULL WHERE customer_id=$1`,
				`UPDATE quote_requests SET contact_name='Deleted customer', contact_phone='', contact_email=NULL, notes='' WHERE customer_id=$1`,
			}
			for _, s := range stmts {
				if _, err := tx.Exec(ctx, s, cid); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET status='deleted', deleted_at=now(), email=NULL, phone=id::text, full_name='Deleted user' WHERE id=$1`, p.UserID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1`, p.UserID); err != nil {
				return err
			}
			return audit.Write(ctx, tx, audit.Entry{Action: "privacy.account_deleted", ObjectType: "customer", ObjectID: cid.String()})
		})
		if err != nil {
			return err
		}
		for _, k := range keys {
			_ = store.Delete(ctx, k)
		}
		http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
		w.WriteHeader(http.StatusNoContent)
		return nil
	}).ServeHTTP
}

// ---- Owner ----

type Summary struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	Phone            *string    `json:"phone"`
	Email            *string    `json:"email"`
	PreferredContact string     `json:"preferredContact"`
	HasAccount       bool       `json:"hasAccount"`
	Orders           int        `json:"orders"`
	Requests         int        `json:"requests"`
	LastActivity     *time.Time `json:"lastActivity"`
	CreatedAt        time.Time  `json:"createdAt"`
}

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := h.Pool.Query(r.Context(), `SELECT c.id, c.full_name, c.phone, c.email, c.preferred_contact, c.user_id IS NOT NULL,
			(SELECT count(*) FROM orders o WHERE o.customer_id=c.id), (SELECT count(*) FROM quote_requests q WHERE q.customer_id=c.id),
			greatest((SELECT max(created_at) FROM orders o WHERE o.customer_id=c.id), (SELECT max(created_at) FROM quote_requests q WHERE q.customer_id=c.id)),
			c.created_at, count(*) OVER()
		FROM customers c WHERE c.deleted_at IS NULL AND ($1 = '' OR c.full_name ILIKE '%'||$1||'%' OR c.phone LIKE '%'||$1||'%' OR c.email ILIKE '%'||$1||'%')
		ORDER BY c.created_at DESC LIMIT $2 OFFSET $3`, q, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	var out []Summary
	total := 0
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.ID, &s.Name, &s.Phone, &s.Email, &s.PreferredContact, &s.HasAccount, &s.Orders, &s.Requests, &s.LastActivity, &s.CreatedAt, &total); err != nil {
			return err
		}
		out = append(out, s)
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
	var c struct {
		Summary
		InternalNotes    string `json:"internalNotes"`
		MarketingConsent bool   `json:"marketingConsent"`
	}
	if err := h.Pool.QueryRow(ctx, `SELECT c.id, c.full_name, c.phone, c.email, c.preferred_contact, c.user_id IS NOT NULL, c.internal_notes,
			c.marketing_consent, c.created_at FROM customers c WHERE c.id=$1`, id).
		Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.PreferredContact, &c.HasAccount, &c.InternalNotes, &c.MarketingConsent, &c.CreatedAt); err != nil {
		return err
	}
	related := map[string]string{
		"orders":       `SELECT coalesce(json_agg(x ORDER BY x.created_at DESC), '[]') FROM (SELECT id, number, status, payment_status, total_minor, currency, created_at FROM orders WHERE customer_id=$1) x`,
		"requests":     `SELECT coalesce(json_agg(x ORDER BY x.created_at DESC), '[]') FROM (SELECT id, number, status, garment_type_key, occasion, created_at FROM quote_requests WHERE customer_id=$1) x`,
		"appointments": `SELECT coalesce(json_agg(x ORDER BY x.starts_at DESC), '[]') FROM (SELECT id, number, type, status, starts_at FROM appointments WHERE customer_id=$1) x`,
		"support":      `SELECT coalesce(json_agg(x ORDER BY x.last_message_at DESC), '[]') FROM (SELECT id, number, subject, status, priority, last_message_at FROM support_threads WHERE customer_id=$1) x`,
		"designs":      `SELECT coalesce(json_agg(x ORDER BY x.updated_at DESC), '[]') FROM (SELECT id, name, garment_type_key, updated_at FROM designs WHERE customer_id=$1 AND deleted_at IS NULL) x`,
	}
	out := map[string]any{"customer": c}
	for k, q := range related {
		var raw []byte
		if err := h.Pool.QueryRow(ctx, q, id).Scan(&raw); err != nil {
			return err
		}
		out[k] = jsonRaw(raw)
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h Handler) OwnerUpdate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		InternalNotes    string `json:"internalNotes"`
		PreferredContact string `json:"preferredContact"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	switch in.PreferredContact {
	case "phone", "whatsapp", "email", "sms":
	default:
		return httpx.Validation(map[string]string{"preferredContact": "Choose a contact channel."})
	}
	if len(in.InternalNotes) > 10000 {
		return httpx.Validation(map[string]string{"internalNotes": "Notes are too long."})
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var beforeNotes, beforePC string
		if err := tx.QueryRow(ctx, `SELECT internal_notes, preferred_contact FROM customers WHERE id=$1 FOR UPDATE`, id).Scan(&beforeNotes, &beforePC); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE customers SET internal_notes=$2, preferred_contact=$3 WHERE id=$1`, id, in.InternalNotes, in.PreferredContact); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "customers.update", ObjectType: "customer", ObjectID: id.String(),
			Before: map[string]string{"internalNotes": beforeNotes, "preferredContact": beforePC}, After: in}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}
