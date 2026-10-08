package tests

import (
	"context"
	"testing"
	"time"
)

// Unsuccessful payments: the provider is down, the customer cancels on the phone, or the request
// times out on the provider's side. Each ends in its own state with a message for the customer,
// the order stays unpaid, and a new payment can be started straight away.
func TestPaymentFailureStates(t *testing.T) {
	c := newClient(t)
	var variantID string
	if err := shared.pool.QueryRow(context.Background(), `SELECT id FROM product_variants WHERE sku='test-shirt-44'`).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	r := c.idem("POST", "/checkout", map[string]any{"items": []map[string]any{{"variantId": variantID, "quantity": 1}},
		"contact": contact("677000044"), "fulfillment": map[string]any{"zoneKey": "pickup"}})
	expect(t, r, 201)
	o := r.m(t)
	orderID := o["orderId"].(string)
	c.token = o["accessToken"].(string)

	for _, tc := range []struct{ msisdn, status string }{
		{"677000004", "failed"},    // provider unavailable
		{"677000007", "cancelled"}, // declined on the phone
		{"677000008", "expired"},   // provider reports a timeout
	} {
		p := c.idem("POST", "/payments/intent", map[string]any{"orderId": orderID, "provider": "mtn", "msisdn": tc.msisdn})
		expect(t, p, 201)
		pid := p.m(t)["id"].(string)
		var last map[string]any
		waitFor(t, 10*time.Second, func() bool {
			last = c.do("GET", "/payments/"+pid, nil).m(t)
			return last["status"] == tc.status
		})
		if msg, _ := last["failureMessage"].(string); msg == "" {
			t.Errorf("%s: no message for the customer: %v", tc.status, last)
		}
		order := c.do("GET", "/orders/"+orderID, nil).m(t)["order"].(map[string]any)
		if order["paymentStatus"] == "paid" || order["amountPaidMinor"].(float64) != 0 {
			t.Fatalf("%s payment credited the order: %v", tc.status, order)
		}
	}
	var n int
	shared.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE event='payment_failed' AND audience='customer'
		AND dedupe_key IN (SELECT 'payment_failed:'||id FROM payments WHERE order_id=$1)`, orderID).Scan(&n)
	if n != 3 {
		t.Errorf("customer notices for unsuccessful payments = %d, want 3", n)
	}
	// After all that, a good payment still goes through once.
	p := c.idem("POST", "/payments/intent", map[string]any{"orderId": orderID, "provider": "orange", "msisdn": "699000001"})
	expect(t, p, 201)
	pid := p.m(t)["id"].(string)
	waitFor(t, 10*time.Second, func() bool { return c.do("GET", "/payments/"+pid, nil).m(t)["status"] == "succeeded" })
}
