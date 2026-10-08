// Package authapi exposes sign up, sign in, sign out, session and password endpoints.
package authapi

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
	"github.com/kuyamcliff/tailor-website/backend/internal/notifications"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/phone"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
)

const (
	maxFailedLogins = 5
	lockDuration    = 15 * time.Minute
)

type Handler struct {
	Pool     *pgxpool.Pool
	Sessions *auth.Sessions
	Settings *settings.Service
}

type meResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	IsStaff     bool       `json:"isStaff"`
	Permissions []string   `json:"permissions"`
	CustomerID  *uuid.UUID `json:"customerId"`
	CSRFToken   string     `json:"csrfToken"`
}

func meFrom(p *auth.Principal) meResponse {
	perms := p.Permissions
	if perms == nil {
		perms = []string{}
	}
	return meResponse{ID: p.UserID, Name: p.Name, Email: p.Email, Role: p.Role, IsStaff: p.IsStaff,
		Permissions: perms, CustomerID: p.CustomerID, CSRFToken: p.CSRFToken}
}

// Me returns the current principal or 200 with null for anonymous visitors.
func (h Handler) Me(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	p := auth.FromContext(r.Context())
	if p == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"user": nil})
		return nil
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": meFrom(p)})
	return nil
}

type signUpRequest struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	Password         string `json:"password"`
	PreferredContact string `json:"preferredContact"`
	MarketingConsent bool   `json:"marketingConsent"`
}

func normalizeEmail(s string) (string, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "", true
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 {
		return "", false
	}
	return s, true
}

func (h Handler) SignUp(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !h.Settings.Flag(ctx, "customer_accounts") {
		return httpx.NewError(http.StatusServiceUnavailable, "feature_disabled", "Account sign up is not available right now.")
	}
	var req signUpRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	biz, err := h.Settings.Business(ctx, h.Pool)
	if err != nil {
		return err
	}
	f := httpx.Fields{}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 160 {
		f.Add("name", "Enter your full name.")
	}
	email, ok := normalizeEmail(req.Email)
	if !ok || email == "" {
		f.Add("email", "Enter a valid email address.")
	}
	var ph *string
	if strings.TrimSpace(req.Phone) != "" {
		n, err := phone.Normalize(req.Phone, biz.CountryCode)
		if err != nil {
			f.Add("phone", "Enter a valid phone number.")
		}
		ph = &n
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		f.Add("password", "Use between 10 and 128 characters.")
	}
	pc := req.PreferredContact
	if pc == "" {
		pc = "email"
	}
	if pc != "phone" && pc != "whatsapp" && pc != "email" && pc != "sms" {
		f.Add("preferredContact", "Choose a contact method.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	var userID uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO users (email, phone, password_hash, full_name, role) VALUES ($1,$2,$3,$4,'customer') RETURNING id`,
			email, ph, hash, req.Name).Scan(&userID)
		if db.IsUniqueViolation(err, "") {
			return httpx.Validation(map[string]string{"email": "An account with this email or phone already exists. Try signing in."})
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO customers (user_id, full_name, phone, email, preferred_contact, marketing_consent)
			VALUES ($1,$2,$3,$4,$5,$6)`, userID, req.Name, ph, email, pc, req.MarketingConsent); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "auth.signup", ObjectType: "user", ObjectID: userID.String()})
	})
	if err != nil {
		return err
	}
	csrf, err := h.Sessions.Create(ctx, w, userID, false, httpx.ClientIP(ctx), r.UserAgent())
	if err != nil {
		return err
	}
	return h.respondWithSession(w, r, userID, csrf)
}

// respondWithSession reloads the principal by user id so the response matches a fresh /me call.
func (h Handler) respondWithSession(w http.ResponseWriter, r *http.Request, userID uuid.UUID, csrf string) error {
	p := &auth.Principal{UserID: userID}
	err := h.Pool.QueryRow(r.Context(), `SELECT s.id, s.csrf_token, u.role, u.full_name, coalesce(u.email,''), ro.permissions, ro.is_staff, c.id
		FROM sessions s JOIN users u ON u.id=s.user_id JOIN roles ro ON ro.key=u.role LEFT JOIN customers c ON c.user_id=u.id
		WHERE s.user_id=$1 AND s.csrf_token=$2 AND s.revoked_at IS NULL`, userID, csrf).
		Scan(&p.SessionID, &p.CSRFToken, &p.Role, &p.Name, &p.Email, &p.Permissions, &p.IsStaff, &p.CustomerID)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": meFrom(p)})
	return nil
}

type signInRequest struct {
	Identifier string `json:"identifier"` // email or phone
	Password   string `json:"password"`
}

var errBadCredentials = httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "The email, phone or password is not correct.")

func (h Handler) SignIn(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req signInRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	ident := strings.TrimSpace(req.Identifier)
	if ident == "" || req.Password == "" || len(req.Password) > 256 {
		return errBadCredentials
	}
	var (
		userID      uuid.UUID
		hash        string
		status      string
		failed      int
		lockedUntil *time.Time
		isStaff     bool
	)
	query := `SELECT u.id, u.password_hash, u.status, u.failed_login_count, u.locked_until, r.is_staff
		FROM users u JOIN roles r ON r.key = u.role WHERE u.deleted_at IS NULL AND `
	var arg string
	if strings.Contains(ident, "@") {
		query += `lower(u.email) = $1`
		arg = strings.ToLower(ident)
	} else {
		biz, _ := h.Settings.Business(ctx, h.Pool)
		n, err := phone.Normalize(ident, biz.CountryCode)
		if err != nil {
			auth.VerifyPassword(req.Password, auth.DummyHash()) // keep timing similar
			return errBadCredentials
		}
		query += `u.phone = $1`
		arg = n
	}
	err := h.Pool.QueryRow(ctx, query, arg).Scan(&userID, &hash, &status, &failed, &lockedUntil, &isStaff)
	if errors.Is(err, pgx.ErrNoRows) {
		auth.VerifyPassword(req.Password, auth.DummyHash())
		return errBadCredentials
	}
	if err != nil {
		return err
	}
	if lockedUntil != nil && lockedUntil.After(time.Now()) {
		return httpx.NewError(http.StatusTooManyRequests, "account_locked",
			"Too many failed attempts. Please wait 15 minutes or reset your password.")
	}
	if status != "active" {
		auth.VerifyPassword(req.Password, hash)
		return errBadCredentials
	}
	if !auth.VerifyPassword(req.Password, hash) {
		return h.recordFailure(ctx, userID, failed+1)
	}
	if _, err := h.Pool.Exec(ctx, `UPDATE users SET failed_login_count=0, locked_until=NULL, last_login_at=now() WHERE id=$1`, userID); err != nil {
		return err
	}
	csrf, err := h.Sessions.Create(ctx, w, userID, isStaff, httpx.ClientIP(ctx), r.UserAgent())
	if err != nil {
		return err
	}
	actx := auth.WithPrincipal(ctx, &auth.Principal{UserID: userID})
	if err := audit.Write(actx, h.Pool, audit.Entry{Action: "auth.login", ObjectType: "user", ObjectID: userID.String()}); err != nil {
		return err
	}
	return h.respondWithSession(w, r, userID, csrf)
}

func (h Handler) recordFailure(ctx context.Context, userID uuid.UUID, failed int) error {
	var lockUntil *time.Time
	if failed >= maxFailedLogins {
		t := time.Now().Add(lockDuration)
		lockUntil = &t
	}
	if _, err := h.Pool.Exec(ctx, `UPDATE users SET failed_login_count=$2, locked_until=$3 WHERE id=$1`, userID, failed, lockUntil); err != nil {
		return err
	}
	if lockUntil != nil {
		_ = audit.Write(ctx, h.Pool, audit.Entry{Action: "auth.lockout", ObjectType: "user", ObjectID: userID.String(),
			After: map[string]any{"failedAttempts": failed, "lockedUntil": lockUntil}})
		if _, err := h.Pool.Exec(ctx, `UPDATE users SET failed_login_count=0 WHERE id=$1`, userID); err != nil {
			return err
		}
	}
	return errBadCredentials
}

func (h Handler) SignOut(w http.ResponseWriter, r *http.Request) error {
	if p := auth.FromContext(r.Context()); p != nil {
		if err := h.Sessions.Revoke(r.Context(), p.SessionID); err != nil {
			return err
		}
	}
	h.Sessions.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type session struct {
	ID         uuid.UUID `json:"id"`
	IP         *string   `json:"ip"`
	UserAgent  *string   `json:"userAgent"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	Current    bool      `json:"current"`
}

func (h Handler) ListSessions(w http.ResponseWriter, r *http.Request) error {
	p := auth.FromContext(r.Context())
	rows, err := h.Pool.Query(r.Context(), `SELECT id, ip, user_agent, created_at, last_seen_at FROM sessions
		WHERE user_id=$1 AND revoked_at IS NULL AND expires_at > now() ORDER BY last_seen_at DESC`, p.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []session{}
	for rows.Next() {
		var s session
		if err := rows.Scan(&s.ID, &s.IP, &s.UserAgent, &s.CreatedAt, &s.LastSeenAt); err != nil {
			return err
		}
		s.Current = s.ID == p.SessionID
		out = append(out, s)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

// RevokeOtherSessions signs out every other device.
func (h Handler) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) error {
	p := auth.FromContext(r.Context())
	if err := h.Sessions.RevokeAll(r.Context(), p.UserID, &p.SessionID); err != nil {
		return err
	}
	_ = audit.Write(r.Context(), h.Pool, audit.Entry{Action: "auth.sessions.revoke_others", ObjectType: "user", ObjectID: p.UserID.String()})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h Handler) ChangePassword(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := auth.FromContext(ctx)
	var req struct {
		Current string `json:"currentPassword"`
		New     string `json:"newPassword"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := auth.ValidatePassword(req.New); err != nil {
		return httpx.Validation(map[string]string{"newPassword": "Use between 10 and 128 characters."})
	}
	var hash string
	if err := h.Pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, p.UserID).Scan(&hash); err != nil {
		return err
	}
	if !auth.VerifyPassword(req.Current, hash) {
		return httpx.Validation(map[string]string{"currentPassword": "Your current password is not correct."})
	}
	newHash, err := auth.HashPassword(req.New)
	if err != nil {
		return err
	}
	if _, err := h.Pool.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, p.UserID, newHash); err != nil {
		return err
	}
	if err := h.Sessions.RevokeAll(ctx, p.UserID, &p.SessionID); err != nil {
		return err
	}
	_ = audit.Write(ctx, h.Pool, audit.Entry{Action: "auth.password.change", ObjectType: "user", ObjectID: p.UserID.String()})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// RequestPasswordReset always answers 202 so it cannot be used to discover accounts.
func (h Handler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	email, ok := normalizeEmail(req.Email)
	if ok && email != "" {
		var userID uuid.UUID
		err := h.Pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email)=$1 AND deleted_at IS NULL AND status IN ('active','locked')`, email).Scan(&userID)
		if err == nil {
			raw := tokens.New(32)
			err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
				var resetID uuid.UUID
				if err := tx.QueryRow(ctx, `INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES ($1,$2, now() + interval '1 hour') RETURNING id`,
					userID, tokens.Hash(raw)).Scan(&resetID); err != nil {
					return err
				}
				return notifications.Enqueue(ctx, tx, notifications.Notice{
					UserID: &userID, Event: "password_reset", DedupeKey: "password_reset:" + resetID.String(),
					Title: "Reset your password", Channels: []string{"email"},
					Body: "We received a request to reset your password. This link is valid for one hour. If you did not ask for this, you can ignore this message.",
					Link: "/account/reset-password?token=" + raw,
				})
			})
			if err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (h Handler) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		return httpx.Validation(map[string]string{"password": "Use between 10 and 128 characters."})
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	var userID uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE password_resets SET used_at=now() WHERE token_hash=$1 AND used_at IS NULL AND expires_at > now()
			RETURNING user_id`, tokens.Hash(req.Token)).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NewError(http.StatusBadRequest, "invalid_token", "This reset link is invalid or has expired. Request a new one.")
		}
		if err != nil {
			return err
		}
		// A successful reset also unlocks the account (owner lockout recovery path).
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2, failed_login_count=0, locked_until=NULL,
			status = CASE WHEN status='locked' THEN 'active' ELSE status END WHERE id=$1`, userID, hash); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
			return err
		}
		actx := auth.WithPrincipal(ctx, &auth.Principal{UserID: userID})
		return audit.Write(actx, tx, audit.Entry{Action: "auth.password.reset", ObjectType: "user", ObjectID: userID.String()})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
