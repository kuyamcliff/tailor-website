// Package bodyestimate estimates body measurements from two photos through an external provider.
// It is optional and off by default: the feature switch can only be turned on once a provider is
// configured on the server. Results are always stored as "estimated", never as measured; the tailor
// verifies them before cutting.
//
// Privacy rules enforced here:
//   - the customer gives explicit consent for each estimate, and is told which provider sees the photos;
//   - photos are deleted from storage as soon as the provider has answered (or failed);
//   - no face recognition or identification is requested, and nothing but measurements is kept.
package bodyestimate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
)

// Input is what a provider needs: two photos (front and right side) and basic body facts.
type Input struct {
	FrontJPEG []byte
	SideJPEG  []byte
	HeightMM  int
	WeightG   int
	Age       int
	BodyModel string // masculine or feminine
}

// Provider returns measurements in millimetres keyed by this site's measurement field keys.
type Provider interface {
	Name() string
	Label() string // shown to customers in the consent text
	Estimate(ctx context.Context, in Input) (map[string]int, error)
}

// ErrUnreadable means the provider could not measure from these photos.
var ErrUnreadable = errors.New("photos could not be measured")

// New returns the configured provider, or nil while the feature is not set up.
func New(cfg config.Config) Provider {
	if cfg.BodyEstimationProvider == "bodygram" && cfg.BodyEstimationAPIKey != "" && cfg.BodygramOrgID != "" &&
		strings.HasPrefix(cfg.BodygramAPIURL, "https://") {
		return &Bodygram{BaseURL: cfg.BodygramAPIURL, OrgID: cfg.BodygramOrgID, APIKey: cfg.BodyEstimationAPIKey,
			Client: &http.Client{Timeout: 90 * time.Second}}
	}
	return nil
}

// Bodygram calls the Bodygram Platform API (docs.bodygram.com/platform):
//
//	POST {base}/api/orgs/{orgId}/scans   Authorization: {API key}
//	{"photoScan": {"age", "gender", "height" (mm), "weight" (g), "frontPhoto", "rightPhoto" (base64)}}
//
// The response's entry.measurements lists {name, unit, value} in millimetres. The avatar and body
// composition it also returns are ignored and never stored.
type Bodygram struct {
	BaseURL string
	OrgID   string
	APIKey  string
	Client  *http.Client
}

func (b *Bodygram) Name() string  { return "bodygram" }
func (b *Bodygram) Label() string { return "Bodygram" }

// bodygramFields maps Bodygram measurement names to this site's measurement keys. Right-side values
// stand for both sides.
var bodygramFields = map[string][]string{
	"neckGirth":               {"neck"},
	"acrossBackShoulderWidth": {"shoulder"},
	"bustGirth":               {"chest", "bust"},
	"underBustGirth":          {"underbust"},
	"waistGirth":              {"waist"},
	"bellyWaistGirth":         {"stomach"},
	"topHipGirth":             {"high_hip"},
	"hipGirth":                {"hip"},
	"upperArmGirthR":          {"bicep"},
	"wristGirthR":             {"wrist"},
	"outerArmLengthR":         {"sleeve_length"},
	"thighGirthR":             {"thigh"},
	"kneeGirthR":              {"knee"},
	"calfGirthR":              {"calf"},
	"insideLegLengthR":        {"inseam"},
	"outsideLegLengthR":       {"outseam"},
	"backNeckPointToWaist":    {"torso_length"},
}

func (b *Bodygram) Estimate(ctx context.Context, in Input) (map[string]int, error) {
	gender := "male"
	if in.BodyModel == "feminine" {
		gender = "female"
	}
	payload, _ := json.Marshal(map[string]any{"photoScan": map[string]any{
		"age": in.Age, "gender": gender, "height": in.HeightMM, "weight": in.WeightG,
		"frontPhoto": base64.StdEncoding.EncodeToString(in.FrontJPEG),
		"rightPhoto": base64.StdEncoding.EncodeToString(in.SideJPEG),
	}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, b.BaseURL+"/api/orgs/"+b.OrgID+"/scans", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", b.APIKey)
	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bodygram: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
		return nil, ErrUnreadable
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("bodygram: status %d", resp.StatusCode)
	}
	var out struct {
		Entry struct {
			Status       string `json:"status"`
			Measurements []struct {
				Name  string  `json:"name"`
				Unit  string  `json:"unit"`
				Value float64 `json:"value"`
			} `json:"measurements"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, errors.New("bodygram: unexpected response")
	}
	if out.Entry.Status != "success" {
		return nil, ErrUnreadable
	}
	values := map[string]int{}
	for _, m := range out.Entry.Measurements {
		if m.Unit != "mm" || m.Value <= 0 {
			continue
		}
		for _, key := range bodygramFields[m.Name] {
			values[key] = int(m.Value + 0.5)
		}
	}
	if len(values) == 0 {
		return nil, ErrUnreadable
	}
	return values, nil
}
