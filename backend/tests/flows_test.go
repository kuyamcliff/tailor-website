package tests

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"testing"
	"time"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
)

func mustHash(t *testing.T, pw string) string {
	h, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func testJPEG(t *testing.T, w, h int, seed uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x) ^ seed, uint8(y), seed, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAuthAndCSRF(t *testing.T) {
	c := newClient(t)
	r := c.do("POST", "/auth/signup", map[string]any{"name": "Ben", "email": "ben@example.test", "password": "short"})
	expect(t, r, 422)
	r = c.do("POST", "/auth/signup", map[string]any{"name": "Ben", "email": "ben@example.test", "password": "a-long-enough-password"})
	expect(t, r, 200)
	csrf := r.m(t)["user"].(map[string]any)["csrfToken"].(string)
	// Unsafe request without CSRF header is rejected.
	expect(t, c.do("PUT", "/me/profile", map[string]any{"name": "Ben B", "preferredContact": "email"}), 403)
	c.csrf = csrf
	expect(t, c.do("PUT", "/me/profile", map[string]any{"name": "Ben B", "preferredContact": "email", "notificationPrefs": map[string]bool{"email": true}}), 204)
	// Duplicate sign up.
	d := newClient(t)
	expect(t, d.do("POST", "/auth/signup", map[string]any{"name": "Ben", "email": "BEN@example.test", "password": "a-long-enough-password"}), 422)
	// Lockout after repeated failures.
	for i := 0; i < 5; i++ {
		expect(t, d.do("POST", "/auth/signin", map[string]any{"identifier": "ben@example.test", "password": "wrong-password-x"}), 401)
	}
	expect(t, d.do("POST", "/auth/signin", map[string]any{"identifier": "ben@example.test", "password": "a-long-enough-password"}), 429)
	// Customers cannot reach the owner API.
	expect(t, c.do("GET", "/owner/dashboard", nil), 403)
	expect(t, newClient(t).do("GET", "/owner/dashboard", nil), 401)
}

func TestCheckoutAndPayments(t *testing.T) {
	c := newClient(t)
	var variantID string
	if err := shared.pool.QueryRow(context.Background(), `SELECT id FROM product_variants WHERE sku='test-shirt-40'`).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"items": []map[string]any{{"variantId": variantID, "quantity": 1}}, "contact": contact("677000001"),
		"fulfillment": map[string]any{"method": "pickup", "zoneKey": "pickup"}}
	key := "checkout-key-0000000001"
	r := c.do("POST", "/checkout", body, "Idempotency-Key", key)
	expect(t, r, 201)
	order := r.m(t)
	orderID := order["orderId"].(string)
	c.token = order["accessToken"].(string)
	if order["totalMinor"].(float64) != 30000 {
		t.Fatalf("total %v", order["totalMinor"])
	}
	// Double submit returns the same order and does not decrement stock twice.
	r2 := c.do("POST", "/checkout", body, "Idempotency-Key", key)
	expect(t, r2, 200)
	if r2.m(t)["orderId"] != orderID {
		t.Fatal("replay returned a different order")
	}
	var stock int
	shared.pool.QueryRow(context.Background(), `SELECT stock_qty FROM product_variants WHERE id=$1`, variantID).Scan(&stock)
	if stock != 1 {
		t.Fatalf("stock %d", stock)
	}
	// Price changed while checking out.
	body2 := map[string]any{"items": []map[string]any{{"variantId": variantID, "quantity": 1}}, "contact": contact("677000002"),
		"fulfillment": map[string]any{"zoneKey": "pickup"}, "expectedTotalMinor": 25000}
	expect(t, c.idem("POST", "/checkout", body2), 409)
	// Overselling is refused.
	body3 := map[string]any{"items": []map[string]any{{"variantId": variantID, "quantity": 5}}, "contact": contact("677000003"), "fulfillment": map[string]any{"zoneKey": "pickup"}}
	expect(t, c.idem("POST", "/checkout", body3), 409)

	// Unauthorized access to the order is hidden.
	expect(t, newClient(t).do("GET", "/orders/"+orderID, nil), 404)
	expect(t, c.do("GET", "/orders/"+orderID, nil), 200)

	// Failed payment (simulator 0003), then success (0001).
	p := c.idem("POST", "/payments/intent", map[string]any{"orderId": orderID, "provider": "mtn", "msisdn": "677000003"})
	expect(t, p, 201)
	pid := p.m(t)["id"].(string)
	waitFor(t, 5*time.Second, func() bool { return c.do("GET", "/payments/"+pid, nil).m(t)["status"] == "failed" })
	p = c.idem("POST", "/payments/intent", map[string]any{"orderId": orderID, "provider": "orange", "msisdn": "699000001"})
	expect(t, p, 201)
	pid2 := p.m(t)["id"].(string)
	// Pressing pay again while in flight returns the same payment instead of charging twice.
	again := c.idem("POST", "/payments/intent", map[string]any{"orderId": orderID, "provider": "mtn", "msisdn": "677000001"})
	expect(t, again, 200)
	if again.m(t)["id"] != pid2 {
		t.Fatal("second intent created a new payment")
	}
	waitFor(t, 5*time.Second, func() bool { return c.do("GET", "/payments/"+pid2, nil).m(t)["status"] == "succeeded" })
	o := c.do("GET", "/orders/"+orderID, nil).m(t)["order"].(map[string]any)
	if o["paymentStatus"] != "paid" || o["amountPaidMinor"].(float64) != 30000 {
		t.Fatalf("order after payment: %v %v", o["paymentStatus"], o["amountPaidMinor"])
	}
	// Nothing left to pay.
	expect(t, c.idem("POST", "/payments/intent", map[string]any{"orderId": orderID, "provider": "mtn", "msisdn": "677000001"}), 409)
	// Forged webhook is rejected.
	expect(t, c.do("POST", "/payments/mtn/webhook?ref=x&sig=bad", map[string]any{"referenceId": "x"}), 401)
}

func TestDuplicateCallbackCreditsOnce(t *testing.T) {
	c := newClient(t)
	var variantID string
	shared.pool.QueryRow(context.Background(), `SELECT id FROM product_variants WHERE sku='test-shirt-42'`).Scan(&variantID)
	r := c.idem("POST", "/checkout", map[string]any{"items": []map[string]any{{"variantId": variantID, "quantity": 1}}, "contact": contact("677000005"),
		"fulfillment": map[string]any{"zoneKey": "city", "address": map[string]any{"recipient": "Ada", "phone": "677000005", "line1": "Rue 1", "city": "Douala"}}})
	expect(t, r, 201)
	o := r.m(t)
	c.token = o["accessToken"].(string)
	if o["totalMinor"].(float64) != 32000 {
		t.Fatalf("delivery fee not applied: %v", o["totalMinor"])
	}
	p := c.idem("POST", "/payments/intent", map[string]any{"orderId": o["orderId"], "provider": "mtn", "msisdn": "677000005"})
	expect(t, p, 201)
	pid := p.m(t)["id"].(string)
	waitFor(t, 5*time.Second, func() bool {
		var n int
		shared.pool.QueryRow(context.Background(), `SELECT count(*) FROM payment_provider_events WHERE payment_id=$1`, pid).Scan(&n)
		var st string
		shared.pool.QueryRow(context.Background(), `SELECT status FROM payments WHERE id=$1`, pid).Scan(&st)
		return st == "succeeded"
	})
	time.Sleep(500 * time.Millisecond) // let the duplicate callback arrive
	var paid int64
	var notes int
	shared.pool.QueryRow(context.Background(), `SELECT amount_paid_minor FROM orders WHERE id=$1`, o["orderId"]).Scan(&paid)
	shared.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE event='payment_succeeded' AND dedupe_key=$1 AND channel='in_app' AND audience='customer'`,
		"payment_succeeded:"+pid).Scan(&notes)
	if paid != 32000 || notes != 1 {
		t.Fatalf("paid %d notifications %d", paid, notes)
	}
}

func TestBespokeJourney(t *testing.T) {
	ctx := context.Background()
	cust := newClient(t)
	// Upload a reference image as a guest; duplicate upload returns the same record.
	img := testJPEG(t, 640, 800, 7)
	up := cust.upload("reference", img, "inspiration.jpg")
	expect(t, up, 201)
	uploadID := up.m(t)["id"].(string)
	dup := cust.upload("reference", img, "inspiration-again.jpg")
	expect(t, dup, 200)
	if dup.m(t)["id"] != uploadID {
		t.Fatal("duplicate upload not detected")
	}
	// Unsupported and tiny files are refused.
	expect(t, cust.upload("reference", []byte("not an image at all, just text"), "x.jpg"), 415)
	expect(t, cust.upload("reference", testJPEG(t, 50, 50, 1), "tiny.jpg"), 422)
	// Another visitor cannot read the private image.
	expect(t, newClient(t).do("GET", "/uploads/"+uploadID, nil), 404)
	expect(t, cust.do("GET", "/uploads/"+uploadID+"/thumb", nil), 200)

	// Save a design from the studio.
	design := map[string]any{"name": "Wedding suit", "config": map[string]any{"garment": "suit", "fabric": "test-wool", "color": "navy",
		"selections": map[string]any{"lapel": "peak", "jacket_length": "short", "lapel_width": 9},
		"measurements": map[string]any{"unit": "cm", "height": 182, "values": map[string]float64{"chest": 102, "waist": 88, "hip": 100, "sleeve_length": 64,
			"jacket_length": 76, "shoulder": 46, "neck": 40, "bicep": 34, "wrist": 18, "inseam": 82, "thigh": 60}}}}
	ds := cust.do("POST", "/designs", design)
	expect(t, ds, 201)
	dm := ds.m(t)
	snap := dm["snapshot"].(map[string]any)
	price := snap["price"].(map[string]any)
	if price["totalMinor"].(float64) != 180000+5000+40000 {
		t.Fatalf("design price %v", price)
	}
	if snap["fit"].(map[string]any)["overall"] == "" {
		t.Fatal("no fit estimate")
	}
	// Archived fabric and removed option values are refused with a clear message.
	bad := map[string]any{"config": map[string]any{"garment": "suit", "fabric": "old-cloth"}}
	expect(t, cust.do("POST", "/designs", bad), 422)
	bad = map[string]any{"config": map[string]any{"garment": "suit", "selections": map[string]any{"lapel": "bogus"}}}
	expect(t, cust.do("POST", "/designs", bad), 422)

	// Submit the request.
	req := map[string]any{"garment": "suit", "occasion": "wedding", "measurementMode": "entered",
		"measurements": map[string]any{"unit": "cm", "height": 182, "values": map[string]float64{"neck": 40, "shoulder": 46, "chest": 102, "waist": 88, "hip": 100,
			"sleeve_length": 64, "jacket_length": 76, "bicep": 34, "wrist": 18, "inseam": 82, "thigh": 60}},
		"fabricMode": "catalog", "fabricKey": "test-wool", "colorKey": "navy", "designVersionId": dm["versionId"],
		"references": []map[string]any{{"uploadId": uploadID, "tag": "overall", "note": "Like this lapel"}},
		"notes":      "For my wedding in December.", "desiredDate": time.Now().AddDate(0, 3, 0).Format("2006-01-02"), "contact": contact("677111222")}
	sub := cust.do("POST", "/requests", req, "Idempotency-Key", "request-key-00000001")
	expect(t, sub, 201)
	sm := sub.m(t)
	reqID := sm["id"].(string)
	cust.token = sm["accessToken"].(string)
	expect(t, cust.do("POST", "/requests", req, "Idempotency-Key", "request-key-00000001"), 200) // double submit
	// Invalid measurement is rejected.
	badReq := map[string]any{}
	for k, v := range req {
		badReq[k] = v
	}
	badReq["measurements"] = map[string]any{"unit": "cm", "values": map[string]float64{"chest": -5}}
	expect(t, cust.idem("POST", "/requests", badReq), 422)

	// Owner sees it, verifies measurements, creates and sends a quote.
	owner := ownerClient(t)
	list := owner.do("GET", "/owner/requests?status=new", nil)
	expect(t, list, 200)
	found := false
	for _, it := range list.m(t)["items"].([]any) {
		found = found || it.(map[string]any)["id"] == reqID
	}
	if !found {
		t.Fatal("request not visible to owner")
	}
	detail := owner.do("GET", "/owner/requests/"+reqID, nil)
	expect(t, detail, 200)
	rv := detail.m(t)["request"].(map[string]any)
	if len(rv["references"].([]any)) != 1 || rv["measurements"] == nil {
		t.Fatal("request detail incomplete")
	}
	// Staff can open the private reference.
	expect(t, owner.do("GET", "/uploads/"+uploadID+"/preview", nil), 200)
	ver := owner.do("POST", "/owner/requests/"+reqID+"/verify-measurements", map[string]any{"unit": "cm", "height": 182,
		"values": map[string]float64{"chest": 103, "waist": 88, "sleeve_length": 65}, "notes": "Measured at consultation"})
	expect(t, ver, 201)
	q := owner.do("POST", "/owner/requests/"+reqID+"/quotes", map[string]any{"lines": []map[string]any{
		{"kind": "garment", "description": "Two-piece suit, peak lapel", "quantity": 1, "unitMinor": 200000},
		{"kind": "fabric", "description": "Navy wool, 3.2 m", "quantity": 1, "unitMinor": 40000},
		{"kind": "discount", "description": "Wedding party", "quantity": 1, "unitMinor": 10000}}, "depositPercentBp": 5000, "validDays": 14})
	expect(t, q, 201)
	qm := q.m(t)
	quoteID := qm["id"].(string)
	cur := qm["current"].(map[string]any)
	if cur["totalMinor"].(float64) != 230000 || cur["depositMinor"].(float64) != 115000 {
		t.Fatalf("quote totals %v", cur)
	}
	// Customer cannot see a draft.
	expect(t, cust.do("GET", "/quotes/"+quoteID, nil), 404)
	expect(t, owner.do("POST", "/owner/quotes/"+quoteID+"/send", nil), 204)
	cq := cust.do("GET", "/quotes/"+quoteID, nil)
	expect(t, cq, 200)
	revID := cq.m(t)["quote"].(map[string]any)["current"].(map[string]any)["id"]

	// Accepting twice (two tabs) yields one order.
	var wg sync.WaitGroup
	results := make([]resp, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = cust.do("POST", "/quotes/"+quoteID+"/accept", map[string]any{"revisionId": revID})
		}(i)
	}
	wg.Wait()
	expect(t, results[0], 200)
	expect(t, results[1], 200)
	orderID := results[0].m(t)["orderId"].(string)
	if results[1].m(t)["orderId"] != orderID {
		t.Fatal("two orders created")
	}
	var orders int
	shared.pool.QueryRow(ctx, `SELECT count(*) FROM orders WHERE quote_id=$1`, quoteID).Scan(&orders)
	if orders != 1 {
		t.Fatalf("orders for quote: %d", orders)
	}
	// The quote link token opens the order; the order snapshot is independent of the catalog.
	co := cust.do("GET", "/orders/"+orderID, nil)
	expect(t, co, 200)
	om := co.m(t)["order"].(map[string]any)
	if om["status"] != "awaiting_customer" || om["depositRequiredMinor"].(float64) != 115000 {
		t.Fatalf("order %v %v", om["status"], om["depositRequiredMinor"])
	}
	shared.pool.Exec(ctx, `UPDATE fabrics SET price_impact_minor=99999 WHERE key='test-wool'`)
	if cust.do("GET", "/orders/"+orderID, nil).m(t)["order"].(map[string]any)["totalMinor"].(float64) != 230000 {
		t.Fatal("order changed with catalog")
	}

	// Pay the deposit.
	p := cust.idem("POST", "/payments/intent", map[string]any{"orderId": orderID, "provider": "mtn", "msisdn": "677111001"})
	expect(t, p, 201)
	if p.m(t)["amountMinor"].(float64) != 115000 || p.m(t)["purpose"] != "deposit" {
		t.Fatalf("deposit intent %s", p.body)
	}
	pid := p.m(t)["id"].(string)
	waitFor(t, 5*time.Second, func() bool { return cust.do("GET", "/payments/"+pid, nil).m(t)["status"] == "succeeded" })
	om = cust.do("GET", "/orders/"+orderID, nil).m(t)["order"].(map[string]any)
	if om["status"] != "deposit_paid" || om["paymentStatus"] != "deposit_paid" {
		t.Fatalf("after deposit: %v %v", om["status"], om["paymentStatus"])
	}

	// Owner moves production forward; stale versions are refused; backwards needs a note.
	og := owner.do("GET", "/owner/orders/"+orderID, nil).m(t)["order"].(map[string]any)
	v := int(og["version"].(float64))
	expect(t, owner.do("POST", "/owner/orders/"+orderID+"/status", map[string]any{"status": "cutting", "version": v, "customerVisible": true}), 204)
	expect(t, owner.do("POST", "/owner/orders/"+orderID+"/status", map[string]any{"status": "sewing", "version": v, "customerVisible": true}), 409)
	expect(t, owner.do("POST", "/owner/orders/"+orderID+"/status", map[string]any{"status": "patterning", "version": v + 1, "customerVisible": true}), 422)
	expect(t, owner.do("POST", "/owner/orders/"+orderID+"/notes", map[string]any{"body": "Customer prefers a slightly shorter sleeve", "visibility": "internal"}), 204)
	expect(t, owner.do("POST", "/owner/orders/"+orderID+"/notes", map[string]any{"body": "Your fabric has arrived.", "visibility": "customer"}), 204)
	om = cust.do("GET", "/orders/"+orderID, nil).m(t)["order"].(map[string]any)
	if om["status"] != "cutting" {
		t.Fatalf("customer sees %v", om["status"])
	}
	for _, n := range om["notes"].([]any) {
		if n.(map[string]any)["visibility"] == "internal" {
			t.Fatal("internal note leaked to customer")
		}
	}
	// Owner books a fitting; the customer sees it on the order.
	var custID string
	shared.pool.QueryRow(ctx, `SELECT customer_id FROM orders WHERE id=$1`, orderID).Scan(&custID)
	start := nextWeekday(time.Now().AddDate(0, 0, 3), 10)
	expect(t, owner.do("POST", "/owner/appointments", map[string]any{"customerId": custID, "type": "fitting", "startsAt": start, "orderId": orderID}), 201)
	om = cust.do("GET", "/orders/"+orderID, nil).m(t)["order"].(map[string]any)
	if len(om["appointments"].([]any)) != 1 || om["status"] != "fitting_scheduled" {
		t.Fatalf("fitting not visible: %v %v", om["appointments"], om["status"])
	}
	// Audit trail recorded.
	var audits int
	shared.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE object_id=$1 AND action='orders.status'`, orderID).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audit entries %d", audits)
	}
}

func nextWeekday(t time.Time, hour int) time.Time {
	loc, _ := time.LoadLocation("Africa/Douala")
	t = t.In(loc)
	for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		t = t.AddDate(0, 0, 1)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), hour, 0, 0, 0, loc)
}

func TestExpiredQuote(t *testing.T) {
	ctx := context.Background()
	cust := newClient(t)
	sub := cust.idem("POST", "/requests", map[string]any{"garment": "shirt", "occasion": "work", "measurementMode": "in_store", "fabricMode": "recommend",
		"contact": contact("677333444")})
	expect(t, sub, 201)
	reqID := sub.m(t)["id"].(string)
	cust.token = sub.m(t)["accessToken"].(string)
	owner := ownerClient(t)
	q := owner.do("POST", "/owner/requests/"+reqID+"/quotes", map[string]any{"lines": []map[string]any{{"kind": "garment", "description": "Shirt", "quantity": 2, "unitMinor": 35000}}, "depositMinor": 0})
	expect(t, q, 201)
	quoteID := q.m(t)["id"].(string)
	expect(t, owner.do("POST", "/owner/quotes/"+quoteID+"/send", nil), 204)
	revID := cust.do("GET", "/quotes/"+quoteID, nil).m(t)["quote"].(map[string]any)["current"].(map[string]any)["id"]
	// Expire the revision while the customer's page is open.
	shared.pool.Exec(ctx, `UPDATE quote_revisions SET expires_at = now() - interval '1 minute' WHERE quote_id=$1`, quoteID)
	r := cust.do("POST", "/quotes/"+quoteID+"/accept", map[string]any{"revisionId": revID})
	expect(t, r, 409)
	if r.m(t)["error"].(map[string]any)["code"] != "quote_expired" {
		t.Fatalf("unexpected %s", r.body)
	}
	// Customer can still ask for changes; owner revises and resends; acceptance works with zero deposit.
	expect(t, cust.do("POST", "/quotes/"+quoteID+"/request-changes", map[string]any{"revisionId": revID, "note": "Please extend the date"}), 204)
	cur := owner.do("GET", "/owner/quotes/"+quoteID, nil).m(t)
	rev := owner.do("PUT", "/owner/quotes/"+quoteID, map[string]any{"lines": []map[string]any{{"kind": "garment", "description": "Shirt", "quantity": 2, "unitMinor": 35000}},
		"depositMinor": 0, "version": int(cur["version"].(float64))})
	expect(t, rev, 200)
	expect(t, owner.do("POST", "/owner/quotes/"+quoteID+"/send", nil), 204)
	// The old revision is refused, the new one accepted.
	expect(t, cust.do("POST", "/quotes/"+quoteID+"/accept", map[string]any{"revisionId": revID}), 409)
	newRev := cust.do("GET", "/quotes/"+quoteID, nil).m(t)["quote"].(map[string]any)["current"].(map[string]any)["id"]
	acc := cust.do("POST", "/quotes/"+quoteID+"/accept", map[string]any{"revisionId": newRev})
	expect(t, acc, 200)
	om := cust.do("GET", "/orders/"+acc.m(t)["orderId"].(string), nil).m(t)["order"].(map[string]any)
	if om["status"] != "measurements_pending" {
		t.Fatalf("zero-deposit order status %v", om["status"])
	}
}

func TestAppointmentDoubleBooking(t *testing.T) {
	a, b := newClient(t), newClient(t)
	slots := a.do("GET", "/appointments/slots?type=consultation", nil).m(t)["slots"].([]any)
	if len(slots) == 0 {
		t.Fatal("no slots")
	}
	slot := slots[len(slots)/2].(string)
	var wg sync.WaitGroup
	res := make([]resp, 2)
	for i, c := range []*client{a, b} {
		wg.Add(1)
		go func(i int, c *client) {
			defer wg.Done()
			res[i] = c.idem("POST", "/appointments", map[string]any{"type": "consultation", "startsAt": slot, "contact": contact(fmt.Sprintf("67755500%d", i))})
		}(i, c)
	}
	wg.Wait()
	ok, taken := 0, 0
	for _, r := range res {
		switch r.status {
		case 201:
			ok++
		case 409:
			taken++
		}
	}
	if ok != 1 || taken != 1 {
		t.Fatalf("expected one booking and one conflict, got %d/%d: %s / %s", ok, taken, res[0].body, res[1].body)
	}
	// The slot disappears from availability.
	for _, s := range a.do("GET", "/appointments/slots?type=consultation", nil).m(t)["slots"].([]any) {
		if s == slot {
			t.Fatal("booked slot still offered")
		}
	}
}

func TestSupportAndPrivacy(t *testing.T) {
	c := newClient(t)
	r := c.idem("POST", "/support", map[string]any{"subject": "Question about fabrics", "category": "general", "message": "Do you have linen in stock?", "contact": contact("677999000")})
	expect(t, r, 201)
	id := r.m(t)["id"].(string)
	c.token = r.m(t)["accessToken"].(string)
	owner := ownerClient(t)
	expect(t, owner.do("POST", "/owner/support/"+id+"/reply", map[string]any{"body": "Internal: check the stock room", "internal": true}), 204)
	expect(t, owner.do("POST", "/owner/support/"+id+"/reply", map[string]any{"body": "Yes, we have natural and sand."}), 204)
	th := c.do("GET", "/support/"+id, nil).m(t)
	msgs := th["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("customer sees %d messages (internal note must be hidden)", len(msgs))
	}
	// Account deletion anonymizes.
	acc := newClient(t)
	s := acc.do("POST", "/auth/signup", map[string]any{"name": "Del Me", "email": "del@example.test", "password": "a-long-enough-password"})
	expect(t, s, 200)
	acc.csrf = s.m(t)["user"].(map[string]any)["csrfToken"].(string)
	expect(t, acc.do("POST", "/me/delete-account", map[string]any{"password": "wrong-password"}), 422)
	expect(t, acc.do("POST", "/me/delete-account", map[string]any{"password": "a-long-enough-password"}), 204)
	expect(t, acc.do("GET", "/me/profile", nil), 401)
}

// TestOwnerReports checks the dashboard and analytics endpoints answer for every period.
func TestOwnerReports(t *testing.T) {
	owner := ownerClient(t)
	expect(t, owner.do("GET", "/owner/dashboard", nil), 200)
	for _, p := range []string{"30", "90", "365"} {
		r := owner.do("GET", "/owner/analytics?period="+p, nil)
		expect(t, r, 200)
		var out struct {
			Values map[string]float64 `json:"values"`
		}
		r.json(t, &out)
		if _, ok := out.Values["outstandingBalances"]; !ok {
			t.Fatalf("analytics missing outstandingBalances: %s", r.body)
		}
	}
}
