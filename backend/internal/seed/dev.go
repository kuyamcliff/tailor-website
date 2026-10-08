package seed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

// Development content. Photos are Unsplash images (Unsplash License) used only to make the
// development site realistic; they are not presented as the atelier's work in production because
// seed-dev refuses to run in production and every image records its source and license.
type devPhoto struct {
	id, alt string
}

func unsplash(id string) string {
	return "https://images.unsplash.com/photo-" + id + "?w=1800&q=82&fm=jpg"
}

var devFabrics = []struct {
	key, name, material, composition, texture, season, care string
	weight                                                  int
	price                                                   int64
	stock                                                   string
	garments                                                []string
	pbr                                                     map[string]any
	colors                                                  [][3]string
}{
	{"worsted-wool-twill", "Worsted wool twill", "Wool", "100% wool, Super 120s", "Fine diagonal twill with a soft hand", "All season",
		"Dry clean only. Brush after wearing and hang on a shaped hanger.", 260, 45000, "available", []string{"suit", "jacket", "trousers"},
		map[string]any{"colorMap": "/textures/worsted-wool-twill/color.webp", "normalMap": "/textures/worsted-wool-twill/normal.webp",
			"roughnessMap": "/textures/worsted-wool-twill/roughness.webp", "repeat": 6, "roughness": 0.85, "sheen": 0.35, "tint": true},
		[][3]string{{"midnight-navy", "Midnight navy", "#1f2a44"}, {"charcoal", "Charcoal", "#3a3b3f"}, {"black", "Black", "#18181a"}, {"mid-grey", "Mid grey", "#6b6d72"}}},
	{"irish-linen", "Irish linen", "Linen", "100% linen", "Open weave with natural slub", "Summer",
		"Dry clean or gentle hand wash. Linen creases naturally.", 230, 30000, "available", []string{"suit", "jacket", "trousers", "shirt", "dress"},
		map[string]any{"colorMap": "/textures/irish-linen/color.webp", "normalMap": "/textures/irish-linen/normal.webp",
			"roughnessMap": "/textures/irish-linen/roughness.webp", "repeat": 5, "roughness": 0.92, "tint": true},
		[][3]string{{"natural", "Natural", "#d8cfbf"}, {"sand", "Sand", "#c4ad8a"}, {"sage", "Sage", "#8f9a83"}, {"white", "White", "#f2efe8"}}},
	{"cotton-poplin", "Cotton poplin", "Cotton", "100% cotton, two-ply", "Smooth, crisp shirting", "All season",
		"Machine wash at 40°C. Iron while slightly damp.", 110, 12000, "available", []string{"shirt", "dress"},
		map[string]any{"colorMap": "/textures/cotton-poplin/color.webp", "normalMap": "/textures/cotton-poplin/normal.webp",
			"roughnessMap": "/textures/cotton-poplin/roughness.webp", "repeat": 10, "roughness": 0.7, "tint": true},
		[][3]string{{"white", "White", "#f4f3ef"}, {"sky", "Sky blue", "#b9cfe6"}, {"blush", "Blush", "#e9cfcc"}, {"ecru", "Ecru", "#ece2cf"}}},
	{"houndstooth-wool", "Houndstooth wool", "Wool", "95% wool, 5% cashmere", "Classic woven houndstooth", "Autumn and winter",
		"Dry clean only.", 320, 55000, "low_stock", []string{"jacket", "suit", "trousers"},
		map[string]any{"colorMap": "/textures/houndstooth-wool/color.webp", "normalMap": "/textures/houndstooth-wool/normal.webp",
			"roughnessMap": "/textures/houndstooth-wool/roughness.webp", "repeat": 4, "roughness": 0.88, "sheen": 0.3, "tint": false},
		[][3]string{{"grey", "Grey houndstooth", "#7b7c7f"}}},
	{"silk-crepe", "Silk crepe", "Silk", "100% silk", "Fluid crepe with a soft lustre", "All season",
		"Dry clean only. Store away from direct light.", 90, 70000, "available", []string{"dress", "gown"},
		map[string]any{"colorMap": "/textures/silk-crepe/color.webp", "normalMap": "/textures/silk-crepe/normal.webp",
			"roughnessMap": "/textures/silk-crepe/roughness.webp", "repeat": 8, "roughness": 0.45, "sheen": 0.8, "clearcoat": 0.05, "tint": true},
		[][3]string{{"ivory", "Ivory", "#efe6d4"}, {"burgundy", "Burgundy", "#5c1a26"}, {"emerald", "Emerald", "#1f4d3a"}, {"onyx", "Onyx", "#151515"}}},
	{"cotton-velvet", "Cotton velvet", "Velvet", "100% cotton pile", "Dense pile with a deep colour", "Autumn and winter",
		"Dry clean only. Steam, never iron the pile.", 380, 60000, "custom_order", []string{"jacket", "dress", "gown"},
		map[string]any{"colorMap": "/textures/cotton-velvet/color.webp", "normalMap": "/textures/cotton-velvet/normal.webp",
			"roughnessMap": "/textures/cotton-velvet/roughness.webp", "repeat": 6, "roughness": 0.95, "sheen": 1, "tint": false},
		[][3]string{{"garnet", "Garnet", "#6e1f2b"}}},
	{"heritage-tartan", "Heritage tartan", "Wool", "100% wool", "Woven tartan check", "Autumn and winter",
		"Dry clean only.", 300, 50000, "out_of_stock", []string{"jacket", "trousers", "traditional"},
		map[string]any{"colorMap": "/textures/heritage-tartan/color.webp", "normalMap": "/textures/heritage-tartan/normal.webp",
			"roughnessMap": "/textures/heritage-tartan/roughness.webp", "repeat": 3, "roughness": 0.85, "tint": false},
		[][3]string{{"night", "Night tartan", "#1d2433"}}},
}

type devProduct struct {
	slug, name, summary, description, category, garment, fabric, fit, care, measurementInfo string
	price                                                                                   int64
	requiresFitting, customizable, featured                                                 bool
	photos                                                                                  []devPhoto
	variants                                                                                [][4]any // size, color, stock, madeToOrder
}

var devProducts = []devProduct{
	{"navy-two-piece-suit", "Navy two-piece suit", "Single breasted, notch lapel, in midnight navy wool.", "A dependable navy suit with a soft shoulder and a clean, slightly tapered trouser. Made in our worsted wool twill and finished by hand.",
		"suits", "suit", "worsted-wool-twill", "Regular fit. Final adjustments to sleeve and trouser length are included.", "Dry clean only.",
		"Sized in standard chest sizes. Each suit is adjusted at a fitting.", 220000, true, true, true,
		[]devPhoto{{"1617137968427-85924c800a22", "Man walking in a navy two-piece suit with a white shirt"}, {"1617137984095-74e4e5e3613f", "Navy suit jacket worn open over a white shirt"}},
		[][4]any{{"46R", "Navy", 2, false}, {"48R", "Navy", 4, false}, {"50R", "Navy", 3, false}, {"52R", "Navy", 0, true}}},
	{"black-evening-suit", "Black evening suit", "Black suit with a sharp peak lapel for evening events.", "A black suit cut for weddings and evening occasions, with a peak lapel and jetted pockets.",
		"suits", "suit", "worsted-wool-twill", "Slim fit through the waist.", "Dry clean only.", "Adjusted at a fitting.", 260000, true, true, true,
		[]devPhoto{{"1598808503746-f34c53b9323e", "Black suit with peak lapels displayed on a mannequin"}, {"1617127365659-c47fa864d8bc", "Man in a black suit and black shirt"}},
		[][4]any{{"48R", "Black", 1, false}, {"50R", "Black", 2, false}, {"52R", "Black", 0, true}}},
	{"blue-check-three-piece", "Blue check three-piece suit", "Windowpane check with a matching waistcoat.", "A three-piece suit in a blue windowpane check, with a five-button waistcoat.",
		"suits", "suit", "worsted-wool-twill", "Regular fit.", "Dry clean only.", "Adjusted at a fitting.", 290000, true, true, false,
		[]devPhoto{{"1594938298603-c8148c4dae35", "Blue check three-piece suit with a tie and pocket square"}},
		[][4]any{{"48R", "Blue check", 0, true}, {"50R", "Blue check", 0, true}}},
	{"forest-green-suit", "Forest green suit", "Deep green two-piece for a confident occasion look.", "A forest green suit with a slim lapel, worn with a dark shirt or a crisp white one.",
		"suits", "suit", "worsted-wool-twill", "Slim fit.", "Dry clean only.", "Adjusted at a fitting.", 240000, true, true, false,
		[]devPhoto{{"1593032465175-481ac7f401a0", "Man in a forest green two-piece suit adjusting his cuff"}},
		[][4]any{{"48R", "Forest green", 1, false}, {"50R", "Forest green", 0, true}}},
	{"navy-check-blazer", "Navy check blazer", "An unstructured blazer in a navy windowpane check.", "A navy check blazer that works with grey trousers or denim. Half lined for comfort.",
		"jackets", "jacket", "houndstooth-wool", "Regular fit.", "Dry clean only.", "Sleeve length adjusted on request.", 150000, false, true, true,
		[]devPhoto{{"1592878904946-b3cd8ae243d0", "Man wearing a navy windowpane check blazer with a white shirt and tie"}},
		[][4]any{{"48", "Navy check", 2, false}, {"50", "Navy check", 2, false}, {"52", "Navy check", 1, false}}},
	{"white-poplin-shirt", "White poplin shirt", "Spread collar, barrel cuffs, in crisp cotton poplin.", "The white shirt every wardrobe needs. Two-ply cotton poplin with a spread collar.",
		"shirts", "shirt", "cotton-poplin", "Regular body.", "Machine wash at 40°C.", "Collar sizes 38 to 44.", 38000, false, true, true,
		[]devPhoto{{"1603252109303-2751441dd157", "White shirts hanging on a rail"}},
		[][4]any{{"38", "White", 6, false}, {"40", "White", 8, false}, {"42", "White", 5, false}, {"44", "White", 3, false}}},
	{"chambray-everyday-shirt", "Chambray everyday shirt", "A soft blue shirt for relaxed days.", "Soft chambray with a button-down collar and a chest pocket.",
		"shirts", "shirt", "cotton-poplin", "Regular body.", "Machine wash at 30°C.", "Collar sizes 38 to 44.", 32000, false, false, false,
		[]devPhoto{{"1558171813-4c088753af8f", "Light blue chambray shirt folded on a chair"}},
		[][4]any{{"40", "Light blue", 4, false}, {"42", "Light blue", 2, false}}},
	{"tapered-chinos", "Tapered chinos", "Mid-rise cotton chinos with a tapered leg.", "Comfortable everyday trousers with a mid rise and a tapered leg, hemmed to your length.",
		"trousers", "trousers", "", "Slim through the thigh.", "Machine wash at 30°C.", "Hemmed to your inseam at no extra cost.", 28000, false, false, false,
		[]devPhoto{{"1473966968600-fa801b869a1a", "Khaki tapered chinos worn with white trainers"}},
		[][4]any{{"30", "Khaki", 3, false}, {"32", "Khaki", 5, false}, {"34", "Khaki", 4, false}, {"36", "Khaki", 2, false}}},
}

var devPortfolio = []struct {
	slug, title, category, description, materials string
	tags                                          []string
	featured                                      bool
	photos                                        []devPhoto
}{
	{"bridal-gown-lace", "Bridal gown with lace overlay", "wedding", "A bridal gown with a fitted bodice and a full skirt finished with lace.", "Silk, lace, tulle",
		[]string{"bridal", "gown"}, true, []devPhoto{{"1594552072238-b8a33785b261", "White bridal gown with a lace skirt displayed in a bright room"}}},
	{"red-evening-gown", "Red evening gown", "gowns", "A flowing evening gown with a full circle skirt.", "Silk crepe",
		[]string{"evening"}, true, []devPhoto{{"1595777457583-95e059d581b8", "Woman in a flowing red evening gown outdoors"}}},
	{"plum-off-shoulder", "Plum off-shoulder dress", "dresses", "An off-shoulder dress with a fitted bodice.", "Silk crepe",
		[]string{"evening"}, false, []devPhoto{{"1566174053879-31528523f8ae", "Woman in a plum off-shoulder dress against a purple backdrop"}}},
	{"red-column-dress", "Red column dress with train", "dresses", "A column dress with a long sleeve and a sweeping train.", "Crepe",
		[]string{"occasion"}, false, []devPhoto{{"1612336307429-8a898d10e223", "Woman in a red long-sleeve dress with a train"}}},
	{"tuxedo-check", "Evening jacket in teal check", "suits", "A peak lapel evening jacket in a teal windowpane check.", "Wool",
		[]string{"evening", "jacket"}, true, []devPhoto{{"1600091166971-7f9faad6c1e2", "Teal check evening jacket with a bow tie on a mannequin"}}},
	{"womens-black-suit", "Women's black trouser suit", "suits", "A double breasted jacket with straight trousers.", "Wool",
		[]string{"womenswear"}, false, []devPhoto{{"1584273143981-41c073dfe8f8", "Woman in a black double breasted trouser suit in the street"}}},
	{"jackets-on-forms", "Jackets in progress", "suits", "Jackets on forms in the atelier during fitting.", "Wool, tweed",
		[]string{"atelier"}, false, []devPhoto{{"1580657018950-c7f7d6a6d990", "Tailored jackets displayed on forms in a shop"}}},
	{"navy-business-suit", "Navy business suit", "suits", "A navy business suit with a striped tie.", "Worsted wool",
		[]string{"business"}, false, []devPhoto{{"1507679799987-c73779587ccf", "Man buttoning a navy suit jacket"}}},
}

// Development loads demonstration content. It is refused in production by the caller.
func Development(ctx context.Context, pool *pgxpool.Pool, store uploads.Storage) error {
	ownerEmail, ownerPassword := "owner@atelier.test", "atelier-owner-dev"
	if err := devUser(ctx, pool, ownerEmail, ownerPassword, "Atelier Owner", "owner"); err != nil {
		return err
	}
	if err := devUser(ctx, pool, "tailor@atelier.test", "atelier-tailor-dev", "Studio Tailor", "tailor"); err != nil {
		return err
	}
	biz := settings.DefaultBusiness()
	biz.Name = "The Atelier"
	biz.Tagline = "Bespoke tailoring"
	biz.Phone = "+237 6 00 00 00 00"
	biz.WhatsApp = "237600000000"
	biz.Email = "studio@atelier.test"
	biz.Address = settings.Address{Line1: "Development address", City: "Douala", Country: "Cameroon"}
	biz.OpeningHours = []settings.OpeningHours{{Days: "Monday to Friday", Hours: "09:00 to 18:00"}, {Days: "Saturday", Hours: "10:00 to 16:00"}}
	biz.TaxRateBP = 0
	biz.Delivery = []settings.DeliveryZone{
		{Key: "pickup", Method: "pickup", Label: "Studio pickup", Description: "Collect from the studio when ready.", Active: true},
		{Key: "douala", Method: "local_delivery", Label: "Delivery in Douala", FeeMinor: 2000, Description: "Delivered within two working days of completion.", Active: true},
		{Key: "courier", Method: "courier", Label: "Courier, rest of Cameroon", FeeMinor: 5000, Description: "Sent by courier.", Active: true},
	}
	biz.Social = []settings.SocialLink{{Network: "instagram", URL: "https://www.instagram.com/"}, {Network: "whatsapp", URL: "https://wa.me/237600000000"},
		{Network: "tiktok", URL: "https://www.tiktok.com/"}, {Network: "facebook", URL: "https://www.facebook.com/"}}
	bj, _ := json.Marshal(biz)
	if _, err := pool.Exec(ctx, `INSERT INTO business_settings (key, value) VALUES ('business',$1) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`, bj); err != nil {
		return err
	}
	for _, k := range []string{"online_payments", "payments_mtn", "payments_orange"} {
		if _, err := pool.Exec(ctx, `UPDATE feature_flags SET enabled=true WHERE key=$1`, k); err != nil {
			return err
		}
	}
	for i, fb := range devFabrics {
		pj, _ := json.Marshal(fb.pbr)
		swatch := "/textures/" + fb.key + "/swatch.webp"
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO fabrics (key, name, material_type, composition, weight_gsm, texture_description, season, care_instructions,
				price_impact_minor, stock_status, stock_meters, low_stock_meters, swatch_url, pbr, suitable_garments, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,5,$12,$13,$14,$15)
			ON CONFLICT (key) DO UPDATE SET pbr=EXCLUDED.pbr, stock_status=EXCLUDED.stock_status RETURNING id`,
			fb.key, fb.name, fb.material, fb.composition, fb.weight, fb.texture, fb.season, fb.care, fb.price, fb.stock,
			stockFor(fb.stock), swatch, pj, fb.garments, i*10).Scan(&id); err != nil {
			return fmt.Errorf("fabric %s: %w", fb.key, err)
		}
		for ci, c := range fb.colors {
			if _, err := pool.Exec(ctx, `INSERT INTO fabric_colors (fabric_id, key, name, hex, sort_order) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
				id, c[0], c[1], c[2], ci); err != nil {
				return err
			}
		}
	}
	cache := filepath.Join(os.TempDir(), "atelier-dev-photos")
	_ = os.MkdirAll(cache, 0o755)
	for i, p := range devProducts {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE slug=$1)`, p.slug).Scan(&exists); err != nil || exists {
			continue
		}
		media, err := devMedia(ctx, pool, store, cache, "product", p.photos)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed: skipping %s photos: %v\n", p.slug, err)
			continue
		}
		err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO products (slug, name, summary, description, category_id, garment_type_id, fabric_id, fit_notes, care,
					measurement_info, requires_fitting, customizable, visibility, price_minor, featured, sort_order)
				VALUES ($1,$2,$3,$4,(SELECT id FROM product_categories WHERE slug=$5),(SELECT id FROM garment_types WHERE key=$6),
					(SELECT id FROM fabrics WHERE key=nullif($7,'')),$8,$9,$10,$11,$12,'published',$13,$14,$15) RETURNING id`,
				p.slug, p.name, p.summary, p.description, p.category, p.garment, p.fabric, p.fit, p.care, p.measurementInfo,
				p.requiresFitting, p.customizable, p.price, p.featured, i*10).Scan(&id); err != nil {
				return err
			}
			for vi, v := range p.variants {
				sku := fmt.Sprintf("%s-%s", p.slug, v[0])
				if _, err := tx.Exec(ctx, `INSERT INTO product_variants (product_id, sku, size_label, color_name, stock_qty, made_to_order, sort_order)
					VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, sku, v[0], v[1], v[2], v[3], vi); err != nil {
					return err
				}
			}
			for mi, m := range media {
				if _, err := tx.Exec(ctx, `INSERT INTO product_media (product_id, url, upload_id, alt, width, height, sort_order) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
					id, m.url, m.id, m.alt, m.w, m.h, mi); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("product %s: %w", p.slug, err)
		}
	}
	for i, p := range devPortfolio {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM portfolio_projects WHERE slug=$1)`, p.slug).Scan(&exists); err != nil || exists {
			continue
		}
		media, err := devMedia(ctx, pool, store, cache, "portfolio", p.photos)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed: skipping %s photos: %v\n", p.slug, err)
			continue
		}
		err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO portfolio_projects (slug, title, category, description, materials, tags, customer_permission, featured, status, sort_order)
				VALUES ($1,$2,$3,$4,$5,$6,'not_required',$7,'published',$8) RETURNING id`, p.slug, p.title, p.category, p.description, p.materials, p.tags,
				p.featured, i*10).Scan(&id); err != nil {
				return err
			}
			for mi, m := range media {
				if _, err := tx.Exec(ctx, `INSERT INTO portfolio_media (project_id, url, upload_id, alt, width, height, sort_order) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
					id, m.url, m.id, m.alt, m.w, m.h, mi); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	// Hero and about imagery reference uploaded development photos.
	hero, err := devMedia(ctx, pool, store, cache, "content", []devPhoto{{"1598808503746-f34c53b9323e", "Black tailored suit on a form in the atelier"}})
	if err == nil && len(hero) == 1 {
		_, _ = pool.Exec(ctx, `UPDATE content_blocks SET value = jsonb_set(jsonb_set(value, '{image}', to_jsonb($1::text)), '{imageAlt}', to_jsonb($2::text)) WHERE key='home.hero'`,
			hero[0].url, hero[0].alt)
	}
	about, err := devMedia(ctx, pool, store, cache, "content", []devPhoto{{"1580657018950-c7f7d6a6d990", "Tailored jackets on forms in the atelier"}})
	if err == nil && len(about) == 1 {
		_, _ = pool.Exec(ctx, `UPDATE content_blocks SET value = jsonb_set(jsonb_set(value, '{image}', to_jsonb($1::text)), '{imageAlt}', to_jsonb($2::text)) WHERE key='about'`,
			about[0].url, about[0].alt)
	}
	fmt.Printf("development data loaded. Owner sign-in: %s / %s\n", ownerEmail, ownerPassword)
	return nil
}

func stockFor(s string) *float64 {
	v := map[string]float64{"available": 40, "low_stock": 4, "out_of_stock": 0}[s]
	if s == "custom_order" || s == "discontinued" {
		return nil
	}
	return &v
}

func devUser(ctx context.Context, pool *pgxpool.Pool, email, password, name, role string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `INSERT INTO users (email, password_hash, full_name, role) VALUES ($1,$2,$3,$4)
		ON CONFLICT (lower(email)) WHERE email IS NOT NULL AND deleted_at IS NULL DO NOTHING`, email, hash, name, role)
	return err
}

type media struct {
	id   uuid.UUID
	url  string
	alt  string
	w, h int
}

// devMedia downloads (with a local cache) and stores photos through the same image pipeline as owner
// uploads: content sniffing, metadata stripping and derivative sizes. License metadata is recorded.
func devMedia(ctx context.Context, pool *pgxpool.Pool, store uploads.Storage, cache, purpose string, photos []devPhoto) ([]media, error) {
	var out []media
	for _, ph := range photos {
		path := filepath.Join(cache, ph.id+".jpg")
		data, err := os.ReadFile(path)
		if err != nil {
			c := &http.Client{Timeout: 60 * time.Second}
			resp, err := c.Get(unsplash(ph.id))
			if err != nil {
				return nil, err
			}
			data, err = io.ReadAll(io.LimitReader(resp.Body, 20<<20))
			resp.Body.Close()
			if err != nil || resp.StatusCode != 200 {
				return nil, fmt.Errorf("download %s: status %d", ph.id, resp.StatusCode)
			}
			_ = os.WriteFile(path, data, 0o644)
		}
		img, err := uploads.ProcessImage(data)
		if err != nil {
			return nil, err
		}
		id := uuid.New()
		base := fmt.Sprintf("%s/dev/%s", purpose, id)
		mainKey := base + "/original.jpg"
		if err := store.Put(ctx, mainKey, bytes.NewReader(img.Main), int64(len(img.Main)), img.MIME); err != nil {
			return nil, err
		}
		deriv := map[string]string{}
		for name, b := range img.Variants {
			k := base + "/" + name + ".jpg"
			if err := store.Put(ctx, k, bytes.NewReader(b), int64(len(b)), "image/jpeg"); err != nil {
				return nil, err
			}
			deriv[name] = k
		}
		sum := sha256.Sum256(data)
		lic := map[string]string{"source": "https://unsplash.com/photos/" + ph.id, "license": "Unsplash License", "licenseUrl": "https://unsplash.com/license",
			"usage": "development only"}
		if _, err := pool.Exec(ctx, `INSERT INTO uploads (id, purpose, visibility, storage_key, original_name, mime, bytes, width, height, sha256, derivatives, license)
			VALUES ($1,$2,'public',$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, purpose, mainKey, ph.id+".jpg", img.MIME, len(img.Main), img.Width, img.Height,
			hex.EncodeToString(sum[:]), deriv, lic); err != nil {
			return nil, err
		}
		out = append(out, media{id: id, url: "/api/v1/uploads/" + id.String() + "/preview", alt: ph.alt, w: img.Width, h: img.Height})
	}
	return out, nil
}
