// Package designs builds and stores reproducible garment design snapshots. The server re-derives
// every label, price, material and fit value from the catalog at save time, so a snapshot never
// depends on later catalog changes and never trusts client-computed prices.
package designs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/fabrics"
	"github.com/kuyamcliff/tailor-website/backend/internal/fit"
	"github.com/kuyamcliff/tailor-website/backend/internal/garments"
	"github.com/kuyamcliff/tailor-website/backend/internal/measurements"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

// Config is what the client sends: choices only, never prices or labels.
type Config struct {
	Garment              string             `json:"garment"`
	Selections           map[string]any     `json:"selections"` // group key -> value key (string) or number
	Fabric               string             `json:"fabric"`
	Color                string             `json:"color"`
	Baseline             string             `json:"baseline"` // "" for made to measure, or a size label
	FitPreference        string             `json:"fitPreference"`
	BodyModel            string             `json:"bodyModel"`
	MeasurementVersionID *uuid.UUID         `json:"measurementVersionId"`
	Measurements         *MeasurementsInput `json:"measurements"`
	Notes                string             `json:"notes"`
}

type MeasurementsInput struct {
	Unit   string             `json:"unit"`
	Height *float64           `json:"height"`
	Values map[string]float64 `json:"values"`
}

type Selection struct {
	Group       string          `json:"group"`
	GroupName   string          `json:"groupName"`
	Section     string          `json:"section"`
	Value       string          `json:"value,omitempty"`
	ValueName   string          `json:"valueName,omitempty"`
	Number      *float64        `json:"number,omitempty"`
	Unit        *string         `json:"unit,omitempty"`
	PriceMinor  int64           `json:"priceMinor"`
	AssetParts  json.RawMessage `json:"assetParts,omitempty"`
	Adjustments map[string]int  `json:"adjustments,omitempty"`
}

type FabricSnap struct {
	Key              string          `json:"key"`
	Name             string          `json:"name"`
	MaterialType     string          `json:"materialType"`
	Composition      string          `json:"composition"`
	ColorKey         string          `json:"colorKey"`
	ColorName        string          `json:"colorName"`
	ColorHex         string          `json:"colorHex"`
	PBR              json.RawMessage `json:"pbr"`
	PriceImpactMinor int64           `json:"priceImpactMinor"`
	StockStatus      string          `json:"stockStatus"`
}

type MeasurementSnap struct {
	VersionID *uuid.UUID          `json:"versionId"`
	Source    string              `json:"source"`
	Unit      string              `json:"unit"`
	HeightMM  *int                `json:"heightMm"`
	ValuesMM  map[string]int      `json:"valuesMm"`
	Flags     []measurements.Flag `json:"flags"`
}

type Price struct {
	Currency     string `json:"currency"`
	BaseMinor    int64  `json:"baseMinor"`
	OptionsMinor int64  `json:"optionsMinor"`
	FabricMinor  int64  `json:"fabricMinor"`
	TotalMinor   int64  `json:"totalMinor"`
	Estimate     bool   `json:"estimate"`
}

type Snapshot struct {
	Schema  int `json:"schema"`
	Garment struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"garment"`
	Asset *struct {
		Key     string `json:"key"`
		Version int    `json:"version"`
	} `json:"asset"`
	Selections    []Selection      `json:"selections"`
	Fabric        *FabricSnap      `json:"fabric"`
	Baseline      map[string]any   `json:"baseline"`
	FitPreference string           `json:"fitPreference"`
	BodyModel     string           `json:"bodyModel"`
	Measurements  *MeasurementSnap `json:"measurements"`
	Fit           fit.Result       `json:"fit"`
	Price         Price            `json:"price"`
	Notes         string           `json:"notes"`
	Warnings      []string         `json:"warnings"`
	BuiltAt       time.Time        `json:"builtAt"`
}

var ErrUnavailable = errors.New("unavailable")

// Build validates a configuration against the live catalog and produces an immutable snapshot.
// customerID scopes access to a referenced measurement version.
func Build(ctx context.Context, q db.Querier, st *settings.Service, c Config, customerID *uuid.UUID, staff bool) (*Snapshot, error) {
	f := httpx.Fields{}
	studio, err := garments.LoadStudio(ctx, q, c.Garment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.Validation(map[string]string{"garment": "This garment is not available."})
	}
	if err != nil {
		return nil, err
	}
	biz, err := st.Business(ctx, q)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Schema: 1, Selections: []Selection{}, Warnings: []string{}, BuiltAt: time.Now().UTC()}
	s.Garment.Key, s.Garment.Name = studio.Garment.Key, studio.Garment.Name
	if studio.Asset != nil {
		s.Asset = &struct {
			Key     string `json:"key"`
			Version int    `json:"version"`
		}{studio.Asset.AssetKey, studio.Asset.Version}
	}
	s.FitPreference = c.FitPreference
	if s.FitPreference == "" {
		s.FitPreference = "regular"
	}
	if s.FitPreference != "slim" && s.FitPreference != "regular" && s.FitPreference != "relaxed" {
		f.Add("fitPreference", "Choose slim, regular or relaxed.")
	}
	s.BodyModel = c.BodyModel
	if s.BodyModel == "" {
		s.BodyModel = "masculine"
	}
	if s.BodyModel != "masculine" && s.BodyModel != "feminine" {
		f.Add("bodyModel", "Choose a body model.")
	}
	price := Price{Currency: biz.Currency, BaseMinor: studio.Garment.BasePriceMinor, Estimate: true}
	adjust := map[string]int{}
	for _, g := range studio.Groups {
		raw, chosen := c.Selections[g.Key]
		if g.Selection == "number" {
			var n float64
			switch {
			case !chosen && g.DefaultNumber != nil:
				n = *g.DefaultNumber
			case !chosen:
				if g.Required {
					f.Add("selections."+g.Key, g.Name+" is required.")
				}
				continue
			default:
				v, ok := raw.(float64)
				if !ok || math.IsNaN(v) {
					f.Add("selections."+g.Key, g.Name+" must be a number.")
					continue
				}
				n = v
			}
			if (g.MinValue != nil && n < *g.MinValue) || (g.MaxValue != nil && n > *g.MaxValue) {
				f.Add("selections."+g.Key, fmt.Sprintf("%s is outside the allowed range.", g.Name))
				continue
			}
			nn := n
			s.Selections = append(s.Selections, Selection{Group: g.Key, GroupName: g.Name, Section: g.Section, Number: &nn, Unit: g.Unit})
			continue
		}
		key, _ := raw.(string)
		if !chosen || key == "" {
			for _, v := range g.Values {
				if v.IsDefault {
					key = v.Key
				}
			}
		}
		if key == "" {
			if g.Required {
				f.Add("selections."+g.Key, "Choose a "+strings.ToLower(g.Name)+".")
			}
			continue
		}
		var found *garments.OptionValue
		for i := range g.Values {
			if g.Values[i].Key == key {
				found = &g.Values[i]
			}
		}
		if found == nil {
			f.Add("selections."+g.Key, "The "+strings.ToLower(g.Name)+" you chose is no longer available. Please choose another.")
			continue
		}
		price.OptionsMinor += found.PriceMinor
		for zone, mm := range found.Adjustments {
			adjust[zone] += mm
		}
		s.Selections = append(s.Selections, Selection{Group: g.Key, GroupName: g.Name, Section: g.Section, Value: found.Key, ValueName: found.Name,
			PriceMinor: found.PriceMinor, AssetParts: found.AssetParts, Adjustments: found.Adjustments})
	}
	for k := range c.Selections {
		known := false
		for _, g := range studio.Groups {
			known = known || g.Key == k
		}
		if !known {
			s.Warnings = append(s.Warnings, "Ignored an option that no longer exists for this garment.")
		}
	}
	if c.Fabric != "" {
		fab, err := fabrics.Get(ctx, q, c.Fabric)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			f.Add("fabric", "This fabric is no longer available. Please choose another.")
		case err != nil:
			return nil, err
		case !fab.Active:
			f.Add("fabric", "This fabric is no longer available. Please choose another.")
		default:
			snap := &FabricSnap{Key: fab.Key, Name: fab.Name, MaterialType: fab.MaterialType, Composition: fab.Composition, PBR: fab.PBR,
				PriceImpactMinor: fab.PriceImpactMinor, StockStatus: fab.StockStatus}
			if len(fab.SuitableGarments) > 0 && !contains(fab.SuitableGarments, studio.Garment.Key) {
				f.Add("fabric", "This fabric is not offered for this garment.")
			}
			if !fabrics.Orderable(fab.StockStatus) {
				s.Warnings = append(s.Warnings, fab.Name+" is currently "+strings.ReplaceAll(fab.StockStatus, "_", " ")+". Your tailor will confirm availability or suggest an alternative.")
			}
			colorKey := c.Color
			if colorKey == "" && len(fab.Colors) > 0 {
				colorKey = fab.Colors[0].Key
			}
			for _, col := range fab.Colors {
				if col.Key == colorKey {
					snap.ColorKey, snap.ColorName, snap.ColorHex = col.Key, col.Name, col.Hex
					if !fabrics.Orderable(col.StockStatus) {
						s.Warnings = append(s.Warnings, col.Name+" is currently "+strings.ReplaceAll(col.StockStatus, "_", " ")+".")
					}
				}
			}
			if colorKey != "" && snap.ColorKey == "" {
				f.Add("color", "This color is no longer available. Please choose another.")
			}
			price.FabricMinor = fab.PriceImpactMinor
			s.Fabric = snap
		}
	}
	s.Baseline = map[string]any{"type": "made_to_measure"}
	var baselineDims map[string]int
	if c.Baseline != "" {
		for _, sz := range studio.Sizes {
			if sz.Label == c.Baseline {
				baselineDims = sz.Dims
				s.Baseline = map[string]any{"type": "size", "label": sz.Label, "dims": sz.Dims}
			}
		}
		if baselineDims == nil {
			f.Add("baseline", "This size is no longer offered.")
		}
	}
	// Measurements: an existing version (owned by this customer, or any for staff) or inline values.
	switch {
	case c.MeasurementVersionID != nil:
		v, err := measurements.GetVersion(ctx, q, *c.MeasurementVersionID)
		if err != nil {
			f.Add("measurementVersionId", "Saved measurements not found.")
			break
		}
		if !staff {
			var owner *uuid.UUID
			_ = q.QueryRow(ctx, `SELECT coalesce(v.customer_id, p.customer_id) FROM measurement_versions v
				LEFT JOIN measurement_profiles p ON p.id = v.profile_id WHERE v.id=$1`, v.ID).Scan(&owner)
			if customerID == nil || owner == nil || *owner != *customerID {
				f.Add("measurementVersionId", "Saved measurements not found.")
				break
			}
		}
		s.Measurements = &MeasurementSnap{VersionID: &v.ID, Source: v.Source, Unit: v.Unit, HeightMM: v.HeightMM, ValuesMM: v.ValuesMM, Flags: v.ReviewFlags}
	case c.Measurements != nil:
		val := measurements.Validate(studio.Measurements, c.Measurements.Values, c.Measurements.Unit, c.Measurements.Height, false)
		for k, msg := range val.Errors {
			f.Add("measurements."+k, msg)
		}
		s.Measurements = &MeasurementSnap{Source: "customer_entered", Unit: c.Measurements.Unit, HeightMM: val.HeightMM, ValuesMM: val.ValuesMM, Flags: val.Flags}
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	body := map[string]int{}
	var flagged []string
	estimated := false
	if s.Measurements != nil {
		body = s.Measurements.ValuesMM
		estimated = s.Measurements.Source == "estimated"
		for _, fl := range s.Measurements.Flags {
			flagged = append(flagged, fl.Keys...)
		}
	}
	s.Fit = fit.Estimate(fit.Input{Rules: studio.FitRules, Body: body, FitPreference: s.FitPreference, Baseline: baselineDims,
		Adjustments: adjust, Estimated: estimated, FlaggedKeys: flagged})
	price.TotalMinor = price.BaseMinor + price.OptionsMinor + price.FabricMinor
	s.Price = price
	s.Notes = strings.TrimSpace(c.Notes)
	if len(s.Notes) > 4000 {
		return nil, httpx.Validation(map[string]string{"notes": "Notes are too long."})
	}
	return s, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

type Design struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Garment   string          `json:"garment"`
	Version   int             `json:"version"`
	VersionID *uuid.UUID      `json:"versionId"`
	Snapshot  json.RawMessage `json:"snapshot"`
	AssetOK   bool            `json:"assetAvailable"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Handler struct {
	Pool     *pgxpool.Pool
	Settings *settings.Service
}

type saveRequest struct {
	Name            string `json:"name"`
	Config          Config `json:"config"`
	ExpectedVersion *int   `json:"expectedVersion"`
}

// Save creates a design (POST /designs) or a new version of one (PUT /designs/{id}).
func (h Handler) Save(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := auth.FromContext(ctx)
	guest := uploads.GuestToken(r)
	var customerID *uuid.UUID
	if p != nil && p.CustomerID != nil {
		customerID = p.CustomerID
	}
	if customerID == nil && guest == "" {
		return httpx.BadRequest("Missing guest session. Refresh the page and try again.")
	}
	var req saveRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "My design"
	}
	if len(req.Name) > 80 {
		return httpx.Validation(map[string]string{"name": "Use a shorter name."})
	}
	snap, err := Build(ctx, h.Pool, h.Settings, req.Config, customerID, false)
	if err != nil {
		return err
	}
	var mvID *uuid.UUID
	if snap.Measurements != nil {
		mvID = snap.Measurements.VersionID
	}
	var assetKey *string
	var assetVer *int
	if snap.Asset != nil {
		assetKey, assetVer = &snap.Asset.Key, &snap.Asset.Version
	}
	var fabricKey, colorKey *string
	if snap.Fabric != nil {
		fabricKey, colorKey = &snap.Fabric.Key, &snap.Fabric.ColorKey
	}
	idParam, hasID := pathID(r)
	var designID, versionID uuid.UUID
	var version int
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if !hasID {
			var gh []byte
			if customerID == nil {
				gh = tokens.Hash(guest)
			}
			if err := tx.QueryRow(ctx, `INSERT INTO designs (customer_id, guest_token_hash, name, garment_type_key) VALUES ($1,$2,$3,$4) RETURNING id, version`,
				customerID, gh, req.Name, snap.Garment.Key).Scan(&designID, &version); err != nil {
				return err
			}
		} else {
			designID = idParam
			var cur int
			var owner *uuid.UUID
			var gh []byte
			err := tx.QueryRow(ctx, `SELECT version, customer_id, guest_token_hash FROM designs WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, designID).
				Scan(&cur, &owner, &gh)
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound("Design not found.")
			}
			if err != nil {
				return err
			}
			if !ownsDesign(customerID, guest, owner, gh) {
				return httpx.NotFound("Design not found.")
			}
			if req.ExpectedVersion != nil && *req.ExpectedVersion != cur {
				return httpx.Conflict("stale_version", "This design was changed on another device. Reload it to see the latest version.")
			}
			if err := tx.QueryRow(ctx, `UPDATE designs SET name=$2, garment_type_key=$3, version=version+1 WHERE id=$1 RETURNING version`,
				designID, req.Name, snap.Garment.Key).Scan(&version); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `INSERT INTO design_versions (design_id, version_no, snapshot, garment_type_key, asset_key, asset_version, fabric_key, color_key,
				measurement_version_id, estimated_price_minor, notes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, designID, version, snap, snap.Garment.Key, assetKey, assetVer, fabricKey, colorKey,
			mvID, snap.Price.TotalMinor, snap.Notes).Scan(&versionID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE designs SET current_version_id=$2 WHERE id=$1`, designID, versionID)
		return err
	})
	if err != nil {
		return err
	}
	status := http.StatusCreated
	if hasID {
		status = http.StatusOK
	}
	httpx.JSON(w, status, map[string]any{"id": designID, "version": version, "versionId": versionID, "snapshot": snap})
	return nil
}

func pathID(r *http.Request) (uuid.UUID, bool) {
	id, err := httpx.PathUUID(r, "id")
	return id, err == nil
}

func ownsDesign(customerID *uuid.UUID, guest string, owner *uuid.UUID, guestHash []byte) bool {
	if customerID != nil && owner != nil && *customerID == *owner {
		return true
	}
	return guest != "" && guestHash != nil && tokens.Matches(guest, guestHash)
}

// Evaluate builds a snapshot without saving it: the authoritative price and fit for the current configuration.
func (h Handler) Evaluate(w http.ResponseWriter, r *http.Request) error {
	var c Config
	if err := httpx.Decode(r, &c); err != nil {
		return err
	}
	var cid *uuid.UUID
	if p := auth.FromContext(r.Context()); p != nil {
		cid = p.CustomerID
	}
	snap, err := Build(r.Context(), h.Pool, h.Settings, c, cid, false)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, snap)
	return nil
}

func (h Handler) load(ctx context.Context, q db.Querier, id uuid.UUID) (*Design, *uuid.UUID, []byte, error) {
	d := &Design{}
	var owner *uuid.UUID
	var gh []byte
	var snap []byte
	err := q.QueryRow(ctx, `SELECT d.id, d.name, d.garment_type_key, d.version, d.current_version_id, v.snapshot, d.customer_id, d.guest_token_hash,
			d.created_at, d.updated_at,
			(v.asset_key IS NULL OR EXISTS (SELECT 1 FROM asset_manifests a WHERE a.asset_key=v.asset_key AND a.version=v.asset_version AND a.status IN ('published','archived')))
		FROM designs d LEFT JOIN design_versions v ON v.id = d.current_version_id WHERE d.id=$1 AND d.deleted_at IS NULL`, id).
		Scan(&d.ID, &d.Name, &d.Garment, &d.Version, &d.VersionID, &snap, &owner, &gh, &d.CreatedAt, &d.UpdatedAt, &d.AssetOK)
	d.Snapshot = snap
	return d, owner, gh, err
}

func (h Handler) Get(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	d, owner, gh, err := h.load(ctx, h.Pool, id)
	if err != nil {
		return err
	}
	p := auth.FromContext(ctx)
	var cid *uuid.UUID
	if p != nil {
		cid = p.CustomerID
	}
	staff := p != nil && (p.Can("requests.read") || p.Can("orders.read"))
	if !staff && !ownsDesign(cid, uploads.GuestToken(r), owner, gh) {
		return httpx.NotFound("Design not found.")
	}
	httpx.JSON(w, http.StatusOK, d)
	return nil
}

// GetVersion returns an exact historical design version (staff, or the design's owner).
func (h Handler) GetVersion(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	vid, err := httpx.PathUUID(r, "versionId")
	if err != nil {
		return err
	}
	var snap []byte
	var owner *uuid.UUID
	var gh []byte
	var designID uuid.UUID
	var no int
	var created time.Time
	if err := h.Pool.QueryRow(ctx, `SELECT v.design_id, v.version_no, v.snapshot, d.customer_id, d.guest_token_hash, v.created_at
		FROM design_versions v JOIN designs d ON d.id=v.design_id WHERE v.id=$1`, vid).Scan(&designID, &no, &snap, &owner, &gh, &created); err != nil {
		return err
	}
	p := auth.FromContext(ctx)
	var cid *uuid.UUID
	if p != nil {
		cid = p.CustomerID
	}
	staff := p != nil && (p.Can("requests.read") || p.Can("orders.read"))
	if !staff && !ownsDesign(cid, uploads.GuestToken(r), owner, gh) {
		return httpx.NotFound("Design not found.")
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": vid, "designId": designID, "versionNo": no, "snapshot": json.RawMessage(snap), "createdAt": created})
	return nil
}

func (h Handler) MyDesigns(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := auth.FromContext(ctx)
	guest := uploads.GuestToken(r)
	var rows pgx.Rows
	var err error
	q := `SELECT d.id, d.name, d.garment_type_key, d.version, d.current_version_id, v.snapshot, d.created_at, d.updated_at,
			(v.asset_key IS NULL OR EXISTS (SELECT 1 FROM asset_manifests a WHERE a.asset_key=v.asset_key AND a.version=v.asset_version AND a.status IN ('published','archived')))
		FROM designs d LEFT JOIN design_versions v ON v.id=d.current_version_id WHERE d.deleted_at IS NULL AND `
	switch {
	case p != nil && p.CustomerID != nil:
		rows, err = h.Pool.Query(ctx, q+`d.customer_id=$1 ORDER BY d.updated_at DESC LIMIT 100`, *p.CustomerID)
	case guest != "":
		rows, err = h.Pool.Query(ctx, q+`d.guest_token_hash=$1 ORDER BY d.updated_at DESC LIMIT 100`, tokens.Hash(guest))
	default:
		httpx.JSON(w, http.StatusOK, []Design{})
		return nil
	}
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Design{}
	for rows.Next() {
		var d Design
		var snap []byte
		if err := rows.Scan(&d.ID, &d.Name, &d.Garment, &d.Version, &d.VersionID, &snap, &d.CreatedAt, &d.UpdatedAt, &d.AssetOK); err != nil {
			return err
		}
		d.Snapshot = snap
		out = append(out, d)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

func (h Handler) Delete(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	_, owner, gh, err := h.load(ctx, h.Pool, id)
	if err != nil {
		return err
	}
	p := auth.FromContext(ctx)
	var cid *uuid.UUID
	if p != nil {
		cid = p.CustomerID
	}
	if !ownsDesign(cid, uploads.GuestToken(r), owner, gh) {
		return httpx.NotFound("Design not found.")
	}
	// Soft delete: versions referenced by requests and orders stay intact.
	if _, err := h.Pool.Exec(ctx, `UPDATE designs SET deleted_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
