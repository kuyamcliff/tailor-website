package payments

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/access"
	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/notifications"
	"github.com/kuyamcliff/tailor-website/backend/internal/orders"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/money"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/phone"
	"github.com/kuyamcliff/tailor-website/backend/internal/pricing"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
)

const paymentTTL = 10 * time.Minute

var providerNames = map[string]string{"mtn": "MTN Mobile Money", "orange": "Orange Money"}

type Service struct {
	Pool      *pgxpool.Pool
	Settings  *settings.Service
	Providers map[string]Provider
	secrets   map[string]string
	APIURL    string
	Log       *slog.Logger
	Metrics   *httpx.Metrics
	// retryBackoff is shortened in tests.
	retryBackoff time.Duration
}

func NewService(cfg config.Config, pool *pgxpool.Pool, st *settings.Service, log *slog.Logger, m *httpx.Metrics) *Service {
	s := &Service{Pool: pool, Settings: st, APIURL: cfg.PublicAPIURL, Log: log, Metrics: m, retryBackoff: 500 * time.Millisecond,
		Providers: map[string]Provider{}, secrets: map[string]string{}}
	if cfg.Payments.DevSimulator && !cfg.IsProduction() {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		secret := hex.EncodeToString(b)
		for _, name := range []string{"mtn", "orange"} {
			s.Providers[name] = NewSimulator(name, 3*time.Second)
			s.secrets[name] = secret
		}
		log.Warn("payment simulator enabled: no real payments will be processed")
		return s
	}
	s.Providers["mtn"] = NewMTN(cfg.Payments)
	s.Providers["orange"] = NewOrange(cfg.Payments)
	s.secrets["mtn"] = cfg.Payments.MTNCallbackSecret
	s.secrets["orange"] = cfg.Payments.OrangeCallbackSecret
	return s
}

// UseProvider replaces a provider (tests).
func (s *Service) UseProvider(p Provider, secret string) {
	s.Providers[p.Name()] = p
	s.secrets[p.Name()] = secret
	s.retryBackoff = 10 * time.Millisecond
}

// FlagBlocker prevents enabling a payment flag before its provider is configured.
func (s *Service) FlagBlocker(_ context.Context, key string) string {
	switch key {
	case "payments_mtn", "payments_orange":
		p := s.Providers[strings.TrimPrefix(key, "payments_")]
		if ok, reason := p.Configured(); !ok {
			return "Add the provider credentials before enabling it. " + reason + "."
		}
	case "online_payments":
		for _, p := range s.Providers {
			if ok, _ := p.Configured(); ok {
				return ""
			}
		}
		return "Configure MTN Mobile Money or Orange Money credentials first."
	}
	return ""
}

type Method struct {
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	Simulated bool   `json:"simulated"`
}

// Available lists the payment methods customers can use right now.
func (s *Service) Available(ctx context.Context) []Method {
	out := []Method{}
	if !s.Settings.Flag(ctx, "online_payments") {
		return out
	}
	for _, name := range []string{"mtn", "orange"} {
		p, ok := s.Providers[name]
		if !ok || !s.Settings.Flag(ctx, "payments_"+name) {
			continue
		}
		if ok, _ := p.Configured(); ok {
			out = append(out, Method{Provider: name, Name: providerNames[name], Simulated: p.Simulated()})
		}
	}
	return out
}

func (s *Service) Methods(w http.ResponseWriter, r *http.Request) error {
	methods := s.Available(r.Context())
	test := false
	for _, m := range methods {
		test = test || m.Simulated
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{"methods": methods, "testMode": test})
	return nil
}

type View struct {
	ID             uuid.UUID  `json:"id"`
	OrderID        uuid.UUID  `json:"orderId"`
	Purpose        string     `json:"purpose"`
	Provider       string     `json:"provider"`
	ProviderName   string     `json:"providerName"`
	AmountMinor    int64      `json:"amountMinor"`
	Currency       string     `json:"currency"`
	Status         Status     `json:"status"`
	Simulated      bool       `json:"simulated"`
	FailureMessage *string    `json:"failureMessage"`
	PayerMasked    string     `json:"payer"`
	ExpiresAt      time.Time  `json:"expiresAt"`
	SucceededAt    *time.Time `json:"succeededAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	Existing       bool       `json:"existing,omitempty"`
}

type row struct {
	View
	reference, txID string
	msisdn          *string
	lastChecked     *time.Time
}

func (s *Service) load(ctx context.Context, q db.Querier, id uuid.UUID, lock bool) (*row, error) {
	sql := `SELECT id, order_id, purpose, provider, amount_minor, currency, status, simulated, failure_message, payer_msisdn, expires_at,
		succeeded_at, created_at, provider_reference, coalesce(provider_transaction_id,''), last_checked_at FROM payments WHERE id=$1`
	if lock {
		sql += " FOR UPDATE"
	}
	p := &row{}
	err := q.QueryRow(ctx, sql, id).Scan(&p.ID, &p.OrderID, &p.Purpose, &p.Provider, &p.AmountMinor, &p.Currency, &p.Status, &p.Simulated,
		&p.FailureMessage, &p.msisdn, &p.ExpiresAt, &p.SucceededAt, &p.CreatedAt, &p.reference, &p.txID, &p.lastChecked)
	if err != nil {
		return nil, err
	}
	p.ProviderName = providerNames[p.Provider]
	if p.Provider == "manual" {
		p.ProviderName = "Recorded by the atelier"
	}
	if p.msisdn != nil {
		p.PayerMasked = phone.Mask(*p.msisdn)
	}
	return p, nil
}

type intentInput struct {
	OrderID  uuid.UUID `json:"orderId"`
	Provider string    `json:"provider"`
	MSISDN   string    `json:"msisdn"`
	Purpose  string    `json:"purpose"`
}

// Intent starts a Mobile Money payment for an order (POST /payments/intent).
func (s *Service) Intent(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		return err
	}
	var in intentInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var provider Provider
	for _, m := range s.Available(ctx) {
		if m.Provider == in.Provider {
			provider = s.Providers[in.Provider]
		}
	}
	if provider == nil {
		return httpx.NewError(http.StatusServiceUnavailable, "provider_unavailable",
			"This payment method is not available right now. Try another method or contact the atelier.")
	}
	biz, err := s.Settings.Business(ctx, s.Pool)
	if err != nil {
		return err
	}
	msisdn, err := phone.Normalize(in.MSISDN, biz.CountryCode)
	if err != nil {
		return httpx.Validation(map[string]string{"msisdn": "Enter the Mobile Money number that will pay."})
	}
	var paymentID uuid.UUID
	existing := false
	err = db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		// Same idempotency key: return the original payment (double click, retry, refresh).
		if err := tx.QueryRow(ctx, `SELECT id FROM payments WHERE idempotency_key=$1`, key).Scan(&paymentID); err == nil {
			existing = true
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var customerID uuid.UUID
		var hash []byte
		var status, currency string
		var total, deposit, paid int64
		if err := tx.QueryRow(ctx, `SELECT customer_id, access_token_hash, status, currency, total_minor, deposit_required_minor, amount_paid_minor
			FROM orders WHERE id=$1 FOR UPDATE`, in.OrderID).Scan(&customerID, &hash, &status, &currency, &total, &deposit, &paid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound("We could not find this order.")
			}
			return err
		}
		if !access.Customer(r, customerID, hash) {
			return httpx.NotFound("We could not find this order.")
		}
		if status == "cancelled" || status == "refunded" || status == "draft" {
			return httpx.Conflict("order_closed", "This order cannot be paid.")
		}
		// An in-flight payment for this order (another tab, a second device) is returned instead of
		// charging twice.
		if err := tx.QueryRow(ctx, `SELECT id FROM payments WHERE order_id=$1 AND status IN ('created','pending','customer_action_required','processing')`,
			in.OrderID).Scan(&paymentID); err == nil {
			existing = true
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		purpose := in.Purpose
		if purpose == "" {
			purpose = "full"
			if deposit > 0 && paid < deposit && deposit < total {
				purpose = "deposit"
			} else if paid > 0 {
				purpose = "balance"
			}
		}
		amount, err := pricing.AmountDue(purpose, total, deposit, paid)
		if errors.Is(err, pricing.ErrNothingDue) {
			return httpx.Conflict("nothing_due", "Nothing is due for this payment right now.")
		}
		if err != nil {
			return httpx.Validation(map[string]string{"purpose": "Choose deposit or balance."})
		}
		ref := uuid.New().String() // MTN requires a UUID v4 reference
		err = tx.QueryRow(ctx, `INSERT INTO payments (order_id, purpose, provider, amount_minor, currency, status, payer_msisdn, provider_reference,
				idempotency_key, expires_at, simulated)
			VALUES ($1,$2,$3,$4,$5,'created',$6,$7,$8,$9,$10) RETURNING id`, in.OrderID, purpose, provider.Name(), amount, currency, msisdn, ref, key,
			time.Now().Add(paymentTTL), provider.Simulated()).Scan(&paymentID)
		if db.IsUniqueViolation(err, "payments_one_active_per_order") {
			return httpx.Conflict("payment_in_progress", "A payment for this order is already in progress. Please wait for it to finish.")
		}
		if err != nil {
			return err
		}
		return audit.Status(ctx, tx, "payment", paymentID, "", string(Created), "", true)
	})
	if err != nil {
		return err
	}
	if !existing {
		s.initiate(ctx, paymentID)
	}
	p, err := s.load(ctx, s.Pool, paymentID, false)
	if err != nil {
		return err
	}
	p.Existing = existing
	status := http.StatusCreated
	if existing {
		status = http.StatusOK
	}
	httpx.JSON(w, status, p.View)
	return nil
}

func (s *Service) callbackURL(provider, ref string) string {
	return fmt.Sprintf("%s/api/v1/payments/%s/webhook?ref=%s&sig=%s", s.APIURL, provider, ref, SignReference(s.secrets[provider], ref))
}

// initiate calls the provider with bounded retries for transient failures. The same reference is reused
// on every retry, so the provider treats retries as the same payment.
func (s *Service) initiate(ctx context.Context, id uuid.UUID) {
	p, err := s.load(ctx, s.Pool, id, false)
	if err != nil {
		s.Log.Error("payment load", "payment_id", id, "error", err)
		return
	}
	provider := s.Providers[p.Provider]
	var order struct{ number string }
	_ = s.Pool.QueryRow(ctx, `SELECT number FROM orders WHERE id=$1`, p.OrderID).Scan(&order.number)
	req := InitiateRequest{Reference: p.reference, PaymentID: p.ID.String(), AmountMinor: p.AmountMinor, Currency: p.Currency,
		MSISDN: deref(p.msisdn), Description: "Order " + order.number, CallbackURL: s.callbackURL(p.Provider, p.reference)}
	var res Result
	for attempt := 1; attempt <= 3; attempt++ {
		start := time.Now()
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		res, err = provider.Initiate(callCtx, req)
		cancel()
		s.recordAttempt(ctx, id, "initiate", res, err, time.Since(start))
		if err == nil || !errors.Is(err, ErrTransient) {
			break
		}
		time.Sleep(s.retryBackoff * time.Duration(attempt))
	}
	switch {
	case err != nil && res.ProviderTxID != "":
		// The provider may have the payment; let reconciliation resolve it rather than failing it.
		res = Result{Status: Processing, ProviderTxID: res.ProviderTxID}
	case err != nil:
		res = Result{Status: Failed, FailureCode: "provider_unavailable",
			FailureMessage: providerNames[p.Provider] + " is not responding right now. Please try again in a few minutes or use another method."}
	}
	if err := s.apply(ctx, id, res, "initiate"); err != nil {
		s.Log.Error("payment apply", "payment_id", id, "error", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Service) recordAttempt(ctx context.Context, id uuid.UUID, op string, res Result, err error, d time.Duration) {
	outcome := string(res.Status)
	detail := map[string]any{}
	if res.FailureCode != "" {
		detail["failureCode"] = res.FailureCode
	}
	for k, v := range res.Raw {
		detail[k] = v
	}
	if err != nil {
		outcome = "error"
		detail["error"] = err.Error()
	}
	var hs *int
	if res.HTTPStatus != 0 {
		hs = &res.HTTPStatus
	}
	if _, e := s.Pool.Exec(context.WithoutCancel(ctx), `INSERT INTO payment_attempts (payment_id, operation, http_status, outcome, detail, duration_ms)
		VALUES ($1,$2,$3,$4,$5,$6)`, id, op, hs, outcome, detail, d.Milliseconds()); e != nil {
		s.Log.Warn("record payment attempt", "error", e)
	}
	s.Log.Info("payment provider call", "payment_id", id, "operation", op, "outcome", outcome, "duration_ms", d.Milliseconds())
}

// apply moves a payment to the provider-reported state under a row lock. Out-of-order or duplicate
// results that would move a final state backwards are ignored.
func (s *Service) apply(ctx context.Context, id uuid.UUID, res Result, source string) error {
	ctx = context.WithoutCancel(ctx)
	return db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		p, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE payments SET last_checked_at=now() WHERE id=$1`, id); err != nil {
			return err
		}
		if res.Status == "" || !canApply(p.Status, res.Status) {
			if res.ProviderTxID != "" && p.txID == "" {
				_, err := tx.Exec(ctx, `UPDATE payments SET provider_transaction_id=$2 WHERE id=$1`, id, res.ProviderTxID)
				return err
			}
			return nil
		}
		var failMsg *string
		if res.FailureMessage != "" {
			failMsg = &res.FailureMessage
		}
		if _, err := tx.Exec(ctx, `UPDATE payments SET status=$2, provider_transaction_id=coalesce(nullif($3,''), provider_transaction_id),
				failure_code=nullif($4,''), failure_message=$5, succeeded_at = CASE WHEN $2='succeeded' THEN now() ELSE succeeded_at END,
				version=version+1 WHERE id=$1`, id, res.Status, res.ProviderTxID, res.FailureCode, failMsg); err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "payment", id, string(p.Status), string(res.Status), source, true); err != nil {
			return err
		}
		var customerID uuid.UUID
		var number string
		if err := tx.QueryRow(ctx, `SELECT customer_id, number FROM orders WHERE id=$1`, p.OrderID).Scan(&customerID, &number); err != nil {
			return err
		}
		label := "Payment"
		if p.Simulated {
			label = "Test payment"
		}
		switch res.Status {
		case Succeeded:
			if _, err := orders.ApplyPayment(ctx, tx, p.OrderID, p.AmountMinor); err != nil {
				return err
			}
			if s.Metrics != nil {
				s.Metrics.Inc("payments_succeeded_total", `provider="`+p.Provider+`"`)
			}
			late := ""
			if p.Status == Expired {
				late = " This payment was confirmed after it had expired locally. Check for a duplicate payment."
			}
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "payment_succeeded",
				DedupeKey: "payment_succeeded:" + id.String(), Title: label + " received for order " + number,
				Body: "We received " + money.Format(p.AmountMinor, p.Currency) + ". Thank you.", Link: "/orders/" + p.OrderID.String()}); err != nil {
				return err
			}
			return notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "payment_succeeded",
				DedupeKey: "payment_succeeded:" + id.String(), Title: label + " received: " + number,
				Body: money.Format(p.AmountMinor, p.Currency) + " via " + providerNames[p.Provider] + "." + late,
				Link: "/owner/orders/" + p.OrderID.String(), Channels: []string{"in_app"}})
		case Failed, Expired, Cancelled:
			if s.Metrics != nil {
				s.Metrics.Inc("payments_unsuccessful_total", `provider="`+p.Provider+`",status="`+string(res.Status)+`"`)
			}
			msg := res.FailureMessage
			if msg == "" {
				msg = "The payment was not completed."
			}
			return notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "payment_failed",
				DedupeKey: "payment_failed:" + id.String(), Title: label + " not completed for order " + number,
				Body: msg + " You can try again from your order page.", Link: "/orders/" + p.OrderID.String(), Channels: []string{"in_app"}})
		}
		return nil
	})
}

// canApply adds one exception to CanTransition: a payment we expired locally may still be confirmed as
// succeeded by the provider (late approval). The money moved, so it must be credited and flagged.
func canApply(from, to Status) bool {
	if from == Expired && to == Succeeded {
		return true
	}
	return CanTransition(from, to)
}

// refresh queries the provider for the latest status and applies it.
func (s *Service) refresh(ctx context.Context, p *row, source string) {
	provider, ok := s.Providers[p.Provider]
	if !ok {
		return
	}
	start := time.Now()
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	res, err := provider.Status(callCtx, p.reference, p.txID)
	cancel()
	s.recordAttempt(ctx, p.ID, "status", res, err, time.Since(start))
	if err != nil {
		_, _ = s.Pool.Exec(context.WithoutCancel(ctx), `UPDATE payments SET last_checked_at=now() WHERE id=$1`, p.ID)
		return
	}
	if res.Status == Pending && time.Now().After(p.ExpiresAt.Add(2*time.Minute)) {
		res = Result{Status: Expired, FailureMessage: "The payment request expired before it was approved. You can start a new payment."}
	}
	if err := s.apply(ctx, p.ID, res, source); err != nil {
		s.Log.Error("payment refresh apply", "payment_id", p.ID, "error", err)
	}
}

// Get returns a payment to its customer, verifying with the provider when it is still in flight.
func (s *Service) Get(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.load(ctx, s.Pool, id, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("We could not find this payment.")
	}
	if err != nil {
		return err
	}
	var customerID uuid.UUID
	var hash []byte
	if err := s.Pool.QueryRow(ctx, `SELECT customer_id, access_token_hash FROM orders WHERE id=$1`, p.OrderID).Scan(&customerID, &hash); err != nil {
		return err
	}
	pr := auth.FromContext(ctx)
	if !access.Customer(r, customerID, hash) && !pr.Can("payments.read") {
		return httpx.NotFound("We could not find this payment.")
	}
	if p.Status.Active() && p.Provider != "manual" && (p.lastChecked == nil || time.Since(*p.lastChecked) > 4*time.Second) {
		s.refresh(ctx, p, "status_check")
		if p, err = s.load(ctx, s.Pool, id, false); err != nil {
			return err
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, p.View)
	return nil
}

// Webhook receives provider notifications. The callback URL carries an HMAC of the payment reference,
// so forged notifications are rejected; accepted notifications are deduplicated and then confirmed
// with a server-side status query before any state changes.
func (s *Service) Webhook(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	name := chi.URLParam(r, "provider")
	provider, ok := s.Providers[name]
	if !ok {
		return httpx.NotFound("Unknown provider.")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return httpx.BadRequest("Unreadable body.")
	}
	ref, sig := r.URL.Query().Get("ref"), r.URL.Query().Get("sig")
	if !VerifyReference(s.secrets[name], ref, sig) {
		s.Log.Warn("payment webhook rejected: bad signature", "provider", name, "ip", httpx.ClientIP(ctx))
		if s.Metrics != nil {
			s.Metrics.Inc("payment_webhooks_rejected_total", `provider="`+name+`"`)
		}
		return httpx.NewError(http.StatusUnauthorized, "invalid_signature", "Invalid callback signature.")
	}
	ev, err := provider.ParseCallback(r, body)
	if err != nil || ev.Reference != ref {
		return httpx.BadRequest("Invalid callback.")
	}
	var paymentID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM payments WHERE provider_reference=$1 AND provider=$2`, ref, name).Scan(&paymentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Unknown payment.")
		}
		return err
	}
	tag, err := s.Pool.Exec(ctx, `INSERT INTO payment_provider_events (provider, event_key, payment_id, payload, verified)
		VALUES ($1,$2,$3,$4,true) ON CONFLICT (provider, event_key) DO NOTHING`, name, ev.EventKey, paymentID, ev.Payload)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		s.Log.Info("duplicate payment callback ignored", "payment_id", paymentID, "provider", name)
		w.WriteHeader(http.StatusOK)
		return nil
	}
	p, err := s.load(ctx, s.Pool, paymentID, false)
	if err != nil {
		return err
	}
	s.refresh(ctx, p, "callback")
	after, _ := s.load(ctx, s.Pool, paymentID, false)
	outcome := ""
	if after != nil {
		outcome = string(after.Status)
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE payment_provider_events SET processed_at=now(), outcome=$3 WHERE provider=$1 AND event_key=$2`, name, ev.EventKey, outcome)
	w.WriteHeader(http.StatusOK)
	return nil
}

// Reconcile polls in-flight payments so customers who closed the page, delayed callbacks and lost
// callbacks are all resolved from the provider's own records.
func (s *Service) Reconcile(ctx context.Context) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM payments WHERE status IN ('created','pending','customer_action_required','processing')
		AND provider <> 'manual' AND (last_checked_at IS NULL OR last_checked_at < now() - interval '20 seconds') ORDER BY last_checked_at NULLS FIRST LIMIT 50`)
	if err != nil {
		s.Log.Error("reconcile query", "error", err)
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return
	}
	for _, id := range ids {
		p, err := s.load(ctx, s.Pool, id, false)
		if err != nil {
			continue
		}
		if p.Status == Created && time.Since(p.CreatedAt) > time.Minute {
			// Initiation never ran to completion (for example the server restarted mid-request).
			s.initiate(ctx, p.ID)
			continue
		}
		s.refresh(ctx, p, "reconciliation")
	}
}

// ---- Owner operations ----

func (s *Service) OwnerList(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	rows, err := s.Pool.Query(r.Context(), `SELECT p.id, p.order_id, o.number, o.contact->>'name', p.purpose, p.provider, p.amount_minor, p.currency,
			p.status, p.simulated, p.refunded_minor, p.failure_message, p.provider_transaction_id, p.created_at, p.succeeded_at, count(*) OVER()
		FROM payments p JOIN orders o ON o.id=p.order_id WHERE ($1 = '' OR p.status=$1) AND ($2 = '' OR p.provider=$2)
		ORDER BY p.created_at DESC LIMIT $3 OFFSET $4`, r.URL.Query().Get("status"), r.URL.Query().Get("provider"), page.Limit, page.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	type item struct {
		ID           uuid.UUID  `json:"id"`
		OrderID      uuid.UUID  `json:"orderId"`
		OrderNumber  string     `json:"orderNumber"`
		Customer     *string    `json:"customer"`
		Purpose      string     `json:"purpose"`
		Provider     string     `json:"provider"`
		AmountMinor  int64      `json:"amountMinor"`
		Currency     string     `json:"currency"`
		Status       string     `json:"status"`
		Simulated    bool       `json:"simulated"`
		Refunded     int64      `json:"refundedMinor"`
		Failure      *string    `json:"failureMessage"`
		ProviderTxID *string    `json:"providerTransactionId"`
		CreatedAt    time.Time  `json:"createdAt"`
		SucceededAt  *time.Time `json:"succeededAt"`
	}
	var out []item
	total := 0
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.OrderID, &it.OrderNumber, &it.Customer, &it.Purpose, &it.Provider, &it.AmountMinor, &it.Currency, &it.Status,
			&it.Simulated, &it.Refunded, &it.Failure, &it.ProviderTxID, &it.CreatedAt, &it.SucceededAt, &total); err != nil {
			return err
		}
		out = append(out, it)
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(out, total, page))
	return rows.Err()
}

// OwnerRecheck forces a provider status check (for staff resolving a customer's question).
func (s *Service) OwnerRecheck(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.load(ctx, s.Pool, id, false)
	if err != nil {
		return err
	}
	if p.Provider != "manual" {
		s.refresh(ctx, p, "staff_recheck")
	}
	if p, err = s.load(ctx, s.Pool, id, false); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p.View)
	return nil
}

// OwnerRecordManual records an offline payment (cash or bank transfer) received by the atelier.
func (s *Service) OwnerRecordManual(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	orderID, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		return err
	}
	var in struct {
		AmountMinor int64  `json:"amountMinor"`
		Purpose     string `json:"purpose"`
		Note        string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Purpose != "deposit" && in.Purpose != "balance" && in.Purpose != "full" {
		return httpx.Validation(map[string]string{"purpose": "Choose deposit, balance or full payment."})
	}
	if strings.TrimSpace(in.Note) == "" {
		return httpx.Validation(map[string]string{"note": "Describe how the payment was received (for example cash at the studio)."})
	}
	err = db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payments WHERE idempotency_key=$1)`, key).Scan(&exists); err != nil || exists {
			return err
		}
		var total, paid, refunded int64
		var currency, status string
		if err := tx.QueryRow(ctx, `SELECT total_minor, amount_paid_minor, amount_refunded_minor, currency, status FROM orders WHERE id=$1 FOR UPDATE`, orderID).
			Scan(&total, &paid, &refunded, &currency, &status); err != nil {
			return err
		}
		if status == "cancelled" || status == "refunded" {
			return httpx.Conflict("order_closed", "This order is closed.")
		}
		if in.AmountMinor <= 0 || in.AmountMinor > total-paid {
			return httpx.Validation(map[string]string{"amountMinor": "Enter an amount up to the outstanding balance of " + money.Format(total-paid, currency) + "."})
		}
		var pid uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO payments (order_id, purpose, provider, amount_minor, currency, status, provider_reference, idempotency_key,
				expires_at, succeeded_at, method_note, recorded_by)
			VALUES ($1,$2,'manual',$3,$4,'succeeded',$5,$6, now(), now(), $7, $8) RETURNING id`, orderID, in.Purpose, in.AmountMinor, currency,
			"manual-"+uuid.NewString(), key, strings.TrimSpace(in.Note), auth.ActorID(ctx)).Scan(&pid); err != nil {
			return err
		}
		if _, err := orders.ApplyPayment(ctx, tx, orderID, in.AmountMinor); err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "payment", pid, "", "succeeded", "Recorded by staff: "+strings.TrimSpace(in.Note), true); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "payments.manual", ObjectType: "order", ObjectID: orderID.String(), After: in})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// OwnerRefund records a refund against a succeeded payment. Provider refund APIs are merchant-specific,
// so refunds are paid out by the atelier and recorded here (method "manual").
func (s *Service) OwnerRefund(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		AmountMinor int64  `json:"amountMinor"`
		Reason      string `json:"reason"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Reason) == "" {
		return httpx.Validation(map[string]string{"reason": "Record why this refund is given."})
	}
	return db.InTx(ctx, s.Pool, func(tx pgx.Tx) error {
		p, err := s.load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		var refunded int64
		if err := tx.QueryRow(ctx, `SELECT refunded_minor FROM payments WHERE id=$1`, id).Scan(&refunded); err != nil {
			return err
		}
		if p.Status != Succeeded && p.Status != PartiallyRefunded {
			return httpx.Conflict("not_refundable", "Only completed payments can be refunded.")
		}
		if in.AmountMinor <= 0 || in.AmountMinor > p.AmountMinor-refunded {
			return httpx.Validation(map[string]string{"amountMinor": "Enter an amount up to " + money.Format(p.AmountMinor-refunded, p.Currency) + "."})
		}
		refunded += in.AmountMinor
		newStatus := PartiallyRefunded
		if refunded == p.AmountMinor {
			newStatus = Refunded
		}
		if _, err := tx.Exec(ctx, `INSERT INTO refunds (payment_id, amount_minor, reason, method, status, created_by) VALUES ($1,$2,$3,'manual','succeeded',$4)`,
			id, in.AmountMinor, strings.TrimSpace(in.Reason), auth.ActorID(ctx)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE payments SET refunded_minor=$2, status=$3, version=version+1 WHERE id=$1`, id, refunded, newStatus); err != nil {
			return err
		}
		if err := orders.ApplyRefund(ctx, tx, p.OrderID, in.AmountMinor); err != nil {
			return err
		}
		if err := audit.Status(ctx, tx, "payment", id, string(p.Status), string(newStatus), strings.TrimSpace(in.Reason), true); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "payments.refund", ObjectType: "payment", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		var customerID uuid.UUID
		var number string
		if err := tx.QueryRow(ctx, `SELECT customer_id, number FROM orders WHERE id=$1`, p.OrderID).Scan(&customerID, &number); err != nil {
			return err
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "refund_recorded",
			DedupeKey: fmt.Sprintf("refund:%s:%d", id, refunded), Title: "Refund for order " + number,
			Body: "A refund of " + money.Format(in.AmountMinor, p.Currency) + " has been recorded.", Link: "/orders/" + p.OrderID.String()}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}
