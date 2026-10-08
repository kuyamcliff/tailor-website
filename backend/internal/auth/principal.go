// Package auth implements local credential authentication, server-side sessions,
// CSRF protection and role-based permissions.
package auth

import (
	"context"
	"net/http"
	"slices"

	"github.com/google/uuid"

	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

// Principal is the authenticated actor for a request.
type Principal struct {
	UserID      uuid.UUID
	SessionID   uuid.UUID
	Role        string
	Name        string
	Email       string
	Permissions []string
	IsStaff     bool
	CustomerID  *uuid.UUID
	CSRFToken   string
}

func (p *Principal) Can(perm string) bool {
	return p != nil && p.IsStaff && slices.Contains(p.Permissions, perm)
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the authenticated principal or nil for anonymous requests.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

// ActorID returns the user id for audit and history records, or nil when anonymous.
func ActorID(ctx context.Context) *uuid.UUID {
	if p := FromContext(ctx); p != nil {
		id := p.UserID
		return &id
	}
	return nil
}

// ActorLabel describes the actor for status histories shown to customers.
func ActorLabel(ctx context.Context) string {
	p := FromContext(ctx)
	switch {
	case p == nil:
		return "customer"
	case p.IsStaff:
		return "atelier"
	default:
		return "customer"
	}
}

// RequireUser rejects anonymous requests.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()) == nil {
			httpx.WriteError(w, r, httpx.Unauthorized())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireStaff rejects anyone who is not an active staff member.
func RequireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := FromContext(r.Context())
		if p == nil {
			httpx.WriteError(w, r, httpx.Unauthorized())
			return
		}
		if !p.IsStaff {
			httpx.WriteError(w, r, httpx.Forbidden())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission enforces a granular permission. It implies RequireStaff.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := FromContext(r.Context())
			if p == nil {
				httpx.WriteError(w, r, httpx.Unauthorized())
				return
			}
			if !p.Can(perm) {
				httpx.WriteError(w, r, httpx.Forbidden())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Check returns a Forbidden error when the principal lacks perm. Used inside handlers
// whose behavior depends on a permission (e.g. editing fit rules within a broader screen).
func Check(ctx context.Context, perm string) error {
	if !FromContext(ctx).Can(perm) {
		return httpx.Forbidden()
	}
	return nil
}
