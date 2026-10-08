package measurements

import "testing"

func f(key string, min, max int, req bool) Field {
	return Field{Key: key, Label: key, MinMM: min, MaxMM: max, Required: req}
}

func TestToMM(t *testing.T) {
	cases := []struct {
		v    float64
		unit string
		want int
		err  bool
	}{
		{100, "cm", 1000, false}, {100.5, "cm", 1005, false}, {100.55, "cm", 0, true},
		{15.25, "in", 387, false}, {15.3, "in", 0, true}, {-1, "cm", 0, true}, {0, "cm", 0, true}, {10, "mm", 0, true},
	}
	for _, c := range cases {
		got, err := ToMM(c.v, c.unit)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("ToMM(%v,%s)=%d,%v", c.v, c.unit, got, err)
		}
	}
}

func TestFromMMRoundTripInches(t *testing.T) {
	for _, v := range []float64{14.5, 15.25, 38, 40.75} {
		mm, err := ToMM(v, "in")
		if err != nil {
			t.Fatal(err)
		}
		if back := FromMM(mm, "in"); back != v {
			t.Errorf("round trip %v -> %d -> %v", v, mm, back)
		}
	}
}

func TestValidate(t *testing.T) {
	fields := []Field{f("chest", 600, 1600, true), f("waist", 500, 1600, true), f("neck", 280, 600, false)}
	h := 180.0
	v := Validate(fields, map[string]float64{"chest": 100, "waist": 2000, "unrelated": 5}, "cm", &h, true)
	if v.ValuesMM["chest"] != 1000 {
		t.Errorf("chest %d", v.ValuesMM["chest"])
	}
	if _, ok := v.Errors["waist"]; !ok {
		t.Error("expected waist range error")
	}
	if _, ok := v.ValuesMM["unrelated"]; ok {
		t.Error("unrelated key must be ignored")
	}
	if v.HeightMM == nil || *v.HeightMM != 1800 {
		t.Error("height")
	}
	missing := Validate(fields, map[string]float64{"chest": 100}, "cm", nil, true)
	if len(missing.Missing) != 1 || missing.Missing[0] != "waist" {
		t.Errorf("missing %v", missing.Missing)
	}
	badH := 90.0
	if e := Validate(fields, nil, "cm", &badH, false); e.Errors["height"] == "" {
		t.Error("expected height error")
	}
}

func TestReviewFlags(t *testing.T) {
	flags := ReviewFlags(map[string]int{"chest": 1000, "waist": 1500, "inseam": 900, "outseam": 850}, nil)
	if len(flags) != 2 {
		t.Fatalf("flags %v", flags)
	}
	h := 1700
	if fl := ReviewFlags(map[string]int{"sleeve_length": 800}, &h); len(fl) != 1 {
		t.Fatalf("expected sleeve flag, got %v", fl)
	}
	if fl := ReviewFlags(map[string]int{"chest": 1000, "waist": 860, "neck": 400}, &h); len(fl) != 0 {
		t.Fatalf("expected no flags, got %v", fl)
	}
}
