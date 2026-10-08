package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/kuyamcliff/tailor-website/backend/internal/bodyestimate"
)

type fakeEstimator struct{ calls int }

func (f *fakeEstimator) Name() string  { return "fake" }
func (f *fakeEstimator) Label() string { return "Test provider" }
func (f *fakeEstimator) Estimate(_ context.Context, in bodyestimate.Input) (map[string]int, error) {
	f.calls++
	if len(in.FrontJPEG) == 0 || len(in.SideJPEG) == 0 {
		return nil, bodyestimate.ErrUnreadable
	}
	return map[string]int{"chest": 1004, "waist": 880, "hip": 1010}, nil
}

// Photo estimation: off until a provider is configured and the switch is on; account holders only;
// consent required; photos deleted once measured; results saved only as an "estimated" version.
func TestPhotoBodyEstimation(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	s := c.do("POST", "/auth/signup", map[string]any{"name": "Photo Est", "email": "photo-est@example.test", "password": "a-long-enough-password"})
	expect(t, s, 200)
	c.csrf = s.m(t)["user"].(map[string]any)["csrfToken"].(string)

	// Off by default: no provider, switch off.
	if r := c.do("GET", "/me/measurement-estimates", nil); r.m(t)["available"] != false {
		t.Fatalf("available without provider: %s", r.body)
	}
	expect(t, c.upload("body_photo", testJPEG(t, 600, 900, 1), "front.jpg"), http.StatusServiceUnavailable)

	// The owner cannot switch it on before a provider is configured.
	owner := ownerClient(t)
	if r := owner.do("PUT", "/owner/flags/photo_body_estimation", map[string]any{"enabled": true}); r.status < 400 {
		t.Fatalf("switch turned on without a provider: %d %s", r.status, r.body)
	}
	fake := &fakeEstimator{}
	shared.app.BodyEstimate.Provider = fake
	defer func() { shared.app.BodyEstimate.Provider = nil }()
	if r := owner.do("PUT", "/owner/flags/photo_body_estimation", map[string]any{"enabled": true}); r.status >= 400 {
		t.Fatalf("switch: %d %s", r.status, r.body)
	}
	defer owner.do("PUT", "/owner/flags/photo_body_estimation", map[string]any{"enabled": false})
	if r := c.do("GET", "/me/measurement-estimates", nil); r.m(t)["available"] != true || r.m(t)["provider"] != "Test provider" {
		t.Fatalf("status %s", r.body)
	}

	// Guests cannot upload body photos.
	expect(t, newClient(t).upload("body_photo", testJPEG(t, 600, 900, 2), "front.jpg"), http.StatusUnauthorized)

	upload := func(seed uint8) string {
		r := c.upload("body_photo", testJPEG(t, 600, 900, seed), "photo.jpg")
		expect(t, r, http.StatusCreated)
		return r.m(t)["id"].(string)
	}
	body := func(front, side string, consent bool) map[string]any {
		return map[string]any{"frontUploadId": front, "sideUploadId": side, "heightMm": 1780, "weightKg": 76,
			"age": 31, "bodyModel": "masculine", "consent": consent}
	}

	// Without consent nothing is sent, and the photos are still deleted.
	f1, s1 := upload(3), upload(4)
	expect(t, c.do("POST", "/me/measurement-estimates", body(f1, s1, false)), http.StatusUnprocessableEntity)
	if fake.calls != 0 {
		t.Fatal("provider called without consent")
	}
	assertDeleted(t, f1, s1)

	// Someone else's photos are refused.
	other := newClient(t)
	o := other.do("POST", "/auth/signup", map[string]any{"name": "Other", "email": "photo-other@example.test", "password": "a-long-enough-password"})
	other.csrf = o.m(t)["user"].(map[string]any)["csrfToken"].(string)
	theirs := other.upload("body_photo", testJPEG(t, 600, 900, 5), "x.jpg").m(t)["id"].(string)
	mine := upload(6)
	expect(t, c.do("POST", "/me/measurement-estimates", body(theirs, mine, true)), http.StatusUnprocessableEntity)
	var gone bool
	shared.pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM uploads WHERE id=$1`, theirs).Scan(&gone)
	if gone {
		t.Fatal("another customer's photo was deleted")
	}

	// The real thing.
	f2, s2 := upload(7), upload(8)
	r := c.do("POST", "/me/measurement-estimates", body(f2, s2, true))
	expect(t, r, http.StatusOK)
	res := r.m(t)
	vals := res["valuesMm"].(map[string]any)
	if vals["chest"].(float64) != 1004 || res["provider"] != "Test provider" {
		t.Fatalf("estimate %s", r.body)
	}
	assertDeleted(t, f2, s2)

	// Saved as an estimate, never as measured.
	p := c.do("POST", "/me/measurement-profiles", map[string]any{"name": "From photos", "bodyModel": "masculine", "unit": "cm", "fitPreference": "regular"})
	expect(t, p, http.StatusCreated)
	pid := p.m(t)["id"].(string)
	v := c.do("POST", "/me/measurement-profiles/"+pid+"/versions", map[string]any{"source": "estimated", "unit": "cm", "height": 178,
		"values": map[string]any{"chest": 100.4, "waist": 88, "hip": 101}})
	expect(t, v, http.StatusCreated)
	var source string
	shared.pool.QueryRow(ctx, `SELECT source FROM measurement_versions WHERE id=$1`, v.m(t)["id"]).Scan(&source)
	if source != "estimated" {
		t.Fatalf("saved with source %q", source)
	}
	// A customer cannot claim tailor verification.
	v2 := c.do("POST", "/me/measurement-profiles/"+pid+"/versions", map[string]any{"source": "tailor_verified", "unit": "cm", "values": map[string]any{"chest": 101}})
	expect(t, v2, http.StatusCreated)
	shared.pool.QueryRow(ctx, `SELECT source FROM measurement_versions WHERE id=$1`, v2.m(t)["id"]).Scan(&source)
	if source != "customer_entered" {
		t.Fatalf("customer saved a %q version", source)
	}
}

func assertDeleted(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		var deleted bool
		if err := shared.pool.QueryRow(context.Background(), `SELECT deleted_at IS NOT NULL FROM uploads WHERE id=$1`, id).Scan(&deleted); err != nil || !deleted {
			t.Fatalf("body photo %s not deleted (%v)", id, err)
		}
	}
}
