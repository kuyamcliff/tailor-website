package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
)

const (
	SessionCookie = "atelier_session"
	CSRFHeader    = "X-CSRF-Token"
	// Staff sessions are shorter lived than customer sessions to protect owner accounts.
	staffSessionTTL = 12 * time.Hour
)

type Sessions struct {
	Pool         *pgxpool.Pool
	TTL          time.Duration
	CookieSecure bool
	CookieDomain string
}

// Create issues a new session for userID and sets the session cookie. Returns the CSRF token.
func (s *Sessions) Create(ctx context.Context, w http.ResponseWriter, userID uuid.UUID, isStaff bool, ip, ua string) (string, error) {
	raw := tokens.New(32)
	csrf := tokens.New(24)
	ttl := s.TTL
	if isStaff && ttl > staffSessionTTL {
		ttl = staffSessionTTL
	}
	expires := time.Now().Add(ttl)
	if len(ua) > 400 {
		ua = ua[:400]
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, csrf_token, ip, user_agent, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, userID, tokens.Hash(raw), csrf, ip, ua, expires)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: raw, Path: "/", Domain: s.CookieDomain, Expires: expires,
		HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	return csrf, nil
}

func (s *Sessions) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", Domain: s.CookieDomain, MaxAge: -1,
		HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

// Revoke ends one session.
func (s *Sessions) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID)
	return err
}

// RevokeAll ends every session of a user, optionally keeping one (the caller's current session).
func (s *Sessions) RevokeAll(ctx context.Context, userID uuid.UUID, keep *uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL AND ($2::uuid IS NULL OR id <> $2)`, userID, keep)
	return err
}

var errNoSession = errors.New("no session")

func (s *Sessions) load(ctx context.Context, raw string) (*Principal, error) {
	if raw == "" {
		return nil, errNoSession
	}
	p := &Principal{}
	var lastSeen time.Time
	err := s.Pool.QueryRow(ctx, `SELECT s.id, s.csrf_token, s.last_seen_at, u.id, u.role, u.full_name, coalesce(u.email,''),
			r.permissions, r.is_staff, c.id
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN roles r ON r.key = u.role
		LEFT JOIN customers c ON c.user_id = u.id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
		  AND u.status = 'active' AND u.deleted_at IS NULL`, tokens.Hash(raw)).
		Scan(&p.SessionID, &p.CSRFToken, &lastSeen, &p.UserID, &p.Role, &p.Name, &p.Email, &p.Permissions, &p.IsStaff, &p.CustomerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNoSession
	}
	if err != nil {
		return nil, err
	}
	if time.Since(lastSeen) > 5*time.Minute {
		_, _ = s.Pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE id = $1`, p.SessionID)
	}
	return p, nil
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// Authenticate attaches the principal for a valid session cookie and enforces the CSRF
// double-submit check on unsafe methods. Anonymous requests pass through.
func (s *Sessions) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(SessionCookie)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		p, err := s.load(r.Context(), c.Value)
		if errors.Is(err, errNoSession) {
			// Stale cookie: clear it and continue anonymously.
			s.ClearCookie(w)
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if !isSafeMethod(r.Method) && r.Header.Get(CSRFHeader) != p.CSRFToken {
			httpx.WriteError(w, r, httpx.NewError(http.StatusForbidden, "csrf_failed",
				"Your session could not be verified. Please refresh the page and try again."))
			return
		}
		ctx := WithPrincipal(r.Context(), p)
		ctx = httpx.WithLogger(ctx, httpx.Logger(ctx).With("user_id", p.UserID.String()))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
