// Package fabrics manages fabrics, their colors, swatches, PBR material parameters and stock state.
package fabrics

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

type Color struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Hex         string    `json:"hex"`
	SwatchURL   *string   `json:"swatchUrl"`
	StockStatus string    `json:"stockStatus"`
	SortOrder   int       `json:"sortOrder"`
}

type Fabric struct {
	ID                 uuid.UUID       `json:"id"`
	Key                string          `json:"key"`
	Name               string          `json:"name"`
	MaterialType       string          `json:"materialType"`
	Composition        string          `json:"composition"`
	WeightGSM          *int            `json:"weightGsm"`
	TextureDescription string          `json:"textureDescription"`
	Season             string          `json:"season"`
	CareInstructions   string          `json:"careInstructions"`
	PriceImpactMinor   int64           `json:"priceImpactMinor"`
	StockStatus        string          `json:"stockStatus"`
	StockMeters        *float64        `json:"stockMeters,omitempty"`
	LowStockMeters     *float64        `json:"lowStockMeters,omitempty"`
	SwatchURL          *string         `json:"swatchUrl"`
	PBR                json.RawMessage `json:"pbr"`
	SuitableGarments   []string        `json:"suitableGarments"`
	SortOrder          int             `json:"sortOrder"`
	Active             bool            `json:"active"`
	Colors             []Color         `json:"colors"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

var StockStatuses = map[string]bool{"available": true, "low_stock": true, "out_of_stock": true, "discontinued": true, "custom_order": true}

// Orderable reports whether a fabric in this state can be selected for an order without a special request.
func Orderable(status string) bool { return status == "available" || status == "low_stock" }

const cols = `id, key, name, material_type, composition, weight_gsm, texture_description, season, care_instructions,
	price_impact_minor, stock_status, stock_meters::float8, low_stock_meters::float8, swatch_url, pbr, suitable_garments, sort_order, active, updated_at`

func scan(row pgx.Row) (Fabric, error) {
	var f Fabric
	var pbr []byte
	err := row.Scan(&f.ID, &f.Key, &f.Name, &f.MaterialType, &f.Composition, &f.WeightGSM, &f.TextureDescription, &f.Season,
		&f.CareInstructions, &f.PriceImpactMinor, &f.StockStatus, &f.StockMeters, &f.LowStockMeters, &f.SwatchURL, &pbr,
		&f.SuitableGarments, &f.SortOrder, &f.Active, &f.UpdatedAt)
	f.PBR = pbr
	f.Colors = []Color{}
	return f, err
}

func attachColors(ctx context.Context, q db.Querier, list []Fabric) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(list))
	idx := map[uuid.UUID]int{}
	for i, f := range list {
		ids[i] = f.ID
		idx[f.ID] = i
	}
	rows, err := q.Query(ctx, `SELECT fabric_id, id, key, name, hex, swatch_url, stock_status, sort_order FROM fabric_colors
		WHERE fabric_id = ANY($1) ORDER BY sort_order, name`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var fid uuid.UUID
		var c Color
		if err := rows.Scan(&fid, &c.ID, &c.Key, &c.Name, &c.Hex, &c.SwatchURL, &c.StockStatus, &c.SortOrder); err != nil {
			return err
		}
		list[idx[fid]].Colors = append(list[idx[fid]].Colors, c)
	}
	return rows.Err()
}

func List(ctx context.Context, q db.Querier, garment string, includeInactive bool) ([]Fabric, error) {
	rows, err := q.Query(ctx, `SELECT `+cols+` FROM fabrics
		WHERE ($1 OR (active AND archived_at IS NULL)) AND ($2 = '' OR $2 = ANY(suitable_garments) OR cardinality(suitable_garments) = 0)
		ORDER BY sort_order, name`, includeInactive, garment)
	if err != nil {
		return nil, err
	}
	list := []Fabric{}
	for rows.Next() {
		f, err := scan(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if !includeInactive {
			f.StockMeters, f.LowStockMeters = nil, nil // inventory quantities are internal
		}
		list = append(list, f)
	}
	rows.Close()
	return list, attachColors(ctx, q, list)
}

// Get returns a fabric by key, including archived ones (saved designs may reference them).
func Get(ctx context.Context, q db.Querier, key string) (*Fabric, error) {
	f, err := scan(q.QueryRow(ctx, `SELECT `+cols+` FROM fabrics WHERE key=$1`, key))
	if err != nil {
		return nil, err
	}
	list := []Fabric{f}
	if err := attachColors(ctx, q, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

type Handler struct{ Pool *pgxpool.Pool }

func (h Handler) PublicList(w http.ResponseWriter, r *http.Request) error {
	list, err := List(r.Context(), h.Pool, r.URL.Query().Get("garment"), false)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

func (h Handler) PublicGet(w http.ResponseWriter, r *http.Request) error {
	f, err := Get(r.Context(), h.Pool, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	f.StockMeters, f.LowStockMeters = nil, nil
	httpx.JSON(w, http.StatusOK, f)
	return nil
}

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	list, err := List(r.Context(), h.Pool, "", true)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

var keyRe = regexp.MustCompile(`^[a-z0-9-]{2,60}$`)
var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type input struct {
	Key                string          `json:"key"`
	Name               string          `json:"name"`
	MaterialType       string          `json:"materialType"`
	Composition        string          `json:"composition"`
	WeightGSM          *int            `json:"weightGsm"`
	TextureDescription string          `json:"textureDescription"`
	Season             string          `json:"season"`
	CareInstructions   string          `json:"careInstructions"`
	PriceImpactMinor   int64           `json:"priceImpactMinor"`
	StockStatus        string          `json:"stockStatus"`
	StockMeters        *float64        `json:"stockMeters"`
	LowStockMeters     *float64        `json:"lowStockMeters"`
	SwatchURL          *string         `json:"swatchUrl"`
	PBR                json.RawMessage `json:"pbr"`
	SuitableGarments   []string        `json:"suitableGarments"`
	SortOrder          int             `json:"sortOrder"`
	Active             bool            `json:"active"`
	Colors             []Color         `json:"colors"`
}

// PBR texture URLs may only point at our own static or API paths so the 3D loader never fetches third-party content.
func validPBR(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	for k, v := range m {
		if s, ok := v.(string); ok && strings.HasSuffix(k, "Map") && !strings.HasPrefix(s, "/") {
			return false
		}
	}
	return true
}

func (in *input) validate() error {
	f := httpx.Fields{}
	if !keyRe.MatchString(in.Key) {
		f.Add("key", "Use lowercase letters, numbers or hyphens.")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		f.Add("name", "Enter a name.")
	}
	if strings.TrimSpace(in.MaterialType) == "" {
		f.Add("materialType", "Enter the material type.")
	}
	if !StockStatuses[in.StockStatus] {
		f.Add("stockStatus", "Choose a stock status.")
	}
	if in.WeightGSM != nil && (*in.WeightGSM <= 0 || *in.WeightGSM > 2000) {
		f.Add("weightGsm", "Weight must be between 1 and 2000 g/m².")
	}
	if !validPBR(in.PBR) {
		f.Add("pbr", "Texture maps must be site-relative paths.")
	}
	if len(in.PBR) == 0 {
		in.PBR = json.RawMessage(`{}`)
	}
	if in.SwatchURL != nil && *in.SwatchURL != "" && !strings.HasPrefix(*in.SwatchURL, "/") {
		f.Add("swatchUrl", "Upload the swatch image to the site.")
	}
	if in.SuitableGarments == nil {
		in.SuitableGarments = []string{}
	}
	seen := map[string]bool{}
	for i, c := range in.Colors {
		if !keyRe.MatchString(c.Key) || seen[c.Key] || !hexRe.MatchString(c.Hex) || strings.TrimSpace(c.Name) == "" {
			f.Add("colors."+itoaInt(i), "Each color needs a unique key, a name and a hex value.")
		}
		if c.StockStatus == "" {
			in.Colors[i].StockStatus = "available"
		} else if !StockStatuses[c.StockStatus] {
			f.Add("colors."+itoaInt(i), "Choose a stock status.")
		}
		seen[c.Key] = true
	}
	return f.Err()
}

func itoaInt(i int) string { return strconv.Itoa(i) }

// OwnerUpsert creates or updates a fabric and replaces its color list. Removing a color is safe for
// history because saved designs and orders store a full snapshot (name, hex, material parameters).
func (h Handler) OwnerUpsert(w http.ResponseWriter, r *http.Request) error {
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		before, _ := Get(ctx, tx, in.Key)
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO fabrics (key, name, material_type, composition, weight_gsm, texture_description, season,
				care_instructions, price_impact_minor, stock_status, stock_meters, low_stock_meters, swatch_url, pbr, suitable_garments, sort_order, active,
				archived_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17, CASE WHEN $17 THEN NULL ELSE now() END)
			ON CONFLICT (key) DO UPDATE SET name=EXCLUDED.name, material_type=EXCLUDED.material_type, composition=EXCLUDED.composition,
				weight_gsm=EXCLUDED.weight_gsm, texture_description=EXCLUDED.texture_description, season=EXCLUDED.season,
				care_instructions=EXCLUDED.care_instructions, price_impact_minor=EXCLUDED.price_impact_minor, stock_status=EXCLUDED.stock_status,
				stock_meters=EXCLUDED.stock_meters, low_stock_meters=EXCLUDED.low_stock_meters, swatch_url=EXCLUDED.swatch_url, pbr=EXCLUDED.pbr,
				suitable_garments=EXCLUDED.suitable_garments, sort_order=EXCLUDED.sort_order, active=EXCLUDED.active,
				archived_at = CASE WHEN EXCLUDED.active THEN NULL ELSE coalesce(fabrics.archived_at, now()) END
			RETURNING id`, in.Key, in.Name, strings.TrimSpace(in.MaterialType), in.Composition, in.WeightGSM, in.TextureDescription,
			orDefault(in.Season, "all season"), in.CareInstructions, in.PriceImpactMinor, in.StockStatus, in.StockMeters, in.LowStockMeters,
			emptyNil(in.SwatchURL), []byte(in.PBR), in.SuitableGarments, in.SortOrder, in.Active).Scan(&id); err != nil {
			return err
		}
		keep := make([]string, 0, len(in.Colors))
		for _, c := range in.Colors {
			keep = append(keep, c.Key)
			if _, err := tx.Exec(ctx, `INSERT INTO fabric_colors (fabric_id, key, name, hex, swatch_url, stock_status, sort_order)
				VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (fabric_id, key) DO UPDATE SET name=EXCLUDED.name, hex=EXCLUDED.hex,
				swatch_url=EXCLUDED.swatch_url, stock_status=EXCLUDED.stock_status, sort_order=EXCLUDED.sort_order`,
				id, c.Key, strings.TrimSpace(c.Name), strings.ToLower(c.Hex), emptyNil(c.SwatchURL), orDefault(c.StockStatus, "available"), c.SortOrder); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM fabric_colors WHERE fabric_id=$1 AND NOT (key = ANY($2))`, id, keep); err != nil {
			return err
		}
		after, err := Get(ctx, tx, in.Key)
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "fabrics.upsert", ObjectType: "fabric", ObjectID: id.String(), Before: before, After: after}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, after)
		return nil
	})
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return strings.TrimSpace(s)
}

func emptyNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}
