package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Simulator is a deterministic development provider that stands in for MTN or Orange when
// PAYMENTS_DEV_SIMULATOR=true. Configuration validation refuses it in production, and every payment
// it handles is stored with simulated=true and labelled "Test payment" in the interface.
//
// The outcome is chosen by the last four digits of the payer's number:
//
//	0001 DEV_SUCCESS             approved after a short delay, callback sent
//	0002 DEV_PENDING             never approved; the payment expires
//	0003 DEV_FAILED              declined (insufficient balance)
//	0004 DEV_UNAVAILABLE         the provider rejects the initiation (outage)
//	0005 DEV_DUPLICATE_CALLBACK  approved, callback delivered twice
//	0006 DEV_DELAYED_CALLBACK    approved, no callback; only status polling finds it
//	0007 DEV_CANCELLED           cancelled on the phone
//	other                        same as 0001
type Simulator struct {
	name    string
	delay   time.Duration
	client  *http.Client
	mu      sync.Mutex
	pending map[string]simPayment
}

type simPayment struct {
	scenario string
	created  time.Time
}

var Scenarios = map[string]string{
	"0001": "DEV_SUCCESS", "0002": "DEV_PENDING", "0003": "DEV_FAILED", "0004": "DEV_UNAVAILABLE",
	"0005": "DEV_DUPLICATE_CALLBACK", "0006": "DEV_DELAYED_CALLBACK", "0007": "DEV_CANCELLED",
}

func NewSimulator(name string, delay time.Duration) *Simulator {
	return &Simulator{name: name, delay: delay, client: &http.Client{Timeout: 5 * time.Second}, pending: map[string]simPayment{}}
}

func (s *Simulator) Name() string               { return s.name }
func (s *Simulator) Simulated() bool            { return true }
func (s *Simulator) Configured() (bool, string) { return true, "" }

func scenarioFor(msisdn string) string {
	if len(msisdn) >= 4 {
		if sc, ok := Scenarios[msisdn[len(msisdn)-4:]]; ok {
			return sc
		}
	}
	return "DEV_SUCCESS"
}

func (s *Simulator) Initiate(_ context.Context, in InitiateRequest) (Result, error) {
	sc := scenarioFor(in.MSISDN)
	if sc == "DEV_UNAVAILABLE" {
		return Result{HTTPStatus: 503}, transient(errors.New("simulated provider outage"))
	}
	s.mu.Lock()
	if _, exists := s.pending[in.Reference]; !exists {
		s.pending[in.Reference] = simPayment{scenario: sc, created: time.Now()}
	}
	s.mu.Unlock()
	if sc == "DEV_SUCCESS" || sc == "DEV_DUPLICATE_CALLBACK" || sc == "DEV_FAILED" || sc == "DEV_CANCELLED" {
		go s.callback(in.CallbackURL, in.Reference, sc == "DEV_DUPLICATE_CALLBACK")
	}
	return Result{Status: CustomerActionRequired, HTTPStatus: 202, ProviderTxID: "SIM-" + in.Reference[:8],
		CustomerMessage: "Test mode: no real money moves. The simulated phone approves automatically."}, nil
}

func (s *Simulator) callback(url, ref string, twice bool) {
	time.Sleep(s.delay + 200*time.Millisecond)
	n := 1
	if twice {
		n = 2
	}
	for i := 0; i < n; i++ {
		body, _ := json.Marshal(map[string]string{"referenceId": ref, "status": "UPDATED"})
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := s.client.Do(req); err == nil {
			resp.Body.Close()
		}
	}
}

func (s *Simulator) Status(_ context.Context, reference, _ string) (Result, error) {
	s.mu.Lock()
	p, ok := s.pending[reference]
	s.mu.Unlock()
	if !ok {
		return Result{Status: Pending, HTTPStatus: 200}, nil
	}
	if time.Since(p.created) < s.delay {
		return Result{Status: Pending, HTTPStatus: 200}, nil
	}
	tx := "SIM-" + strings.ToUpper(reference[:8])
	switch p.scenario {
	case "DEV_PENDING":
		return Result{Status: Pending, HTTPStatus: 200}, nil
	case "DEV_FAILED":
		return Result{Status: Failed, HTTPStatus: 200, FailureCode: "NOT_ENOUGH_FUNDS",
			FailureMessage: "There is not enough balance on this Mobile Money account."}, nil
	case "DEV_CANCELLED":
		return Result{Status: Cancelled, HTTPStatus: 200, FailureMessage: "The payment was declined on the phone."}, nil
	default:
		return Result{Status: Succeeded, HTTPStatus: 200, ProviderTxID: tx}, nil
	}
}

func (s *Simulator) ParseCallback(r *http.Request, body []byte) (CallbackEvent, error) {
	var cb struct {
		ReferenceID string `json:"referenceId"`
	}
	_ = json.Unmarshal(body, &cb)
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = cb.ReferenceID
	}
	if ref == "" {
		return CallbackEvent{}, errors.New("callback without reference")
	}
	// Each delivery has the same key, so duplicate callbacks are recognized and ignored.
	return CallbackEvent{EventKey: "cb:" + ref, Reference: ref, Payload: map[string]any{"simulated": true}}, nil
}
