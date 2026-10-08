// Package measurements manages measurement fields per garment type, customer measurement
// profiles and immutable measurement versions, with unit conversion and validation.
package measurements

import (
	"fmt"
	"math"
	"strings"
)

type Field struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	BodyLocation string  `json:"bodyLocation"`
	Instruction  string  `json:"instruction"`
	HelperNote   string  `json:"helperNote"`
	DiagramKey   *string `json:"diagramKey"`
	Kind         string  `json:"kind"`
	MinMM        int     `json:"minMm"`
	MaxMM        int     `json:"maxMm"`
	Required     bool    `json:"required"`
	SortOrder    int     `json:"sortOrder"`
}

const (
	MinHeightMM = 1200
	MaxHeightMM = 2300
)

// ToMM converts an entered value to integer millimetres. Centimetres allow one decimal place;
// inches allow quarter-inch precision. Values with more precision are rejected rather than silently rounded.
func ToMM(v float64, unit string) (int, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0, fmt.Errorf("enter a positive number")
	}
	switch unit {
	case "cm":
		if !nearMultiple(v, 0.1) {
			return 0, fmt.Errorf("use at most one decimal place")
		}
		return int(math.Round(v * 10)), nil
	case "in":
		if !nearMultiple(v, 0.25) {
			return 0, fmt.Errorf("use quarter-inch steps, for example 15.25")
		}
		return int(math.Round(v * 25.4)), nil
	}
	return 0, fmt.Errorf("unknown unit")
}

// FromMM converts millimetres to the display unit with the unit's precision.
func FromMM(mm int, unit string) float64 {
	if unit == "in" {
		return math.Round(float64(mm)/25.4*4) / 4
	}
	return math.Round(float64(mm)) / 10
}

func nearMultiple(v, step float64) bool {
	q := v / step
	return math.Abs(q-math.Round(q)) < 1e-6
}

type Flag struct {
	Keys    []string `json:"keys"`
	Message string   `json:"message"`
}

type Validated struct {
	ValuesMM map[string]int     `json:"valuesMm"`
	Entered  map[string]float64 `json:"entered"`
	HeightMM *int               `json:"heightMm"`
	Flags    []Flag             `json:"flags"`
	Errors   map[string]string  `json:"errors"`
	Missing  []string           `json:"missing"`
}

func (v Validated) FlaggedKeys() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range v.Flags {
		for _, k := range f.Keys {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// Validate converts and range-checks entered values against the field definitions.
// Values for keys that are not fields of this garment are ignored (they may belong to another garment),
// and required fields without a value are reported in Missing when requireAll is set.
func Validate(fields []Field, entered map[string]float64, unit string, height *float64, requireAll bool) Validated {
	out := Validated{ValuesMM: map[string]int{}, Entered: map[string]float64{}, Errors: map[string]string{}, Flags: []Flag{}, Missing: []string{}}
	if unit != "cm" && unit != "in" {
		out.Errors["unit"] = "Choose centimetres or inches."
		return out
	}
	if height != nil {
		mm, err := ToMM(*height, unit)
		switch {
		case err != nil:
			out.Errors["height"] = "Height: " + err.Error() + "."
		case mm < MinHeightMM || mm > MaxHeightMM:
			out.Errors["height"] = fmt.Sprintf("Height must be between %s and %s.", display(MinHeightMM, unit), display(MaxHeightMM, unit))
		default:
			out.HeightMM = &mm
		}
	}
	for _, f := range fields {
		v, ok := entered[f.Key]
		if !ok {
			if f.Required && requireAll {
				out.Missing = append(out.Missing, f.Key)
			}
			continue
		}
		mm, err := ToMM(v, unit)
		if err != nil {
			out.Errors[f.Key] = f.Label + ": " + err.Error() + "."
			continue
		}
		if mm < f.MinMM || mm > f.MaxMM {
			out.Errors[f.Key] = fmt.Sprintf("%s must be between %s and %s.", f.Label, display(f.MinMM, unit), display(f.MaxMM, unit))
			continue
		}
		out.ValuesMM[f.Key] = mm
		out.Entered[f.Key] = v
	}
	out.Flags = ReviewFlags(out.ValuesMM, out.HeightMM)
	return out
}

func display(mm int, unit string) string {
	v := FromMM(mm, unit)
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	return s + " " + unit
}

// ReviewFlags detects combinations that are possible but unusual enough that the tailor
// should double-check them. Flags never block saving.
func ReviewFlags(v map[string]int, height *int) []Flag {
	flags := []Flag{}
	ratio := func(a, b string, lo, hi float64, msg string) {
		x, okA := v[a]
		y, okB := v[b]
		if !okA || !okB || y == 0 {
			return
		}
		r := float64(x) / float64(y)
		if r < lo || r > hi {
			flags = append(flags, Flag{Keys: []string{a, b}, Message: msg})
		}
	}
	ratio("waist", "chest", 0.55, 1.35, "Waist and chest look unusual together. Please re-measure both.")
	ratio("hip", "waist", 0.8, 1.9, "Hip and waist look unusual together. Please re-measure both.")
	ratio("neck", "chest", 0.28, 0.55, "Neck looks unusual for this chest size.")
	ratio("bicep", "chest", 0.2, 0.5, "Bicep looks unusual for this chest size.")
	ratio("wrist", "bicep", 0.4, 0.9, "Wrist looks unusual for this bicep size.")
	ratio("underbust", "bust", 0.65, 0.98, "Underbust should be smaller than bust. Please check.")
	ratio("high_hip", "full_hip", 0.75, 1.02, "High hip should be close to or smaller than full hip.")
	ratio("knee", "thigh", 0.45, 0.95, "Knee looks unusual for this thigh size.")
	if o, okO := v["outseam"]; okO {
		if i, okI := v["inseam"]; okI && i >= o {
			flags = append(flags, Flag{Keys: []string{"inseam", "outseam"}, Message: "Inseam should be shorter than outseam."})
		}
	}
	if height != nil {
		h := float64(*height)
		lengthLimits := []struct {
			key string
			lim float64
		}{{"inseam", 0.56}, {"outseam", 0.72}, {"sleeve_length", 0.42}, {"jacket_length", 0.5},
			{"dress_length", 0.9}, {"shoulder_to_floor", 0.88}, {"shirt_length", 0.5}}
		for _, l := range lengthLimits {
			if x, ok := v[l.key]; ok && float64(x) > h*l.lim {
				flags = append(flags, Flag{Keys: []string{l.key}, Message: "This length looks long for your height. Please check it."})
			}
		}
	}
	return flags
}
