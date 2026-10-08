// Package references provides optional analysis of customer reference images. It is a provider
// abstraction: when no provider is configured the feature stays disabled and customers use the
// manual annotation workflow. Results are hints for the tailor, never presented as certainties,
// and confidence values only ever come from the provider itself.
package references

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

// Hints are structured garment cues extracted from one reference image.
type Hints struct {
	GarmentCategory string   `json:"garmentCategory"`
	Colors          []string `json:"colors"`
	Silhouette      string   `json:"silhouette"`
	Collar          string   `json:"collar"`
	Sleeves         string   `json:"sleeves"`
	Pockets         string   `json:"pockets"`
	FabricCues      string   `json:"fabricCues"`
	NotableDetails  []string `json:"notableDetails"`
	Confidence      string   `json:"confidence"` // low, medium or high, as reported by the provider
}

// Provider analyzes one image with optional context (garment the customer chose, the tag they set).
type Provider interface {
	Name() string
	Analyze(ctx context.Context, jpeg []byte, garment, tag string) (*Hints, error)
}

// ErrDeclined means the provider declined to analyze the image (for example a safety refusal).
var ErrDeclined = errors.New("analysis declined")

// New returns the configured provider, or nil when analysis is disabled.
func New(cfg config.Config) Provider {
	if cfg.ReferenceAnalysisProvider == "anthropic" && cfg.ReferenceAnalysisAPIKey != "" {
		return newAnthropic(cfg.ReferenceAnalysisAPIKey, cfg.ReferenceAnalysisModel)
	}
	return nil
}

type anthropicProvider struct {
	client anthropic.Client
	model  string
}

func newAnthropic(key, model string) *anthropicProvider {
	if model == "" {
		model = "claude-opus-5-5"
	}
	return &anthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(key), option.WithRequestTimeout(60*time.Second), option.WithMaxRetries(2)),
		model:  model,
	}
}

func (a *anthropicProvider) Name() string { return "anthropic" }

var hintsSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"garmentCategory", "colors", "silhouette", "collar", "sleeves", "pockets", "fabricCues", "notableDetails", "confidence"},
	"properties": map[string]any{
		"garmentCategory": map[string]any{"type": "string"},
		"colors":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"silhouette":      map[string]any{"type": "string"},
		"collar":          map[string]any{"type": "string"},
		"sleeves":         map[string]any{"type": "string"},
		"pockets":         map[string]any{"type": "string"},
		"fabricCues":      map[string]any{"type": "string"},
		"notableDetails":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"confidence":      map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
	},
}

const instructions = `You help a tailor read a customer's reference photo for a custom garment.
Describe only what is visible in the clothing. Do not describe or identify the person.
Use short tailoring terms (for example "peak lapel", "puff sleeve", "A-line"). Use an empty string when a detail is not visible.
Set confidence to how clearly the garment details can be seen in this photo.`

func (a *anthropicProvider) Analyze(ctx context.Context, jpeg []byte, garment, tag string) (*Hints, error) {
	prompt := instructions + "\nThe customer is ordering: " + garment + ". They marked this image as showing: " + tag + "."
	resp, err := a.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 2000,
		// A refused request is re-served by the fallback model the platform picks for the refusal category.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortLow, // short visual extraction
			Format: anthropic.BetaJSONOutputFormatParam{Schema: hintsSchema},
		},
		Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(
			anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{Data: base64.StdEncoding.EncodeToString(jpeg), MediaType: "image/jpeg"}),
			anthropic.NewBetaTextBlock(prompt),
		)},
	})
	if err != nil {
		return nil, err
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return nil, ErrDeclined
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var h Hints
	if err := json.Unmarshal([]byte(text.String()), &h); err != nil {
		return nil, errors.New("analysis returned an unreadable result")
	}
	return &h, nil
}

type Handler struct {
	Pool     *pgxpool.Pool
	Store    uploads.Storage
	Settings *settings.Service
	Provider Provider
}

// Blocker prevents enabling analysis flags before a provider is configured.
func (h Handler) Blocker(_ context.Context, key string) string {
	if key == "reference_analysis" && h.Provider == nil {
		return "Set AI_REFERENCE_PROVIDER and AI_REFERENCE_API_KEY on the server first."
	}
	return ""
}

// Analyze returns hints for one reference image owned by the requester.
func (h Handler) Analyze(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if h.Provider == nil || !h.Settings.Flag(ctx, "reference_analysis") {
		return httpx.NewError(http.StatusServiceUnavailable, "feature_disabled", "Image suggestions are not available. Describe the image yourself instead.")
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Garment string `json:"garment"`
		Tag     string `json:"tag"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var cust *uuid.UUID
	var gh []byte
	var purpose string
	var deriv map[string]string
	var deleted *time.Time
	if err := h.Pool.QueryRow(ctx, `SELECT customer_id, guest_token_hash, purpose, derivatives, deleted_at FROM uploads WHERE id=$1`, id).
		Scan(&cust, &gh, &purpose, &deriv, &deleted); err != nil {
		return err
	}
	p := auth.FromContext(ctx)
	g := uploads.GuestToken(r)
	owner := (p != nil && p.CustomerID != nil && cust != nil && *p.CustomerID == *cust) || (g != "" && gh != nil && tokens.Matches(g, gh)) ||
		p.Can("requests.read")
	if !owner || deleted != nil || purpose != "reference" || deriv["preview"] == "" {
		return httpx.NotFound("We could not find this image.")
	}
	obj, err := h.Store.Get(ctx, deriv["preview"])
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(obj, 8<<20))
	obj.Close()
	if err != nil {
		return err
	}
	hints, err := h.Provider.Analyze(ctx, data, truncate(in.Garment, 60), truncate(in.Tag, 30))
	if errors.Is(err, ErrDeclined) {
		return httpx.NewError(http.StatusUnprocessableEntity, "analysis_declined", "We could not read this image. Please describe it yourself.")
	}
	if err != nil {
		httpx.Logger(ctx).Warn("reference analysis failed", "error", err)
		return httpx.NewError(http.StatusBadGateway, "analysis_unavailable", "Image suggestions are unavailable right now. Describe the image yourself instead.")
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hints": hints, "provider": h.Provider.Name()})
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
