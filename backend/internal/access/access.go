// Package access authorizes customer access to requests, quotes, orders, appointments and support
// threads, either through the signed-in account or through an unguessable access token sent with
// the confirmation link (for guests). Tokens are stored only as SHA-256 hashes.
package access

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
)

const Header = "X-Access-Token"

// Token reads the access token from the header (preferred) or the query string (links in messages).
func Token(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get(Header)); t != "" {
		return t
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

// Customer reports whether the request may act as the customer who owns the resource.
func Customer(r *http.Request, ownerID uuid.UUID, tokenHash []byte) bool {
	if p := auth.FromContext(r.Context()); p != nil && p.CustomerID != nil && *p.CustomerID == ownerID {
		return true
	}
	t := Token(r)
	return t != "" && tokens.Matches(t, tokenHash)
}

// NewToken returns a raw token and its hash.
func NewToken() (string, []byte) {
	t := tokens.New(24)
	return t, tokens.Hash(t)
}

// Number allocates a human-readable reference such as ORD-26-000123 from a sequence.
func Number(ctx context.Context, q db.Querier, sequence, prefix string) (string, error) {
	var n int64
	if err := q.QueryRow(ctx, "SELECT nextval('"+sequence+"')").Scan(&n); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%06d", prefix, time.Now().UTC().Format("06"), n), nil
}
