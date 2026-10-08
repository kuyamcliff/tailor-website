// Package payments implements Mobile Money payments through provider adapters (MTN MoMo, Orange Money).
//
// Rules enforced here:
//   - Payment state only changes from verified provider results. Callbacks are authenticated and then
//     re-confirmed with a server-side status query; a browser redirect never marks anything paid.
//   - One in-flight payment per order (database partial unique index) prevents double charges.
//   - Each payment's transition to succeeded happens once under a row lock, so the order is credited once.
//   - Provider secrets live only in backend environment variables.
package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
)

// Status is the internal payment state machine.
type Status string

const (
	Created                Status = "created"
	Pending                Status = "pending"
	CustomerActionRequired Status = "customer_action_required"
	Processing             Status = "processing"
	Succeeded              Status = "succeeded"
	Failed                 Status = "failed"
	Expired                Status = "expired"
	Cancelled              Status = "cancelled"
	Refunded               Status = "refunded"
	PartiallyRefunded      Status = "partially_refunded"
)

func (s Status) Active() bool {
	return s == Created || s == Pending || s == CustomerActionRequired || s == Processing
}

func (s Status) Final() bool { return !s.Active() }

// CanTransition guards against out-of-order or duplicate provider events: final states never move
// back to pending, and a succeeded payment never becomes failed.
func CanTransition(from, to Status) bool {
	if from == to {
		return false
	}
	switch from {
	case Created:
		return to != Refunded && to != PartiallyRefunded
	case Pending, CustomerActionRequired, Processing:
		return to != Created && to != Refunded && to != PartiallyRefunded
	case Succeeded:
		return to == Refunded || to == PartiallyRefunded
	case PartiallyRefunded:
		return to == Refunded
	default:
		return false
	}
}

type InitiateRequest struct {
	Reference   string // our unique reference, also the provider idempotency key
	PaymentID   string
	AmountMinor int64
	Currency    string
	MSISDN      string // E.164 digits without plus
	Description string
	CallbackURL string
}

type Result struct {
	Status          Status
	ProviderTxID    string
	PaymentURL      string
	CustomerMessage string
	FailureCode     string
	FailureMessage  string
	Raw             map[string]any // minimized, for the attempts log
	HTTPStatus      int
}

// CallbackEvent is an authenticated notification that a payment's state may have changed.
type CallbackEvent struct {
	EventKey  string
	Reference string
	Payload   map[string]any
}

// Provider is implemented by each Mobile Money adapter.
type Provider interface {
	Name() string
	// Configured reports whether credentials are present; reason explains what is missing.
	Configured() (bool, string)
	Simulated() bool
	Initiate(ctx context.Context, req InitiateRequest) (Result, error)
	Status(ctx context.Context, reference, providerTxID string) (Result, error)
	// ParseCallback extracts the reference from a provider notification. Authentication of the
	// notification is done by the service (signed callback URL) before this is trusted at all,
	// and the body's status is never used directly.
	ParseCallback(r *http.Request, body []byte) (CallbackEvent, error)
}

// ErrTransient marks errors worth retrying (network failures, provider 5xx, rate limits).
var ErrTransient = errors.New("transient provider error")

type transientError struct{ err error }

func (t transientError) Error() string { return t.err.Error() }
func (t transientError) Unwrap() error { return ErrTransient }

func transient(err error) error { return transientError{err} }

// SignReference produces the callback signature embedded in callback URLs.
func SignReference(secret, reference string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(reference))
	return hex.EncodeToString(m.Sum(nil))
}

func VerifyReference(secret, reference, sig string) bool {
	if secret == "" || sig == "" {
		return false
	}
	return hmac.Equal([]byte(SignReference(secret, reference)), []byte(sig))
}
