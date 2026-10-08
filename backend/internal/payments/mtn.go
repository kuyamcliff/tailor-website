package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/money"
)

// MTN implements the MTN Mobile Money Collection API (request to pay):
//
//	POST {base}/collection/token/                       basic auth (API user, API key) -> bearer token
//	POST {base}/collection/v1_0/requesttopay            X-Reference-Id = our reference (idempotent)
//	GET  {base}/collection/v1_0/requesttopay/{ref}      PENDING | SUCCESSFUL | FAILED
//
// The exact base URL, target environment (sandbox or the merchant's market, e.g. mtncameroon) and
// subscription key come from the merchant's MTN onboarding and are injected via environment variables.
type MTN struct {
	cfg    config.PaymentsConfig
	client *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func NewMTN(cfg config.PaymentsConfig) *MTN {
	return &MTN{cfg: cfg, client: &http.Client{Timeout: 20 * time.Second}}
}

func (m *MTN) Name() string    { return "mtn" }
func (m *MTN) Simulated() bool { return false }

func (m *MTN) Configured() (bool, string) {
	var missing []string
	for name, v := range map[string]string{
		"MTN_MOMO_API_URL": m.cfg.MTNBaseURL, "MTN_MOMO_TARGET_ENVIRONMENT": m.cfg.MTNTargetEnv,
		"MTN_MOMO_SUBSCRIPTION_KEY": m.cfg.MTNSubscriptionKey, "MTN_MOMO_API_USER": m.cfg.MTNAPIUser,
		"MTN_MOMO_API_KEY": m.cfg.MTNAPIKey, "MTN_MOMO_CALLBACK_SECRET": m.cfg.MTNCallbackSecret,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return false, "Missing " + strings.Join(missing, ", ")
	}
	if !strings.HasPrefix(m.cfg.MTNBaseURL, "https://") {
		return false, "MTN_MOMO_API_URL must use https"
	}
	return true, ""
}

func (m *MTN) accessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token != "" && time.Now().Before(m.tokenExp) {
		return m.token, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.MTNBaseURL+"/collection/token/", nil)
	req.SetBasicAuth(m.cfg.MTNAPIUser, m.cfg.MTNAPIKey)
	req.Header.Set("Ocp-Apim-Subscription-Key", m.cfg.MTNSubscriptionKey)
	resp, err := m.client.Do(req)
	if err != nil {
		return "", transient(fmt.Errorf("mtn token: %w", err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		return "", transient(fmt.Errorf("mtn token: status %d", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mtn token rejected: status %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("mtn token: unexpected response")
	}
	m.token = tok.AccessToken
	m.tokenExp = time.Now().Add(time.Duration(max(tok.ExpiresIn-60, 30)) * time.Second)
	return m.token, nil
}

func (m *MTN) do(ctx context.Context, method, path string, headers map[string]string, payload any) (int, []byte, error) {
	tok, err := m.accessToken(ctx)
	if err != nil {
		return 0, nil, err
	}
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(ctx, method, m.cfg.MTNBaseURL+path, body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Target-Environment", m.cfg.MTNTargetEnv)
	req.Header.Set("Ocp-Apim-Subscription-Key", m.cfg.MTNSubscriptionKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return 0, nil, transient(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == http.StatusUnauthorized {
		m.mu.Lock()
		m.token = ""
		m.mu.Unlock()
		return resp.StatusCode, b, transient(errors.New("mtn: token expired"))
	}
	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		return resp.StatusCode, b, transient(fmt.Errorf("mtn: status %d", resp.StatusCode))
	}
	return resp.StatusCode, b, nil
}

// MoMo amounts are strings in major units. XAF has no minor unit.
func majorAmount(amount int64, currency string) string {
	c, err := money.Lookup(currency)
	if err != nil || c.Exponent == 0 {
		return strconv.FormatInt(amount, 10)
	}
	div := int64(1)
	for i := 0; i < c.Exponent; i++ {
		div *= 10
	}
	return fmt.Sprintf("%d.%0*d", amount/div, c.Exponent, amount%div)
}

func (m *MTN) Initiate(ctx context.Context, in InitiateRequest) (Result, error) {
	payload := map[string]any{
		"amount":       majorAmount(in.AmountMinor, in.Currency),
		"currency":     in.Currency,
		"externalId":   in.PaymentID,
		"payer":        map[string]string{"partyIdType": "MSISDN", "partyId": in.MSISDN},
		"payerMessage": truncate(in.Description, 160),
		"payeeNote":    truncate(in.Description, 160),
	}
	status, body, err := m.do(ctx, http.MethodPost, "/collection/v1_0/requesttopay",
		map[string]string{"X-Reference-Id": in.Reference, "X-Callback-Url": in.CallbackURL}, payload)
	if err != nil {
		return Result{HTTPStatus: status}, err
	}
	switch status {
	case http.StatusAccepted:
		return Result{Status: CustomerActionRequired, HTTPStatus: status,
			CustomerMessage: "Approve the payment on your phone. Dial *126# if you do not see a prompt."}, nil
	case http.StatusConflict:
		// Same reference already submitted: the original request stands, check its status instead.
		return m.Status(ctx, in.Reference, "")
	default:
		code, msg := mtnError(body)
		return Result{Status: Failed, HTTPStatus: status, FailureCode: code, FailureMessage: msg}, nil
	}
}

func (m *MTN) Status(ctx context.Context, reference, _ string) (Result, error) {
	status, body, err := m.do(ctx, http.MethodGet, "/collection/v1_0/requesttopay/"+reference, nil, nil)
	if err != nil {
		return Result{HTTPStatus: status}, err
	}
	if status == http.StatusNotFound {
		return Result{Status: Pending, HTTPStatus: status}, nil
	}
	if status != http.StatusOK {
		code, msg := mtnError(body)
		return Result{HTTPStatus: status, FailureCode: code, FailureMessage: msg}, fmt.Errorf("mtn status: %d", status)
	}
	var r struct {
		Status                 string `json:"status"`
		FinancialTransactionID string `json:"financialTransactionId"`
		Reason                 any    `json:"reason"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return Result{HTTPStatus: status}, errors.New("mtn status: unexpected response")
	}
	res := Result{HTTPStatus: status, ProviderTxID: r.FinancialTransactionID, Raw: map[string]any{"status": r.Status}}
	switch strings.ToUpper(r.Status) {
	case "SUCCESSFUL":
		res.Status = Succeeded
	case "FAILED":
		res.Status = Failed
		res.FailureCode = fmt.Sprint(r.Reason)
		res.FailureMessage = mtnReason(res.FailureCode)
	case "REJECTED":
		res.Status = Cancelled
		res.FailureMessage = "The payment was declined on the phone."
	case "TIMEOUT", "EXPIRED":
		res.Status = Expired
		res.FailureMessage = "The payment request expired before it was approved."
	default:
		res.Status = Pending
	}
	return res, nil
}

func (m *MTN) ParseCallback(r *http.Request, body []byte) (CallbackEvent, error) {
	var cb struct {
		ReferenceID            string `json:"referenceId"`
		ExternalID             string `json:"externalId"`
		Status                 string `json:"status"`
		FinancialTransactionID string `json:"financialTransactionId"`
	}
	_ = json.Unmarshal(body, &cb)
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = cb.ReferenceID
	}
	if ref == "" {
		return CallbackEvent{}, errors.New("callback without reference")
	}
	return CallbackEvent{
		EventKey:  "cb:" + ref + ":" + strings.ToUpper(cb.Status) + ":" + cb.FinancialTransactionID,
		Reference: ref,
		Payload:   map[string]any{"status": cb.Status, "financialTransactionId": cb.FinancialTransactionID},
	}, nil
}

func mtnError(body []byte) (string, string) {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	return e.Code, mtnReason(e.Code)
}

func mtnReason(code string) string {
	switch strings.ToUpper(code) {
	case "PAYER_NOT_FOUND", "PAYEE_NOT_FOUND":
		return "This number is not registered for MTN Mobile Money."
	case "NOT_ENOUGH_FUNDS":
		return "There is not enough balance on this Mobile Money account."
	case "PAYER_LIMIT_REACHED":
		return "This account has reached its Mobile Money limit."
	case "APPROVAL_REJECTED", "NOT_ALLOWED":
		return "The payment was declined."
	case "EXPIRED":
		return "The payment request expired before it was approved."
	default:
		return "The payment could not be completed."
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
