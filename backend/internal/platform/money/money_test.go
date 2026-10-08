package money

import "testing"

func TestApplyBasisPoints(t *testing.T) {
	cases := []struct{ amt, bp, want int64 }{
		{10000, 1925, 1925}, {999, 5000, 500}, {1, 5000, 1}, {0, 1925, 0}, {-999, 5000, -500}, {150000, 3000, 45000},
	}
	for _, c := range cases {
		if got := ApplyBasisPoints(c.amt, c.bp); got != c.want {
			t.Errorf("ApplyBasisPoints(%d,%d)=%d want %d", c.amt, c.bp, got, c.want)
		}
	}
}

func TestOverflow(t *testing.T) {
	if _, err := Add(1<<62, 1<<62); err == nil {
		t.Fatal("expected overflow")
	}
	if _, err := Mul(1<<62, 4); err == nil {
		t.Fatal("expected overflow")
	}
	if v, err := Mul(2500, 3); err != nil || v != 7500 {
		t.Fatalf("Mul got %d %v", v, err)
	}
}

func TestFormat(t *testing.T) {
	if got := Format(1250000, "XAF"); got != "1 250 000 XAF" {
		t.Errorf("got %q", got)
	}
	if got := Format(123456, "EUR"); got != "1 234.56 EUR" {
		t.Errorf("got %q", got)
	}
}
