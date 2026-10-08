// Package money handles currency metadata and integer minor-unit arithmetic.
// Amounts are always int64 minor units; floating point is never used for money.
package money

import (
	"errors"
	"fmt"
	"strings"
)

type Currency struct {
	Code     string `json:"code"`
	Exponent int    `json:"exponent"` // number of minor-unit digits
	Symbol   string `json:"symbol"`
}

var currencies = map[string]Currency{
	"XAF": {Code: "XAF", Exponent: 0, Symbol: "FCFA"},
	"XOF": {Code: "XOF", Exponent: 0, Symbol: "CFA"},
	"NGN": {Code: "NGN", Exponent: 2, Symbol: "₦"},
	"GHS": {Code: "GHS", Exponent: 2, Symbol: "GH₵"},
	"EUR": {Code: "EUR", Exponent: 2, Symbol: "€"},
	"USD": {Code: "USD", Exponent: 2, Symbol: "$"},
	"GBP": {Code: "GBP", Exponent: 2, Symbol: "£"},
}

func Lookup(code string) (Currency, error) {
	c, ok := currencies[strings.ToUpper(code)]
	if !ok {
		return Currency{}, fmt.Errorf("unsupported currency %q", code)
	}
	return c, nil
}

var ErrOverflow = errors.New("money: overflow")

// Add returns a+b, failing on int64 overflow.
func Add(a, b int64) (int64, error) {
	s := a + b
	if (b > 0 && s < a) || (b < 0 && s > a) {
		return 0, ErrOverflow
	}
	return s, nil
}

// Mul returns a*n, failing on overflow.
func Mul(a int64, n int64) (int64, error) {
	if a == 0 || n == 0 {
		return 0, nil
	}
	p := a * n
	if p/n != a {
		return 0, ErrOverflow
	}
	return p, nil
}

// ApplyBasisPoints returns round_half_up(amount * bp / 10000). 1 bp = 0.01%.
func ApplyBasisPoints(amount int64, bp int64) int64 {
	if amount < 0 {
		return -ApplyBasisPoints(-amount, bp)
	}
	return (amount*bp + 5000) / 10000
}

// Format renders an amount for logs and plain-text documents. The frontend uses Intl for display.
func Format(amount int64, code string) string {
	c, err := Lookup(code)
	if err != nil {
		return fmt.Sprintf("%d %s", amount, code)
	}
	neg := amount < 0
	if neg {
		amount = -amount
	}
	div := int64(1)
	for i := 0; i < c.Exponent; i++ {
		div *= 10
	}
	whole := amount / div
	frac := amount % div
	ws := groupThousands(fmt.Sprint(whole))
	s := ws
	if c.Exponent > 0 {
		s = fmt.Sprintf("%s.%0*d", ws, c.Exponent, frac)
	}
	if neg {
		s = "-" + s
	}
	return s + " " + c.Code
}

func groupThousands(s string) string {
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
