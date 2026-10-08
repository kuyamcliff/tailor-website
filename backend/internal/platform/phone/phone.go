// Package phone normalizes phone numbers to E.164 digits without a plus sign.
package phone

import (
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid phone number")

// Normalize strips formatting and applies a default country calling code to local numbers.
// For Cameroon (237) local mobile numbers have 9 digits starting with 6.
func Normalize(raw, defaultCC string) (string, error) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", ErrInvalid
		}
	}
	d := b.String()
	d = strings.TrimPrefix(d, "00")
	if defaultCC != "" && !strings.HasPrefix(d, defaultCC) && len(d) <= 10 {
		d = defaultCC + strings.TrimPrefix(d, "0")
	}
	if len(d) < 8 || len(d) > 15 {
		return "", ErrInvalid
	}
	return d, nil
}

// Mask hides all but the last three digits, for logs and staff views that do not need the full number.
func Mask(d string) string {
	if len(d) <= 3 {
		return "***"
	}
	return strings.Repeat("•", len(d)-3) + d[len(d)-3:]
}
