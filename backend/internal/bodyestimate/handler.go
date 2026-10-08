package bodyestimate

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/measurements"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

type Handler struct {
	Pool     *pgxpool.Pool
	Uploads  *uploads.Service
	Settings *settings.Service
	Provider Provider
}

// Blocker explains why the feature switch cannot be turned on.
func (h *Handler) Blocker(_ context.Context, key string) string {
	if key == "photo_body_estimation" && h.Provider == nil {
		return "Set BODY_ESTIMATION_PROVIDER=bodygram, BODY_ESTIMATION_API_KEY and BODYGRAM_ORG_ID on the server first."
	}
	return ""
}

// Status tells the site whether estimation is available and who processes the photos.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) error {
	on := h.Provider != nil && h.Settings.Flag(r.Context(), "photo_body_estimation")
	out := map[string]any{"available": on}
	if on {
		out["provider"] = h.Provider.Label()
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

type estimateInput struct {
	FrontUploadID uuid.UUID `json:"frontUploadId"`
	SideUploadID  uuid.UUID `json:"sideUploadId"`
	HeightMM      int       `json:"heightMm"`
	WeightKg      float64   `json:"weightKg"`
	Age           int       `json:"age"`
	BodyModel     string    `json:"bodyModel"`
	Consent       bool      `json:"consent"`
}

// Estimate sends the customer's two body photos to the provider and returns estimated measurements
// in millimetres. The photos are deleted whatever the outcome. Nothing is saved here: the customer
// reviews the numbers and saves them as an "estimated" measurement version if they want to.
func (h *Handler) Estimate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if h.Provider == nil || !h.Settings.Flag(ctx, "photo_body_estimation") {
		return httpx.NewError(http.StatusServiceUnavailable, "feature_disabled", "Measuring from photos is not available. Enter your measurements or book a measuring session.")
	}
	p := auth.FromContext(ctx)
	if p == nil || p.CustomerID == nil {
		return httpx.NewError(http.StatusUnauthorized, "sign_in_required", "Sign in to measure from photos, so you can delete them and the results later.")
	}
	var in estimateInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	// Whatever happens next, the photos do not outlive this request.
	defer func() {
		for _, id := range []uuid.UUID{in.FrontUploadID, in.SideUploadID} {
			_ = h.Uploads.PurgeOwned(context.WithoutCancel(ctx), id, *p.CustomerID, "body_photo")
		}
	}()
	f := httpx.Fields{}
	if !in.Consent {
		f.Add("consent", "Please agree before we send your photos for measuring.")
	}
	if in.HeightMM < 1200 || in.HeightMM > 2300 {
		f.Add("heightMm", "Enter your height between 120 and 230 cm.")
	}
	if in.WeightKg < 30 || in.WeightKg > 250 {
		f.Add("weightKg", "Enter your weight between 30 and 250 kg.")
	}
	if in.Age < 16 || in.Age > 100 {
		f.Add("age", "Measuring from photos is for ages 16 and over.")
	}
	if in.BodyModel != "masculine" && in.BodyModel != "feminine" {
		f.Add("bodyModel", "Choose a figure.")
	}
	if in.FrontUploadID == in.SideUploadID {
		f.Add("sideUploadId", "Add a front photo and a side photo.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	front, err := h.Uploads.ReadOwned(ctx, in.FrontUploadID, *p.CustomerID, "body_photo")
	if err != nil {
		return httpx.Validation(map[string]string{"frontUploadId": "Add the front photo again."})
	}
	side, err := h.Uploads.ReadOwned(ctx, in.SideUploadID, *p.CustomerID, "body_photo")
	if err != nil {
		return httpx.Validation(map[string]string{"sideUploadId": "Add the side photo again."})
	}
	values, err := h.Provider.Estimate(ctx, Input{FrontJPEG: front, SideJPEG: side, HeightMM: in.HeightMM,
		WeightG: int(in.WeightKg * 1000), Age: in.Age, BodyModel: in.BodyModel})
	if errors.Is(err, ErrUnreadable) {
		return httpx.NewError(http.StatusUnprocessableEntity, "photos_unreadable",
			"We could not measure from these photos. Check the tips and try again, or enter your measurements yourself.")
	}
	if err != nil {
		return httpx.NewError(http.StatusBadGateway, "provider_error", "Measuring from photos is not working right now. Please try again later.")
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	_ = audit.Write(ctx, h.Pool, audit.Entry{Action: "measurements.estimate", ObjectType: "customer", ObjectID: p.CustomerID.String(),
		After: map[string]any{"provider": h.Provider.Name(), "fields": keys}})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"provider": h.Provider.Label(),
		"heightMm": in.HeightMM,
		"valuesMm": values,
		"flags":    measurements.ReviewFlags(values, &in.HeightMM),
	})
	return nil
}
