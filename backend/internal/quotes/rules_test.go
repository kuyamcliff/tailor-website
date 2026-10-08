package quotes

import (
	"testing"
	"time"
)

func TestQuoteLifecycle(t *testing.T) {
	statuses := []string{"draft", "sent", "accepted", "declined", "changes_requested", "expired", "withdrawn"}
	want := map[action][]string{
		actRevise:         {"draft", "sent", "changes_requested", "expired"},
		actSend:           {"draft", "sent", "changes_requested", "expired"},
		actWithdraw:       {"draft", "sent", "declined", "changes_requested", "expired"},
		actAccept:         {"sent"},
		actDecline:        {"sent"},
		actRequestChanges: {"sent", "expired"},
	}
	for a, ok := range want {
		allowed := map[string]bool{}
		for _, s := range ok {
			allowed[s] = true
		}
		for _, s := range statuses {
			if got := canDo(s, a); got != allowed[s] {
				t.Errorf("canDo(%s, %s) = %v, want %v", s, a, got, allowed[s])
			}
		}
	}
	if canDo("sent", action("delete")) {
		t.Error("unknown action allowed")
	}
}

func TestQuoteExpiry(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if !isExpired("sent", now.Add(-time.Minute), now) {
		t.Error("sent quote past its date should be expired")
	}
	if isExpired("sent", now.Add(time.Minute), now) {
		t.Error("sent quote still valid")
	}
	// Only sent quotes expire; an accepted quote keeps its order even after the date passes.
	for _, s := range []string{"draft", "accepted", "declined", "withdrawn"} {
		if isExpired(s, now.Add(-time.Hour), now) {
			t.Errorf("%s quote marked expired", s)
		}
	}
}

func TestRequestPipeline(t *testing.T) {
	// Staff move requests through New, Reviewing, Need information, Quote sent, Accepted, Converted, Closed.
	// Quote sent, Accepted and Converted are reached only through quotes, never set by hand.
	for from, next := range requestTransitions {
		for _, to := range next {
			if to == "quote_sent" || to == "accepted" {
				t.Errorf("%s -> %s must happen through a quote", from, to)
			}
			if to == from {
				t.Errorf("%s -> %s is not a transition", from, to)
			}
		}
	}
	if len(requestTransitions["converted"]) != 0 {
		t.Error("a converted request is final")
	}
	if !contains(requestTransitions["closed"], "reviewing") {
		t.Error("a closed request can be reopened for review")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
