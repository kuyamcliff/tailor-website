// Package seed loads reference data (garments, measurements, fit rules, defaults) and, for
// development only, demonstration content.
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
)

type field struct {
	key, label, location, instruction, helper, kind string
	min, max                                        int
}

// Guided measurement definitions. Ranges are in millimetres and deliberately generous;
// unusual combinations are flagged for review rather than blocked.
var fields = []field{
	{"neck", "Neck", "Base of the neck", "Wrap the tape around the base of the neck where a shirt collar sits. Keep one finger under the tape.", "Stand straight and look ahead.", "circumference", 280, 600},
	{"shoulder", "Shoulder width", "Back, shoulder point to shoulder point", "Measure across the back from the edge of one shoulder bone to the other.", "Ask someone to help for this one.", "width", 330, 620},
	{"chest", "Chest", "Fullest part of the chest", "Wrap the tape under the arms around the fullest part of the chest, keeping it level.", "Breathe normally and relax your arms.", "circumference", 650, 1650},
	{"upper_chest", "Upper chest", "Above the chest, under the arms", "Measure high under the arms, above the fullest part of the chest.", "", "circumference", 650, 1600},
	{"bust", "Bust", "Fullest part of the bust", "Measure around the fullest part of the bust with the tape level across the back.", "Wear the undergarments you plan to wear with the garment.", "circumference", 650, 1650},
	{"underbust", "Underbust", "Directly under the bust", "Measure around the ribcage directly under the bust.", "", "circumference", 550, 1450},
	{"waist", "Waist", "Natural waist", "Measure around the narrowest part of the torso, usually just above the navel.", "Do not hold your breath.", "circumference", 480, 1650},
	{"stomach", "Stomach", "Fullest part of the stomach", "Measure around the fullest part of the stomach, usually at the navel.", "", "circumference", 550, 1750},
	{"high_hip", "High hip", "Top of the hip bones", "Measure around the hips about 10 cm below the natural waist.", "", "circumference", 600, 1650},
	{"hip", "Seat / full hip", "Fullest part of the seat", "Stand with feet together and measure around the fullest part of the seat.", "", "circumference", 650, 1750},
	{"bicep", "Upper arm", "Fullest part of the upper arm", "Measure around the fullest part of the upper arm with the arm relaxed.", "", "circumference", 200, 600},
	{"wrist", "Wrist", "Wrist bone", "Measure around the wrist just over the wrist bone.", "Add room if you wear a watch on this wrist.", "circumference", 120, 280},
	{"sleeve_length", "Sleeve length", "Shoulder point to wrist", "With the arm relaxed, measure from the shoulder point down to the wrist bone.", "", "length", 450, 780},
	{"shirt_length", "Shirt length", "Base of the collar to hem", "Measure down the back from the base of the collar to where you want the shirt to end.", "", "length", 550, 950},
	{"jacket_length", "Jacket length", "Base of the collar to hem", "Measure down the back from the base of the collar to where the jacket should end, usually covering the seat.", "", "length", 550, 950},
	{"back_width", "Back width", "Across the shoulder blades", "Measure across the back between the arm creases, about 15 cm below the base of the neck.", "", "width", 280, 560},
	{"thigh", "Thigh", "Fullest part of the thigh", "Measure around the fullest part of one thigh, just below the crotch.", "", "circumference", 380, 950},
	{"knee", "Knee", "Around the knee", "Measure around the knee with the leg straight.", "", "circumference", 280, 650},
	{"calf", "Calf", "Fullest part of the calf", "Measure around the fullest part of the calf.", "", "circumference", 250, 600},
	{"ankle", "Ankle", "Above the ankle bone", "Measure around the leg just above the ankle bone.", "", "circumference", 170, 380},
	{"rise", "Rise", "Waistband to crotch, front", "Measure from the front waistband down between the legs to the crotch seam.", "Take this measurement on well-fitting trousers if easier.", "length", 180, 420},
	{"inseam", "Inseam", "Crotch to hem", "Measure from the crotch down the inside of the leg to where the trousers should end.", "", "length", 550, 1000},
	{"outseam", "Outseam", "Waist to hem, outside leg", "Measure from the waist down the outside of the leg to where the trousers should end.", "", "length", 800, 1250},
	{"torso_length", "Torso length", "Shoulder to waist, front", "Measure from the highest point of the shoulder over the bust down to the natural waist.", "", "length", 330, 620},
	{"dress_length", "Dress length", "Shoulder to hem", "Measure from the highest point of the shoulder down the front to where the dress should end.", "", "length", 700, 1650},
	{"shoulder_to_floor", "Shoulder to floor", "Shoulder to floor", "Standing barefoot, measure from the highest point of the shoulder to the floor.", "Add your heel height in the notes.", "length", 1050, 1850},
}

type garment struct {
	key, name, category, description string
	basePrice                        int64
	studio                           bool
	bodyHint                         string
	fields                           []string // required fields first are marked required
	optional                         []string
	sort                             int
}

var garmentTypes = []garment{
	{"suit", "Suit", "menswear", "A two-piece suit cut to your measurements.", 180000, true, "masculine",
		[]string{"neck", "shoulder", "chest", "waist", "hip", "sleeve_length", "jacket_length", "bicep", "wrist", "inseam", "thigh"},
		[]string{"upper_chest", "stomach", "back_width", "knee", "ankle", "rise", "outseam"}, 10},
	{"jacket", "Jacket", "menswear", "A tailored jacket or blazer.", 120000, true, "masculine",
		[]string{"shoulder", "chest", "waist", "sleeve_length", "jacket_length", "bicep"},
		[]string{"neck", "upper_chest", "stomach", "hip", "wrist", "back_width"}, 20},
	{"shirt", "Shirt", "menswear", "A shirt with your choice of collar, cuff and fit.", 35000, true, "masculine",
		[]string{"neck", "shoulder", "chest", "waist", "sleeve_length", "shirt_length"},
		[]string{"hip", "bicep", "wrist"}, 30},
	{"trousers", "Trousers", "menswear", "Trousers cut to your waist, seat and leg.", 45000, true, "masculine",
		[]string{"waist", "hip", "thigh", "inseam"},
		[]string{"knee", "calf", "ankle", "rise", "outseam"}, 40},
	{"dress", "Dress", "womenswear", "A dress shaped to your figure.", 90000, true, "feminine",
		[]string{"shoulder", "bust", "waist", "hip", "dress_length"},
		[]string{"underbust", "high_hip", "torso_length", "sleeve_length", "bicep", "wrist"}, 50},
	{"gown", "Gown", "womenswear", "An evening or bridal gown.", 250000, true, "feminine",
		[]string{"shoulder", "bust", "underbust", "waist", "hip", "torso_length", "shoulder_to_floor"},
		[]string{"high_hip", "sleeve_length", "bicep", "wrist", "dress_length"}, 60},
	{"traditional", "Traditional wear", "traditional", "Traditional and ceremonial garments made with the care of everyday tailoring.", 80000, false, "any",
		[]string{"shoulder", "chest", "waist", "hip"},
		[]string{"neck", "bust", "sleeve_length", "shirt_length", "dress_length", "inseam"}, 70},
	{"uniform", "Uniform", "other", "Uniforms for teams, schools and staff.", 30000, false, "any",
		[]string{"chest", "waist", "hip"}, []string{"neck", "shoulder", "sleeve_length", "inseam"}, 80},
	{"alteration", "Alteration", "services", "Adjustments to garments you already own.", 5000, false, "any",
		[]string{}, []string{"waist", "hip", "sleeve_length", "inseam", "chest"}, 90},
	{"other", "Something else", "other", "Tell us what you have in mind.", 0, false, "any", []string{}, []string{"chest", "waist", "hip"}, 100},
}

type optVal struct {
	key, name, desc string
	price           int64
	parts           map[string]any
	adjust          map[string]int
	def             bool
}

type optGroup struct {
	key, name, section, selection string
	required                      bool
	min, max, step, def           *float64
	unit                          *string
	values                        []optVal
}

func f(v float64) *float64 { return &v }
func s(v string) *string   { return &v }

func show(on []string, off []string) map[string]any { return map[string]any{"show": on, "hide": off} }

var lapels = []string{"lapel_notch", "lapel_peak", "lapel_shawl"}
var buttons = []string{"buttons_1", "buttons_2", "buttons_3", "buttons_db"}
var pockets = []string{"pocket_flap", "pocket_jetted", "pocket_patch"}

func except(all []string, keep ...string) []string {
	var out []string
	for _, a := range all {
		skip := false
		for _, k := range keep {
			skip = skip || a == k
		}
		if !skip {
			out = append(out, a)
		}
	}
	return out
}

func jacketGroups(includeLining bool) []optGroup {
	g := []optGroup{
		{key: "construction", name: "Construction", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "single_breasted", name: "Single breasted", desc: "The classic choice for business and everyday wear.", def: true},
			{key: "double_breasted", name: "Double breasted", desc: "Overlapping front with two columns of buttons.", price: 15000, parts: show([]string{"buttons_db"}, []string{"buttons_1", "buttons_2", "buttons_3"})},
		}},
		{key: "buttons", name: "Front buttons", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "one", name: "One button", parts: show([]string{"buttons_1"}, except(buttons, "buttons_1"))},
			{key: "two", name: "Two buttons", def: true, parts: show([]string{"buttons_2"}, except(buttons, "buttons_2"))},
			{key: "three", name: "Three buttons", parts: show([]string{"buttons_3"}, except(buttons, "buttons_3"))},
		}},
		{key: "lapel", name: "Lapel", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "notch", name: "Notch lapel", desc: "Versatile and understated.", def: true, parts: show([]string{"lapel_notch"}, except(lapels, "lapel_notch"))},
			{key: "peak", name: "Peak lapel", desc: "Sharper and more formal. A favourite for weddings.", price: 5000, parts: show([]string{"lapel_peak"}, except(lapels, "lapel_peak"))},
			{key: "shawl", name: "Shawl lapel", desc: "A rounded lapel for evening and dinner jackets.", price: 5000, parts: show([]string{"lapel_shawl"}, except(lapels, "lapel_shawl"))},
		}},
		{key: "lapel_width", name: "Lapel width", section: "jacket", selection: "number", required: false, min: f(6), max: f(11), step: f(0.5), def: f(8), unit: s("cm")},
		{key: "jacket_length", name: "Jacket length", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "short", name: "Short", desc: "A modern, slightly cropped length.", adjust: map[string]int{"jacket_length": -20}},
			{key: "regular", name: "Regular", desc: "Covers the seat.", def: true},
			{key: "long", name: "Long", desc: "A traditional, longer cut.", adjust: map[string]int{"jacket_length": 20}},
		}},
		{key: "vent", name: "Vent", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "none", name: "No vent"}, {key: "single", name: "Centre vent"}, {key: "double", name: "Side vents", def: true},
		}},
		{key: "pockets", name: "Hip pockets", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "flap", name: "Flap pockets", def: true, parts: show([]string{"pocket_flap"}, except(pockets, "pocket_flap"))},
			{key: "jetted", name: "Jetted pockets", desc: "Clean and formal.", parts: show([]string{"pocket_jetted"}, except(pockets, "pocket_jetted"))},
			{key: "patch", name: "Patch pockets", desc: "Relaxed and casual.", parts: show([]string{"pocket_patch"}, except(pockets, "pocket_patch"))},
		}},
		{key: "chest_pocket", name: "Chest pocket", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "welt", name: "Welt pocket", def: true, parts: show([]string{"chest_pocket"}, nil)},
			{key: "none", name: "No chest pocket", parts: show(nil, []string{"chest_pocket"})},
		}},
		{key: "sleeve_buttons", name: "Sleeve buttons", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "three", name: "Three buttons"}, {key: "four", name: "Four buttons", def: true},
			{key: "four_working", name: "Four working buttons", desc: "Functional buttonholes, a mark of handwork.", price: 8000},
		}},
	}
	if includeLining {
		g = append(g, optGroup{key: "lining", name: "Lining", section: "jacket", selection: "single", required: true, values: []optVal{
			{key: "full", name: "Full lining", def: true}, {key: "half", name: "Half lining", desc: "Lighter for warm weather."},
			{key: "contrast", name: "Contrast lining", desc: "A coloured lining of your choice.", price: 6000},
		}})
	}
	return g
}

func trouserGroups(section string) []optGroup {
	return []optGroup{
		{key: "pleats", name: "Front", section: section, selection: "single", required: true, values: []optVal{
			{key: "flat", name: "Flat front", def: true}, {key: "single", name: "Single pleat"}, {key: "double", name: "Double pleat"},
		}},
		{key: "leg", name: "Leg shape", section: section, selection: "single", required: true, values: []optVal{
			{key: "tapered", name: "Tapered", def: true, parts: map[string]any{"morphs": map[string]float64{"leg_wide": 0}}},
			{key: "straight", name: "Straight", parts: map[string]any{"morphs": map[string]float64{"leg_wide": 0.5}}},
			{key: "wide", name: "Wide", parts: map[string]any{"morphs": map[string]float64{"leg_wide": 1}}},
		}},
		{key: "hem", name: "Hem", section: section, selection: "single", required: true, values: []optVal{
			{key: "plain", name: "Plain hem", def: true, parts: show(nil, []string{"trouser_cuff"})},
			{key: "cuff", name: "Turn-up cuff", parts: show([]string{"trouser_cuff"}, nil)},
		}},
		{key: "waistband", name: "Waistband", section: section, selection: "single", required: true, values: []optVal{
			{key: "belt_loops", name: "Belt loops", def: true}, {key: "side_adjusters", name: "Side adjusters", price: 4000},
		}},
	}
}

var collars = []string{"collar_spread", "collar_button_down", "collar_mandarin"}

var shirtGroups = []optGroup{
	{key: "collar", name: "Collar", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "spread", name: "Spread collar", def: true, parts: show([]string{"collar_spread"}, except(collars, "collar_spread"))},
		{key: "button_down", name: "Button-down collar", parts: show([]string{"collar_button_down"}, except(collars, "collar_button_down"))},
		{key: "mandarin", name: "Mandarin collar", parts: show([]string{"collar_mandarin"}, except(collars, "collar_mandarin"))},
	}},
	{key: "sleeve", name: "Sleeve", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "long", name: "Long sleeve", def: true, parts: show([]string{"sleeve_long"}, []string{"sleeve_short"})},
		{key: "short", name: "Short sleeve", parts: show([]string{"sleeve_short"}, []string{"sleeve_long", "cuff_barrel", "cuff_french"})},
	}},
	{key: "cuff", name: "Cuff", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "barrel", name: "Barrel cuff", def: true, parts: show([]string{"cuff_barrel"}, []string{"cuff_french"})},
		{key: "french", name: "French cuff", desc: "Folded back, worn with cufflinks.", price: 3000, parts: show([]string{"cuff_french"}, []string{"cuff_barrel"})},
	}},
	{key: "placket", name: "Placket", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "standard", name: "Standard placket", def: true}, {key: "hidden", name: "Hidden placket"},
	}},
	{key: "pocket", name: "Pocket", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "none", name: "No pocket", def: true, parts: show(nil, []string{"shirt_pocket"})},
		{key: "left", name: "Left chest pocket", parts: show([]string{"shirt_pocket"}, nil)},
	}},
	{key: "fit", name: "Body", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "slim", name: "Slim body", adjust: map[string]int{"chest": -20, "waist": -30}},
		{key: "regular", name: "Regular body", def: true},
		{key: "relaxed", name: "Relaxed body", adjust: map[string]int{"chest": 30, "waist": 40}},
	}},
	{key: "hem_style", name: "Hem", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "curved", name: "Curved hem, worn tucked", def: true}, {key: "straight", name: "Straight hem, worn untucked", adjust: map[string]int{"shirt_length": -60}},
	}},
	{key: "monogram", name: "Monogram", section: "shirt", selection: "single", required: true, values: []optVal{
		{key: "none", name: "No monogram", def: true}, {key: "cuff", name: "On the cuff", price: 2500}, {key: "chest", name: "On the chest", price: 2500},
	}},
}

var skirts = []string{"skirt_a_line", "skirt_pencil", "skirt_ballgown"}
var sleevesD = []string{"sleeve_none", "sleeve_cap", "sleeve_long"}
var necklines = []string{"neckline_round", "neckline_v", "neckline_sweetheart"}

func dressGroups(gown bool) []optGroup {
	lengthVals := []optVal{
		{key: "knee", name: "Knee length", def: !gown, parts: map[string]any{"morphs": map[string]float64{"length_midi": 0, "length_floor": 0}}, adjust: map[string]int{"dress_length": -150}},
		{key: "midi", name: "Midi", parts: map[string]any{"morphs": map[string]float64{"length_midi": 1, "length_floor": 0}}},
		{key: "floor", name: "Floor length", def: gown, parts: map[string]any{"morphs": map[string]float64{"length_midi": 0, "length_floor": 1}}, adjust: map[string]int{"dress_length": 350}},
	}
	g := []optGroup{
		{key: "neckline", name: "Neckline", section: "bodice", selection: "single", required: true, values: []optVal{
			{key: "round", name: "Round neck", def: true, parts: show([]string{"neckline_round"}, except(necklines, "neckline_round"))},
			{key: "v", name: "V neck", parts: show([]string{"neckline_v"}, except(necklines, "neckline_v"))},
			{key: "sweetheart", name: "Sweetheart", parts: show([]string{"neckline_sweetheart"}, except(necklines, "neckline_sweetheart"))},
		}},
		{key: "sleeve", name: "Sleeve", section: "bodice", selection: "single", required: true, values: []optVal{
			{key: "sleeveless", name: "Sleeveless", def: true, parts: show([]string{"sleeve_none"}, except(sleevesD, "sleeve_none"))},
			{key: "cap", name: "Cap sleeve", parts: show([]string{"sleeve_cap"}, except(sleevesD, "sleeve_cap"))},
			{key: "long", name: "Long sleeve", price: 8000, parts: show([]string{"sleeve_long"}, except(sleevesD, "sleeve_long"))},
		}},
		{key: "bodice_fit", name: "Bodice fit", section: "bodice", selection: "single", required: true, values: []optVal{
			{key: "fitted", name: "Fitted", def: true}, {key: "corseted", name: "Corseted", desc: "Structured with boning.", price: 20000},
			{key: "relaxed", name: "Relaxed", adjust: map[string]int{"bust": 30, "waist": 40}},
		}},
		{key: "waist_shape", name: "Waist", section: "bodice", selection: "single", required: true, values: []optVal{
			{key: "natural", name: "Natural waist", def: true}, {key: "empire", name: "Empire waist"}, {key: "drop", name: "Dropped waist"},
		}},
		{key: "skirt", name: "Skirt shape", section: "skirt", selection: "single", required: true, values: []optVal{
			{key: "a_line", name: "A-line", def: !gown, parts: show([]string{"skirt_a_line"}, except(skirts, "skirt_a_line"))},
			{key: "pencil", name: "Pencil", parts: show([]string{"skirt_pencil"}, except(skirts, "skirt_pencil"))},
			{key: "ballgown", name: "Ball gown", def: gown, price: 25000, parts: show([]string{"skirt_ballgown"}, except(skirts, "skirt_ballgown"))},
		}},
		{key: "length", name: "Length", section: "skirt", selection: "single", required: true, values: lengthVals},
		{key: "slit", name: "Slit", section: "skirt", selection: "single", required: true, values: []optVal{
			{key: "none", name: "No slit", def: true}, {key: "side", name: "Side slit"}, {key: "back", name: "Back slit"},
		}},
		{key: "back", name: "Back", section: "bodice", selection: "single", required: true, values: []optVal{
			{key: "closed", name: "Closed back", def: true}, {key: "low", name: "Low back"}, {key: "lace_up", name: "Lace-up back", price: 12000},
		}},
		{key: "lining", name: "Lining", section: "skirt", selection: "single", required: true, values: []optVal{
			{key: "full", name: "Fully lined", def: true}, {key: "unlined", name: "Unlined"},
		}},
		{key: "embellishment", name: "Embellishment", section: "finish", selection: "single", required: true, values: []optVal{
			{key: "none", name: "None", def: true}, {key: "beading", name: "Hand beading", price: 40000},
			{key: "embroidery", name: "Embroidery", price: 30000}, {key: "lace", name: "Lace overlay", price: 35000},
		}},
	}
	if gown {
		g = append(g, optGroup{key: "train", name: "Train", section: "skirt", selection: "single", required: true, values: []optVal{
			{key: "none", name: "No train", def: true}, {key: "sweep", name: "Sweep train", price: 15000}, {key: "chapel", name: "Chapel train", price: 30000},
		}})
	}
	return g
}

var studioGroups = map[string][]optGroup{
	"suit":     append(jacketGroups(true), trouserGroups("trousers")...),
	"jacket":   jacketGroups(true),
	"shirt":    shirtGroups,
	"trousers": trouserGroups("trousers"),
	"dress":    dressGroups(false),
	"gown":     dressGroups(true),
}

type rule struct {
	zone, label, key, kind      string
	slim, reg, relaxed, tol, st int
}

var fitRules = map[string][]rule{
	"suit": {
		{"chest", "Chest", "chest", "circumference", 60, 100, 140, 15, 0},
		{"waist", "Jacket waist", "waist", "circumference", 50, 90, 130, 15, 0},
		{"hip", "Seat", "hip", "circumference", 40, 70, 100, 15, 0},
		{"shoulders", "Shoulders", "shoulder", "length", 10, 15, 20, 8, 0},
		{"sleeve", "Sleeves", "sleeve_length", "length", 0, 0, 0, 8, 0},
		{"jacket_length", "Jacket length", "jacket_length", "length", 0, 0, 0, 10, 0},
		{"thigh", "Thigh", "thigh", "circumference", 30, 50, 70, 15, 0},
		{"inseam", "Trouser length", "inseam", "length", 0, 0, 0, 10, 0},
	},
	"jacket": {
		{"chest", "Chest", "chest", "circumference", 60, 100, 140, 15, 0},
		{"waist", "Waist", "waist", "circumference", 50, 90, 130, 15, 0},
		{"shoulders", "Shoulders", "shoulder", "length", 10, 15, 20, 8, 0},
		{"sleeve", "Sleeves", "sleeve_length", "length", 0, 0, 0, 8, 0},
		{"jacket_length", "Jacket length", "jacket_length", "length", 0, 0, 0, 10, 0},
	},
	"shirt": {
		{"neck", "Collar", "neck", "circumference", 10, 15, 20, 5, 0},
		{"chest", "Chest", "chest", "circumference", 60, 90, 130, 15, 0},
		{"waist", "Waist", "waist", "circumference", 40, 80, 120, 15, 0},
		{"sleeve", "Sleeves", "sleeve_length", "length", 0, 0, 0, 8, 0},
		{"shirt_length", "Shirt length", "shirt_length", "length", 0, 0, 0, 15, 0},
	},
	"trousers": {
		{"waist", "Waist", "waist", "circumference", 0, 10, 20, 10, 0},
		{"hip", "Seat", "hip", "circumference", 30, 60, 90, 15, 0},
		{"thigh", "Thigh", "thigh", "circumference", 30, 50, 70, 15, 0},
		{"inseam", "Length", "inseam", "length", 0, 0, 0, 10, 0},
	},
	"dress": {
		{"bust", "Bust", "bust", "circumference", 40, 70, 100, 12, 0},
		{"waist", "Waist", "waist", "circumference", 20, 50, 80, 12, 0},
		{"hip", "Hip", "hip", "circumference", 40, 70, 110, 15, 0},
		{"shoulders", "Shoulders", "shoulder", "length", 0, 5, 10, 8, 0},
		{"dress_length", "Length", "dress_length", "length", 0, 0, 0, 15, 0},
	},
	"gown": {
		{"bust", "Bust", "bust", "circumference", 30, 60, 90, 10, 0},
		{"waist", "Waist", "waist", "circumference", 15, 40, 70, 10, 0},
		{"hip", "Hip", "hip", "circumference", 40, 80, 120, 15, 0},
		{"torso", "Bodice length", "torso_length", "length", 0, 0, 0, 10, 0},
	},
}

// Standard size baselines for ready-to-adjust patterns (finished garment dimensions, mm).
var sizes = map[string][]struct {
	label string
	dims  map[string]int
}{
	"suit": {
		{"46R", map[string]int{"chest": 1020, "waist": 920, "hip": 1020, "shoulders": 445, "sleeve": 620, "jacket_length": 740, "thigh": 640, "inseam": 800}},
		{"48R", map[string]int{"chest": 1060, "waist": 960, "hip": 1060, "shoulders": 455, "sleeve": 630, "jacket_length": 750, "thigh": 660, "inseam": 810}},
		{"50R", map[string]int{"chest": 1100, "waist": 1000, "hip": 1100, "shoulders": 465, "sleeve": 640, "jacket_length": 760, "thigh": 680, "inseam": 820}},
		{"52R", map[string]int{"chest": 1140, "waist": 1050, "hip": 1140, "shoulders": 475, "sleeve": 650, "jacket_length": 770, "thigh": 700, "inseam": 830}},
		{"54L", map[string]int{"chest": 1180, "waist": 1100, "hip": 1180, "shoulders": 485, "sleeve": 670, "jacket_length": 800, "thigh": 720, "inseam": 860}},
	},
	"shirt": {
		{"38", map[string]int{"neck": 395, "chest": 1000, "waist": 920, "sleeve": 630, "shirt_length": 780}},
		{"40", map[string]int{"neck": 415, "chest": 1060, "waist": 980, "sleeve": 640, "shirt_length": 790}},
		{"42", map[string]int{"neck": 435, "chest": 1120, "waist": 1050, "sleeve": 650, "shirt_length": 800}},
		{"44", map[string]int{"neck": 455, "chest": 1180, "waist": 1120, "sleeve": 655, "shirt_length": 810}},
	},
}

// Reference loads or refreshes reference data. It is idempotent and safe in production:
// it never overwrites owner-edited settings, content or availability.
func Reference(ctx context.Context, pool *pgxpool.Pool) error {
	return db.InTx(ctx, pool, func(tx pgx.Tx) error {
		for i, fl := range fields {
			if _, err := tx.Exec(ctx, `INSERT INTO measurement_fields (key, label, body_location, instruction, helper_note, diagram_key, kind, min_mm, max_mm, sort_order)
				VALUES ($1,$2,$3,$4,$5,$1,$6,$7,$8,$9) ON CONFLICT (key) DO NOTHING`,
				fl.key, fl.label, fl.location, fl.instruction, fl.helper, fl.kind, fl.min, fl.max, i*10); err != nil {
				return fmt.Errorf("field %s: %w", fl.key, err)
			}
		}
		for _, g := range garmentTypes {
			var id string
			err := tx.QueryRow(ctx, `INSERT INTO garment_types (key, name, category, description, base_price_minor, studio_enabled, body_model_hint, sort_order)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (key) DO NOTHING RETURNING id`,
				g.key, g.name, g.category, g.description, g.basePrice, g.studio, g.bodyHint, g.sort).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // already present; the owner may have edited it
			}
			if err != nil {
				return fmt.Errorf("garment %s: %w", g.key, err)
			}
			for i, k := range g.fields {
				if _, err := tx.Exec(ctx, `INSERT INTO garment_measurement_fields (garment_type_id, field_id, required, sort_order)
					SELECT $1, id, true, $3 FROM measurement_fields WHERE key=$2 ON CONFLICT DO NOTHING`, id, k, i*10); err != nil {
					return err
				}
			}
			for i, k := range g.optional {
				if _, err := tx.Exec(ctx, `INSERT INTO garment_measurement_fields (garment_type_id, field_id, required, sort_order)
					SELECT $1, id, false, $3 FROM measurement_fields WHERE key=$2 ON CONFLICT DO NOTHING`, id, k, 500+i*10); err != nil {
					return err
				}
			}
			for gi, og := range studioGroups[g.key] {
				var gid string
				if err := tx.QueryRow(ctx, `INSERT INTO option_groups (garment_type_id, key, name, section, selection, required, min_value, max_value, step_value, default_number, unit, sort_order)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`, id, og.key, og.name, og.section, og.selection, og.required,
					og.min, og.max, og.step, og.def, og.unit, gi*10).Scan(&gid); err != nil {
					return fmt.Errorf("group %s/%s: %w", g.key, og.key, err)
				}
				for vi, v := range og.values {
					parts := v.parts
					if parts == nil {
						parts = map[string]any{}
					}
					adj := v.adjust
					if adj == nil {
						adj = map[string]int{}
					}
					pj, _ := json.Marshal(parts)
					if _, err := tx.Exec(ctx, `INSERT INTO option_values (group_id, key, name, description, price_minor, asset_parts, adjustments, is_default, sort_order)
						VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, gid, v.key, v.name, v.desc, v.price, pj, adj, v.def, vi*10); err != nil {
						return fmt.Errorf("value %s/%s/%s: %w", g.key, og.key, v.key, err)
					}
				}
			}
			for _, r := range fitRules[g.key] {
				if _, err := tx.Exec(ctx, `INSERT INTO fit_rules (garment_type_id, zone, label, measurement_key, kind, ease_slim_mm, ease_regular_mm, ease_relaxed_mm, tolerance_mm, stretch_pct)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, r.zone, r.label, r.key, r.kind, r.slim, r.reg, r.relaxed, r.tol, r.st); err != nil {
					return err
				}
			}
			for i, sz := range sizes[g.key] {
				if _, err := tx.Exec(ctx, `INSERT INTO garment_sizes (garment_type_id, label, dims, sort_order) VALUES ($1,$2,$3,$4)`, id, sz.label, sz.dims, i); err != nil {
					return err
				}
			}
		}
		if err := seedAssets(ctx, tx); err != nil {
			return err
		}
		for i, c := range []struct{ slug, name string }{{"suits", "Suits"}, {"jackets", "Jackets"}, {"shirts", "Shirts"}, {"trousers", "Trousers"},
			{"dresses", "Dresses"}, {"traditional", "Traditional"}, {"accessories", "Accessories"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO product_categories (slug, name, sort_order) VALUES ($1,$2,$3) ON CONFLICT (slug) DO NOTHING`, c.slug, c.name, i*10); err != nil {
				return err
			}
		}
		// Defaults only when the owner has not configured them yet.
		biz := settings.DefaultBusiness()
		bj, _ := json.Marshal(biz)
		if _, err := tx.Exec(ctx, `INSERT INTO business_settings (key, value) VALUES ('business', $1) ON CONFLICT DO NOTHING`, bj); err != nil {
			return err
		}
		var rules int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM availability_rules`).Scan(&rules); err != nil {
			return err
		}
		if rules == 0 {
			// Monday to Friday 09:00 to 18:00, Saturday 10:00 to 16:00. The owner adjusts these in the dashboard.
			for wd := 1; wd <= 5; wd++ {
				if _, err := tx.Exec(ctx, `INSERT INTO availability_rules (weekday, start_minute, end_minute) VALUES ($1, 540, 1080)`, wd); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO availability_rules (weekday, start_minute, end_minute) VALUES (6, 600, 960)`); err != nil {
				return err
			}
		}
		for key, val := range defaultContent {
			vj, _ := json.Marshal(val)
			if _, err := tx.Exec(ctx, `INSERT INTO content_blocks (key, value) VALUES ($1,$2) ON CONFLICT DO NOTHING`, key, vj); err != nil {
				return err
			}
		}
		return nil
	})
}

// BootstrapOwner creates the first owner account. It refuses to run if an owner already exists.
func BootstrapOwner(ctx context.Context, pool *pgxpool.Pool, email, password, name string) error {
	if email == "" || password == "" {
		return errors.New("set BOOTSTRAP_OWNER_EMAIL and BOOTSTRAP_OWNER_PASSWORD")
	}
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	var owners int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='owner' AND deleted_at IS NULL`).Scan(&owners); err != nil {
		return err
	}
	if owners > 0 {
		return errors.New("an owner account already exists; manage staff from the dashboard")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `INSERT INTO users (email, password_hash, full_name, role) VALUES (lower($1), $2, $3, 'owner')`, email, hash, name)
	return err
}
