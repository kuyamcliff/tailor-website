// Package analytics serves the owner dashboard's action summary and business metrics, computed from
// real records only.
package analytics

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
)

type Handler struct {
	Pool     *pgxpool.Pool
	Settings *settings.Service
}

// Dashboard returns what needs attention now.
func (h Handler) Dashboard(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(biz.Timezone)
	if err != nil {
		loc = time.UTC
	}
	y, m, d := time.Now().In(loc).Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
	counts := map[string]int{}
	queries := map[string]string{
		"newRequests":     `SELECT count(*) FROM quote_requests WHERE status='new'`,
		"needsReview":     `SELECT count(*) FROM quote_requests WHERE status IN ('new','reviewing')`,
		"ordersAttention": `SELECT count(*) FROM orders WHERE status IN ('submitted','under_review','measurements_pending','alteration')`,
		"pendingQuotes":   `SELECT count(*) FROM quotes WHERE status IN ('sent','changes_requested')`,
		"draftQuotes":     `SELECT count(*) FROM quotes WHERE status='draft'`,
		"paymentsPending": `SELECT count(*) FROM payments WHERE status IN ('created','pending','customer_action_required','processing')`,
		"overdueOrders":   `SELECT count(*) FROM orders WHERE due_date < current_date AND status NOT IN ('completed','cancelled','refunded','delivered')`,
		"unreadSupport":   `SELECT count(*) FROM support_threads WHERE unread_for_staff > 0`,
		"openSupport":     `SELECT count(*) FROM support_threads WHERE status <> 'resolved'`,
		"lowStockFabrics": `SELECT count(*) FROM fabrics WHERE active AND stock_status IN ('low_stock','out_of_stock')`,
		"awaitingDeposit": `SELECT count(*) FROM orders WHERE status='awaiting_customer'`,
	}
	for k, q := range queries {
		var n int
		if err := h.Pool.QueryRow(ctx, q).Scan(&n); err != nil {
			return err
		}
		counts[k] = n
	}
	lists := map[string]string{
		"todayAppointments": `SELECT coalesce(json_agg(x ORDER BY x.starts_at), '[]') FROM (SELECT a.id, a.number, a.type, a.status, a.starts_at, a.ends_at,
			c.full_name AS customer FROM appointments a JOIN customers c ON c.id=a.customer_id
			WHERE a.starts_at >= $1 AND a.starts_at < $1 + interval '1 day' AND a.status <> 'cancelled') x`,
		"newRequestList": `SELECT coalesce(json_agg(x ORDER BY x.created_at DESC), '[]') FROM (SELECT r.id, r.number, r.contact_name AS customer,
			coalesce(g.name, r.garment_type_key) AS garment, r.occasion, r.urgency, r.created_at FROM quote_requests r
			LEFT JOIN garment_types g ON g.key=r.garment_type_key WHERE r.status IN ('new','reviewing') ORDER BY r.created_at DESC LIMIT 6) x`,
		"overdueList": `SELECT coalesce(json_agg(x ORDER BY x.due_date), '[]') FROM (SELECT id, number, status, due_date, contact->>'name' AS customer
			FROM orders WHERE due_date < current_date AND status NOT IN ('completed','cancelled','refunded','delivered') ORDER BY due_date LIMIT 6) x`,
		"lowStockList": `SELECT coalesce(json_agg(x), '[]') FROM (SELECT key, name, stock_status, stock_meters FROM fabrics
			WHERE active AND stock_status IN ('low_stock','out_of_stock') ORDER BY name LIMIT 8) x`,
		"recentActivity": `SELECT coalesce(json_agg(x ORDER BY x.created_at DESC), '[]') FROM (SELECT a.action, a.object_type, a.object_id, u.full_name AS actor,
			a.created_at FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id ORDER BY a.created_at DESC LIMIT 12) x`,
	}
	out := map[string]any{"counts": counts, "currency": biz.Currency, "timezone": biz.Timezone}
	for k, q := range lists {
		var raw []byte
		args := []any{}
		if k == "todayAppointments" {
			args = append(args, dayStart)
		}
		if err := h.Pool.QueryRow(ctx, q, args...).Scan(&raw); err != nil {
			return err
		}
		out[k] = json.RawMessage(raw)
	}
	if !auth.FromContext(ctx).Can("audit.read") {
		delete(out, "recentActivity")
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

// Metrics returns business metrics over a period (default: last 90 days).
func (h Handler) Metrics(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	days := 90
	switch r.URL.Query().Get("period") {
	case "30":
		days = 30
	case "365":
		days = 365
	}
	since := time.Now().AddDate(0, 0, -days)
	type metric struct {
		key, sql string
	}
	scalars := []metric{
		{"requests", `SELECT count(*)::float8 FROM quote_requests WHERE created_at >= $1`},
		{"quotesDecided", `SELECT count(*)::float8 FROM quotes WHERE decided_at >= $1 AND status IN ('accepted','declined')`},
		{"quotesAccepted", `SELECT count(*)::float8 FROM quotes WHERE decided_at >= $1 AND status='accepted'`},
		{"quotesExpired", `SELECT count(*)::float8 FROM quotes q JOIN quote_revisions rv ON rv.id=q.current_revision_id WHERE q.status='expired' AND rv.expires_at >= $1`},
		{"requestsConverted", `SELECT count(*)::float8 FROM quote_requests WHERE created_at >= $1 AND status='converted'`},
		{"orders", `SELECT count(*)::float8 FROM orders WHERE created_at >= $1 AND status NOT IN ('cancelled')`},
		{"averageOrderValue", `SELECT coalesce(avg(total_minor),0)::float8 FROM orders WHERE created_at >= $1 AND status NOT IN ('cancelled','refunded')`},
		{"revenueCollected", `SELECT coalesce(sum(amount_minor - refunded_minor),0)::float8 FROM payments WHERE succeeded_at >= $1 AND NOT simulated`},
		{"depositsCollected", `SELECT coalesce(sum(amount_minor),0)::float8 FROM payments WHERE succeeded_at >= $1 AND purpose='deposit' AND NOT simulated`},
		{"outstandingBalances", `SELECT coalesce(sum(total_minor - amount_paid_minor + amount_refunded_minor),0)::float8 FROM orders
			WHERE status NOT IN ('cancelled','refunded','completed') AND total_minor > amount_paid_minor - amount_refunded_minor`},
		{"appointmentsBooked", `SELECT count(*)::float8 FROM appointments WHERE created_at >= $1`},
		{"consultationsFollowedByOrder", `SELECT count(DISTINCT a.customer_id)::float8 FROM appointments a WHERE a.created_at >= $1
			AND a.type IN ('consultation','video_consultation') AND a.status='completed'
			AND EXISTS (SELECT 1 FROM orders o WHERE o.customer_id=a.customer_id AND o.created_at >= a.starts_at)`},
		{"consultationsCompleted", `SELECT count(DISTINCT customer_id)::float8 FROM appointments WHERE created_at >= $1
			AND type IN ('consultation','video_consultation') AND status='completed'`},
		{"supportFirstResponseHours", `SELECT coalesce(avg(extract(epoch FROM first_response_at - created_at))/3600, 0)::float8 FROM support_threads
			WHERE created_at >= $1 AND first_response_at IS NOT NULL`},
		{"orderCompletionDays", `SELECT coalesce(avg(extract(epoch FROM h.created_at - o.created_at))/86400, 0)::float8 FROM orders o
			JOIN status_history h ON h.object_type='order' AND h.object_id=o.id AND h.new_status='completed' WHERE o.created_at >= $1`},
	}
	values := map[string]float64{}
	for _, m := range scalars {
		var v float64
		if err := h.Pool.QueryRow(ctx, m.sql, since).Scan(&v); err != nil {
			return err
		}
		values[m.key] = v
	}
	rate := func(a, b float64) *float64 {
		if b == 0 {
			return nil
		}
		v := a / b
		return &v
	}
	breakdowns := map[string]string{
		"topGarments": `SELECT coalesce(json_agg(x), '[]') FROM (SELECT coalesce(g.name, r.garment_type_key) AS label, count(*) AS count FROM quote_requests r
			LEFT JOIN garment_types g ON g.key=r.garment_type_key WHERE r.created_at >= $1 GROUP BY 1 ORDER BY 2 DESC LIMIT 8) x`,
		"topFabrics": `SELECT coalesce(json_agg(x), '[]') FROM (SELECT coalesce(f.name, r.fabric_key) AS label, count(*) AS count FROM quote_requests r
			LEFT JOIN fabrics f ON f.key=r.fabric_key WHERE r.created_at >= $1 AND r.fabric_key IS NOT NULL GROUP BY 1 ORDER BY 2 DESC LIMIT 8) x`,
		"requestsByWeek": `SELECT coalesce(json_agg(x ORDER BY x.week), '[]') FROM (SELECT date_trunc('week', created_at)::date AS week, count(*) AS count
			FROM quote_requests WHERE created_at >= $1 GROUP BY 1) x`,
		"revenueByWeek": `SELECT coalesce(json_agg(x ORDER BY x.week), '[]') FROM (SELECT date_trunc('week', succeeded_at)::date AS week,
			sum(amount_minor - refunded_minor) AS amount FROM payments WHERE succeeded_at >= $1 AND NOT simulated GROUP BY 1) x`,
		"ordersByStatus": `SELECT coalesce(json_agg(x), '[]') FROM (SELECT status AS label, count(*) AS count FROM orders
			WHERE status NOT IN ('completed','cancelled','refunded') AND $1::timestamptz IS NOT NULL GROUP BY 1 ORDER BY 2 DESC) x`,
	}
	out := map[string]any{
		"periodDays": days,
		"values":     values,
		"rates": map[string]*float64{
			"quoteAcceptance":       rate(values["quotesAccepted"], values["quotesDecided"]),
			"requestConversion":     rate(values["requestsConverted"], values["requests"]),
			"appointmentConversion": rate(values["consultationsFollowedByOrder"], values["consultationsCompleted"]),
		},
	}
	for k, q := range breakdowns {
		var raw []byte
		if err := h.Pool.QueryRow(ctx, q, since).Scan(&raw); err != nil {
			return err
		}
		out[k] = json.RawMessage(raw)
	}
	biz, _ := h.Settings.Business(ctx, h.Pool)
	out["currency"] = biz.Currency
	httpx.JSON(w, http.StatusOK, out)
	return nil
}
