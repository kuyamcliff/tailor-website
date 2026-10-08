package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
)

func TestStateMachine(t *testing.T) {
	all := []Status{Created, Pending, CustomerActionRequired, Processing, Succeeded, Failed, Expired, Cancelled, Refunded, PartiallyRefunded}
	allowed := map[Status][]Status{
		Created:                {Pending, CustomerActionRequired, Processing, Succeeded, Failed, Expired, Cancelled},
		Pending:                {CustomerActionRequired, Processing, Succeeded, Failed, Expired, Cancelled},
		CustomerActionRequired: {Pending, Processing, Succeeded, Failed, Expired, Cancelled},
		Processing:             {Pending, CustomerActionRequired, Succeeded, Failed, Expired, Cancelled},
		Succeeded:              {Refunded, PartiallyRefunded},
		PartiallyRefunded:      {Refunded},
	}
	for _, from := range all {
		ok := map[Status]bool{}
		for _, to := range allowed[from] {
			ok[to] = true
		}
		for _, to := range all {
			if got := CanTransition(from, to); got != ok[to] {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, ok[to])
			}
		}
	}
	// A succeeded payment never fails, and nothing final returns to pending.
	for _, final := range []Status{Succeeded, Failed, Expired, Cancelled, Refunded} {
		if CanTransition(final, Pending) {
			t.Errorf("%s moved back to pending", final)
		}
	}
	if CanTransition(Succeeded, Failed) {
		t.Error("succeeded became failed")
	}
	// Late approval of a payment we expired locally is credited.
	if !canApply(Expired, Succeeded) || canApply(Expired, Failed) || canApply(Failed, Succeeded) {
		t.Error("canApply exception wrong")
	}
	for _, s := range []Status{Created, Pending, CustomerActionRequired, Processing} {
		if !s.Active() || s.Final() {
			t.Errorf("%s should be active", s)
		}
	}
	for _, s := range []Status{Succeeded, Failed, Expired, Cancelled, Refunded, PartiallyRefunded} {
		if s.Active() || !s.Final() {
			t.Errorf("%s should be final", s)
		}
	}
}

func TestCallbackSignature(t *testing.T) {
	sig := SignReference("secret-a", "ref-1")
	if !VerifyReference("secret-a", "ref-1", sig) {
		t.Fatal("valid signature rejected")
	}
	for _, c := range []struct{ secret, ref, sig string }{
		{"secret-b", "ref-1", sig},  // other secret
		{"secret-a", "ref-2", sig},  // other reference
		{"secret-a", "ref-1", ""},   // missing
		{"", "ref-1", sig},          // provider not configured
		{"secret-a", "ref-1", "00"}, // garbage
	} {
		if VerifyReference(c.secret, c.ref, c.sig) {
			t.Errorf("accepted %+v", c)
		}
	}
}

func TestSimulatorScenarios(t *testing.T) {
	cases := map[string]Status{
		"237677000001": Succeeded, "237677000002": Pending, "237677000003": Failed, "237677000005": Succeeded,
		"237677000006": Succeeded, "237677000007": Cancelled, "237677000008": Expired, "237677123456": Succeeded,
	}
	var callbacks atomic.Int32
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { callbacks.Add(1) }))
	defer cb.Close()
	sim := NewSimulator("mtn", 0)
	for msisdn, want := range cases {
		ref := "ref-" + msisdn + "-0000000000000000"
		res, err := sim.Initiate(context.Background(), InitiateRequest{Reference: ref, MSISDN: msisdn, CallbackURL: cb.URL})
		if err != nil || res.Status != CustomerActionRequired {
			t.Fatalf("%s initiate: %v %v", msisdn, res.Status, err)
		}
		got, err := sim.Status(context.Background(), ref, "")
		if err != nil || got.Status != want {
			t.Errorf("%s status = %s, want %s (%v)", msisdn, got.Status, want, err)
		}
		if want == Expired && got.FailureMessage == "" {
			t.Errorf("%s: expired payment needs a message for the customer", msisdn)
		}
	}
	// Outage is a transient error so the service retries with the same reference.
	_, err := sim.Initiate(context.Background(), InitiateRequest{Reference: "ref-outage-000000000000", MSISDN: "237677000004", CallbackURL: cb.URL})
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("outage error = %v", err)
	}
	// Callbacks: 0001, 0003, 0005 (twice), 0007, 0008 and the default number; none for 0002 or 0006.
	deadline := time.Now().Add(3 * time.Second)
	for callbacks.Load() < 7 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := callbacks.Load(); n != 7 {
		t.Fatalf("callbacks = %d, want 7", n)
	}
	// Duplicate deliveries share one event key.
	req := httptest.NewRequest(http.MethodPost, "/cb?ref=abc", nil)
	a, _ := sim.ParseCallback(req, nil)
	b, _ := sim.ParseCallback(req, []byte(`{"referenceId":"abc"}`))
	if a.EventKey != b.EventKey || a.Reference != "abc" {
		t.Fatalf("event keys differ: %q %q", a.EventKey, b.EventKey)
	}
}

func TestMajorAmount(t *testing.T) {
	for _, c := range []struct {
		minor    int64
		currency string
		want     string
	}{{25000, "XAF", "25000"}, {12345, "EUR", "123.45"}, {5, "EUR", "0.05"}, {700, "XOF", "700"}} {
		if got := majorAmount(c.minor, c.currency); got != c.want {
			t.Errorf("majorAmount(%d, %s) = %s, want %s", c.minor, c.currency, got, c.want)
		}
	}
}

func TestProvidersStayOffWithoutCredentials(t *testing.T) {
	if ok, why := NewMTN(config.PaymentsConfig{}).Configured(); ok || !strings.Contains(why, "MTN_MOMO_API_KEY") {
		t.Errorf("MTN configured with nothing: %v %q", ok, why)
	}
	if ok, why := NewOrange(config.PaymentsConfig{}).Configured(); ok || !strings.Contains(why, "ORANGE_MONEY_PIN") {
		t.Errorf("Orange configured with nothing: %v %q", ok, why)
	}
	plain := config.PaymentsConfig{MTNBaseURL: "http://momo.example", MTNTargetEnv: "sandbox", MTNSubscriptionKey: "k",
		MTNAPIUser: "u", MTNAPIKey: "p", MTNCallbackSecret: "s"}
	if ok, why := NewMTN(plain).Configured(); ok || !strings.Contains(why, "https") {
		t.Errorf("MTN accepted a plain http URL: %v %q", ok, why)
	}
}

// fakeMTN is a minimal MTN MoMo Collection API: token, request to pay and status.
type fakeMTN struct {
	statuses  map[string]string // reference -> PENDING | SUCCESSFUL | FAILED | REJECTED | TIMEOUT
	reasons   map[string]string
	tokens    atomic.Int32
	requests  atomic.Int32
	failFirst atomic.Int32 // number of 503s before answering
	seenRefs  []string
}

func (f *fakeMTN) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/collection/token/":
		if u, p, ok := r.BasicAuth(); !ok || u != "api-user" || p != "api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.tokens.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	case r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Target-Environment") != "sandbox":
		w.WriteHeader(http.StatusUnauthorized)
	case r.Method == http.MethodPost && r.URL.Path == "/collection/v1_0/requesttopay":
		f.requests.Add(1)
		if f.failFirst.Load() > 0 {
			f.failFirst.Add(-1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["amount"] != "25000" || body["currency"] != "XAF" || r.Header.Get("X-Callback-Url") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ref := r.Header.Get("X-Reference-Id")
		for _, s := range f.seenRefs {
			if s == ref {
				w.WriteHeader(http.StatusConflict)
				return
			}
		}
		f.seenRefs = append(f.seenRefs, ref)
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/collection/v1_0/requesttopay/"):
		ref := strings.TrimPrefix(r.URL.Path, "/collection/v1_0/requesttopay/")
		st, ok := f.statuses[ref]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": st, "financialTransactionId": "FT-" + ref, "reason": f.reasons[ref]})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestMTN(t *testing.T, f *fakeMTN) *MTN {
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	m := NewMTN(config.PaymentsConfig{MTNBaseURL: srv.URL, MTNTargetEnv: "sandbox", MTNSubscriptionKey: "sub",
		MTNAPIUser: "api-user", MTNAPIKey: "api-key", MTNCallbackSecret: "cb"})
	m.client = srv.Client()
	if ok, why := m.Configured(); !ok {
		t.Fatal(why)
	}
	return m
}

func TestMTNAdapter(t *testing.T) {
	f := &fakeMTN{
		statuses: map[string]string{"r-ok": "SUCCESSFUL", "r-fail": "FAILED", "r-rej": "REJECTED", "r-exp": "TIMEOUT", "r-wait": "PENDING"},
		reasons:  map[string]string{"r-fail": "NOT_ENOUGH_FUNDS"},
	}
	m := newTestMTN(t, f)
	ctx := context.Background()
	in := InitiateRequest{Reference: "r-new", PaymentID: "p1", AmountMinor: 25000, Currency: "XAF", MSISDN: "237677000001",
		Description: "Order A-1", CallbackURL: "https://api.example/cb"}
	res, err := m.Initiate(ctx, in)
	if err != nil || res.Status != CustomerActionRequired {
		t.Fatalf("initiate: %v %v", res, err)
	}
	// Resubmitting the same reference does not create a second request; MTN answers 409 and we read the status.
	f.statuses["r-new"] = "PENDING"
	res, err = m.Initiate(ctx, in)
	if err != nil || res.Status != Pending {
		t.Fatalf("replay: %v %v", res, err)
	}
	want := map[string]Status{"r-ok": Succeeded, "r-fail": Failed, "r-rej": Cancelled, "r-exp": Expired, "r-wait": Pending, "r-unknown": Pending}
	for ref, st := range want {
		res, err := m.Status(ctx, ref, "")
		if err != nil || res.Status != st {
			t.Errorf("status %s = %s (%v), want %s", ref, res.Status, err, st)
		}
	}
	res, _ = m.Status(ctx, "r-fail", "")
	if res.FailureMessage != "There is not enough balance on this Mobile Money account." {
		t.Errorf("failure message %q", res.FailureMessage)
	}
	res, _ = m.Status(ctx, "r-ok", "")
	if res.ProviderTxID != "FT-r-ok" {
		t.Errorf("transaction id %q", res.ProviderTxID)
	}
	if f.tokens.Load() != 1 {
		t.Errorf("token fetched %d times; it should be cached", f.tokens.Load())
	}
	// Provider outage is transient, so the service retries.
	f.failFirst.Store(1)
	_, err = m.Initiate(ctx, InitiateRequest{Reference: "r-outage", AmountMinor: 25000, Currency: "XAF", MSISDN: "237677000001", CallbackURL: "https://x"})
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("outage error = %v", err)
	}
	// The callback body is never trusted for status; it only names the reference.
	req := httptest.NewRequest(http.MethodPost, "/cb?ref=r-ok", strings.NewReader(`{"status":"SUCCESSFUL"}`))
	body, _ := io.ReadAll(req.Body)
	ev, err := m.ParseCallback(req, body)
	if err != nil || ev.Reference != "r-ok" {
		t.Fatalf("callback: %v %v", ev, err)
	}
}

// fakeOrange is a minimal Orange Money Core API: token, init, pay and payment status.
type fakeOrange struct {
	statuses map[string]string // payToken -> status
	payBody  map[string]any
}

func (f *fakeOrange) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/token" {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "otok", "expires_in": 3600})
		return
	}
	if r.Header.Get("Authorization") != "Bearer otok" || r.Header.Get("X-AUTH-TOKEN") != "merchant" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/omcoreapis/1.0.2/mp/init":
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"payToken": "MP-1"}})
	case r.URL.Path == "/omcoreapis/1.0.2/mp/pay":
		_ = json.NewDecoder(r.Body).Decode(&f.payBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "PENDING", "payToken": "MP-1"}})
	case strings.HasPrefix(r.URL.Path, "/omcoreapis/1.0.2/mp/paymentstatus/"):
		tok := strings.TrimPrefix(r.URL.Path, "/omcoreapis/1.0.2/mp/paymentstatus/")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": f.statuses[tok], "txnid": "TX-" + tok}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestOrangeAdapter(t *testing.T) {
	t.Setenv("ORANGE_MONEY_API_VERSION", "")
	f := &fakeOrange{statuses: map[string]string{"MP-ok": "SUCCESSFULL", "MP-fail": "FAILED", "MP-exp": "EXPIRED", "MP-can": "CANCELLED", "MP-wait": "PENDING"}}
	srv := httptest.NewTLSServer(f)
	defer srv.Close()
	o := NewOrange(config.PaymentsConfig{OrangeBaseURL: srv.URL, OrangeClientID: "id", OrangeClientSecret: "secret",
		OrangeAuthToken: "merchant", OrangeChannelMSISDN: "690000000", OrangePIN: "0000", OrangeCallbackSecret: "cb"})
	o.client = srv.Client()
	ctx := context.Background()
	res, err := o.Initiate(ctx, InitiateRequest{Reference: "ref-1", AmountMinor: 25000, Currency: "XAF", MSISDN: "237699000001",
		Description: "Order A-2", CallbackURL: "https://api.example/cb"})
	if err != nil || res.Status != CustomerActionRequired || res.ProviderTxID != "MP-1" {
		t.Fatalf("initiate: %+v %v", res, err)
	}
	if f.payBody["subscriberMsisdn"] != "699000001" || f.payBody["amount"] != "25000" || f.payBody["orderId"] != "ref-1" {
		t.Errorf("pay request %+v", f.payBody)
	}
	want := map[string]Status{"MP-ok": Succeeded, "MP-fail": Failed, "MP-exp": Expired, "MP-can": Cancelled, "MP-wait": Pending}
	for tok, st := range want {
		res, err := o.Status(ctx, "ref", tok)
		if err != nil || res.Status != st {
			t.Errorf("status %s = %s (%v), want %s", tok, res.Status, err, st)
		}
	}
	// Without a payToken there is nothing to ask; the payment stays pending until reconciliation.
	if res, _ := o.Status(ctx, "ref", ""); res.Status != Pending {
		t.Errorf("no pay token: %s", res.Status)
	}
}
