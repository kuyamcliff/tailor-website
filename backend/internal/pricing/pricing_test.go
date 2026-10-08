package pricing

import "testing"

func ptr(v int64) *int64 { return &v }

func TestCalculateExclusiveTax(t *testing.T) {
	tot, err := Calculate(Input{
		Lines: []Line{
			{Kind: "garment", Description: "Two piece suit", Quantity: 1, UnitMinor: 150000},
			{Kind: "fabric", Description: "Navy wool, 3.2 m", Quantity: 1, UnitMinor: 45000},
			{Kind: "rush", Description: "Rush fee", Quantity: 1, UnitMinor: 20000},
			{Kind: "delivery", Description: "Delivery", Quantity: 1, UnitMinor: 2000},
			{Kind: "discount", Description: "Returning client", Quantity: 1, UnitMinor: 15000},
		},
		TaxRateBP: 1925, DepositBP: ptr(5000),
	})
	if err != nil {
		t.Fatal(err)
	}
	// subtotal 215000, discount 15000, delivery 2000 => taxable 202000; tax 19.25% = 38885; total 240885
	if tot.SubtotalMinor != 215000 || tot.DiscountMinor != 15000 || tot.DeliveryMinor != 2000 {
		t.Fatalf("components %+v", tot)
	}
	if tot.TaxMinor != 38885 || tot.TotalMinor != 240885 {
		t.Fatalf("tax %d total %d", tot.TaxMinor, tot.TotalMinor)
	}
	if tot.DepositMinor != 120443 || tot.BalanceMinor != 120442 {
		t.Fatalf("deposit %d balance %d", tot.DepositMinor, tot.BalanceMinor)
	}
}

func TestCalculateInclusiveTax(t *testing.T) {
	tot, err := Calculate(Input{Lines: []Line{{Kind: "garment", Description: "Shirt", Quantity: 2, UnitMinor: 59625}},
		TaxRateBP: 1925, PricesIncludeTax: true, DepositMinor: ptr(0)})
	if err != nil {
		t.Fatal(err)
	}
	// gross 119250 = net 100000 + tax 19250
	if tot.TotalMinor != 119250 || tot.TaxMinor != 19250 || tot.BalanceMinor != 119250 {
		t.Fatalf("%+v", tot)
	}
}

func TestCalculateValidation(t *testing.T) {
	bad := []Input{
		{},
		{Lines: []Line{{Kind: "garment", Description: "x", Quantity: 0, UnitMinor: 1}}},
		{Lines: []Line{{Kind: "garment", Description: "x", Quantity: 1, UnitMinor: -1}}},
		{Lines: []Line{{Kind: "nope", Description: "x", Quantity: 1, UnitMinor: 1}}},
		{Lines: []Line{{Kind: "garment", Description: "", Quantity: 1, UnitMinor: 1}}},
		{Lines: []Line{{Kind: "garment", Description: "x", Quantity: 1, UnitMinor: 100}, {Kind: "discount", Description: "d", Quantity: 1, UnitMinor: 200}}},
		{Lines: []Line{{Kind: "garment", Description: "x", Quantity: 1, UnitMinor: 100}}, DepositMinor: ptr(101)},
		{Lines: []Line{{Kind: "garment", Description: "x", Quantity: 1000, UnitMinor: 1 << 62}}},
	}
	for i, in := range bad {
		if _, err := Calculate(in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestAmountDueAndStatus(t *testing.T) {
	if d, err := AmountDue("deposit", 1000, 500, 0); err != nil || d != 500 {
		t.Fatal(d, err)
	}
	if _, err := AmountDue("deposit", 1000, 500, 500); err != ErrNothingDue {
		t.Fatal("expected nothing due")
	}
	if d, _ := AmountDue("balance", 1000, 500, 500); d != 500 {
		t.Fatal(d)
	}
	cases := []struct {
		total, dep, paid, ref int64
		want                  string
	}{
		{1000, 500, 0, 0, "unpaid"}, {1000, 500, 500, 0, "deposit_paid"}, {1000, 500, 1000, 0, "paid"},
		{1000, 0, 1000, 200, "partially_refunded"}, {1000, 0, 1000, 1000, "refunded"}, {1000, 0, 400, 0, "unpaid"},
	}
	for _, c := range cases {
		if got := PaymentStatus(c.total, c.dep, c.paid, c.ref); got != c.want {
			t.Errorf("%+v got %s", c, got)
		}
	}
}
