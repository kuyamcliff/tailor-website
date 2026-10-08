// Package garments manages garment types, their configuration options, measurement fields,
// fit rules, size baselines and versioned 3D asset manifests.
package garments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/fit"
	"github.com/kuyamcliff/tailor-website/backend/internal/measurements"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

type GarmentType struct {
	ID             uuid.UUID `json:"id"`
	Key            string    `json:"key"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	Description    string    `json:"description"`
	BasePriceMinor int64     `json:"basePriceMinor"`
	StudioEnabled  bool      `json:"studioEnabled"`
	BodyModelHint  string    `json:"bodyModelHint"`
	QuoteOnly      bool      `json:"quoteOnly"`
	SortOrder      int       `json:"sortOrder"`
	Active         bool      `json:"active"`
}

type OptionValue struct {
	ID          uuid.UUID       `json:"id"`
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	PriceMinor  int64           `json:"priceMinor"`
	AssetParts  json.RawMessage `json:"assetParts"`
	Adjustments map[string]int  `json:"adjustments"`
	IsDefault   bool            `json:"isDefault"`
	SortOrder   int             `json:"sortOrder"`
	Active      bool            `json:"active"`
}

type OptionGroup struct {
	ID            uuid.UUID     `json:"id"`
	Key           string        `json:"key"`
	Name          string        `json:"name"`
	Section       string        `json:"section"`
	Selection     string        `json:"selection"`
	Required      bool          `json:"required"`
	MinValue      *float64      `json:"minValue"`
	MaxValue      *float64      `json:"maxValue"`
	StepValue     *float64      `json:"stepValue"`
	DefaultNumber *float64      `json:"defaultNumber"`
	Unit          *string       `json:"unit"`
	SortOrder     int           `json:"sortOrder"`
	Active        bool          `json:"active"`
	Values        []OptionValue `json:"values"`
}

type Size struct {
	ID        uuid.UUID      `json:"id"`
	Label     string         `json:"label"`
	Dims      map[string]int `json:"dims"`
	SortOrder int            `json:"sortOrder"`
}

type AssetFile struct {
	LOD    string `json:"lod"`
	URL    string `json:"url"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type Asset struct {
	ID                uuid.UUID       `json:"id"`
	AssetKey          string          `json:"assetKey"`
	Version           int             `json:"version"`
	Kind              string          `json:"kind"`
	GarmentTypeKey    *string         `json:"garmentTypeKey"`
	Status            string          `json:"status"`
	Files             []AssetFile     `json:"files"`
	BodyCompat        json.RawMessage `json:"bodyCompat"`
	SupportedOptions  json.RawMessage `json:"supportedOptions"`
	TextureSetVersion string          `json:"textureSetVersion"`
	License           json.RawMessage `json:"license"`
	ProductionQuality bool            `json:"productionQuality"`
	Notes             string          `json:"notes"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

// StudioConfig is everything the 3D studio needs for one garment type.
type StudioConfig struct {
	Garment      GarmentType          `json:"garment"`
	Groups       []OptionGroup        `json:"groups"`
	FitRules     []fit.Rule           `json:"fitRules"`
	Sizes        []Size               `json:"sizes"`
	Measurements []measurements.Field `json:"measurements"`
	Asset        *Asset               `json:"asset"`
	BodyAssets   []Asset              `json:"bodyAssets"`
}

const garmentCols = `id, key, name, category, description, base_price_minor, studio_enabled, body_model_hint, quote_only, sort_order, active`

func scanGarment(row pgx.Row) (GarmentType, error) {
	var g GarmentType
	err := row.Scan(&g.ID, &g.Key, &g.Name, &g.Category, &g.Description, &g.BasePriceMinor, &g.StudioEnabled, &g.BodyModelHint, &g.QuoteOnly, &g.SortOrder, &g.Active)
	return g, err
}

func ListTypes(ctx context.Context, q db.Querier, includeInactive bool) ([]GarmentType, error) {
	rows, err := q.Query(ctx, `SELECT `+garmentCols+` FROM garment_types WHERE ($1 OR active) ORDER BY sort_order, name`, includeInactive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GarmentType{}
	for rows.Next() {
		g, err := scanGarment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func GetType(ctx context.Context, q db.Querier, key string) (GarmentType, error) {
	return scanGarment(q.QueryRow(ctx, `SELECT `+garmentCols+` FROM garment_types WHERE key=$1`, key))
}

// Groups loads option groups with values. Archived values are only included for staff.
func Groups(ctx context.Context, q db.Querier, garmentID uuid.UUID, includeInactive bool) ([]OptionGroup, error) {
	rows, err := q.Query(ctx, `SELECT id, key, name, section, selection, required, min_value::float8, max_value::float8, step_value::float8,
			default_number::float8, unit, sort_order, active
		FROM option_groups WHERE garment_type_id=$1 AND ($2 OR active) ORDER BY sort_order, key`, garmentID, includeInactive)
	if err != nil {
		return nil, err
	}
	var groups []OptionGroup
	idx := map[uuid.UUID]int{}
	for rows.Next() {
		var g OptionGroup
		if err := rows.Scan(&g.ID, &g.Key, &g.Name, &g.Section, &g.Selection, &g.Required, &g.MinValue, &g.MaxValue, &g.StepValue,
			&g.DefaultNumber, &g.Unit, &g.SortOrder, &g.Active); err != nil {
			rows.Close()
			return nil, err
		}
		g.Values = []OptionValue{}
		idx[g.ID] = len(groups)
		groups = append(groups, g)
	}
	rows.Close()
	vrows, err := q.Query(ctx, `SELECT v.group_id, v.id, v.key, v.name, v.description, v.price_minor, v.asset_parts, v.adjustments,
			v.is_default, v.sort_order, v.active
		FROM option_values v JOIN option_groups g ON g.id = v.group_id
		WHERE g.garment_type_id=$1 AND ($2 OR (v.active AND v.archived_at IS NULL)) ORDER BY v.sort_order, v.key`, garmentID, includeInactive)
	if err != nil {
		return nil, err
	}
	defer vrows.Close()
	for vrows.Next() {
		var gid uuid.UUID
		var v OptionValue
		var parts []byte
		if err := vrows.Scan(&gid, &v.ID, &v.Key, &v.Name, &v.Description, &v.PriceMinor, &parts, &v.Adjustments, &v.IsDefault, &v.SortOrder, &v.Active); err != nil {
			return nil, err
		}
		v.AssetParts = parts
		if i, ok := idx[gid]; ok {
			groups[i].Values = append(groups[i].Values, v)
		}
	}
	if groups == nil {
		groups = []OptionGroup{}
	}
	return groups, vrows.Err()
}

func FitRules(ctx context.Context, q db.Querier, garmentID uuid.UUID) ([]fit.Rule, error) {
	rows, err := q.Query(ctx, `SELECT zone, label, measurement_key, kind, ease_slim_mm, ease_regular_mm, ease_relaxed_mm, tolerance_mm, stretch_pct
		FROM fit_rules WHERE garment_type_id=$1 ORDER BY zone`, garmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []fit.Rule{}
	for rows.Next() {
		var r fit.Rule
		if err := rows.Scan(&r.Zone, &r.Label, &r.MeasurementKey, &r.Kind, &r.EaseSlimMM, &r.EaseRegularMM, &r.EaseRelaxedMM, &r.ToleranceMM, &r.StretchPct); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func Sizes(ctx context.Context, q db.Querier, garmentID uuid.UUID) ([]Size, error) {
	rows, err := q.Query(ctx, `SELECT id, label, dims, sort_order FROM garment_sizes WHERE garment_type_id=$1 ORDER BY sort_order, label`, garmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Size{}
	for rows.Next() {
		var s Size
		if err := rows.Scan(&s.ID, &s.Label, &s.Dims, &s.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

const assetCols = `a.id, a.asset_key, a.version, a.kind, g.key, a.status, a.files, a.body_compat, a.supported_options,
	a.texture_set_version, a.license, a.production_quality, a.notes, a.created_at, a.updated_at`

func scanAsset(row pgx.Row) (Asset, error) {
	var a Asset
	var bc, so, lic []byte
	err := row.Scan(&a.ID, &a.AssetKey, &a.Version, &a.Kind, &a.GarmentTypeKey, &a.Status, &a.Files, &bc, &so,
		&a.TextureSetVersion, &lic, &a.ProductionQuality, &a.Notes, &a.CreatedAt, &a.UpdatedAt)
	a.BodyCompat, a.SupportedOptions, a.License = bc, so, lic
	if a.Files == nil {
		a.Files = []AssetFile{}
	}
	return a, err
}

// PublishedAsset returns the live asset manifest for a garment type.
func PublishedAsset(ctx context.Context, q db.Querier, garmentID uuid.UUID) (*Asset, error) {
	a, err := scanAsset(q.QueryRow(ctx, `SELECT `+assetCols+` FROM asset_manifests a LEFT JOIN garment_types g ON g.id=a.garment_type_id
		WHERE a.garment_type_id=$1 AND a.kind='garment' AND a.status='published' ORDER BY a.version DESC LIMIT 1`, garmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

func PublishedBodies(ctx context.Context, q db.Querier) ([]Asset, error) {
	rows, err := q.Query(ctx, `SELECT `+assetCols+` FROM asset_manifests a LEFT JOIN garment_types g ON g.id=a.garment_type_id
		WHERE a.kind='body' AND a.status='published' ORDER BY a.asset_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AssetVersion fetches an exact asset version, used to reproduce historical designs.
func AssetVersion(ctx context.Context, q db.Querier, key string, version int) (*Asset, error) {
	a, err := scanAsset(q.QueryRow(ctx, `SELECT `+assetCols+` FROM asset_manifests a LEFT JOIN garment_types g ON g.id=a.garment_type_id
		WHERE a.asset_key=$1 AND a.version=$2`, key, version))
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func LoadStudio(ctx context.Context, q db.Querier, key string) (*StudioConfig, error) {
	g, err := GetType(ctx, q, key)
	if err != nil {
		return nil, err
	}
	if !g.Active {
		return nil, pgx.ErrNoRows
	}
	cfg := &StudioConfig{Garment: g}
	if cfg.Groups, err = Groups(ctx, q, g.ID, false); err != nil {
		return nil, err
	}
	if cfg.FitRules, err = FitRules(ctx, q, g.ID); err != nil {
		return nil, err
	}
	if cfg.Sizes, err = Sizes(ctx, q, g.ID); err != nil {
		return nil, err
	}
	if cfg.Measurements, err = measurements.Fields(ctx, q, g.Key); err != nil {
		return nil, err
	}
	if cfg.Asset, err = PublishedAsset(ctx, q, g.ID); err != nil {
		return nil, err
	}
	if cfg.BodyAssets, err = PublishedBodies(ctx, q); err != nil {
		return nil, err
	}
	return cfg, nil
}

type Handler struct{ Pool *pgxpool.Pool }

func (h Handler) PublicList(w http.ResponseWriter, r *http.Request) error {
	list, err := ListTypes(r.Context(), h.Pool, false)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

func (h Handler) PublicStudio(w http.ResponseWriter, r *http.Request) error {
	cfg, err := LoadStudio(r.Context(), h.Pool, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	httpx.JSON(w, http.StatusOK, cfg)
	return nil
}

// PublicAssetVersion serves an exact (possibly archived) manifest so old designs stay reproducible.
func (h Handler) PublicAssetVersion(w http.ResponseWriter, r *http.Request) error {
	var version int
	if _, err := fmtSscan(chi.URLParam(r, "version"), &version); err != nil {
		return httpx.NotFound("Asset not found.")
	}
	a, err := AssetVersion(r.Context(), h.Pool, chi.URLParam(r, "key"), version)
	if err != nil {
		return err
	}
	if a.Status != "published" && a.Status != "archived" {
		return httpx.NotFound("Asset not found.")
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	httpx.JSON(w, http.StatusOK, a)
	return nil
}

// ---- Owner endpoints ----

var keyRe = regexp.MustCompile(`^[a-z0-9-]{2,40}$`)
var optKeyRe = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	list, err := ListTypes(r.Context(), h.Pool, true)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

func (h Handler) OwnerGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	g, err := GetType(ctx, h.Pool, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	groups, err := Groups(ctx, h.Pool, g.ID, true)
	if err != nil {
		return err
	}
	rules, err := FitRules(ctx, h.Pool, g.ID)
	if err != nil {
		return err
	}
	sizes, err := Sizes(ctx, h.Pool, g.ID)
	if err != nil {
		return err
	}
	fields, err := measurements.Fields(ctx, h.Pool, g.Key)
	if err != nil {
		return err
	}
	allFields, err := measurements.Fields(ctx, h.Pool, "")
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"garment": g, "groups": groups, "fitRules": rules, "sizes": sizes,
		"measurements": fields, "allMeasurementFields": allFields})
	return nil
}

type typeInput struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Description    string `json:"description"`
	BasePriceMinor int64  `json:"basePriceMinor"`
	StudioEnabled  bool   `json:"studioEnabled"`
	BodyModelHint  string `json:"bodyModelHint"`
	QuoteOnly      bool   `json:"quoteOnly"`
	SortOrder      int    `json:"sortOrder"`
	Active         bool   `json:"active"`
}

func (in *typeInput) validate() error {
	f := httpx.Fields{}
	if !keyRe.MatchString(in.Key) {
		f.Add("key", "Use 2 to 40 lowercase letters, numbers or hyphens.")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		f.Add("name", "Enter a name.")
	}
	if in.BasePriceMinor < 0 {
		f.Add("basePriceMinor", "Price cannot be negative.")
	}
	if in.BodyModelHint == "" {
		in.BodyModelHint = "any"
	}
	if in.BodyModelHint != "any" && in.BodyModelHint != "masculine" && in.BodyModelHint != "feminine" {
		f.Add("bodyModelHint", "Choose a body model.")
	}
	if strings.TrimSpace(in.Category) == "" {
		in.Category = "other"
	}
	return f.Err()
}

func (h Handler) OwnerUpsertType(w http.ResponseWriter, r *http.Request) error {
	var in typeInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		before, _ := GetType(ctx, tx, in.Key)
		g, err := scanGarment(tx.QueryRow(ctx, `INSERT INTO garment_types (key, name, category, description, base_price_minor, studio_enabled,
				body_model_hint, quote_only, sort_order, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (key) DO UPDATE SET name=EXCLUDED.name, category=EXCLUDED.category, description=EXCLUDED.description,
				base_price_minor=EXCLUDED.base_price_minor, studio_enabled=EXCLUDED.studio_enabled, body_model_hint=EXCLUDED.body_model_hint,
				quote_only=EXCLUDED.quote_only, sort_order=EXCLUDED.sort_order, active=EXCLUDED.active
			RETURNING `+garmentCols, in.Key, in.Name, in.Category, in.Description, in.BasePriceMinor, in.StudioEnabled,
			in.BodyModelHint, in.QuoteOnly, in.SortOrder, in.Active))
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "garments.type.upsert", ObjectType: "garment_type", ObjectID: g.ID.String(), Before: before, After: g}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, g)
		return nil
	})
}

type groupInput struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Section       string   `json:"section"`
	Selection     string   `json:"selection"`
	Required      bool     `json:"required"`
	MinValue      *float64 `json:"minValue"`
	MaxValue      *float64 `json:"maxValue"`
	StepValue     *float64 `json:"stepValue"`
	DefaultNumber *float64 `json:"defaultNumber"`
	Unit          *string  `json:"unit"`
	SortOrder     int      `json:"sortOrder"`
	Active        bool     `json:"active"`
}

func (h Handler) OwnerUpsertGroup(w http.ResponseWriter, r *http.Request) error {
	var in groupInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	if !optKeyRe.MatchString(in.Key) {
		f.Add("key", "Use lowercase letters, numbers or underscores.")
	}
	if strings.TrimSpace(in.Name) == "" {
		f.Add("name", "Enter a name.")
	}
	if in.Selection != "single" && in.Selection != "number" {
		f.Add("selection", "Choose single choice or number.")
	}
	if in.Selection == "number" && (in.MinValue == nil || in.MaxValue == nil || *in.MaxValue <= *in.MinValue) {
		f.Add("minValue", "Number options need a valid minimum and maximum.")
	}
	if in.Section == "" {
		in.Section = "general"
	}
	if err := f.Err(); err != nil {
		return err
	}
	ctx := r.Context()
	g, err := GetType(ctx, h.Pool, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO option_groups (garment_type_id, key, name, section, selection, required, min_value, max_value,
				step_value, default_number, unit, sort_order, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (garment_type_id, key) DO UPDATE SET name=EXCLUDED.name, section=EXCLUDED.section, selection=EXCLUDED.selection,
				required=EXCLUDED.required, min_value=EXCLUDED.min_value, max_value=EXCLUDED.max_value, step_value=EXCLUDED.step_value,
				default_number=EXCLUDED.default_number, unit=EXCLUDED.unit, sort_order=EXCLUDED.sort_order, active=EXCLUDED.active
			RETURNING id`, g.ID, in.Key, strings.TrimSpace(in.Name), in.Section, in.Selection, in.Required, in.MinValue, in.MaxValue,
			in.StepValue, in.DefaultNumber, in.Unit, in.SortOrder, in.Active).Scan(&id); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "garments.option_group.upsert", ObjectType: "option_group", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"id": id})
		return nil
	})
}

type valueInput struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	PriceMinor  int64           `json:"priceMinor"`
	AssetParts  json.RawMessage `json:"assetParts"`
	Adjustments map[string]int  `json:"adjustments"`
	IsDefault   bool            `json:"isDefault"`
	SortOrder   int             `json:"sortOrder"`
	Active      bool            `json:"active"`
}

func (h Handler) OwnerUpsertValue(w http.ResponseWriter, r *http.Request) error {
	groupID, err := httpx.PathUUID(r, "groupId")
	if err != nil {
		return err
	}
	var in valueInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	if !optKeyRe.MatchString(in.Key) {
		f.Add("key", "Use lowercase letters, numbers or underscores.")
	}
	if strings.TrimSpace(in.Name) == "" {
		f.Add("name", "Enter a name.")
	}
	if len(in.AssetParts) == 0 {
		in.AssetParts = json.RawMessage(`{}`)
	}
	var parts struct {
		Show []string `json:"show"`
		Hide []string `json:"hide"`
	}
	if err := json.Unmarshal(in.AssetParts, &parts); err != nil {
		f.Add("assetParts", "Asset parts must list part names to show and hide.")
	}
	for zone, mm := range in.Adjustments {
		if !optKeyRe.MatchString(zone) || mm < -300 || mm > 300 {
			f.Add("adjustments", "Adjustments must be between -300 and 300 mm per zone.")
		}
	}
	if in.Adjustments == nil {
		in.Adjustments = map[string]int{}
	}
	if err := f.Err(); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if in.IsDefault {
			if _, err := tx.Exec(ctx, `UPDATE option_values SET is_default=false WHERE group_id=$1 AND key<>$2`, groupID, in.Key); err != nil {
				return err
			}
		}
		var id uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO option_values (group_id, key, name, description, price_minor, asset_parts, adjustments, is_default, sort_order, active, archived_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, CASE WHEN $10 THEN NULL ELSE now() END)
			ON CONFLICT (group_id, key) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description, price_minor=EXCLUDED.price_minor,
				asset_parts=EXCLUDED.asset_parts, adjustments=EXCLUDED.adjustments, is_default=EXCLUDED.is_default, sort_order=EXCLUDED.sort_order,
				active=EXCLUDED.active, archived_at=CASE WHEN EXCLUDED.active THEN NULL ELSE coalesce(option_values.archived_at, now()) END
			RETURNING id`, groupID, in.Key, strings.TrimSpace(in.Name), in.Description, in.PriceMinor, []byte(in.AssetParts), in.Adjustments,
			in.IsDefault, in.SortOrder, in.Active).Scan(&id)
		if db.IsForeignKeyViolation(err) {
			return httpx.NotFound("Option group not found.")
		}
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "garments.option_value.upsert", ObjectType: "option_value", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"id": id})
		return nil
	})
}

// OwnerPutFitRules replaces a garment's fit rules. Requires fit_rules.write.
func (h Handler) OwnerPutFitRules(w http.ResponseWriter, r *http.Request) error {
	var rules []fit.Rule
	if err := httpx.Decode(r, &rules); err != nil {
		return err
	}
	f := httpx.Fields{}
	seen := map[string]bool{}
	for i, ru := range rules {
		p := "rules." + itoa(i)
		if !optKeyRe.MatchString(ru.Zone) || seen[ru.Zone] {
			f.Add(p, "Each zone needs a unique key.")
		}
		seen[ru.Zone] = true
		if ru.Kind != "circumference" && ru.Kind != "length" {
			f.Add(p, "Kind must be circumference or length.")
		}
		if ru.ToleranceMM < 1 || ru.ToleranceMM > 100 || ru.StretchPct < 0 || ru.StretchPct > 50 {
			f.Add(p, "Tolerance must be 1 to 100 mm and stretch 0 to 50 percent.")
		}
		for _, e := range []int{ru.EaseSlimMM, ru.EaseRegularMM, ru.EaseRelaxedMM} {
			if e < -100 || e > 400 {
				f.Add(p, "Ease must be between -100 and 400 mm.")
			}
		}
		if strings.TrimSpace(ru.Label) == "" || !optKeyRe.MatchString(ru.MeasurementKey) {
			f.Add(p, "Each rule needs a label and a measurement key.")
		}
	}
	if err := f.Err(); err != nil {
		return err
	}
	ctx := r.Context()
	g, err := GetType(ctx, h.Pool, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		before, err := FitRules(ctx, tx, g.ID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM fit_rules WHERE garment_type_id=$1`, g.ID); err != nil {
			return err
		}
		for _, ru := range rules {
			if _, err := tx.Exec(ctx, `INSERT INTO fit_rules (garment_type_id, zone, label, measurement_key, kind, ease_slim_mm, ease_regular_mm,
				ease_relaxed_mm, tolerance_mm, stretch_pct, updated_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
				g.ID, ru.Zone, ru.Label, ru.MeasurementKey, ru.Kind, ru.EaseSlimMM, ru.EaseRegularMM, ru.EaseRelaxedMM, ru.ToleranceMM, ru.StretchPct,
				actor(ctx)); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "fit_rules.replace", ObjectType: "garment_type", ObjectID: g.ID.String(), Before: before, After: rules}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

type fieldLink struct {
	Key       string `json:"key"`
	Required  bool   `json:"required"`
	SortOrder int    `json:"sortOrder"`
}

// OwnerPutMeasurementFields sets which measurement fields apply to a garment type.
func (h Handler) OwnerPutMeasurementFields(w http.ResponseWriter, r *http.Request) error {
	var links []fieldLink
	if err := httpx.Decode(r, &links); err != nil {
		return err
	}
	ctx := r.Context()
	g, err := GetType(ctx, h.Pool, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM garment_measurement_fields WHERE garment_type_id=$1`, g.ID); err != nil {
			return err
		}
		for _, l := range links {
			tag, err := tx.Exec(ctx, `INSERT INTO garment_measurement_fields (garment_type_id, field_id, required, sort_order)
				SELECT $1, id, $3, $4 FROM measurement_fields WHERE key=$2`, g.ID, l.Key, l.Required, l.SortOrder)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return httpx.Validation(map[string]string{"key": "Unknown measurement field " + l.Key + "."})
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "garments.measurement_fields.replace", ObjectType: "garment_type", ObjectID: g.ID.String(), After: links}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

type measurementFieldInput struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	BodyLocation string  `json:"bodyLocation"`
	Instruction  string  `json:"instruction"`
	HelperNote   string  `json:"helperNote"`
	DiagramKey   *string `json:"diagramKey"`
	Kind         string  `json:"kind"`
	MinMM        int     `json:"minMm"`
	MaxMM        int     `json:"maxMm"`
	SortOrder    int     `json:"sortOrder"`
	Active       bool    `json:"active"`
}

// OwnerUpsertMeasurementField lets the owner add custom measurement fields (e.g. for traditional wear).
func (h Handler) OwnerUpsertMeasurementField(w http.ResponseWriter, r *http.Request) error {
	var in measurementFieldInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	if !optKeyRe.MatchString(in.Key) {
		f.Add("key", "Use lowercase letters, numbers or underscores.")
	}
	if strings.TrimSpace(in.Label) == "" {
		f.Add("label", "Enter a label.")
	}
	if in.MinMM < 1 || in.MaxMM <= in.MinMM || in.MaxMM > 3000 {
		f.Add("minMm", "Enter a valid range in millimetres.")
	}
	switch in.Kind {
	case "circumference", "length", "width", "height":
	default:
		f.Add("kind", "Choose a measurement kind.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO measurement_fields (key, label, body_location, instruction, helper_note, diagram_key, kind, min_mm, max_mm, sort_order, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (key) DO UPDATE SET label=EXCLUDED.label, body_location=EXCLUDED.body_location, instruction=EXCLUDED.instruction,
				helper_note=EXCLUDED.helper_note, diagram_key=EXCLUDED.diagram_key, kind=EXCLUDED.kind, min_mm=EXCLUDED.min_mm,
				max_mm=EXCLUDED.max_mm, sort_order=EXCLUDED.sort_order, active=EXCLUDED.active RETURNING id`,
			in.Key, strings.TrimSpace(in.Label), in.BodyLocation, in.Instruction, in.HelperNote, in.DiagramKey, in.Kind, in.MinMM, in.MaxMM, in.SortOrder, in.Active).Scan(&id); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "measurements.field.upsert", ObjectType: "measurement_field", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"id": id})
		return nil
	})
}

func (h Handler) OwnerPutSizes(w http.ResponseWriter, r *http.Request) error {
	var sizes []Size
	if err := httpx.Decode(r, &sizes); err != nil {
		return err
	}
	ctx := r.Context()
	g, err := GetType(ctx, h.Pool, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM garment_sizes WHERE garment_type_id=$1`, g.ID); err != nil {
			return err
		}
		for i, s := range sizes {
			if strings.TrimSpace(s.Label) == "" {
				return httpx.Validation(map[string]string{"sizes." + itoa(i): "Each size needs a label."})
			}
			if _, err := tx.Exec(ctx, `INSERT INTO garment_sizes (garment_type_id, label, dims, sort_order) VALUES ($1,$2,$3,$4)`,
				g.ID, strings.TrimSpace(s.Label), s.Dims, i); err != nil {
				if db.IsUniqueViolation(err, "") {
					return httpx.Validation(map[string]string{"sizes." + itoa(i): "Size labels must be unique."})
				}
				return err
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "garments.sizes.replace", ObjectType: "garment_type", ObjectID: g.ID.String(), After: sizes}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}
