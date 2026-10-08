package tests

import (
	"bufio"
	"bytes"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kuyamcliff/tailor-website/backend/openapi"
)

// specOperations reads "METHOD /path" pairs and their x-permissions from the OpenAPI document. The
// document is generated in a fixed layout (paths at two spaces, methods at four), so a line reader
// is enough and the test needs no YAML dependency.
func specOperations(t *testing.T) map[string][]string {
	t.Helper()
	ops := map[string][]string{}
	pathRe := regexp.MustCompile(`^  "?(/[^":]*)"?:$`)
	methodRe := regexp.MustCompile(`^    (get|post|put|patch|delete):$`)
	permRe := regexp.MustCompile(`^        - "([a-z_]+\.[a-z_]+)"$`)
	inPaths, inPerms := false, false
	var path, cur string
	sc := bufio.NewScanner(bytes.NewReader(openapi.Spec))
	for sc.Scan() {
		line := sc.Text()
		if line == "paths:" {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if m := pathRe.FindStringSubmatch(line); m != nil {
			path = m[1]
			continue
		}
		if m := methodRe.FindStringSubmatch(line); m != nil {
			cur = strings.ToUpper(m[1]) + " " + path
			ops[cur] = []string{}
			inPerms = false
			continue
		}
		if strings.TrimSpace(line) == "x-permissions:" {
			inPerms = true
			continue
		}
		if inPerms {
			if m := permRe.FindStringSubmatch(line); m != nil {
				ops[cur] = append(ops[cur], m[1])
				continue
			}
			inPerms = false
		}
	}
	if len(ops) < 100 {
		t.Fatalf("parsed only %d operations from the OpenAPI document", len(ops))
	}
	return ops
}

// The OpenAPI document must describe exactly the routes the server has, with the same permissions.
func TestOpenAPIMatchesRouter(t *testing.T) {
	spec := specOperations(t)
	routes := map[string]bool{}
	err := chi.Walk(shared.app.Router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1/") {
			return nil // health, readiness and metrics are for the platform, not API clients
		}
		routes[method+" "+strings.TrimPrefix(route, "/api/v1")] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var missing, extra []string
	for r := range routes {
		if _, ok := spec[r]; !ok {
			missing = append(missing, r)
		}
	}
	for r := range spec {
		if !routes[r] {
			extra = append(extra, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("routes missing from backend/openapi/openapi.yaml:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("documented operations the server does not have:\n  %s", strings.Join(extra, "\n  "))
	}
	// Every owner operation states its permission, except the few any staff member may use.
	anyStaff := map[string]bool{"GET /owner/dashboard": true, "GET /owner/team": true, "GET /owner/notifications": true, "POST /owner/notifications/read": true}
	for op, perms := range spec {
		if strings.Contains(op, " /owner/") && len(perms) == 0 && !anyStaff[op] {
			t.Errorf("%s has no x-permissions", op)
		}
	}
	// And the permission documented is the one enforced: a staff member without it is refused.
	tailor := staffClient(t, "tailor")
	for op, perms := range map[string][]string{"GET /owner/staff": spec["GET /owner/staff"], "GET /owner/audit": spec["GET /owner/audit"]} {
		if len(perms) == 0 {
			t.Fatalf("%s should document a permission", op)
		}
		path := strings.SplitN(op, " ", 2)[1]
		if r := tailor.do("GET", path, nil); r.status != http.StatusForbidden {
			t.Errorf("%s without %v: status %d", op, perms, r.status)
		}
	}
}

// The document is served to clients.
func TestOpenAPIServed(t *testing.T) {
	r := newClient(t).do("GET", "/openapi.yaml", nil)
	expect(t, r, http.StatusOK)
	if !bytes.HasPrefix(r.body, []byte("openapi: 3.1")) {
		t.Fatalf("unexpected document start: %.40s", r.body)
	}
}
