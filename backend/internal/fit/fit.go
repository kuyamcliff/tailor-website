// Package fit estimates garment fit per zone from body measurements, data-driven fit rules,
// a pattern baseline and design adjustments. It never claims an exact physical fit: results are
// estimates, and estimated or flagged measurements always require tailor verification.
//
// The same algorithm is implemented in frontend/lib/fit.ts; both are tested against
// backend/internal/fit/testdata/fixtures.json to keep them identical.
package fit

import (
	"fmt"
	"math"
)

type Rule struct {
	Zone           string `json:"zone"`
	Label          string `json:"label"`
	MeasurementKey string `json:"measurementKey"`
	Kind           string `json:"kind"` // circumference or length
	EaseSlimMM     int    `json:"easeSlimMm"`
	EaseRegularMM  int    `json:"easeRegularMm"`
	EaseRelaxedMM  int    `json:"easeRelaxedMm"`
	ToleranceMM    int    `json:"toleranceMm"`
	StretchPct     int    `json:"stretchPct"`
}

func (r Rule) ease(pref string) int {
	switch pref {
	case "slim":
		return r.EaseSlimMM
	case "relaxed":
		return r.EaseRelaxedMM
	default:
		return r.EaseRegularMM
	}
}

type State string

const (
	Good             State = "good"
	Close            State = "close"
	Tight            State = "tight"
	Loose            State = "loose"
	InsufficientData State = "insufficient_data"
	VerifyRequired   State = "verify"
)

type Input struct {
	Rules         []Rule         `json:"rules"`
	Body          map[string]int `json:"body"`          // measurement key -> mm
	FitPreference string         `json:"fitPreference"` // slim, regular, relaxed
	Baseline      map[string]int `json:"baseline"`      // zone -> finished garment mm; nil means made to measure
	Adjustments   map[string]int `json:"adjustments"`   // zone -> mm from selected design options
	Estimated     bool           `json:"estimated"`     // measurements came from estimation
	FlaggedKeys   []string       `json:"flaggedKeys"`   // measurements with review flags
}

type ZoneResult struct {
	Zone    string `json:"zone"`
	Label   string `json:"label"`
	State   State  `json:"state"`
	DeltaMM *int   `json:"deltaMm"`
	Message string `json:"message"`
}

type Result struct {
	Zones   []ZoneResult `json:"zones"`
	Overall State        `json:"overall"`
	Summary string       `json:"summary"`
}

// Estimate computes per-zone fit. For made-to-measure designs the garment dimension is the body
// plus the preferred ease, so only design adjustments move a zone away from Good.
func Estimate(in Input) Result {
	flagged := map[string]bool{}
	for _, k := range in.FlaggedKeys {
		flagged[k] = true
	}
	res := Result{Zones: make([]ZoneResult, 0, len(in.Rules))}
	counts := map[State]int{}
	for _, rule := range in.Rules {
		z := ZoneResult{Zone: rule.Zone, Label: rule.Label}
		body, ok := in.Body[rule.MeasurementKey]
		if !ok || body <= 0 {
			z.State = InsufficientData
			z.Message = "Add your " + humanKey(rule.MeasurementKey) + " measurement"
			res.Zones = append(res.Zones, z)
			counts[z.State]++
			continue
		}
		target := body + rule.ease(in.FitPreference)
		dim := target
		if in.Baseline != nil {
			if b, ok := in.Baseline[rule.Zone]; ok {
				dim = b
			} else {
				z.State = InsufficientData
				z.Message = "This size has no reference dimension for this area"
				res.Zones = append(res.Zones, z)
				counts[z.State]++
				continue
			}
		}
		dim += in.Adjustments[rule.Zone]
		delta := dim - target
		// Stretch fabrics tolerate some negative ease in circumference zones.
		if rule.Kind == "circumference" && delta < 0 && rule.StretchPct > 0 {
			give := int(math.Round(float64(body) * float64(rule.StretchPct) / 100))
			delta = min(0, delta+give)
		}
		d := delta
		z.DeltaMM = &d
		tol := max(rule.ToleranceMM, 1)
		abs := delta
		if abs < 0 {
			abs = -abs
		}
		switch {
		case abs <= tol:
			z.State, z.Message = Good, "Good"
		case abs <= 2*tol:
			z.State = Close
			z.Message = describe(rule.Kind, delta, true)
		case delta < 0:
			z.State = Tight
			z.Message = describe(rule.Kind, delta, false)
		default:
			z.State = Loose
			z.Message = describe(rule.Kind, delta, false)
		}
		if in.Estimated || flagged[rule.MeasurementKey] {
			z.State = VerifyRequired
			z.Message += ". Tailor verification required"
		}
		counts[z.State]++
		res.Zones = append(res.Zones, z)
	}
	switch {
	case len(in.Rules) == 0:
		res.Overall, res.Summary = InsufficientData, "No fit rules are defined for this garment yet"
	case counts[VerifyRequired] > 0:
		res.Overall, res.Summary = VerifyRequired, "Your tailor will verify these measurements before cutting"
	case counts[InsufficientData] == len(in.Rules):
		res.Overall, res.Summary = InsufficientData, "Add measurements to see a fit estimate"
	case counts[Tight] > 0 || counts[Loose] > 0:
		res.Overall, res.Summary = Tight, "Some areas need attention"
		if counts[Tight] == 0 {
			res.Overall = Loose
		}
	case counts[Close] > 0:
		res.Overall, res.Summary = Close, "Close fit with small differences"
	case counts[InsufficientData] > 0:
		res.Overall, res.Summary = InsufficientData, "Some measurements are missing"
	default:
		res.Overall, res.Summary = Good, "Estimated good fit across measured areas"
	}
	return res
}

func describe(kind string, delta int, slight bool) string {
	cm := math.Abs(float64(delta)) / 10
	if kind == "length" {
		dir := "long"
		if delta < 0 {
			dir = "short"
		}
		return fmt.Sprintf("%.1f cm %s", cm, dir)
	}
	word := "loose"
	if delta < 0 {
		word = "tight"
	}
	if slight {
		return "Slightly " + word
	}
	return fmt.Sprintf("%s by %.1f cm", capitalize(word), cm)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}

func humanKey(k string) string {
	b := []byte(k)
	for i := range b {
		if b[i] == '_' {
			b[i] = ' '
		}
	}
	return string(b)
}
