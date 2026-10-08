// Package pricing implements the quote and order pricing engine with integer minor units:
//
//	base garment + fabric + complexity + customization + rush + alteration allowance + delivery - discount + tax = total
//
// Tax can be inclusive (already inside prices, reported for information) or exclusive (added on top).
package pricing

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kuyamcliff/tailor-website/backend/internal/platform/money"
)

var LineKinds = map[string]bool{
	"garment": true, "fabric": true, "complexity": true, "customization": true, "rush": true,
	"alteration_allowance": true, "delivery": true, "discount": true, "other": true,
}

type Line struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Quantity    int64  `json:"quantity"`
	UnitMinor   int64  `json:"unitMinor"`
	TotalMinor  int64  `json:"totalMinor"`
}

type Input struct {
	Lines            []Line
	TaxRateBP        int64 // 1925 = 19.25%
	PricesIncludeTax bool
	DepositBP        *int64 // percentage of total in basis points
	DepositMinor     *int64 // fixed deposit amount; takes precedence over DepositBP
}

type Totals struct {
	Lines         []Line `json:"lines"`
	SubtotalMinor int64  `json:"subtotalMinor"`
	DiscountMinor int64  `json:"discountMinor"`
	DeliveryMinor int64  `json:"deliveryMinor"`
	TaxMinor      int64  `json:"taxMinor"`
	TaxRateBP     int64  `json:"taxRateBp"`
	TaxInclusive  bool   `json:"taxInclusive"`
	TotalMinor    int64  `json:"totalMinor"`
	DepositMinor  int64  `json:"depositMinor"`
	BalanceMinor  int64  `json:"balanceMinor"`
}

type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }

// Calculate validates lines and computes totals. It never uses floating point.
func Calculate(in Input) (Totals, error) {
	t := Totals{TaxRateBP: in.TaxRateBP, TaxInclusive: in.PricesIncludeTax}
	if len(in.Lines) == 0 {
		return t, &FieldError{"lines", "Add at least one line."}
	}
	if len(in.Lines) > 50 {
		return t, &FieldError{"lines", "A quote can have at most 50 lines."}
	}
	if in.TaxRateBP < 0 || in.TaxRateBP > 5000 {
		return t, &FieldError{"taxRateBp", "Tax rate must be between 0 and 50 percent."}
	}
	for i, l := range in.Lines {
		field := fmt.Sprintf("lines.%d", i)
		l.Description = strings.TrimSpace(l.Description)
		if !LineKinds[l.Kind] {
			return t, &FieldError{field, "Unknown line type."}
		}
		if l.Description == "" || len(l.Description) > 300 {
			return t, &FieldError{field, "Each line needs a short description."}
		}
		if l.Quantity <= 0 || l.Quantity > 1000 {
			return t, &FieldError{field, "Quantity must be between 1 and 1000."}
		}
		if l.UnitMinor < 0 {
			return t, &FieldError{field, "Amounts cannot be negative. Use a discount line instead."}
		}
		total, err := money.Mul(l.UnitMinor, l.Quantity)
		if err != nil {
			return t, &FieldError{field, "Amount is too large."}
		}
		l.TotalMinor = total
		var addErr error
		switch l.Kind {
		case "discount":
			t.DiscountMinor, addErr = money.Add(t.DiscountMinor, total)
		case "delivery":
			t.DeliveryMinor, addErr = money.Add(t.DeliveryMinor, total)
		default:
			t.SubtotalMinor, addErr = money.Add(t.SubtotalMinor, total)
		}
		if addErr != nil {
			return t, &FieldError{field, "Amount is too large."}
		}
		t.Lines = append(t.Lines, l)
	}
	if t.DiscountMinor > t.SubtotalMinor {
		return t, &FieldError{"lines", "The discount cannot be larger than the subtotal."}
	}
	taxable := t.SubtotalMinor - t.DiscountMinor + t.DeliveryMinor
	if in.PricesIncludeTax {
		// tax portion inside a gross amount: gross - gross / (1 + rate), rounded half up
		if in.TaxRateBP > 0 {
			net := (taxable*10000 + (10000+in.TaxRateBP)/2) / (10000 + in.TaxRateBP)
			t.TaxMinor = taxable - net
		}
		t.TotalMinor = taxable
	} else {
		t.TaxMinor = money.ApplyBasisPoints(taxable, in.TaxRateBP)
		var err error
		if t.TotalMinor, err = money.Add(taxable, t.TaxMinor); err != nil {
			return t, &FieldError{"lines", "Total is too large."}
		}
	}
	switch {
	case in.DepositMinor != nil:
		if *in.DepositMinor < 0 || *in.DepositMinor > t.TotalMinor {
			return t, &FieldError{"depositMinor", "The deposit must be between zero and the total."}
		}
		t.DepositMinor = *in.DepositMinor
	case in.DepositBP != nil:
		if *in.DepositBP < 0 || *in.DepositBP > 10000 {
			return t, &FieldError{"depositPercentBp", "Deposit must be between 0 and 100 percent."}
		}
		t.DepositMinor = money.ApplyBasisPoints(t.TotalMinor, *in.DepositBP)
	}
	t.BalanceMinor = t.TotalMinor - t.DepositMinor
	return t, nil
}

var ErrNothingDue = errors.New("nothing due")

// AmountDue returns what a payment for purpose should collect, given what is already paid.
func AmountDue(purpose string, total, depositRequired, paid int64) (int64, error) {
	var due int64
	switch purpose {
	case "deposit":
		due = depositRequired - paid
	case "balance", "full":
		due = total - paid
	default:
		return 0, fmt.Errorf("unknown purpose %q", purpose)
	}
	if due <= 0 {
		return 0, ErrNothingDue
	}
	return due, nil
}

// PaymentStatus derives the order payment status from amounts.
func PaymentStatus(total, depositRequired, paid, refunded int64) string {
	switch {
	case refunded > 0 && refunded >= paid:
		return "refunded"
	case refunded > 0:
		return "partially_refunded"
	case paid >= total && total > 0:
		return "paid"
	case depositRequired > 0 && paid >= depositRequired:
		return "deposit_paid"
	default:
		return "unpaid"
	}
}
