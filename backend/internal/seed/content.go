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
			{"heading": "What we collect", "body": "Your name and contact details, the measurements you enter or that we take, the designs you save, the reference images you upload, and the details of your orders, payments, appointments and messages. We only ask for what we need to make and deliver your garment."},
			{"heading": "Why we collect it", "body": "To make, fit and deliver your garments, to contact you about your orders, requests and appointments, and to keep the records the law requires for accounting. We do not sell your data and we do not use it for advertising."},
			{"heading": "Your images", "body": "Reference images and body photos are private. Only you and the atelier staff working on your request can see them. We never use them for marketing or in our portfolio without your separate written permission. Reference images not attached to a request are deleted automatically after the retention period."},
			{"heading": "Who else processes your data", "body": "Mobile Money payments are processed by the provider you choose (MTN Mobile Money or Orange Money). They receive your phone number and the amount, and we never see or store your PIN. Order emails and text messages are sent through our email and SMS providers. Files are stored with our hosting provider. If you choose the optional reference image analysis, the image is sent to an analysis provider for that one request and is not used to train their models."},
			{"heading": "How long we keep it", "body": "Account data is kept while your account is open. Order and payment records are kept for the period required by accounting law, with your contact details removed if you delete your account. Unattached uploads are deleted automatically."},
			{"heading": "Children", "body": "Our online services are for adults. If you are under 18, a parent or guardian must place the order and enter any measurements on your behalf. We do not knowingly collect data from children without that consent."},
			{"heading": "Your rights", "body": "You can see, correct, download and delete your data. With an account, use Settings and privacy to download your data or delete your account. Without an account, send us a message through the Help page or contact details below and we will act on your request within 30 days."},
		},
	},
	"policy.terms": map[string]any{
		"title": "Terms",
		"sections": []map[string]string{
			{"heading": "Who we are", "body": "These terms apply to orders placed with the atelier named at the bottom of this page. Our address and contact details are shown on the Contact page."},
			{"heading": "Prices", "body": "Prices are shown in the currency of the shop and include any tax that applies. Delivery fees are shown before you place your order. There are no other charges. Bespoke work is quoted individually and the quote shows every cost."},
			{"heading": "Quotes and deposits", "body": "A quote is valid until the date shown on it. Accepting a quote creates your order. Production starts once the deposit shown on your quote is received, and the balance is due when your garment is ready."},
			{"heading": "Measurements", "body": "Measurements you provide are checked by your tailor before cutting. Where we take your measurements ourselves, we are responsible for their accuracy. The fitting studio shows an estimate to help you choose; it is not a guarantee of fit."},
			{"heading": "Cancelling", "body": "You can cancel a ready-to-wear order before it is dispatched or collected for a full refund. A bespoke order can be cancelled free of charge until we start cutting your cloth. After that, the deposit covers the cloth and work already done; anything paid above the cost of that work is refunded."},
			{"heading": "Age", "body": "You must be 18 or older to create an account or place an order. A parent or guardian can order for a child."},
		},
	},
	"policy.refunds": map[string]any{
		"title": "Returns and refunds",
		"sections": []map[string]string{
			{"heading": "Ready-to-wear", "body": "You can return an unworn ready-to-wear item with its labels within 14 days of collection or delivery for a refund or exchange. Items altered to your measurements cannot be returned, but we will correct any fault."},
			{"heading": "Bespoke garments", "body": "A bespoke garment is made for one person, so it cannot be returned for a change of mind. If it does not match the agreed design or measurements, we will alter or remake it at no cost. See Alterations and remakes."},
			{"heading": "How refunds are paid", "body": "Refunds go back to the Mobile Money account that paid, or by the method we agree with you for payments made at the studio. We send them within 10 working days of approving the refund and tell you when it is sent."},
			{"heading": "Faults", "body": "If you find a fault, contact us with a photo through the Help page. Your legal rights as a customer are not affected by this policy."},
		},
	},
	"policy.cookies": map[string]any{
		"title": "Cookies and storage",
		"sections": []map[string]string{
			{"heading": "What we store", "body": "We use one cookie, atelier_session, which keeps you signed in. It is created only when you sign in and is removed when you sign out. Your browser also stores your shopping bag, wishlist, comparison list, a random guest code that keeps your designs and uploads on this device, and private links to orders and requests you made from this device."},
			{"heading": "What we do not use", "body": "We do not use advertising cookies, tracking pixels or third-party analytics. Nothing we store is shared with advertisers. Because everything we store is needed for the site to work, we do not show a cookie banner."},
			{"heading": "Removing it", "body": "You can clear this data at any time in your browser settings. Your bag and wishlist on this device will be emptied; anything saved to your account stays in your account."},
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
