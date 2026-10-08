package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

// defaultContent is starter copy the owner edits in the dashboard. It describes the service the
// software actually provides and makes no claims about the business (no statistics, awards or reviews).
var defaultContent = map[string]any{
	"home.hero": map[string]any{
		"eyebrow":      "Bespoke tailoring",
		"title":        "Made for your measurements.",
		"subtitle":     "Suits, shirts, dresses and gowns cut by hand for one person: you. Design your piece, see it from every angle, then let us make it.",
		"primaryCta":   map[string]string{"label": "Create your outfit", "href": "/custom-tailor"},
		"secondaryCta": map[string]string{"label": "Book a consultation", "href": "/appointments"},
		"image":        "",
	},
	"home.signature": map[string]any{
		"title": "Your garment begins with a conversation.",
		"body":  "Tell us about the occasion, choose a fabric and the details that matter to you. We take your measurements, cut a pattern for your body and fit the garment on you before it is finished.",
		"cta":   map[string]string{"label": "Start a custom request", "href": "/custom-tailor/request"},
	},
	"home.studio": map[string]any{
		"title": "See the fit from every angle.",
		"body":  "Build your garment in the fitting studio. Change the lapel, the fabric, the length, and turn the figure to check the front, the back and the side. Enter your measurements and the studio estimates how each area will fit.",
		"note":  "The studio gives an estimate. Your tailor confirms every measurement before cutting.",
		"cta":   map[string]string{"label": "Open the fitting studio", "href": "/studio"},
	},
	"home.process": map[string]any{
		"title": "How we work",
		"steps": []map[string]string{
			{"title": "Consultation", "body": "We talk about the occasion, your style and how you like your clothes to feel."},
			{"title": "Measurements", "body": "We measure you in the studio, or you send your own and we verify them."},
			{"title": "Fabric and design", "body": "Choose from our fabrics and decide every detail, from lapels to hems."},
			{"title": "Cutting and sewing", "body": "Your pattern is drafted and the garment is cut and sewn by hand."},
			{"title": "Fitting", "body": "You try the garment on and we adjust it until it sits right."},
			{"title": "Collection", "body": "Collect from the studio or have it delivered."},
		},
	},
	"services": map[string]any{
		"title": "Services",
		"items": []map[string]string{
			{"title": "Bespoke suits and jackets", "body": "Single or double breasted, cut from your measurements.", "href": "/custom-tailor"},
			{"title": "Shirts", "body": "Collars, cuffs and fit chosen by you.", "href": "/custom-tailor"},
			{"title": "Dresses and gowns", "body": "From everyday dresses to bridal and evening gowns.", "href": "/custom-tailor"},
			{"title": "Traditional wear", "body": "Ceremonial and traditional garments made to measure.", "href": "/custom-tailor/request"},
			{"title": "Uniforms", "body": "Consistent, well-fitting uniforms for teams and schools.", "href": "/contact"},
			{"title": "Alterations", "body": "Hems, waists, sleeves and refits for garments you already own.", "href": "/appointments"},
		},
	},
	"faqs": map[string]any{
		"title": "Questions",
		"items": []map[string]string{
			{"q": "How long does a bespoke garment take?", "a": "It depends on the garment and the fabric. Your quote includes an estimated ready date, and you can follow each stage of production from your order page."},
			{"q": "Do I have to come to the studio?", "a": "You can send your own measurements using our guide and we will check them. For the best result we recommend at least one fitting in person."},
			{"q": "Is the 3D studio exact?", "a": "The studio shows an estimate based on your measurements. Your tailor verifies every measurement and the garment is fitted on you before it is finished."},
			{"q": "How do I pay?", "a": "For bespoke work we send you a quote first. When you accept it you pay the deposit shown on the quote, and the balance when your garment is ready."},
			{"q": "Can I change something after I accept a quote?", "a": "Contact us as soon as possible. Small changes are usually possible before cutting begins."},
		},
	},
	"about": map[string]any{
		"title": "About the atelier",
		"body":  "We make clothes for one person at a time. Every garment starts with your measurements and is finished with a fitting, so it sits the way it should.",
		"image": "",
	},
	"contact": map[string]any{
		"title": "Visit or contact us",
		"body":  "Book a consultation, ask a question about an order, or come and see the fabrics in person.",
	},
	"footer": map[string]any{
		"note": "Bespoke tailoring, alterations and made-to-measure clothing.",
	},
	"custom.landing": map[string]any{
		"title":    "Create your outfit",
		"subtitle": "Design a garment that is yours alone. Choose the style, define your measurements, see it in 3D, then request a quote.",
		"steps": []map[string]string{
			{"title": "Choose a garment", "body": "Suit, shirt, dress, gown, trousers or something else."},
			{"title": "Configure", "body": "Lapels, collars, sleeves, lengths and finishing."},
			{"title": "Your body profile", "body": "Enter your measurements or book a measuring session."},
			{"title": "Preview in 3D", "body": "Turn the figure and check every angle."},
			{"title": "Add references", "body": "Upload photos of styles, fabrics and details you like."},
			{"title": "Request a quote", "body": "No payment until we have reviewed your request."},
			{"title": "Fitting", "body": "Try it on and we adjust it to you."},
			{"title": "Production", "body": "Follow your garment through each stage."},
		},
	},
	"policy.privacy": map[string]any{
		"title": "Privacy",
		"sections": []map[string]string{
			{"heading": "What we collect", "body": "Your name and contact details, your measurements, the designs you save, the reference images you upload, and the details of your orders and appointments."},
			{"heading": "Why we collect it", "body": "To make, fit and deliver your garments, to contact you about your orders, and to keep accurate records for accounting."},
			{"heading": "Your images", "body": "Reference images are private. Only you and the atelier staff working on your request can see them. We do not use them for marketing unless you give separate permission. Reference images are deleted automatically after the retention period."},
			{"heading": "Your choices", "body": "From your account you can download your data, delete saved measurements and designs, and delete your account. Order records are kept for accounting with your contact details removed."},
			{"heading": "Payments", "body": "Mobile Money payments are processed by the provider you choose. We never see or store your Mobile Money PIN."},
		},
	},
	"policy.terms": map[string]any{
		"title": "Terms",
		"sections": []map[string]string{
			{"heading": "Quotes", "body": "Bespoke work is quoted individually. A quote is valid until the date shown on it. Accepting a quote creates your order."},
			{"heading": "Deposits", "body": "Production starts once the deposit shown on your quote is received. The balance is due when your garment is ready."},
			{"heading": "Measurements", "body": "Measurements you provide are checked by your tailor. Where we take your measurements ourselves, we are responsible for their accuracy."},
		},
	},
	"policy.delivery": map[string]any{
		"title": "Delivery and pickup",
		"sections": []map[string]string{
			{"heading": "Pickup", "body": "You can collect your order from the studio during opening hours. We will tell you when it is ready."},
			{"heading": "Delivery", "body": "Where delivery is offered, the fee is shown at checkout or on your quote."},
		},
	},
	"policy.alterations": map[string]any{
		"title": "Alterations and remakes",
		"sections": []map[string]string{
			{"heading": "Fitting adjustments", "body": "Adjustments found at your fittings are included in the price of a bespoke garment."},
			{"heading": "After collection", "body": "If something does not fit as agreed, contact us and we will arrange a fitting to put it right."},
		},
	},
}

type manifestFile struct {
	Assets []struct {
		AssetKey          string          `json:"assetKey"`
		Version           int             `json:"version"`
		Kind              string          `json:"kind"`
		GarmentTypeKeys   []string        `json:"garmentTypeKeys"`
		Files             json.RawMessage `json:"files"`
		BodyCompat        json.RawMessage `json:"bodyCompat"`
		SupportedOptions  json.RawMessage `json:"supportedOptions"`
		TextureSetVersion string          `json:"textureSetVersion"`
		License           json.RawMessage `json:"license"`
		ProductionQuality bool            `json:"productionQuality"`
		Notes             string          `json:"notes"`
	} `json:"assets"`
}

// seedAssets registers the 3D assets listed in the frontend asset manifest (generated by the asset
// build script with sizes and checksums). Each asset version is published only if no version of that
// asset is live yet, so owner-published versions are never replaced.
func seedAssets(ctx context.Context, tx pgx.Tx) error {
	path := os.Getenv("ASSET_MANIFEST_PATH")
	if path == "" {
		path = filepath.Join("..", "frontend", "public", "3d", "manifest.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed: no 3D asset manifest at %s, skipping asset registration\n", path)
		return nil
	}
	var m manifestFile
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("asset manifest: %w", err)
	}
	for _, a := range m.Assets {
		keys := a.GarmentTypeKeys
		if len(keys) == 0 {
			keys = []string{""}
		}
		for _, gk := range keys {
			assetKey := a.AssetKey
			if gk != "" && len(a.GarmentTypeKeys) > 1 {
				assetKey = a.AssetKey + "-" + gk
			}
			var live bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM asset_manifests WHERE asset_key=$1 AND status='published')`, assetKey).Scan(&live); err != nil {
				return err
			}
			status := "published"
			if live {
				status = "approved"
			}
			if _, err := tx.Exec(ctx, `INSERT INTO asset_manifests (asset_key, version, kind, garment_type_id, status, files, body_compat, supported_options,
					texture_set_version, license, production_quality, notes)
				VALUES ($1,$2,$3,(SELECT id FROM garment_types WHERE key=$4),$5,$6,$7,$8,$9,$10,$11,$12)
				ON CONFLICT (asset_key, version) DO UPDATE SET files=EXCLUDED.files, body_compat=EXCLUDED.body_compat,
					supported_options=EXCLUDED.supported_options, license=EXCLUDED.license, notes=EXCLUDED.notes`,
				assetKey, a.Version, a.Kind, gk, status, []byte(a.Files), []byte(orObj(a.BodyCompat)), []byte(orObj(a.SupportedOptions)),
				a.TextureSetVersion, []byte(orObj(a.License)), a.ProductionQuality, a.Notes); err != nil {
				return fmt.Errorf("asset %s: %w", assetKey, err)
			}
		}
	}
	return nil
}

func orObj(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(`{}`)
	}
	return b
}
