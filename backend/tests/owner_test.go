package tests

import (
	"net/http"
	"strings"
	"testing"
)

// The team list feeds assignment pickers for any staff member, so it carries names only.
func TestTeamListIsNamesOnly(t *testing.T) {
	owner := ownerClient(t)
	r := owner.do("GET", "/owner/team", nil)
	expect(t, r, http.StatusOK)
	if strings.Contains(string(r.body), "@") || strings.Contains(string(r.body), "role") {
		t.Fatalf("team list exposes more than names: %s", r.body)
	}
	var team []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	r.json(t, &team)
	if len(team) == 0 || team[0].ID == "" || team[0].Name == "" {
		t.Fatalf("unexpected team list %s", r.body)
	}

	guest := newClient(t)
	if r := guest.do("GET", "/owner/team", nil); r.status != http.StatusUnauthorized && r.status != http.StatusForbidden {
		t.Fatalf("guests must not list staff: %d", r.status)
	}
}
