package fit

import (
	"encoding/json"
	"os"
	"testing"
)

type fixtureCase struct {
	Name   string `json:"name"`
	Rules  []Rule `json:"rules"`
	Input  Input  `json:"input"`
	Expect struct {
		Overall State                        `json:"overall"`
		Zones   map[string][]json.RawMessage `json:"zones"`
	} `json:"expect"`
}

type fixtures struct {
	Rules       []Rule        `json:"rules"`
	Cases       []fixtureCase `json:"cases"`
	StretchCase fixtureCase   `json:"stretchCase"`
}

func load(t *testing.T) fixtures {
	b, err := os.ReadFile("testdata/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func check(t *testing.T, c fixtureCase) {
	t.Helper()
	res := Estimate(c.Input)
	if res.Overall != c.Expect.Overall {
		t.Errorf("%s: overall %s want %s", c.Name, res.Overall, c.Expect.Overall)
	}
	byZone := map[string]ZoneResult{}
	for _, z := range res.Zones {
		byZone[z.Zone] = z
	}
	for zone, exp := range c.Expect.Zones {
		z, ok := byZone[zone]
		if !ok {
			t.Fatalf("%s: zone %s missing", c.Name, zone)
		}
		var state State
		_ = json.Unmarshal(exp[0], &state)
		if z.State != state {
			t.Errorf("%s/%s: state %s want %s (%s)", c.Name, zone, z.State, state, z.Message)
		}
		var delta *int
		_ = json.Unmarshal(exp[1], &delta)
		if (delta == nil) != (z.DeltaMM == nil) || (delta != nil && *delta != *z.DeltaMM) {
			t.Errorf("%s/%s: delta %v want %v", c.Name, zone, z.DeltaMM, delta)
		}
		if len(exp) > 2 {
			var msg string
			_ = json.Unmarshal(exp[2], &msg)
			if z.Message != msg {
				t.Errorf("%s/%s: message %q want %q", c.Name, zone, z.Message, msg)
			}
		}
	}
}

func TestFixtures(t *testing.T) {
	f := load(t)
	for _, c := range f.Cases {
		c.Input.Rules = f.Rules
		check(t, c)
	}
	sc := f.StretchCase
	sc.Name = "stretch"
	sc.Input.Rules = sc.Rules
	check(t, sc)
}

func TestNoRules(t *testing.T) {
	if r := Estimate(Input{}); r.Overall != InsufficientData {
		t.Fatalf("got %s", r.Overall)
	}
}
