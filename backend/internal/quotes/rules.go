package quotes

import "time"

// Quote lifecycle:
//
//	draft -> sent -> accepted | declined | changes_requested | expired
//
// Staff can revise (which returns the quote to draft) or resend anything still open, and withdraw
// anything not yet accepted. Customers decide only on a sent quote; they can still ask for changes
// once it has expired, so the atelier can send a fresh one.
type action string

const (
	actRevise         action = "revise"
	actSend           action = "send"
	actWithdraw       action = "withdraw"
	actAccept         action = "accept"
	actDecline        action = "decline"
	actRequestChanges action = "request_changes"
)

// canDo reports whether a quote in the given status allows the action. Whether a sent quote has
// passed its validity date is checked separately with isExpired, because it depends on the clock.
func canDo(status string, a action) bool {
	closed := status == "accepted" || status == "declined" || status == "withdrawn"
	switch a {
	case actRevise, actSend:
		return !closed
	case actWithdraw:
		return status != "accepted" && status != "withdrawn"
	case actAccept, actDecline:
		return status == "sent"
	case actRequestChanges:
		return status == "sent" || status == "expired"
	}
	return false
}

// isExpired reports whether a sent quote's current revision is past its validity date.
func isExpired(status string, expiresAt, now time.Time) bool {
	return status == "sent" && expiresAt.Before(now)
}
