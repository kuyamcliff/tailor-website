// Package settings manages business settings, server-controlled feature flags and editable content blocks.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

// Business is the owner-configurable business profile and policy set.
type Business struct {
	Name              string           `json:"name"`
	Tagline           string           `json:"tagline"`
	LogoURL           string           `json:"logoUrl"`
	Phone             string           `json:"phone"`
	WhatsApp          string           `json:"whatsapp"`
	Email             string           `json:"email"`
	Address           Address          `json:"address"`
	OpeningHours      []OpeningHours   `json:"openingHours"`
	Currency          string           `json:"currency"`
	Locale            string           `json:"locale"`
	Timezone          string           `json:"timezone"`
	CountryCode       string           `json:"countryCode"`
	TaxRateBP         int              `json:"taxRateBp"`
	TaxLabel          string           `json:"taxLabel"`
	PricesIncludeTax  bool             `json:"pricesIncludeTax"`
	QuoteValidityDays int              `json:"quoteValidityDays"`
	DepositPercentBP  int              `json:"depositPercentBp"`
	Appointment       AppointmentRules `json:"appointment"`
	Delivery          []DeliveryZone   `json:"delivery"`
	Social            []SocialLink     `json:"social"`
	OrderWorkflow     []string         `json:"orderWorkflow"`
	RetentionDays     int              `json:"referenceRetentionDays"`
}

type Address struct {
	Line1      string   `json:"line1"`
	Line2      string   `json:"line2"`
	City       string   `json:"city"`
	Region     string   `json:"region"`
	Country    string   `json:"country"`
	PostalCode string   `json:"postalCode"`
	MapURL     string   `json:"mapUrl"`
	Lat        *float64 `json:"lat"`
	Lng        *float64 `json:"lng"`
}

type OpeningHours struct {
	Days  string `json:"days"`
	Hours string `json:"hours"`
}

type AppointmentRules struct {
	SlotMinutes       int            `json:"slotMinutes"`
	BufferMinutes     int            `json:"bufferMinutes"`
	MinNoticeHours    int            `json:"minNoticeHours"`
	MaxAdvanceDays    int            `json:"maxAdvanceDays"`
	Durations         map[string]int `json:"durations"`
	CancelNoticeHours int            `json:"cancelNoticeHours"`
}

type DeliveryZone struct {
	Key         string `json:"key"`
	Method      string `json:"method"` // pickup, local_delivery, courier
	Label       string `json:"label"`
	FeeMinor    int64  `json:"feeMinor"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

type SocialLink struct {
	Network string `json:"network"`
	URL     string `json:"url"`
}

var AllowedSocialNetworks = map[string]bool{"whatsapp": true, "instagram": true, "tiktok": true, "facebook": true, "youtube": true, "x": true}

// DefaultOrderWorkflow is the full production workflow; the owner may disable stages.
var DefaultOrderWorkflow = []string{
	"submitted", "under_review", "quote_sent", "awaiting_customer", "deposit_paid", "measurements_pending",
	"measurements_verified", "material_pending", "patterning", "cutting", "sewing", "quality_check",
	"fitting_scheduled", "fitting", "alteration", "ready", "dispatched", "delivered", "completed",
}

func DefaultBusiness() Business {
	return Business{
		Currency: "XAF", Locale: "fr-CM", Timezone: "Africa/Douala", CountryCode: "237",
		QuoteValidityDays: 14, DepositPercentBP: 5000, TaxLabel: "VAT", PricesIncludeTax: true,
		Appointment: AppointmentRules{SlotMinutes: 30, BufferMinutes: 15, MinNoticeHours: 12, MaxAdvanceDays: 60, CancelNoticeHours: 12,
			Durations: map[string]int{"consultation": 45, "measuring": 30, "fitting": 45, "final_fitting": 30, "pickup": 15,
				"alteration": 30, "video_consultation": 30, "other": 30}},
		Delivery: []DeliveryZone{
			{Key: "pickup", Method: "pickup", Label: "Studio pickup", Active: true},
		},
		OrderWorkflow: DefaultOrderWorkflow,
		RetentionDays: 365,
	}
}

type Service struct {
	Pool  *pgxpool.Pool
	mu    sync.RWMutex
	flags map[string]bool
	at    time.Time
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{Pool: pool} }

// Flag returns a feature flag value with a short cache so flag flips take effect within seconds.
func (s *Service) Flag(ctx context.Context, key string) bool {
	s.mu.RLock()
	fresh := time.Since(s.at) < 10*time.Second
	v, ok := s.flags[key]
	s.mu.RUnlock()
	if fresh && ok {
		return v
	}
	flags, err := s.loadFlags(ctx)
	if err != nil {
		return v // keep last known value on DB hiccups
	}
	return flags[key]
}

func (s *Service) loadFlags(ctx context.Context) (map[string]bool, error) {
	rows, err := s.Pool.Query(ctx, `SELECT key, enabled FROM feature_flags`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var k string
		var e bool
		if err := rows.Scan(&k, &e); err != nil {
			return nil, err
		}
		m[k] = e
	}
	s.mu.Lock()
	s.flags, s.at = m, time.Now()
	s.mu.Unlock()
	return m, rows.Err()
}

func (s *Service) invalidate() {
	s.mu.Lock()
	s.at = time.Time{}
	s.mu.Unlock()
}

// RequireFlag returns 404-style unavailability when a feature is disabled.
func (s *Service) RequireFlag(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.Flag(r.Context(), key) {
				httpx.WriteError(w, r, httpx.NewError(http.StatusServiceUnavailable, "feature_disabled",
					"This feature is not available right now."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Business loads the business profile merged over defaults.
func (s *Service) Business(ctx context.Context, q db.Querier) (Business, error) {
	b := DefaultBusiness()
	var raw []byte
	err := q.QueryRow(ctx, `SELECT value FROM business_settings WHERE key='business'`).Scan(&raw)
	if err == pgx.ErrNoRows {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return b, err
	}
	if len(b.OrderWorkflow) == 0 {
		b.OrderWorkflow = DefaultOrderWorkflow
	}
	return b, nil
}

var hexKey = regexp.MustCompile(`^[a-z0-9_-]+$`)

func validateBusiness(b *Business) error {
	f := httpx.Fields{}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len(b.Name) > 120 {
		f.Add("name", "Enter the business name.")
	}
	if b.QuoteValidityDays < 1 || b.QuoteValidityDays > 180 {
		f.Add("quoteValidityDays", "Quote validity must be between 1 and 180 days.")
	}
	if b.DepositPercentBP < 0 || b.DepositPercentBP > 10000 {
		f.Add("depositPercentBp", "Deposit must be between 0 and 100 percent.")
	}
	if b.TaxRateBP < 0 || b.TaxRateBP > 5000 {
		f.Add("taxRateBp", "Tax rate must be between 0 and 50 percent.")
	}
	if _, err := time.LoadLocation(b.Timezone); err != nil {
		f.Add("timezone", "Unknown timezone.")
	}
	a := b.Appointment
	if a.SlotMinutes < 5 || a.SlotMinutes > 240 || a.BufferMinutes < 0 || a.BufferMinutes > 120 {
		f.Add("appointment", "Check slot length and buffer time.")
	}
	for i, d := range b.Delivery {
		if !hexKey.MatchString(d.Key) || (d.Method != "pickup" && d.Method != "local_delivery" && d.Method != "courier") || d.FeeMinor < 0 {
			f.Add(fmt.Sprintf("delivery.%d", i), "Delivery option is not valid.")
		}
	}
	for i, sl := range b.Social {
		if !AllowedSocialNetworks[sl.Network] || !strings.HasPrefix(sl.URL, "https://") {
			f.Add(fmt.Sprintf("social.%d", i), "Social links must be https URLs for a supported network.")
		}
	}
	valid := map[string]bool{}
	for _, s := range DefaultOrderWorkflow {
		valid[s] = true
	}
	for _, s := range b.OrderWorkflow {
		if !valid[s] {
			f.Add("orderWorkflow", "Unknown production stage "+s+".")
		}
	}
	if b.RetentionDays < 30 || b.RetentionDays > 3650 {
		f.Add("referenceRetentionDays", "Retention must be between 30 and 3650 days.")
	}
	return f.Err()
}

type Handler struct {
	Svc *Service
}

// PublicConfig returns non-sensitive business info and feature flags the storefront needs.
func (h Handler) PublicConfig(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	b, err := h.Svc.Business(ctx, h.Svc.Pool)
	if err != nil {
		return err
	}
	flags, err := h.Svc.loadFlags(ctx)
	if err != nil {
		return err
	}
	public := map[string]bool{}
	for _, k := range []string{"studio", "appointments", "customer_accounts", "support_inbox", "guest_checkout", "online_payments", "reference_analysis", "photo_body_estimation"} {
		public[k] = flags[k]
	}
	// demoContent is true while licensed stock photos seeded for development are still in use,
	// so the storefront can label them instead of implying they show the atelier's own work.
	var demo bool
	if err := h.Svc.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM uploads WHERE license->>'usage' = 'development only' AND deleted_at IS NULL)`).Scan(&demo); err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"demoContent": demo,
		"business": map[string]any{
			"name": b.Name, "tagline": b.Tagline, "logoUrl": b.LogoURL, "phone": b.Phone, "whatsapp": b.WhatsApp,
			"email": b.Email, "address": b.Address, "openingHours": b.OpeningHours, "currency": b.Currency,
			"locale": b.Locale, "timezone": b.Timezone, "countryCode": b.CountryCode, "social": b.Social,
			"delivery": activeDelivery(b.Delivery), "quoteValidityDays": b.QuoteValidityDays, "depositPercentBp": b.DepositPercentBP,
			"taxLabel": b.TaxLabel, "taxRateBp": b.TaxRateBP, "pricesIncludeTax": b.PricesIncludeTax,
		},
		"flags": public,
	})
	return nil
}

func activeDelivery(zs []DeliveryZone) []DeliveryZone {
	out := []DeliveryZone{}
	for _, z := range zs {
		if z.Active {
			out = append(out, z)
		}
	}
	return out
}

func (h Handler) GetBusiness(w http.ResponseWriter, r *http.Request) error {
	b, err := h.Svc.Business(r.Context(), h.Svc.Pool)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, b)
	return nil
}

func (h Handler) PutBusiness(w http.ResponseWriter, r *http.Request) error {
	var b Business
	if err := httpx.Decode(r, &b); err != nil {
		return err
	}
	if err := validateBusiness(&b); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Svc.Pool, func(tx pgx.Tx) error {
		before, err := h.Svc.Business(ctx, tx)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(b)
		if _, err := tx.Exec(ctx, `INSERT INTO business_settings (key, value, updated_by, updated_at) VALUES ('business', $1, $2, now())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			raw, auth.ActorID(ctx)); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "settings.business.update", ObjectType: "business_settings", ObjectID: "business", Before: before, After: b}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, b)
		return nil
	})
}

type Flag struct {
	Key         string    `json:"key"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Blocker     string    `json:"blocker,omitempty"`
}

// FlagBlocker lets the server refuse enabling a flag whose dependency is not configured
// (for example a payment provider without validated credentials).
type FlagBlocker func(ctx context.Context, key string) string

func (h Handler) ListFlags(blocker FlagBlocker) http.HandlerFunc {
	return httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		rows, err := h.Svc.Pool.Query(r.Context(), `SELECT key, enabled, description, updated_at FROM feature_flags ORDER BY key`)
		if err != nil {
			return err
		}
		defer rows.Close()
		var out []Flag
		for rows.Next() {
			var f Flag
			if err := rows.Scan(&f.Key, &f.Enabled, &f.Description, &f.UpdatedAt); err != nil {
				return err
			}
			if blocker != nil {
				f.Blocker = blocker(r.Context(), f.Key)
			}
			out = append(out, f)
		}
		httpx.JSON(w, http.StatusOK, out)
		return rows.Err()
	}).ServeHTTP
}

func (h Handler) SetFlag(blocker FlagBlocker) http.HandlerFunc {
	return httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		key := chi.URLParam(r, "key")
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := httpx.Decode(r, &body); err != nil {
			return err
		}
		ctx := r.Context()
		if body.Enabled && blocker != nil {
			if reason := blocker(ctx, key); reason != "" {
				return httpx.Conflict("flag_blocked", reason)
			}
		}
		if strings.HasPrefix(key, "payments") || key == "online_payments" {
			if err := auth.Check(ctx, "payments.settings"); err != nil {
				return err
			}
		}
		err := db.InTx(ctx, h.Svc.Pool, func(tx pgx.Tx) error {
			var before bool
			if err := tx.QueryRow(ctx, `SELECT enabled FROM feature_flags WHERE key=$1 FOR UPDATE`, key).Scan(&before); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE feature_flags SET enabled=$2, updated_by=$3, updated_at=now() WHERE key=$1`, key, body.Enabled, auth.ActorID(ctx)); err != nil {
				return err
			}
			return audit.Write(ctx, tx, audit.Entry{Action: "settings.flag.update", ObjectType: "feature_flag", ObjectID: key,
				Before: map[string]bool{"enabled": before}, After: map[string]bool{"enabled": body.Enabled}})
		})
		if err != nil {
			return err
		}
		h.Svc.invalidate()
		w.WriteHeader(http.StatusNoContent)
		return nil
	}).ServeHTTP
}

// Content blocks hold structured copy (hero, services, FAQs, policies). Values are plain JSON
// strings and arrays; HTML is never rendered from them, so markup injection is impossible.
var ContentKeys = map[string]bool{
	"home.hero": true, "home.signature": true, "home.process": true, "home.studio": true, "services": true, "faqs": true,
	"about": true, "contact": true, "footer": true, "policy.privacy": true, "policy.terms": true,
	"policy.delivery": true, "policy.alterations": true, "custom.landing": true,
}

var htmlTag = regexp.MustCompile(`<\s*/?\s*[a-zA-Z][^>]*>`)

func validateContent(v any, path string, f httpx.Fields) {
	switch t := v.(type) {
	case string:
		if htmlTag.MatchString(t) {
			f.Add(path, "HTML is not allowed in content fields.")
		}
		if strings.ContainsRune(t, '—') {
			f.Add(path, "Use a comma, colon or hyphen instead of an em dash.")
		}
		if len(t) > 20000 {
			f.Add(path, "Text is too long.")
		}
	case []any:
		for i, x := range t {
			validateContent(x, fmt.Sprintf("%s.%d", path, i), f)
		}
	case map[string]any:
		for k, x := range t {
			validateContent(x, path+"."+k, f)
		}
	case float64, bool, nil:
	default:
		f.Add(path, "Unsupported value.")
	}
}

type Block struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Published bool            `json:"published"`
	Version   int             `json:"version"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func (h Handler) PublicContent(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Svc.Pool.Query(r.Context(), `SELECT key, value FROM content_blocks WHERE published`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		out[k] = v
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

func (h Handler) ListContent(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Svc.Pool.Query(r.Context(), `SELECT key, value, published, version, updated_at FROM content_blocks ORDER BY key`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Block{}
	for rows.Next() {
		var b Block
		var v []byte
		if err := rows.Scan(&b.Key, &v, &b.Published, &b.Version, &b.UpdatedAt); err != nil {
			return err
		}
		b.Value = v
		out = append(out, b)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "keys": sortedKeys(ContentKeys)})
	return rows.Err()
}

func (h Handler) PutContent(w http.ResponseWriter, r *http.Request) error {
	key := chi.URLParam(r, "key")
	if !ContentKeys[key] {
		return httpx.NotFound("Unknown content block.")
	}
	var body struct {
		Value     json.RawMessage `json:"value"`
		Published bool            `json:"published"`
		Version   int             `json:"version"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		return err
	}
	var parsed any
	if err := json.Unmarshal(body.Value, &parsed); err != nil {
		return httpx.BadRequest("Content value must be valid JSON.")
	}
	f := httpx.Fields{}
	validateContent(parsed, "value", f)
	if err := f.Err(); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Svc.Pool, func(tx pgx.Tx) error {
		var before []byte
		var curVersion int
		err := tx.QueryRow(ctx, `SELECT value, version FROM content_blocks WHERE key=$1 FOR UPDATE`, key).Scan(&before, &curVersion)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil && body.Version != curVersion {
			return httpx.Conflict("stale_version", "This content was changed by someone else. Reload to see the latest version.")
		}
		var out Block
		var v []byte
		if err := tx.QueryRow(ctx, `INSERT INTO content_blocks (key, value, published, version, updated_by, updated_at) VALUES ($1,$2,$3,1,$4,now())
			ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, published=EXCLUDED.published, version=content_blocks.version+1,
			updated_by=EXCLUDED.updated_by, updated_at=now()
			RETURNING key, value, published, version, updated_at`, key, []byte(body.Value), body.Published, auth.ActorID(ctx)).
			Scan(&out.Key, &v, &out.Published, &out.Version, &out.UpdatedAt); err != nil {
			return err
		}
		out.Value = v
		if err := audit.Write(ctx, tx, audit.Entry{Action: "content.publish", ObjectType: "content_block", ObjectID: key,
			Before: json.RawMessage(nonEmpty(before)), After: json.RawMessage(body.Value)}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, out)
		return nil
	})
}

func nonEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
