package phone

import "testing"

func TestNormalize(t *testing.T) {
	ok := map[string]string{
		"677 12 34 56":     "237677123456",
		"+237 677-123-456": "237677123456",
		"00237677123456":   "237677123456",
		"237677123456":     "237677123456",
		"+44 7700 900123":  "447700900123",
	}
	for in, want := range ok {
		got, err := Normalize(in, "237")
		if err != nil || got != want {
			t.Errorf("Normalize(%q)=%q,%v want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "12", "6771234<script>"} {
		if _, err := Normalize(bad, "237"); err == nil {
			t.Errorf("Normalize(%q) expected error", bad)
		}
	}
}
