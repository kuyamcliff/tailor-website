// Package notifications queues customer and staff notifications and delivers them through
// pluggable email and SMS senders. A unique (dedupe_key, channel) constraint guarantees that a
// retried business event never produces a duplicate message.
package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

type Notice struct {
	CustomerID *uuid.UUID
	UserID     *uuid.UUID
	Audience   string // customer or staff
	Event      string
	DedupeKey  string
	Title      string
	Body       string
	Link       string
	Channels   []string // subset of in_app, email, sms; empty means customer preferences
}

// Sender delivers a message to one address.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
	Name() string
}

type Service struct {
	Pool    *pgxpool.Pool
	Email   Sender
	SMS     Sender
	Log     *slog.Logger
	SiteURL string
}

func NewService(pool *pgxpool.Pool, cfg config.Config, log *slog.Logger) *Service {
	return &Service{Pool: pool, Email: newEmailSender(cfg.Email, log), SMS: newSMSSender(cfg.SMS, log), Log: log, SiteURL: cfg.PublicSiteURL}
}

// Enqueue inserts one row per channel inside the caller's transaction. Duplicates are ignored.
func Enqueue(ctx context.Context, q db.Querier, n Notice) error {
	if n.Audience == "" {
		n.Audience = "customer"
	}
	channels := n.Channels
	if len(channels) == 0 {
		channels = []string{"in_app", "email", "sms"}
		if n.CustomerID != nil {
			var prefs map[string]bool
			if err := q.QueryRow(ctx, `SELECT notification_prefs FROM customers WHERE id=$1`, *n.CustomerID).Scan(&prefs); err == nil {
				filtered := channels[:0]
				for _, c := range channels {
					if c == "in_app" || prefs[c] {
						filtered = append(filtered, c)
					}
				}
				channels = filtered
			}
		}
	}
	for _, ch := range channels {
		status := "pending"
		var sentAt *time.Time
		if ch == "in_app" {
			now := time.Now()
			status, sentAt = "sent", &now
		}
		if _, err := q.Exec(ctx, `INSERT INTO notifications (customer_id, user_id, audience, channel, event, dedupe_key, title, body, link, status, sent_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (dedupe_key, channel) DO NOTHING`,
			n.CustomerID, n.UserID, n.Audience, ch, n.Event, n.DedupeKey, n.Title, n.Body, nullable(n.Link), status, sentAt); err != nil {
			return fmt.Errorf("enqueue notification: %w", err)
		}
	}
	return nil
}

// Run delivers pending email and SMS notifications until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.DeliverPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
				s.Log.Error("notification delivery", "error", err)
			}
		}
	}
}

const maxAttempts = 5

// DeliverPending claims a batch with SKIP LOCKED so multiple instances never double-send.
func (s *Service) DeliverPending(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT n.id, n.channel, n.title, n.body, n.link, n.attempts,
			coalesce(c.email, u.email, ''), coalesce(c.phone, u.phone, '')
		FROM notifications n
		LEFT JOIN customers c ON c.id = n.customer_id
		LEFT JOIN users u ON u.id = n.user_id
		WHERE n.status = 'pending' AND n.channel IN ('email','sms')
		ORDER BY n.created_at LIMIT 20 FOR UPDATE OF n SKIP LOCKED`)
	if err != nil {
		return err
	}
	type job struct {
		id                   uuid.UUID
		channel, title, body string
		link                 *string
		attempts             int
		email, phone         string
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.channel, &j.title, &j.body, &j.link, &j.attempts, &j.email, &j.phone); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	for _, j := range jobs {
		body := j.body
		if j.link != nil {
			body += "\n\n" + s.SiteURL + *j.link
		}
		if j.channel == "email" {
			body += EmailFooter(s.SiteURL)
		}
		var sendErr error
		var to string
		var sender Sender
		if j.channel == "email" {
			to, sender = j.email, s.Email
		} else {
			to, sender = j.phone, s.SMS
		}
		if to == "" || sender == nil {
			_, err = tx.Exec(ctx, `UPDATE notifications SET status='skipped', last_error=$2 WHERE id=$1`, j.id, "no address or provider")
			if err != nil {
				return err
			}
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		sendErr = sender.Send(sendCtx, to, j.title, body)
		cancel()
		switch {
		case sendErr == nil:
			_, err = tx.Exec(ctx, `UPDATE notifications SET status='sent', sent_at=now(), attempts=attempts+1 WHERE id=$1`, j.id)
		case errors.Is(sendErr, errDisabled):
			_, err = tx.Exec(ctx, `UPDATE notifications SET status='skipped', last_error='provider disabled' WHERE id=$1`, j.id)
		case j.attempts+1 >= maxAttempts:
			_, err = tx.Exec(ctx, `UPDATE notifications SET status='failed', attempts=attempts+1, last_error=$2 WHERE id=$1`, j.id, sendErr.Error())
		default:
			_, err = tx.Exec(ctx, `UPDATE notifications SET attempts=attempts+1, last_error=$2 WHERE id=$1`, j.id, sendErr.Error())
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

var errDisabled = errors.New("provider disabled")

type disabledSender struct{}

func (disabledSender) Send(context.Context, string, string, string) error { return errDisabled }
func (disabledSender) Name() string                                       { return "disabled" }

// logSender records messages in the structured log for development. Refused in production by config validation.
type logSender struct {
	log  *slog.Logger
	kind string
}

func (l logSender) Send(_ context.Context, to, subject, body string) error {
	l.log.Info("notification (development log sender)", "kind", l.kind, "to", to, "subject", subject, "body", body)
	return nil
}
func (l logSender) Name() string { return "log" }

type smtpSender struct{ cfg config.EmailConfig }

func (s smtpSender) Name() string { return "smtp" }

func (s smtpSender) Send(ctx context.Context, to, subject, body string) error {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(subject, "\r\n") {
		return errors.New("invalid header characters")
	}
	msg := "From: " + s.cfg.From + "\r\nTo: " + to + "\r\nSubject: " + subject +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)
	var a smtp.Auth
	if s.cfg.SMTPUser != "" {
		a = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
	}
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(addr, a, s.cfg.From, []string{to}, []byte(msg)) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newEmailSender(cfg config.EmailConfig, log *slog.Logger) Sender {
	switch cfg.Provider {
	case "smtp":
		if cfg.SMTPHost != "" && cfg.From != "" {
			return smtpSender{cfg: cfg}
		}
		log.Warn("EMAIL_PROVIDER=smtp but SMTP_HOST or EMAIL_FROM missing; email disabled")
		return disabledSender{}
	case "log":
		return logSender{log: log, kind: "email"}
	default:
		return disabledSender{}
	}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Handler exposes in-app notifications to signed-in customers.
type Handler struct{ Pool *pgxpool.Pool }

type Item struct {
	ID        uuid.UUID  `json:"id"`
	Event     string     `json:"event"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      *string    `json:"link"`
	ReadAt    *time.Time `json:"readAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) error {
	p := auth.FromContext(r.Context())
	if p.CustomerID == nil && !p.IsStaff {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": []Item{}, "unread": 0})
		return nil
	}
	audience, subject := "customer", any(p.CustomerID)
	cond := "customer_id = $1"
	if p.IsStaff {
		audience, subject, cond = "staff", p.UserID, "(user_id = $1 OR user_id IS NULL)"
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT id, event, title, body, link, read_at, created_at FROM notifications
		WHERE `+cond+` AND audience = $2 AND channel = 'in_app' ORDER BY created_at DESC LIMIT 50`, subject, audience)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []Item{}
	unread := 0
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Event, &it.Title, &it.Body, &it.Link, &it.ReadAt, &it.CreatedAt); err != nil {
			return err
		}
		if it.ReadAt == nil {
			unread++
		}
		items = append(items, it)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "unread": unread})
	return rows.Err()
}

func (h Handler) MarkRead(w http.ResponseWriter, r *http.Request) error {
	p := auth.FromContext(r.Context())
	var err error
	if p.IsStaff {
		_, err = h.Pool.Exec(r.Context(), `UPDATE notifications SET read_at = now() WHERE audience='staff' AND (user_id=$1 OR user_id IS NULL) AND read_at IS NULL`, p.UserID)
	} else if p.CustomerID != nil {
		_, err = h.Pool.Exec(r.Context(), `UPDATE notifications SET read_at = now() WHERE customer_id=$1 AND audience='customer' AND read_at IS NULL`, *p.CustomerID)
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// EmailFooter explains why the message was sent and how to stop it. The atelier only sends
// service messages about the customer's own orders, requests, appointments and account.
func EmailFooter(siteURL string) string {
	return "\n\n--\nYou are receiving this because of an order, request, appointment or account with us." +
		"\nChoose which updates you get by email: " + siteURL + "/account/settings" +
		"\nNo account? Reply to this message or write to us at " + siteURL + "/support and we will stop emailing you."
}
