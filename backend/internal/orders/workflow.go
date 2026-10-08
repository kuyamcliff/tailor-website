package orders

import "slices"

// AllStatuses lists every order status in production order.
var AllStatuses = []string{
	"draft", "submitted", "under_review", "quote_sent", "awaiting_customer", "deposit_paid", "measurements_pending",
	"measurements_verified", "material_pending", "patterning", "cutting", "sewing", "quality_check", "fitting_scheduled",
	"fitting", "alteration", "ready", "dispatched", "delivered", "completed", "cancelled", "refunded",
}

var terminal = map[string]bool{"cancelled": true, "refunded": true}

// Transition describes the outcome of a requested status change.
type Transition struct {
	Allowed   bool
	Backwards bool
	NeedsNote bool
	Reason    string
}

func position(list []string, s string) int { return slices.Index(list, s) }

// Check validates moving an order from one status to another under the owner's active workflow.
// Moving backwards is allowed (for example returning to alteration after a fitting) but requires a note,
// so the audit trail explains why.
func Check(from, to string, workflow []string) Transition {
	switch {
	case from == to:
		return Transition{Reason: "The order is already in this stage."}
	case position(AllStatuses, to) < 0:
		return Transition{Reason: "Unknown stage."}
	case terminal[from]:
		return Transition{Reason: "Cancelled or refunded orders cannot change stage."}
	case to == "refunded":
		return Transition{Allowed: true, NeedsNote: true}
	case from == "completed":
		return Transition{Reason: "A completed order can only be refunded."}
	case to == "cancelled":
		if from == "delivered" {
			return Transition{Reason: "A delivered order cannot be cancelled. Record a refund instead."}
		}
		return Transition{Allowed: true, NeedsNote: true}
	case to == "draft":
		return Transition{Reason: "Orders cannot return to draft."}
	}
	if position(workflow, to) < 0 && to != "completed" {
		return Transition{Reason: "This stage is turned off in your workflow settings."}
	}
	fp, tp := position(AllStatuses, from), position(AllStatuses, to)
	if tp < fp {
		return Transition{Allowed: true, Backwards: true, NeedsNote: true}
	}
	return Transition{Allowed: true}
}

// CustomerLabel returns customer-facing wording for a status.
var CustomerLabel = map[string]string{
	"draft": "Draft", "submitted": "Received", "under_review": "Being reviewed", "quote_sent": "Quote sent",
	"awaiting_customer": "Waiting for you", "deposit_paid": "Deposit received", "measurements_pending": "Measurements needed",
	"measurements_verified": "Measurements confirmed", "material_pending": "Sourcing fabric", "patterning": "Pattern making",
	"cutting": "Cutting", "sewing": "Sewing", "quality_check": "Quality check", "fitting_scheduled": "Fitting scheduled",
	"fitting": "Fitting", "alteration": "Adjustments", "ready": "Ready", "dispatched": "On its way", "delivered": "Delivered",
	"completed": "Completed", "cancelled": "Cancelled", "refunded": "Refunded",
}
