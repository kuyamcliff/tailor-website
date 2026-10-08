// Package orders implements ready-made checkout, bespoke order creation from accepted quotes,
// the production workflow, payment bookkeeping and the customer and owner order views.
package orders

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
	"github.com/kuyamcliff/tailor-website/backend/internal/customers"
	"github.com/kuyamcliff/tailor-website/backend/internal/notifications"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/money"
	"github.com/kuyamcliff/tailor-website/backend/internal/pricing"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
)

type Handler struct {
	Pool     *pgxpool.Pool
	Settings *settings.Service
}

// ---- Checkout (ready-made and configured products) ----

type CheckoutItem struct {
	VariantID uuid.UUID `json:"variantId"`
	Quantity  int       `json:"quantity"`
}

type Fulfillment struct {
	Method  string             `json:"method"`
	ZoneKey string             `json:"zoneKey"`
	Address *customers.Address `json:"address"`
	Note    string             `json:"note"`
}

type CheckoutRequest struct {
	Items       []CheckoutItem    `json:"items"`
	Contact     customers.Contact `json:"contact"`
	Fulfillment Fulfillment       `json:"fulfillment"`
	Notes       string            `json:"notes"`
	// ExpectedTotalMinor lets the server detect that prices changed while the customer was checking out.
	ExpectedTotalMinor *int64 `json:"expectedTotalMinor"`
}

type CheckoutResult struct {
	OrderID     uuid.UUID `json:"orderId"`
	Number      string    `json:"number"`
	AccessToken string    `json:"accessToken,omitempty"`
	TotalMinor  int64     `json:"totalMinor"`
	Currency    string    `json:"currency"`
	Replayed    bool      `json:"replayed"`
}

// Checkout creates a ready-made order. Prices and stock are read from the database under row locks,
// never from the client. A repeated Idempotency-Key returns the original order (double-submit safe).
func (h Handler) Checkout(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		return err
	}
	var req CheckoutRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if existing, ok, err := h.replay(ctx, key); err != nil {
		return err
	} else if ok {
		httpx.JSON(w, http.StatusOK, existing)
		return nil
	}
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	if !h.Settings.Flag(ctx, "guest_checkout") && authCustomer(ctx) == nil {
		return httpx.Unauthorized()
	}
	f := httpx.Fields{}
	req.Contact.Normalize(biz.CountryCode, "contact.", f)
	if len(req.Items) == 0 || len(req.Items) > 30 {
		f.Add("items", "Your bag is empty.")
	}
	merged := map[uuid.UUID]int{}
	for _, it := range req.Items {
		if it.Quantity < 1 || it.Quantity > 20 {
			f.Add("items", "Quantities must be between 1 and 20.")
		}
		merged[it.VariantID] += it.Quantity
	}
	var zone *settings.DeliveryZone
	for i := range biz.Delivery {
		z := biz.Delivery[i]
		if z.Active && z.Key == req.Fulfillment.ZoneKey {
			zone = &z
		}
	}
	if zone == nil {
		f.Add("fulfillment.zoneKey", "Choose pickup or a delivery option.")
	} else if zone.Method != "pickup" {
		if req.Fulfillment.Address == nil {
			f.Add("fulfillment.address", "Enter a delivery address.")
		} else {
			req.Fulfillment.Address.Validate(f, "fulfillment.address.")
		}
	}
	if len(req.Notes) > 2000 || len(req.Fulfillment.Note) > 500 {
		f.Add("notes", "Notes are too long.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	var result CheckoutResult
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		ids := make([]uuid.UUID, 0, len(merged))
		for id := range merged {
			ids = append(ids, id)
		}
		// Lock variants in a stable order to avoid deadlocks between concurrent checkouts.
		rows, err := tx.Query(ctx, `SELECT v.id, v.sku, v.size_label, v.color_name, coalesce(v.price_minor, p.price_minor), v.stock_qty, v.made_to_order,
				v.active, p.id, p.name, p.slug, p.visibility, p.version, p.requires_fitting,
				(SELECT url FROM product_media m WHERE m.product_id=p.id ORDER BY sort_order LIMIT 1)
			FROM product_variants v JOIN products p ON p.id=v.product_id WHERE v.id = ANY($1) ORDER BY v.id FOR UPDATE OF v`, ids)
		if err != nil {
			return err
		}
		type line struct {
			variantID, productID                     uuid.UUID
			sku, size, color, name, slug, visibility string
			price                                    int64
			stock                                    int
			mto, active, fitting                     bool
			version                                  int
			image                                    *string
		}
		var lines []line
		for rows.Next() {
			var l line
			if err := rows.Scan(&l.variantID, &l.sku, &l.size, &l.color, &l.price, &l.stock, &l.mto, &l.active, &l.productID, &l.name, &l.slug,
				&l.visibility, &l.version, &l.fitting, &l.image); err != nil {
				rows.Close()
				return err
			}
			lines = append(lines, l)
		}
		rows.Close()
		if len(lines) != len(merged) {
			return httpx.Conflict("item_unavailable", "An item in your bag is no longer available. Please review your bag.")
		}
		var pl []pricing.Line
		for _, l := range lines {
			qty := merged[l.variantID]
			if !l.active || l.visibility != "published" {
				return httpx.Conflict("item_unavailable", l.name+" is no longer available. Please remove it from your bag.")
			}
			if !l.mto && l.stock < qty {
				if l.stock == 0 {
					return httpx.Conflict("out_of_stock", l.name+" ("+l.size+") just sold out. Please remove it from your bag.")
				}
				return httpx.Conflict("insufficient_stock", fmt.Sprintf("Only %d of %s (%s) left. Please update the quantity.", l.stock, l.name, l.size))
			}
			pl = append(pl, pricing.Line{Kind: "garment", Description: l.name + " (" + l.size + ")", Quantity: int64(qty), UnitMinor: l.price})
		}
		if zone.FeeMinor > 0 {
			pl = append(pl, pricing.Line{Kind: "delivery", Description: zone.Label, Quantity: 1, UnitMinor: zone.FeeMinor})
		}
		full := int64(10000)
		totals, err := pricing.Calculate(pricing.Input{Lines: pl, TaxRateBP: int64(biz.TaxRateBP), PricesIncludeTax: biz.PricesIncludeTax, DepositBP: &full})
		if err != nil {
			return err
		}
		if req.ExpectedTotalMinor != nil && *req.ExpectedTotalMinor != totals.TotalMinor {
			return httpx.Conflict("price_changed", fmt.Sprintf("Prices changed while you were checking out. The new total is %s. Please review and confirm.",
				money.Format(totals.TotalMinor, biz.Currency)))
		}
		customerID, err := customers.Resolve(ctx, tx, req.Contact)
		if err != nil {
			return err
		}
		number, err := access.Number(ctx, tx, "order_number_seq", "ORD")
		if err != nil {
			return err
		}
		token, tokenHash := access.NewToken()
		var addr any
		if zone.Method != "pickup" {
			addr = req.Fulfillment.Address
		}
		var orderID uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO orders (number, customer_id, kind, status, currency, subtotal_minor, discount_minor, delivery_minor, tax_minor,
				total_minor, deposit_required_minor, fulfillment_method, delivery_address, delivery_note, contact, customer_notes, access_token_hash, idempotency_key)
			VALUES ($1,$2,'ready_made','submitted',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
			number, customerID, biz.Currency, totals.SubtotalMinor, totals.DiscountMinor, totals.DeliveryMinor, totals.TaxMinor, totals.TotalMinor,
			totals.TotalMinor, zone.Method, addr, strings.TrimSpace(req.Fulfillment.Note), req.Contact, strings.TrimSpace(req.Notes), tokenHash, key).Scan(&orderID)
		if db.IsUniqueViolation(err, "orders_idempotency_key_key") {
			return errReplay
		}
		if err != nil {
			return err
		}
		for _, l := range lines {
			qty := merged[l.variantID]
			snap := map[string]any{"productSlug": l.slug, "productVersion": l.version, "sku": l.sku, "size": l.size, "color": l.color,
				"image": l.image, "requiresFitting": l.fitting}
			if _, err := tx.Exec(ctx, `INSERT INTO order_items (order_id, kind, product_id, variant_id, name, description, snapshot, quantity, unit_price_minor, total_minor)
				VALUES ($1,'product',$2,$3,$4,$5,$6,$7,$8,$9)`, orderID, l.productID, l.variantID, l.name,
				strings.TrimSpace(l.size+" "+l.color), snap, qty, l.price, l.price*int64(qty)); err != nil {
				return err
			}
			if !l.mto {
				if _, err := tx.Exec(ctx, `UPDATE product_variants SET stock_qty = stock_qty - $2 WHERE id=$1`, l.variantID, qty); err != nil {
					return err
				}
			}
		}
		if err := audit.Status(ctx, tx, "order", orderID, "", "submitted", "", true); err != nil {
			return err
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{Audience: "staff", Event: "order_created", DedupeKey: "order_created:" + orderID.String(),
			Title: "New order " + number, Body: req.Contact.Name + " placed an order for " + money.Format(totals.TotalMinor, biz.Currency) + ".",
			Link: "/owner/orders/" + orderID.String(), Channels: []string{"in_app"}}); err != nil {
			return err
		}
		if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "order_created", DedupeKey: "order_created:" + orderID.String(),
			Title: "Order " + number + " received", Body: "Thank you. We have received your order and will be in touch about payment and " + fulfillmentWords(zone.Method) + ".",
			Link: "/orders/" + orderID.String() + "?token=" + token}); err != nil {
			return err
		}
		result = CheckoutResult{OrderID: orderID, Number: number, AccessToken: token, TotalMinor: totals.TotalMinor, Currency: biz.Currency}
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
	httpx.JSON(w, http.StatusCreated, result)
	return nil
}

var errReplay = errors.New("idempotent replay")

func fulfillmentWords(method string) string {
	if method == "pickup" {
		return "pickup"
	}
	return "delivery"
}

// replay returns the order created earlier with this idempotency key. The raw access token is not
// stored, so a replay from a different device without the account cannot obtain it.
func (h Handler) replay(ctx context.Context, key string) (CheckoutResult, bool, error) {
	var res CheckoutResult
	err := h.Pool.QueryRow(ctx, `SELECT id, number, total_minor, currency FROM orders WHERE idempotency_key=$1`, key).
		Scan(&res.OrderID, &res.Number, &res.TotalMinor, &res.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return res, false, nil
	}
	res.Replayed = true
	return res, err == nil, err
}

func authCustomer(ctx context.Context) *uuid.UUID {
	if p := authFrom(ctx); p != nil {
		return p.CustomerID
	}
	return nil
}

// ---- Shared bookkeeping used by quotes and payments ----

// FromQuote is the immutable snapshot an accepted quote turns into.
type FromQuote struct {
	QuoteID              uuid.UUID
	RevisionID           uuid.UUID
	RequestID            *uuid.UUID
	CustomerID           uuid.UUID
	Currency             string
	Lines                []pricing.Line
	Totals               pricing.Totals
	MeasurementVersionID *uuid.UUID
	DesignVersionID      *uuid.UUID
	Contact              customers.Contact
	Urgency              string
	DueDate              *time.Time
	MeasurementsVerified bool
	// AccessTokenHash is shared with the quote so a guest's quote link also opens the order.
	AccessTokenHash []byte
}

// CreateFromQuote creates the bespoke order for an accepted quote inside the caller's transaction.
// A unique index on orders.quote_id guarantees at most one order per quote.
func CreateFromQuote(ctx context.Context, tx pgx.Tx, q FromQuote) (uuid.UUID, string, error) {
	number, err := access.Number(ctx, tx, "order_number_seq", "ORD")
	if err != nil {
		return uuid.Nil, "", err
	}
	hash := q.AccessTokenHash
	status := "awaiting_customer"
	if q.Totals.DepositMinor == 0 {
		status = "measurements_pending"
		if q.MeasurementsVerified {
			status = "measurements_verified"
		}
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO orders (number, customer_id, kind, status, quote_id, quote_revision_id, request_id, measurement_version_id,
			currency, subtotal_minor, discount_minor, delivery_minor, tax_minor, total_minor, deposit_required_minor, contact, urgency, due_date,
			access_token_hash, idempotency_key)
		VALUES ($1,$2,'bespoke',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id`,
		number, q.CustomerID, status, q.QuoteID, q.RevisionID, q.RequestID, q.MeasurementVersionID, q.Currency, q.Totals.SubtotalMinor,
		q.Totals.DiscountMinor, q.Totals.DeliveryMinor, q.Totals.TaxMinor, q.Totals.TotalMinor, q.Totals.DepositMinor, q.Contact,
		orDefault(q.Urgency, "standard"), q.DueDate, hash, "quote:"+q.QuoteID.String()).Scan(&id)
	if err != nil {
		return uuid.Nil, "", err
	}
	for _, l := range q.Lines {
		if l.Kind == "delivery" || l.Kind == "discount" {
			continue
		}
		kind := "fee"
		var dv *uuid.UUID
		if l.Kind == "garment" {
			kind, dv = "bespoke", q.DesignVersionID
		}
		if _, err := tx.Exec(ctx, `INSERT INTO order_items (order_id, kind, design_version_id, name, description, snapshot, quantity, unit_price_minor, total_minor)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, kind, dv, l.Description, strings.ReplaceAll(l.Kind, "_", " "),
			map[string]any{"lineKind": l.Kind, "quoteRevisionId": q.RevisionID}, l.Quantity, l.UnitMinor, l.TotalMinor); err != nil {
			return uuid.Nil, "", err
		}
	}
	if err := audit.Status(ctx, tx, "order", id, "", status, "Created from accepted quote", true); err != nil {
		return uuid.Nil, "", err
	}
	return id, number, nil
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// ApplyPayment records a succeeded payment on the order. It must be called exactly once per payment,
// in the same transaction as the payment's transition to succeeded (the caller guarantees this by
// locking the payment row and only calling on the first transition), which prevents double fulfillment.
func ApplyPayment(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, amount int64) (newStatus string, err error) {
	var total, deposit, paid, refunded int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT total_minor, deposit_required_minor, amount_paid_minor, amount_refunded_minor, status FROM orders WHERE id=$1 FOR UPDATE`,
		orderID).Scan(&total, &deposit, &paid, &refunded, &status); err != nil {
		return "", err
	}
	paid += amount
	ps := pricing.PaymentStatus(total, deposit, paid, refunded)
	newStatus = status
	if status == "awaiting_customer" && (ps == "deposit_paid" || ps == "paid") {
		newStatus = "deposit_paid"
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET amount_paid_minor=$2, payment_status=$3, status=$4, version=version+1 WHERE id=$1`,
		orderID, paid, ps, newStatus); err != nil {
		return "", err
	}
	if newStatus != status {
		if err := audit.Status(ctx, tx, "order", orderID, status, newStatus, "Payment received", true); err != nil {
			return "", err
		}
	}
	return newStatus, nil
}

// ApplyRefund records a refund amount on the order.
func ApplyRefund(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, amount int64) error {
	var total, deposit, paid, refunded int64
	if err := tx.QueryRow(ctx, `SELECT total_minor, deposit_required_minor, amount_paid_minor, amount_refunded_minor FROM orders WHERE id=$1 FOR UPDATE`,
		orderID).Scan(&total, &deposit, &paid, &refunded); err != nil {
		return err
	}
	if refunded+amount > paid {
		return httpx.Validation(map[string]string{"amountMinor": "Refunds cannot exceed what was paid."})
	}
	refunded += amount
	_, err := tx.Exec(ctx, `UPDATE orders SET amount_refunded_minor=$2, payment_status=$3, version=version+1 WHERE id=$1`,
		orderID, refunded, pricing.PaymentStatus(total, deposit, paid, refunded))
	return err
}

// ---- Views ----

type Item struct {
	ID              uuid.UUID       `json:"id"`
	Kind            string          `json:"kind"`
	ProductID       *uuid.UUID      `json:"productId"`
	DesignVersionID *uuid.UUID      `json:"designVersionId"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Snapshot        json.RawMessage `json:"snapshot"`
	Quantity        int             `json:"quantity"`
	UnitPriceMinor  int64           `json:"unitPriceMinor"`
	TotalMinor      int64           `json:"totalMinor"`
}

type Note struct {
	ID         uuid.UUID `json:"id"`
	Visibility string    `json:"visibility"`
	Body       string    `json:"body"`
	Author     *string   `json:"author"`
	CreatedAt  time.Time `json:"createdAt"`
}

type PaymentSummary struct {
	ID            uuid.UUID  `json:"id"`
	Purpose       string     `json:"purpose"`
	Provider      string     `json:"provider"`
	AmountMinor   int64      `json:"amountMinor"`
	Status        string     `json:"status"`
	Simulated     bool       `json:"simulated"`
	FailureMsg    *string    `json:"failureMessage"`
	RefundedMinor int64      `json:"refundedMinor"`
	CreatedAt     time.Time  `json:"createdAt"`
	SucceededAt   *time.Time `json:"succeededAt"`
}

type Fitting struct {
	ID            uuid.UUID       `json:"id"`
	AppointmentID *uuid.UUID      `json:"appointmentId"`
	Notes         string          `json:"notes,omitempty"`
	CustomerNotes string          `json:"customerNotes"`
	Adjustments   json.RawMessage `json:"adjustments"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type AppointmentRef struct {
	ID       uuid.UUID `json:"id"`
	Number   string    `json:"number"`
	Type     string    `json:"type"`
	Status   string    `json:"status"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

type View struct {
	ID                   uuid.UUID           `json:"id"`
	Number               string              `json:"number"`
	Kind                 string              `json:"kind"`
	Status               string              `json:"status"`
	StatusLabel          string              `json:"statusLabel"`
	CustomerID           uuid.UUID           `json:"customerId"`
	QuoteID              *uuid.UUID          `json:"quoteId"`
	QuoteRevisionID      *uuid.UUID          `json:"quoteRevisionId"`
	RequestID            *uuid.UUID          `json:"requestId"`
	MeasurementVersionID *uuid.UUID          `json:"measurementVersionId"`
	Currency             string              `json:"currency"`
	SubtotalMinor        int64               `json:"subtotalMinor"`
	DiscountMinor        int64               `json:"discountMinor"`
	DeliveryMinor        int64               `json:"deliveryMinor"`
	TaxMinor             int64               `json:"taxMinor"`
	TotalMinor           int64               `json:"totalMinor"`
	DepositRequiredMinor int64               `json:"depositRequiredMinor"`
	AmountPaidMinor      int64               `json:"amountPaidMinor"`
	AmountRefundedMinor  int64               `json:"amountRefundedMinor"`
	BalanceMinor         int64               `json:"balanceMinor"`
	PaymentStatus        string              `json:"paymentStatus"`
	FulfillmentMethod    string              `json:"fulfillmentMethod"`
	DeliveryAddress      json.RawMessage     `json:"deliveryAddress"`
	DeliveryNote         string              `json:"deliveryNote"`
	DeliveryStatus       string              `json:"deliveryStatus"`
	Contact              json.RawMessage     `json:"contact"`
	CustomerNotes        string              `json:"customerNotes"`
	Urgency              string              `json:"urgency"`
	DueDate              *time.Time          `json:"dueDate"`
	Version              int                 `json:"version"`
	Items                []Item              `json:"items"`
	History              []audit.HistoryItem `json:"history"`
	Notes                []Note              `json:"notes"`
	Payments             []PaymentSummary    `json:"payments"`
	Fittings             []Fitting           `json:"fittings"`
	Appointments         []AppointmentRef    `json:"appointments"`
	CreatedAt            time.Time           `json:"createdAt"`
	UpdatedAt            time.Time           `json:"updatedAt"`
	accessHash           []byte
}

// Load builds the order view. staff=false hides internal notes, internal history and internal fitting notes.
func Load(ctx context.Context, q db.Querier, id uuid.UUID, staff bool) (*View, error) {
	v := &View{}
	var addr, contact []byte
	err := q.QueryRow(ctx, `SELECT id, number, kind, status, customer_id, quote_id, quote_revision_id, request_id, measurement_version_id, currency,
			subtotal_minor, discount_minor, delivery_minor, tax_minor, total_minor, deposit_required_minor, amount_paid_minor, amount_refunded_minor,
			payment_status, fulfillment_method, delivery_address, delivery_note, delivery_status, contact, customer_notes, urgency, due_date, version,
			created_at, updated_at, access_token_hash
		FROM orders WHERE id=$1`, id).Scan(&v.ID, &v.Number, &v.Kind, &v.Status, &v.CustomerID, &v.QuoteID, &v.QuoteRevisionID, &v.RequestID,
		&v.MeasurementVersionID, &v.Currency, &v.SubtotalMinor, &v.DiscountMinor, &v.DeliveryMinor, &v.TaxMinor, &v.TotalMinor, &v.DepositRequiredMinor,
		&v.AmountPaidMinor, &v.AmountRefundedMinor, &v.PaymentStatus, &v.FulfillmentMethod, &addr, &v.DeliveryNote, &v.DeliveryStatus, &contact,
		&v.CustomerNotes, &v.Urgency, &v.DueDate, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.accessHash)
	if err != nil {
		return nil, err
	}
	v.DeliveryAddress, v.Contact = addr, contact
	v.StatusLabel = CustomerLabel[v.Status]
	v.BalanceMinor = v.TotalMinor - v.AmountPaidMinor + v.AmountRefundedMinor
	if v.Status == "refunded" || v.Status == "cancelled" {
		v.BalanceMinor = 0
	}
	rows, err := q.Query(ctx, `SELECT id, kind, product_id, design_version_id, name, description, snapshot, quantity, unit_price_minor, total_minor
		FROM order_items WHERE order_id=$1 ORDER BY kind DESC, name`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it Item
		var snap []byte
		if err := rows.Scan(&it.ID, &it.Kind, &it.ProductID, &it.DesignVersionID, &it.Name, &it.Description, &snap, &it.Quantity, &it.UnitPriceMinor, &it.TotalMinor); err != nil {
			rows.Close()
			return nil, err
		}
		it.Snapshot = snap
		v.Items = append(v.Items, it)
	}
	rows.Close()
	if v.History, err = audit.History(ctx, q, "order", id, !staff); err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT n.id, n.visibility, n.body, u.full_name, n.created_at FROM order_notes n LEFT JOIN users u ON u.id=n.author_id
		WHERE n.order_id=$1 AND ($2 OR n.visibility='customer') ORDER BY n.created_at`, id, staff)
	if err != nil {
		return nil, err
	}
	v.Notes = []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Visibility, &n.Body, &n.Author, &n.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if !staff {
			n.Author = nil
		}
		v.Notes = append(v.Notes, n)
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT id, purpose, provider, amount_minor, status, simulated, failure_message, refunded_minor, created_at, succeeded_at
		FROM payments WHERE order_id=$1 ORDER BY created_at DESC`, id)
	if err != nil {
		return nil, err
	}
	v.Payments = []PaymentSummary{}
	for rows.Next() {
		var p PaymentSummary
		if err := rows.Scan(&p.ID, &p.Purpose, &p.Provider, &p.AmountMinor, &p.Status, &p.Simulated, &p.FailureMsg, &p.RefundedMinor, &p.CreatedAt, &p.SucceededAt); err != nil {
			rows.Close()
			return nil, err
		}
		v.Payments = append(v.Payments, p)
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT id, appointment_id, notes, customer_notes, adjustments, created_at FROM fitting_sessions WHERE order_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return nil, err
	}
	v.Fittings = []Fitting{}
	for rows.Next() {
		var f Fitting
		var adj []byte
		if err := rows.Scan(&f.ID, &f.AppointmentID, &f.Notes, &f.CustomerNotes, &adj, &f.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		f.Adjustments = adj
		if !staff {
			f.Notes = ""
		}
		v.Fittings = append(v.Fittings, f)
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT id, number, type, status, starts_at, ends_at FROM appointments WHERE order_id=$1 ORDER BY starts_at`, id)
	if err != nil {
		return nil, err
	}
	v.Appointments = []AppointmentRef{}
	for rows.Next() {
		var a AppointmentRef
		if err := rows.Scan(&a.ID, &a.Number, &a.Type, &a.Status, &a.StartsAt, &a.EndsAt); err != nil {
			rows.Close()
			return nil, err
		}
		v.Appointments = append(v.Appointments, a)
	}
	rows.Close()
	if v.Items == nil {
		v.Items = []Item{}
	}
	return v, nil
}

// CustomerGet serves an order to its customer (account or access token).
func (h Handler) CustomerGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	v, err := Load(ctx, h.Pool, id, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("We could not find this order.")
	}
	if err != nil {
		return err
	}
	if !access.Customer(r, v.CustomerID, v.accessHash) {
		return httpx.NotFound("We could not find this order.")
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"order":          v,
		"onlinePayments": h.Settings.Flag(ctx, "online_payments"),
	})
	return nil
}

// MyOrders lists the signed-in customer's orders.
func (h Handler) MyOrders(w http.ResponseWriter, r *http.Request) error {
	cid := authCustomer(r.Context())
	if cid == nil {
		return httpx.Unauthorized()
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT o.id, o.number, o.kind, o.status, o.payment_status, o.total_minor, o.currency, o.created_at,
			(SELECT string_agg(name, ', ') FROM order_items i WHERE i.order_id=o.id AND i.kind <> 'fee')
		FROM orders o WHERE o.customer_id=$1 ORDER BY o.created_at DESC LIMIT 100`, *cid)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		ID            uuid.UUID `json:"id"`
		Number        string    `json:"number"`
		Kind          string    `json:"kind"`
		Status        string    `json:"status"`
		StatusLabel   string    `json:"statusLabel"`
		PaymentStatus string    `json:"paymentStatus"`
		TotalMinor    int64     `json:"totalMinor"`
		Currency      string    `json:"currency"`
		CreatedAt     time.Time `json:"createdAt"`
		Summary       *string   `json:"summary"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Number, &x.Kind, &x.Status, &x.PaymentStatus, &x.TotalMinor, &x.Currency, &x.CreatedAt, &x.Summary); err != nil {
			return err
		}
		x.StatusLabel = CustomerLabel[x.Status]
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

// ---- Owner ----

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	qs := r.URL.Query()
	sort := "o.created_at DESC"
	switch qs.Get("sort") {
	case "oldest":
		sort = "o.created_at"
	case "due":
		sort = "o.due_date NULLS LAST, o.created_at"
	case "total":
		sort = "o.total_minor DESC"
	case "updated":
		sort = "o.updated_at DESC"
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT o.id, o.number, o.kind, o.status, o.payment_status, o.delivery_status, o.total_minor, o.amount_paid_minor,
			o.currency, o.urgency, o.due_date, o.contact->>'name', o.created_at, o.updated_at,
			(SELECT string_agg(name, ', ') FROM order_items i WHERE i.order_id=o.id AND i.kind <> 'fee'),
			coalesce((SELECT q.garment_type_key FROM quote_requests q WHERE q.id=o.request_id), ''),
			count(*) OVER()
		FROM orders o
		WHERE ($1 = '' OR o.status = $1) AND ($2 = '' OR o.payment_status = $2) AND ($3 = '' OR o.kind = $3)
		  AND ($4 = '' OR o.number ILIKE '%'||$4||'%' OR o.contact->>'name' ILIKE '%'||$4||'%' OR o.contact->>'phone' LIKE '%'||$4||'%')
		  AND ($5 = '' OR o.urgency = $5)
		  AND ($6 = '' OR EXISTS (SELECT 1 FROM quote_requests q WHERE q.id=o.request_id AND q.garment_type_key=$6))
		  AND ($7::date IS NULL OR o.created_at >= $7::date) AND ($8::date IS NULL OR o.created_at < $8::date + 1)
		  AND ($9 = '' OR o.customer_id::text = $9)
		  AND (NOT $10 OR (o.due_date < current_date AND o.status NOT IN ('completed','cancelled','refunded','delivered')))
		ORDER BY `+sort+` LIMIT $11 OFFSET $12`,
		qs.Get("status"), qs.Get("paymentStatus"), qs.Get("kind"), strings.TrimSpace(qs.Get("q")), qs.Get("urgency"), qs.Get("garment"),
		dateOrNil(qs.Get("from")), dateOrNil(qs.Get("to")), qs.Get("customerId"), qs.Get("overdue") == "true", page.Limit, page.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		ID             uuid.UUID  `json:"id"`
		Number         string     `json:"number"`
		Kind           string     `json:"kind"`
		Status         string     `json:"status"`
		PaymentStatus  string     `json:"paymentStatus"`
		DeliveryStatus string     `json:"deliveryStatus"`
		TotalMinor     int64      `json:"totalMinor"`
		PaidMinor      int64      `json:"paidMinor"`
		Currency       string     `json:"currency"`
		Urgency        string     `json:"urgency"`
		DueDate        *time.Time `json:"dueDate"`
		CustomerName   *string    `json:"customerName"`
		CreatedAt      time.Time  `json:"createdAt"`
		UpdatedAt      time.Time  `json:"updatedAt"`
		Summary        *string    `json:"summary"`
		Garment        string     `json:"garment"`
	}
	var out []row
	total := 0
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Number, &x.Kind, &x.Status, &x.PaymentStatus, &x.DeliveryStatus, &x.TotalMinor, &x.PaidMinor, &x.Currency,
			&x.Urgency, &x.DueDate, &x.CustomerName, &x.CreatedAt, &x.UpdatedAt, &x.Summary, &x.Garment, &total); err != nil {
			return err
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(out, total, page))
	return rows.Err()
}

func dateOrNil(s string) *string {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return nil
	}
	return &s
}

func (h Handler) OwnerGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	v, err := Load(ctx, h.Pool, id, true)
	if err != nil {
		return err
	}
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	var tasks []map[string]any
	rows, err := h.Pool.Query(ctx, `SELECT t.id, t.stage, t.title, t.status, t.assigned_to, u.full_name, t.due_at, t.completed_at
		FROM production_tasks t LEFT JOIN users u ON u.id=t.assigned_to WHERE t.order_id=$1 ORDER BY t.created_at`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var tid uuid.UUID
		var stage, title, status string
		var assigned *uuid.UUID
		var assignee *string
		var due, done *time.Time
		if err := rows.Scan(&tid, &stage, &title, &status, &assigned, &assignee, &due, &done); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, map[string]any{"id": tid, "stage": stage, "title": title, "status": status, "assignedTo": assigned,
			"assignee": assignee, "dueAt": due, "completedAt": done})
	}
	rows.Close()
	if tasks == nil {
		tasks = []map[string]any{}
	}
	var threads []byte
	if err := h.Pool.QueryRow(ctx, `SELECT coalesce(json_agg(x ORDER BY x.last_message_at DESC), '[]') FROM
		(SELECT id, number, subject, status, last_message_at FROM support_threads WHERE order_id=$1) x`, id).Scan(&threads); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"order": v, "tasks": tasks, "supportThreads": json.RawMessage(threads), "workflow": biz.OrderWorkflow,
		"statusLabels": CustomerLabel,
	})
	return nil
}

type transitionInput struct {
	Status          string `json:"status"`
	Note            string `json:"note"`
	CustomerVisible bool   `json:"customerVisible"`
	Version         int    `json:"version"`
}

// OwnerTransition changes the production stage with optimistic concurrency: two screens changing the
// same order cannot silently overwrite each other.
func (h Handler) OwnerTransition(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in transitionInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Note = strings.TrimSpace(in.Note)
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var cur, number string
		var version int
		var customerID uuid.UUID
		var kind, fulfillment string
		if err := tx.QueryRow(ctx, `SELECT status, version, number, customer_id, kind, fulfillment_method FROM orders WHERE id=$1 FOR UPDATE`, id).
			Scan(&cur, &version, &number, &customerID, &kind, &fulfillment); err != nil {
			return err
		}
		if in.Version != version {
			return httpx.Conflict("stale_version", "This order was updated elsewhere. Reload to see its current stage.")
		}
		tr := Check(cur, in.Status, biz.OrderWorkflow)
		if !tr.Allowed {
			return httpx.Conflict("invalid_transition", tr.Reason)
		}
		if tr.NeedsNote && in.Note == "" {
			return httpx.Validation(map[string]string{"note": "Add a note explaining this change."})
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET status=$2, version=version+1 WHERE id=$1`, id, in.Status); err != nil {
			return err
		}
		if in.Status == "cancelled" && kind == "ready_made" {
			// Return reserved stock for items that were not made to order.
			if _, err := tx.Exec(ctx, `UPDATE product_variants v SET stock_qty = v.stock_qty + i.quantity
				FROM order_items i WHERE i.order_id=$1 AND i.variant_id=v.id AND NOT v.made_to_order`, id); err != nil {
				return err
			}
		}
		delivery := ""
		switch in.Status {
		case "ready":
			if fulfillment == "pickup" {
				delivery = "ready_for_pickup"
			} else {
				delivery = "preparing"
			}
		case "dispatched":
			delivery = "dispatched"
		case "delivered":
			delivery = "delivered"
		}
		if delivery != "" {
			if _, err := tx.Exec(ctx, `UPDATE orders SET delivery_status=$2 WHERE id=$1`, id, delivery); err != nil {
				return err
			}
		}
		if err := audit.Status(ctx, tx, "order", id, cur, in.Status, in.Note, in.CustomerVisible); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "orders.status", ObjectType: "order", ObjectID: id.String(),
			Before: map[string]string{"status": cur}, After: map[string]any{"status": in.Status, "note": in.Note, "backwards": tr.Backwards}}); err != nil {
			return err
		}
		if in.CustomerVisible {
			title, body := "Order "+number+": "+CustomerLabel[in.Status], "Your order has moved to "+strings.ToLower(CustomerLabel[in.Status])+"."
			event := "production_stage_changed"
			switch in.Status {
			case "ready":
				event = "ready"
				if fulfillment == "pickup" {
					body = "Your order is ready for pickup at the studio."
				} else {
					body = "Your order is ready and will be dispatched soon."
				}
			case "dispatched":
				event, body = "dispatched", "Your order is on its way."
			}
			if in.Note != "" {
				body += " " + in.Note
			}
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: event,
				DedupeKey: fmt.Sprintf("order_status:%s:%s:%d", id, in.Status, version+1), Title: title, Body: body, Link: "/orders/" + id.String()}); err != nil {
				return err
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) OwnerAddNote(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" || len(in.Body) > 4000 {
		return httpx.Validation(map[string]string{"body": "Write a note (up to 4000 characters)."})
	}
	if in.Visibility != "internal" && in.Visibility != "customer" {
		return httpx.Validation(map[string]string{"visibility": "Choose internal or visible to customer."})
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var noteID uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO order_notes (order_id, author_id, visibility, body) VALUES ($1,$2,$3,$4) RETURNING id`,
			id, actorID(ctx), in.Visibility, in.Body).Scan(&noteID); err != nil {
			if db.IsForeignKeyViolation(err) {
				return httpx.NotFound("Order not found.")
			}
			return err
		}
		if in.Visibility == "customer" {
			var customerID uuid.UUID
			var number string
			if err := tx.QueryRow(ctx, `SELECT customer_id, number FROM orders WHERE id=$1`, id).Scan(&customerID, &number); err != nil {
				return err
			}
			if err := notifications.Enqueue(ctx, tx, notifications.Notice{CustomerID: &customerID, Event: "order_note",
				DedupeKey: "order_note:" + noteID.String(), Title: "A note about order " + number, Body: in.Body, Link: "/orders/" + id.String()}); err != nil {
				return err
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) OwnerUpdateDetails(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		DeliveryStatus string  `json:"deliveryStatus"`
		Urgency        string  `json:"urgency"`
		DueDate        *string `json:"dueDate"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	switch in.DeliveryStatus {
	case "not_started", "preparing", "ready_for_pickup", "dispatched", "delivered", "picked_up":
	default:
		return httpx.Validation(map[string]string{"deliveryStatus": "Choose a delivery status."})
	}
	if in.Urgency != "standard" && in.Urgency != "soon" && in.Urgency != "urgent" {
		return httpx.Validation(map[string]string{"urgency": "Choose an urgency."})
	}
	var due *time.Time
	if in.DueDate != nil && *in.DueDate != "" {
		t, err := time.Parse("2006-01-02", *in.DueDate)
		if err != nil {
			return httpx.Validation(map[string]string{"dueDate": "Enter a valid date."})
		}
		due = &t
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var before struct {
			DeliveryStatus string     `json:"deliveryStatus"`
			Urgency        string     `json:"urgency"`
			DueDate        *time.Time `json:"dueDate"`
		}
		if err := tx.QueryRow(ctx, `SELECT delivery_status, urgency, due_date FROM orders WHERE id=$1 FOR UPDATE`, id).
			Scan(&before.DeliveryStatus, &before.Urgency, &before.DueDate); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET delivery_status=$2, urgency=$3, due_date=$4, version=version+1 WHERE id=$1`,
			id, in.DeliveryStatus, in.Urgency, due); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "orders.details", ObjectType: "order", ObjectID: id.String(), Before: before, After: in}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func (h Handler) OwnerSaveTask(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		TaskID     *uuid.UUID `json:"taskId"`
		Stage      string     `json:"stage"`
		Title      string     `json:"title"`
		Status     string     `json:"status"`
		AssignedTo *uuid.UUID `json:"assignedTo"`
		DueAt      *time.Time `json:"dueAt"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Title) == "" || (in.Status != "open" && in.Status != "done") {
		return httpx.Validation(map[string]string{"title": "Enter a task and choose its status."})
	}
	if in.AssignedTo != nil {
		var staff bool
		if err := h.Pool.QueryRow(ctx, `SELECT r.is_staff FROM users u JOIN roles r ON r.key=u.role WHERE u.id=$1`, *in.AssignedTo).Scan(&staff); err != nil || !staff {
			return httpx.Validation(map[string]string{"assignedTo": "Assign a staff member."})
		}
	}
	var err2 error
	if in.TaskID == nil {
		_, err2 = h.Pool.Exec(ctx, `INSERT INTO production_tasks (order_id, stage, title, status, assigned_to, due_at, completed_at)
			VALUES ($1,$2,$3,$4,$5,$6, CASE WHEN $4='done' THEN now() END)`, id, in.Stage, strings.TrimSpace(in.Title), in.Status, in.AssignedTo, in.DueAt)
	} else {
		_, err2 = h.Pool.Exec(ctx, `UPDATE production_tasks SET stage=$3, title=$4, status=$5, assigned_to=$6, due_at=$7,
			completed_at = CASE WHEN $5='done' THEN coalesce(completed_at, now()) ELSE NULL END WHERE id=$2 AND order_id=$1`,
			id, *in.TaskID, in.Stage, strings.TrimSpace(in.Title), in.Status, in.AssignedTo, in.DueAt)
	}
	if err2 != nil {
		return err2
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h Handler) OwnerAddFitting(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		AppointmentID *uuid.UUID      `json:"appointmentId"`
		Notes         string          `json:"notes"`
		CustomerNotes string          `json:"customerNotes"`
		Adjustments   json.RawMessage `json:"adjustments"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Adjustments) == 0 {
		in.Adjustments = json.RawMessage(`[]`)
	}
	var adj []struct {
		Area   string `json:"area"`
		Change string `json:"change"`
	}
	if err := json.Unmarshal(in.Adjustments, &adj); err != nil || len(adj) > 50 {
		return httpx.Validation(map[string]string{"adjustments": "Adjustments must be a list of areas and changes."})
	}
	if strings.TrimSpace(in.Notes) == "" && strings.TrimSpace(in.CustomerNotes) == "" && len(adj) == 0 {
		return httpx.Validation(map[string]string{"notes": "Record at least one note or adjustment."})
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var fid uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO fitting_sessions (order_id, appointment_id, notes, customer_notes, adjustments, created_by)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, id, in.AppointmentID, strings.TrimSpace(in.Notes), strings.TrimSpace(in.CustomerNotes),
			[]byte(in.Adjustments), actorID(ctx)).Scan(&fid); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "orders.fitting.add", ObjectType: "order", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}
