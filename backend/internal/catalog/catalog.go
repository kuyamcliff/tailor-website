// Package catalog serves ready-made and made-to-order products with filtering, and owner product management.
package catalog

import (
	"context"
	"fmt"
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

type Category struct {
	ID        uuid.UUID `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sortOrder"`
}

type Media struct {
	ID       uuid.UUID  `json:"id"`
	URL      string     `json:"url"`
	UploadID *uuid.UUID `json:"uploadId"`
	Alt      string     `json:"alt"`
	Width    *int       `json:"width"`
	Height   *int       `json:"height"`
}

type Variant struct {
	ID          uuid.UUID `json:"id"`
	SKU         string    `json:"sku"`
	SizeLabel   string    `json:"sizeLabel"`
	ColorName   string    `json:"colorName"`
	ColorHex    *string   `json:"colorHex"`
	PriceMinor  int64     `json:"priceMinor"`
	StockQty    int       `json:"stockQty,omitempty"`
	MadeToOrder bool      `json:"madeToOrder"`
	Available   bool      `json:"available"`
	LowStock    bool      `json:"lowStock"`
	Active      bool      `json:"active"`
	SortOrder   int       `json:"sortOrder"`
}

type FabricRef struct {
	Key              string  `json:"key"`
	Name             string  `json:"name"`
	Composition      string  `json:"composition"`
	WeightGSM        *int    `json:"weightGsm"`
	CareInstructions string  `json:"careInstructions"`
	SwatchURL        *string `json:"swatchUrl"`
}

type Product struct {
	ID              uuid.UUID  `json:"id"`
	Slug            string     `json:"slug"`
	Name            string     `json:"name"`
	Summary         string     `json:"summary"`
	Description     string     `json:"description,omitempty"`
	Category        *Category  `json:"category"`
	GarmentTypeKey  *string    `json:"garmentTypeKey"`
	Fabric          *FabricRef `json:"fabric"`
	FitNotes        string     `json:"fitNotes,omitempty"`
	Care            string     `json:"care,omitempty"`
	MeasurementInfo string     `json:"measurementInfo,omitempty"`
	RequiresFitting bool       `json:"requiresFitting"`
	Customizable    bool       `json:"customizable"`
	Visibility      string     `json:"visibility"`
	PriceMinor      int64      `json:"priceMinor"`
	PriceMaxMinor   int64      `json:"priceMaxMinor"`
	Featured        bool       `json:"featured"`
	SortOrder       int        `json:"sortOrder"`
	Availability    string     `json:"availability"` // in_stock, low_stock, made_to_order, out_of_stock
	Media           []Media    `json:"media"`
	Variants        []Variant  `json:"variants"`
	Sizes           []string   `json:"sizes"`
	Colors          []string   `json:"colors"`
	Version         int        `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

const lowStockThreshold = 3

type Filter struct {
	Query        string
	Category     string
	Size         string
	Color        string
	Fabric       string
	Availability string
	MinPrice     *int64
	MaxPrice     *int64
	Sort         string
	Featured     bool
	IDs          []uuid.UUID
	Staff        bool
	Visibility   string
}

func parseFilter(r *http.Request) Filter {
	q := r.URL.Query()
	f := Filter{Query: strings.TrimSpace(q.Get("q")), Category: q.Get("category"), Size: q.Get("size"), Color: q.Get("color"),
		Fabric: q.Get("fabric"), Availability: q.Get("availability"), Sort: q.Get("sort"), Featured: q.Get("featured") == "true",
		Visibility: q.Get("visibility")}
	if v, err := strconv.ParseInt(q.Get("minPrice"), 10, 64); err == nil && v >= 0 {
		f.MinPrice = &v
	}
	if v, err := strconv.ParseInt(q.Get("maxPrice"), 10, 64); err == nil && v >= 0 {
		f.MaxPrice = &v
	}
	for _, s := range strings.Split(q.Get("ids"), ",") {
		if id, err := uuid.Parse(strings.TrimSpace(s)); err == nil && len(f.IDs) < 50 {
			f.IDs = append(f.IDs, id)
		}
	}
	return f
}

// productBase computes price range and availability from active variants.
const productBase = `
WITH v AS (
  SELECT product_id,
    min(coalesce(pv.price_minor, p.price_minor)) AS pmin,
    max(coalesce(pv.price_minor, p.price_minor)) AS pmax,
    sum(pv.stock_qty) AS qty,
    bool_or(pv.made_to_order) AS mto,
    array_agg(DISTINCT pv.size_label) AS sizes,
    array_agg(DISTINCT pv.color_name) FILTER (WHERE pv.color_name <> '') AS colors
  FROM product_variants pv JOIN products p ON p.id = pv.product_id
  WHERE pv.active GROUP BY product_id
)
SELECT p.id, p.slug, p.name, p.summary, c.id, c.slug, c.name, c.sort_order, g.key,
  f.key, f.name, f.composition, f.weight_gsm, f.care_instructions, f.swatch_url,
  p.requires_fitting, p.customizable, p.visibility, coalesce(v.pmin, p.price_minor), coalesce(v.pmax, p.price_minor),
  p.featured, p.sort_order, coalesce(v.qty, 0), coalesce(v.mto, false), coalesce(v.sizes, '{}'), coalesce(v.colors, '{}'),
  p.version, p.created_at, p.updated_at, count(*) OVER ()
FROM products p
LEFT JOIN v ON v.product_id = p.id
LEFT JOIN product_categories c ON c.id = p.category_id
LEFT JOIN garment_types g ON g.id = p.garment_type_id
LEFT JOIN fabrics f ON f.id = p.fabric_id
`

func availability(qty int, mto bool) string {
	switch {
	case qty > lowStockThreshold:
		return "in_stock"
	case qty > 0:
		return "low_stock"
	case mto:
		return "made_to_order"
	default:
		return "out_of_stock"
	}
}

func scanProduct(row pgx.Row, total *int) (Product, error) {
	var p Product
	var catID *uuid.UUID
	var catSlug, catName *string
	var catSort *int
	var fKey, fName, fComp, fCare, fSwatch *string
	var fWeight *int
	var qty int
	var mto bool
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Summary, &catID, &catSlug, &catName, &catSort, &p.GarmentTypeKey,
		&fKey, &fName, &fComp, &fWeight, &fCare, &fSwatch,
		&p.RequiresFitting, &p.Customizable, &p.Visibility, &p.PriceMinor, &p.PriceMaxMinor, &p.Featured, &p.SortOrder, &qty, &mto,
		&p.Sizes, &p.Colors, &p.Version, &p.CreatedAt, &p.UpdatedAt, total)
	if catID != nil {
		p.Category = &Category{ID: *catID, Slug: *catSlug, Name: *catName, SortOrder: *catSort}
	}
	if fKey != nil {
		p.Fabric = &FabricRef{Key: *fKey, Name: *fName, Composition: *fComp, WeightGSM: fWeight, CareInstructions: *fCare, SwatchURL: fSwatch}
	}
	p.Availability = availability(qty, mto)
	p.Media, p.Variants = []Media{}, []Variant{}
	return p, err
}

func List(ctx context.Context, q db.Querier, f Filter, page httpx.Page) ([]Product, int, error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "$?", fmt.Sprintf("$%d", len(args))))
	}
	if !f.Staff {
		where = append(where, "p.visibility = 'published'")
	} else if f.Visibility != "" {
		add("p.visibility = $?", f.Visibility)
	}
	if f.Query != "" {
		add("(to_tsvector('simple', p.name || ' ' || p.summary || ' ' || p.description) @@ plainto_tsquery('simple', $?) OR p.name ILIKE '%' || $? || '%')", f.Query)
	}
	if f.Category != "" {
		add("c.slug = $?", f.Category)
	}
	if f.Fabric != "" {
		add("f.key = $?", f.Fabric)
	}
	if f.Size != "" {
		add("$? = ANY(v.sizes)", f.Size)
	}
	if f.Color != "" {
		add("$? = ANY(v.colors)", f.Color)
	}
	switch f.Availability {
	case "in_stock":
		where = append(where, "coalesce(v.qty,0) > 0")
	case "made_to_order":
		where = append(where, "coalesce(v.mto,false)")
	}
	if f.MinPrice != nil {
		add("coalesce(v.pmax, p.price_minor) >= $?", *f.MinPrice)
	}
	if f.MaxPrice != nil {
		add("coalesce(v.pmin, p.price_minor) <= $?", *f.MaxPrice)
	}
	if f.Featured {
		where = append(where, "p.featured")
	}
	if len(f.IDs) > 0 {
		add("p.id = ANY($?)", f.IDs)
	}
	order := "p.featured DESC, p.sort_order, p.created_at DESC"
	switch f.Sort {
	case "price_asc":
		order = "coalesce(v.pmin, p.price_minor), p.name"
	case "price_desc":
		order = "coalesce(v.pmax, p.price_minor) DESC, p.name"
	case "newest":
		order = "p.created_at DESC"
	case "name":
		order = "p.name"
	}
	args = append(args, page.Limit, page.Offset)
	sql := productBase + " WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	var list []Product
	total := 0
	for rows.Next() {
		p, err := scanProduct(rows, &total)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		list = append(list, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := attachMedia(ctx, q, list, 2); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func attachMedia(ctx context.Context, q db.Querier, list []Product, perProduct int) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(list))
	idx := map[uuid.UUID]int{}
	for i, p := range list {
		ids[i] = p.ID
		idx[p.ID] = i
	}
	rows, err := q.Query(ctx, `SELECT product_id, id, url, upload_id, alt, width, height FROM (
		SELECT *, row_number() OVER (PARTITION BY product_id ORDER BY sort_order, id) rn FROM product_media WHERE product_id = ANY($1)) m
		WHERE $2 = 0 OR rn <= $2 ORDER BY product_id, rn`, ids, perProduct)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid uuid.UUID
		var m Media
		if err := rows.Scan(&pid, &m.ID, &m.URL, &m.UploadID, &m.Alt, &m.Width, &m.Height); err != nil {
			return err
		}
		list[idx[pid]].Media = append(list[idx[pid]].Media, m)
	}
	return rows.Err()
}

func variants(ctx context.Context, q db.Querier, productID uuid.UUID, basePrice int64, staff bool) ([]Variant, error) {
	rows, err := q.Query(ctx, `SELECT id, sku, size_label, color_name, color_hex, price_minor, stock_qty, made_to_order, active, sort_order
		FROM product_variants WHERE product_id=$1 AND ($2 OR active) ORDER BY sort_order, size_label`, productID, staff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Variant{}
	for rows.Next() {
		var v Variant
		var price *int64
		if err := rows.Scan(&v.ID, &v.SKU, &v.SizeLabel, &v.ColorName, &v.ColorHex, &price, &v.StockQty, &v.MadeToOrder, &v.Active, &v.SortOrder); err != nil {
			return nil, err
		}
		v.PriceMinor = basePrice
		if price != nil {
			v.PriceMinor = *price
		}
		v.Available = v.StockQty > 0 || v.MadeToOrder
		v.LowStock = v.StockQty > 0 && v.StockQty <= lowStockThreshold
		if !staff {
			v.StockQty = 0 // exact stock counts are not exposed publicly
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Get loads a full product by slug (or id for staff).
func Get(ctx context.Context, q db.Querier, slugOrID string, staff bool) (*Product, error) {
	cond := "p.slug = $1"
	if staff {
		if _, err := uuid.Parse(slugOrID); err == nil {
			cond = "p.id::text = $1"
		}
	}
	if !staff {
		cond += " AND p.visibility = 'published'"
	}
	var total int
	p, err := scanProduct(q.QueryRow(ctx, productBase+" WHERE "+cond, slugOrID), &total)
	if err != nil {
		return nil, err
	}
	var base int64
	if err := q.QueryRow(ctx, `SELECT description, fit_notes, care, measurement_info, price_minor FROM products WHERE id=$1`, p.ID).
		Scan(&p.Description, &p.FitNotes, &p.Care, &p.MeasurementInfo, &base); err != nil {
		return nil, err
	}
	if p.Variants, err = variants(ctx, q, p.ID, base, staff); err != nil {
		return nil, err
	}
	list := []Product{p}
	if err := attachMedia(ctx, q, list, 0); err != nil {
		return nil, err
	}
	return &list[0], nil
}

type Handler struct{ Pool *pgxpool.Pool }

func (h Handler) PublicList(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 24, 60)
	list, total, err := List(r.Context(), h.Pool, parseFilter(r), page)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	httpx.JSON(w, http.StatusOK, httpx.NewList(list, total, page))
	return nil
}

// Facets returns the values available for shop filters, from published products only.
func (h Handler) Facets(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	out := map[string]any{}
	cats, err := Categories(ctx, h.Pool, true)
	if err != nil {
		return err
	}
	out["categories"] = cats
	var sizes, colors []string
	var minP, maxP *int64
	if err := h.Pool.QueryRow(ctx, `SELECT coalesce(array_agg(DISTINCT pv.size_label) FILTER (WHERE pv.size_label <> ''), '{}'),
			coalesce(array_agg(DISTINCT pv.color_name) FILTER (WHERE pv.color_name <> ''), '{}'),
			min(coalesce(pv.price_minor, p.price_minor)), max(coalesce(pv.price_minor, p.price_minor))
		FROM products p JOIN product_variants pv ON pv.product_id = p.id AND pv.active WHERE p.visibility='published'`).
		Scan(&sizes, &colors, &minP, &maxP); err != nil {
		return err
	}
	rows, err := h.Pool.Query(ctx, `SELECT DISTINCT f.key, f.name FROM products p JOIN fabrics f ON f.id = p.fabric_id
		WHERE p.visibility='published' ORDER BY f.name`)
	if err != nil {
		return err
	}
	type fab struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	fabs := []fab{}
	for rows.Next() {
		var f fab
		if err := rows.Scan(&f.Key, &f.Name); err != nil {
			rows.Close()
			return err
		}
		fabs = append(fabs, f)
	}
	rows.Close()
	out["sizes"], out["colors"], out["fabrics"] = sizes, colors, fabs
	out["price"] = map[string]any{"min": minP, "max": maxP}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h Handler) PublicGet(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p, err := Get(ctx, h.Pool, chi.URLParam(r, "slug"), false)
	if err != nil {
		return err
	}
	related := []Product{}
	if p.Category != nil {
		list, _, err := List(ctx, h.Pool, Filter{Category: p.Category.Slug}, httpx.Page{Limit: 5})
		if err != nil {
			return err
		}
		for _, x := range list {
			if x.ID != p.ID && len(related) < 4 {
				related = append(related, x)
			}
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	httpx.JSON(w, http.StatusOK, map[string]any{"product": p, "related": related})
	return nil
}

func Categories(ctx context.Context, q db.Querier, onlyWithProducts bool) ([]Category, error) {
	rows, err := q.Query(ctx, `SELECT c.id, c.slug, c.name, c.sort_order FROM product_categories c
		WHERE NOT $1 OR EXISTS (SELECT 1 FROM products p WHERE p.category_id = c.id AND p.visibility='published')
		ORDER BY c.sort_order, c.name`, onlyWithProducts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- Owner ----

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	f := parseFilter(r)
	f.Staff = true
	list, total, err := List(r.Context(), h.Pool, f, page)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(list, total, page))
	return nil
}

func (h Handler) OwnerGet(w http.ResponseWriter, r *http.Request) error {
	p, err := Get(r.Context(), h.Pool, chi.URLParam(r, "id"), true)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

func (h Handler) OwnerCategories(w http.ResponseWriter, r *http.Request) error {
	c, err := Categories(r.Context(), h.Pool, false)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, c)
	return nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func (h Handler) OwnerUpsertCategory(w http.ResponseWriter, r *http.Request) error {
	var in Category
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if !slugRe.MatchString(in.Slug) || strings.TrimSpace(in.Name) == "" {
		return httpx.Validation(map[string]string{"slug": "Enter a name and a lowercase slug."})
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO product_categories (slug, name, sort_order) VALUES ($1,$2,$3)
			ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name, sort_order=EXCLUDED.sort_order RETURNING id`,
			in.Slug, strings.TrimSpace(in.Name), in.SortOrder).Scan(&in.ID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "products.category.upsert", ObjectType: "product_category", ObjectID: in.ID.String(), After: in}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, in)
		return nil
	})
}

type variantInput struct {
	SKU         string  `json:"sku"`
	SizeLabel   string  `json:"sizeLabel"`
	ColorName   string  `json:"colorName"`
	ColorHex    *string `json:"colorHex"`
	PriceMinor  *int64  `json:"priceMinor"`
	StockQty    int     `json:"stockQty"`
	MadeToOrder bool    `json:"madeToOrder"`
	Active      bool    `json:"active"`
}

type mediaInput struct {
	URL      string     `json:"url"`
	UploadID *uuid.UUID `json:"uploadId"`
	Alt      string     `json:"alt"`
	Width    *int       `json:"width"`
	Height   *int       `json:"height"`
}

type productInput struct {
	Slug            string         `json:"slug"`
	Name            string         `json:"name"`
	Summary         string         `json:"summary"`
	Description     string         `json:"description"`
	CategorySlug    *string        `json:"categorySlug"`
	GarmentTypeKey  *string        `json:"garmentTypeKey"`
	FabricKey       *string        `json:"fabricKey"`
	FitNotes        string         `json:"fitNotes"`
	Care            string         `json:"care"`
	MeasurementInfo string         `json:"measurementInfo"`
	RequiresFitting bool           `json:"requiresFitting"`
	Customizable    bool           `json:"customizable"`
	Visibility      string         `json:"visibility"`
	PriceMinor      int64          `json:"priceMinor"`
	Featured        bool           `json:"featured"`
	SortOrder       int            `json:"sortOrder"`
	Variants        []variantInput `json:"variants"`
	Media           []mediaInput   `json:"media"`
	Version         int            `json:"version"`
}

func (in *productInput) validate() error {
	f := httpx.Fields{}
	if !slugRe.MatchString(in.Slug) || len(in.Slug) > 80 {
		f.Add("slug", "Use lowercase words separated by hyphens.")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 {
		f.Add("name", "Enter a product name.")
	}
	if in.PriceMinor < 0 {
		f.Add("priceMinor", "Price cannot be negative.")
	}
	switch in.Visibility {
	case "draft", "published", "hidden", "archived":
	default:
		f.Add("visibility", "Choose a visibility.")
	}
	skus := map[string]bool{}
	for i, v := range in.Variants {
		p := "variants." + strconv.Itoa(i)
		v.SKU = strings.TrimSpace(v.SKU)
		if v.SKU == "" || skus[v.SKU] {
			f.Add(p, "Each variant needs a unique SKU.")
		}
		skus[v.SKU] = true
		if v.StockQty < 0 || (v.PriceMinor != nil && *v.PriceMinor < 0) {
			f.Add(p, "Stock and price cannot be negative.")
		}
		if strings.TrimSpace(v.SizeLabel) == "" {
			in.Variants[i].SizeLabel = "One size"
		}
	}
	if in.Visibility == "published" && len(in.Variants) == 0 {
		f.Add("variants", "Add at least one variant before publishing.")
	}
	for i, m := range in.Media {
		if strings.TrimSpace(m.Alt) == "" {
			f.Add("media."+strconv.Itoa(i), "Describe each image for people using screen readers.")
		}
		if m.UploadID == nil && !strings.HasPrefix(m.URL, "/") {
			f.Add("media."+strconv.Itoa(i), "Use an uploaded image.")
		}
	}
	if in.Visibility == "published" && len(in.Media) == 0 {
		f.Add("media", "Add at least one photo before publishing.")
	}
	return f.Err()
}

func (h Handler) OwnerSave(w http.ResponseWriter, r *http.Request) error {
	var in productInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	ctx := r.Context()
	idParam := chi.URLParam(r, "id")
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var catID, garmentID, fabricID *uuid.UUID
		lookup := func(table, col string, val *string, dst **uuid.UUID, field string) error {
			if val == nil || *val == "" {
				return nil
			}
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id FROM `+table+` WHERE `+col+`=$1`, *val).Scan(&id); err != nil {
				return httpx.Validation(map[string]string{field: "Not found."})
			}
			*dst = &id
			return nil
		}
		if err := lookup("product_categories", "slug", in.CategorySlug, &catID, "categorySlug"); err != nil {
			return err
		}
		if err := lookup("garment_types", "key", in.GarmentTypeKey, &garmentID, "garmentTypeKey"); err != nil {
			return err
		}
		if err := lookup("fabrics", "key", in.FabricKey, &fabricID, "fabricKey"); err != nil {
			return err
		}
		var id uuid.UUID
		var before *Product
		if idParam == "" {
			err := tx.QueryRow(ctx, `INSERT INTO products (slug, name, summary, description, category_id, garment_type_id, fabric_id, fit_notes, care,
				measurement_info, requires_fitting, customizable, visibility, price_minor, featured, sort_order)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
				in.Slug, in.Name, in.Summary, in.Description, catID, garmentID, fabricID, in.FitNotes, in.Care, in.MeasurementInfo,
				in.RequiresFitting, in.Customizable, in.Visibility, in.PriceMinor, in.Featured, in.SortOrder).Scan(&id)
			if db.IsUniqueViolation(err, "") {
				return httpx.Validation(map[string]string{"slug": "Another product already uses this slug."})
			}
			if err != nil {
				return err
			}
		} else {
			var err error
			if id, err = uuid.Parse(idParam); err != nil {
				return httpx.NotFound("Product not found.")
			}
			before, _ = Get(ctx, tx, id.String(), true)
			// Optimistic concurrency: two screens editing the same product cannot overwrite each other silently.
			tag, err := tx.Exec(ctx, `UPDATE products SET slug=$2, name=$3, summary=$4, description=$5, category_id=$6, garment_type_id=$7, fabric_id=$8,
				fit_notes=$9, care=$10, measurement_info=$11, requires_fitting=$12, customizable=$13, visibility=$14, price_minor=$15, featured=$16,
				sort_order=$17, version=version+1 WHERE id=$1 AND version=$18`,
				id, in.Slug, in.Name, in.Summary, in.Description, catID, garmentID, fabricID, in.FitNotes, in.Care, in.MeasurementInfo,
				in.RequiresFitting, in.Customizable, in.Visibility, in.PriceMinor, in.Featured, in.SortOrder, in.Version)
			if db.IsUniqueViolation(err, "") {
				return httpx.Validation(map[string]string{"slug": "Another product already uses this slug."})
			}
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return httpx.Conflict("stale_version", "This product was changed by someone else. Reload to see the latest version.")
			}
		}
		// Variants are matched by SKU so their IDs (referenced by orders) are stable across edits.
		keep := []string{}
		for i, v := range in.Variants {
			keep = append(keep, strings.TrimSpace(v.SKU))
			_, err := tx.Exec(ctx, `INSERT INTO product_variants (product_id, sku, size_label, color_name, color_hex, price_minor, stock_qty, made_to_order, sort_order, active)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
				ON CONFLICT (sku) DO UPDATE SET size_label=EXCLUDED.size_label, color_name=EXCLUDED.color_name, color_hex=EXCLUDED.color_hex,
					price_minor=EXCLUDED.price_minor, stock_qty=EXCLUDED.stock_qty, made_to_order=EXCLUDED.made_to_order,
					sort_order=EXCLUDED.sort_order, active=EXCLUDED.active
				WHERE product_variants.product_id = EXCLUDED.product_id`,
				id, strings.TrimSpace(v.SKU), strings.TrimSpace(v.SizeLabel), strings.TrimSpace(v.ColorName), v.ColorHex, v.PriceMinor, v.StockQty,
				v.MadeToOrder, i, v.Active)
			if err != nil {
				return err
			}
		}
		// Variants removed from the form are deactivated, never deleted, so order history keeps its references.
		if _, err := tx.Exec(ctx, `UPDATE product_variants SET active=false WHERE product_id=$1 AND NOT (sku = ANY($2))`, id, keep); err != nil {
			return err
		}
		var owned int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM product_variants WHERE product_id=$1 AND sku = ANY($2)`, id, keep).Scan(&owned); err != nil {
			return err
		}
		if owned != len(keep) {
			return httpx.Validation(map[string]string{"variants": "A SKU is already used by another product."})
		}
		if _, err := tx.Exec(ctx, `DELETE FROM product_media WHERE product_id=$1`, id); err != nil {
			return err
		}
		for i, m := range in.Media {
			url := m.URL
			if m.UploadID != nil {
				url = "/api/v1/uploads/" + m.UploadID.String() + "/preview"
			}
			if _, err := tx.Exec(ctx, `INSERT INTO product_media (product_id, url, upload_id, alt, width, height, sort_order) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
				id, url, m.UploadID, strings.TrimSpace(m.Alt), m.Width, m.Height, i); err != nil {
				return err
			}
		}
		after, err := Get(ctx, tx, id.String(), true)
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "products.save", ObjectType: "product", ObjectID: id.String(), Before: before, After: after}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, after)
		return nil
	})
}
