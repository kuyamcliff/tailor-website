package bodyestimate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
)

func TestBodygram(t *testing.T) {
	var got map[string]map[string]any
	status, entryStatus := http.StatusOK, "success"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orgs/org-1/scans" || r.Header.Get("Authorization") != "key-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"entry": map[string]any{
			"status": entryStatus,
			"avatar": map[string]any{"data": "ignored"},
			"measurements": []map[string]any{
				{"name": "bustGirth", "unit": "mm", "value": 991.4},
				{"name": "waistGirth", "unit": "mm", "value": 856},
				{"name": "hipGirth", "unit": "mm", "value": 998},
				{"name": "acrossBackShoulderWidth", "unit": "mm", "value": 452},
				{"name": "insideLegLengthR", "unit": "mm", "value": 801},
				{"name": "backNeckHeight", "unit": "mm", "value": 1520}, // no matching field: dropped
			},
		}})
	}))
	defer srv.Close()
	b := &Bodygram{BaseURL: srv.URL, OrgID: "org-1", APIKey: "key-1", Client: srv.Client()}
	in := Input{FrontJPEG: []byte("front"), SideJPEG: []byte("side"), HeightMM: 1800, WeightG: 78000, Age: 34, BodyModel: "feminine"}
	values, err := b.Estimate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"chest": 991, "bust": 991, "waist": 856, "hip": 998, "shoulder": 452, "inseam": 801}
	if len(values) != len(want) {
		t.Fatalf("values %v", values)
	}
	for k, v := range want {
		if values[k] != v {
			t.Errorf("%s = %d, want %d", k, values[k], v)
		}
	}
	scan := got["photoScan"]
	if scan["gender"] != "female" || scan["height"].(float64) != 1800 || scan["weight"].(float64) != 78000 || scan["age"].(float64) != 34 {
		t.Errorf("request %v", scan)
	}
	if scan["frontPhoto"] != base64.StdEncoding.EncodeToString([]byte("front")) || scan["rightPhoto"] != base64.StdEncoding.EncodeToString([]byte("side")) {
		t.Error("photos not sent as base64")
	}

	entryStatus = "failure"
	if _, err := b.Estimate(context.Background(), in); !errors.Is(err, ErrUnreadable) {
		t.Errorf("failed scan: %v", err)
	}
	status = http.StatusBadRequest
	if _, err := b.Estimate(context.Background(), in); !errors.Is(err, ErrUnreadable) {
		t.Errorf("rejected photos: %v", err)
	}
	status = http.StatusInternalServerError
	if _, err := b.Estimate(context.Background(), in); err == nil || errors.Is(err, ErrUnreadable) {
		t.Errorf("provider error: %v", err)
	}
}

func TestProviderNeedsConfiguration(t *testing.T) {
	cases := []config.Config{
		{},
		{BodyEstimationProvider: "bodygram", BodyEstimationAPIKey: "k", BodygramAPIURL: "https://platform.bodygram.com"}, // no org
		{BodyEstimationProvider: "bodygram", BodyEstimationAPIKey: "k", BodygramOrgID: "o", BodygramAPIURL: "http://insecure"},
	}
	for i, c := range cases {
		if New(c) != nil {
			t.Errorf("case %d: provider enabled without full configuration", i)
		}
	}
	if New(config.Config{BodyEstimationProvider: "bodygram", BodyEstimationAPIKey: "k", BodygramOrgID: "o", BodygramAPIURL: "https://platform.bodygram.com"}) == nil {
		t.Error("configured provider not enabled")
	}
	h := &Handler{}
	if h.Blocker(context.Background(), "photo_body_estimation") == "" {
		t.Error("switch must be blocked without a provider")
	}
}
