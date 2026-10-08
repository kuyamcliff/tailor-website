package orders

import "testing"

func TestCheck(t *testing.T) {
	wf := []string{"submitted", "under_review", "awaiting_customer", "deposit_paid", "cutting", "sewing", "fitting", "alteration", "ready", "delivered", "completed"}
	cases := []struct {
		from, to            string
		allowed, back, note bool
	}{
		{"deposit_paid", "cutting", true, false, false},
		{"sewing", "cutting", true, true, true},
		{"fitting", "alteration", true, false, false},
		{"alteration", "fitting", true, true, true},
		{"cutting", "patterning", false, false, false}, // disabled in workflow
		{"cancelled", "sewing", false, false, false},
		{"completed", "sewing", false, false, false},
		{"completed", "refunded", true, false, true},
		{"delivered", "cancelled", false, false, false},
		{"sewing", "cancelled", true, false, true},
		{"sewing", "sewing", false, false, false},
		{"sewing", "bogus", false, false, false},
		{"ready", "completed", true, false, false},
	}
	for _, c := range cases {
		tr := Check(c.from, c.to, wf)
		if tr.Allowed != c.allowed || tr.Backwards != c.back || tr.NeedsNote != c.note {
			t.Errorf("%s->%s: %+v", c.from, c.to, tr)
		}
	}
}
