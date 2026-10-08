package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
)

// Orange implements the Orange Money Cameroon merchant payment flow (Orange Money Core APIs):
//
//	POST {base}/token                                  client credentials -> bearer token
//	POST {base}/omcoreapis/{v}/mp/init                 -> payToken
//	POST {base}/omcoreapis/{v}/mp/pay                  prompts the customer to confirm with their PIN
//	GET  {base}/omcoreapis/{v}/mp/paymentstatus/{payToken}
//
// Requests also carry the merchant X-AUTH-TOKEN. Credentials, channel MSISDN and PIN come from the
// merchant's Orange onboarding. The API version defaults to 1.0.2 and can be overridden with
// ORANGE_MONEY_API_VERSION if Orange publishes a new contract.
type Orange struct {
	cfg     config.PaymentsConfig
	client  *http.Client
	version string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func NewOrange(cfg config.PaymentsConfig) *Orange {
	v := os.Getenv("ORANGE_MONEY_API_VERSION")
	if v == "" {
		v = "1.0.2"
	}
	return &Orange{cfg: cfg, client: &http.Client{Timeout: 25 * time.Second}, version: v}
}

func (o *Orange) Name() string    { return "orange" }
func (o *Orange) Simulated() bool { return false }

func (o *Orange) Configured() (bool, string) {
	var missing []string
	for name, v := range map[string]string{
		"ORANGE_MONEY_API_URL": o.cfg.OrangeBaseURL, "ORANGE_MONEY_CLIENT_ID": o.cfg.OrangeClientID,
		"ORANGE_MONEY_CLIENT_SECRET": o.cfg.OrangeClientSecret, "ORANGE_MONEY_AUTH_TOKEN": o.cfg.OrangeAuthToken,
		"ORANGE_MONEY_CHANNEL_MSISDN": o.cfg.OrangeChannelMSISDN, "ORANGE_MONEY_PIN": o.cfg.OrangePIN,
		"ORANGE_MONEY_CALLBACK_SECRET": o.cfg.OrangeCallbackSecret,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return false, "Missing " + strings.Join(missing, ", ")
	}
	if !strings.HasPrefix(o.cfg.OrangeBaseURL, "https://") {
		return false, "ORANGE_MONEY_API_URL must use https"
	}
	return true, ""
}

func (o *Orange) accessToken(ctx context.Context) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.token != "" && time.Now().Before(o.tokenExp) {
		return o.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.OrangeBaseURL+"/token", strings.NewReader(form.Encode()))
	req.SetBasicAuth(o.cfg.OrangeClientID, o.cfg.OrangeClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client.Do(req)
	if err != nil {
		return "", transient(fmt.Errorf("orange token: %w", err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		return "", transient(fmt.Errorf("orange token: status %d", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("orange token rejected: status %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("orange token: unexpected response")
	}
	o.token = tok.AccessToken
	o.tokenExp = time.Now().Add(time.Duration(max(tok.ExpiresIn-60, 30)) * time.Second)
	return o.token, nil
}

type orangeEnvelope struct {
	Message string `json:"message"`
	Data    struct {
		PayToken     string `json:"payToken"`
		Status       string `json:"status"`
		TxnID        string `json:"txnid"`
		InitTxnMsg   string `json:"inittxnmessage"`
		ConfirmTxMsg string `json:"confirmtxnmessage"`
	} `json:"data"`
}

func (o *Orange) call(ctx context.Context, method, path string, payload any) (int, orangeEnvelope, error) {
	var env orangeEnvelope
	tok, err := o.accessToken(ctx)
	if err != nil {
		return 0, env, err
	}
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(ctx, method, o.cfg.OrangeBaseURL+"/omcoreapis/"+o.version+path, body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-AUTH-TOKEN", o.cfg.OrangeAuthToken)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return 0, env, transient(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == http.StatusUnauthorized {
		o.mu.Lock()
		o.token = ""
		o.mu.Unlock()
		return resp.StatusCode, env, transient(errors.New("orange: token expired"))
	}
	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		return resp.StatusCode, env, transient(fmt.Errorf("orange: status %d", resp.StatusCode))
	}
	_ = json.Unmarshal(b, &env)
	return resp.StatusCode, env, nil
}

// Orange Money expects the subscriber number in national format (9 digits for Cameroon).
func localMSISDN(e164 string) string {
	return strings.TrimPrefix(e164, "237")
}

func (o *Orange) Initiate(ctx context.Context, in InitiateRequest) (Result, error) {
	status, env, err := o.call(ctx, http.MethodPost, "/mp/init", nil)
	if err != nil {
		return Result{HTTPStatus: status}, err
	}
	if status != http.StatusOK || env.Data.PayToken == "" {
		return Result{Status: Failed, HTTPStatus: status, FailureCode: "init_failed", FailureMessage: "Orange Money could not start the payment."}, nil
	}
	payToken := env.Data.PayToken
	payload := map[string]any{
		"notifUrl":          in.CallbackURL,
		"channelUserMsisdn": o.cfg.OrangeChannelMSISDN,
		"amount":            strconv.FormatInt(in.AmountMinor, 10),
		"subscriberMsisdn":  localMSISDN(in.MSISDN),
		"pin":               o.cfg.OrangePIN,
		"orderId":           in.Reference,
		"description":       truncate(in.Description, 120),
		"payToken":          payToken,
	}
	status, env, err = o.call(ctx, http.MethodPost, "/mp/pay", payload)
	if err != nil {
		// The pay call may or may not have reached Orange; keep the payToken so status checks can resolve it.
		return Result{HTTPStatus: status, ProviderTxID: payToken}, err
	}
	res := Result{HTTPStatus: status, ProviderTxID: payToken, Raw: map[string]any{"status": env.Data.Status}}
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusAccepted {
		res.Status, res.FailureCode, res.FailureMessage = Failed, "pay_rejected", orangeMessage(env.Message)
		return res, nil
	}
	res.Status = mapOrangeStatus(env.Data.Status)
	if res.Status == Pending {
		res.Status = CustomerActionRequired
		res.CustomerMessage = "Confirm the payment on your phone with your Orange Money PIN. Dial #150*50# if you do not see a prompt."
	}
	return res, nil
}

func (o *Orange) Status(ctx context.Context, _ string, payToken string) (Result, error) {
	if payToken == "" {
		return Result{Status: Pending}, nil
	}
	status, env, err := o.call(ctx, http.MethodGet, "/mp/paymentstatus/"+url.PathEscape(payToken), nil)
	if err != nil {
		return Result{HTTPStatus: status}, err
	}
	if status != http.StatusOK {
		return Result{HTTPStatus: status}, fmt.Errorf("orange status: %d", status)
	}
	res := Result{HTTPStatus: status, Status: mapOrangeStatus(env.Data.Status), ProviderTxID: payToken,
		Raw: map[string]any{"status": env.Data.Status, "txnid": env.Data.TxnID}}
	switch res.Status {
	case Failed:
		res.FailureCode, res.FailureMessage = "failed", "The payment could not be completed."
	case Expired:
		res.FailureMessage = "The payment request expired before it was confirmed."
	case Cancelled:
		res.FailureMessage = "The payment was cancelled on the phone."
	}
	return res, nil
}

func mapOrangeStatus(s string) Status {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SUCCESSFULL", "SUCCESSFUL", "SUCCESS":
		return Succeeded
	case "FAILED":
		return Failed
	case "EXPIRED":
		return Expired
	case "CANCELLED", "CANCELED":
		return Cancelled
	default:
		return Pending
	}
}

func orangeMessage(m string) string {
	if strings.Contains(strings.ToLower(m), "balance") {
		return "There is not enough balance on this Orange Money account."
	}
	return "Orange Money could not complete the payment."
}

func (o *Orange) ParseCallback(r *http.Request, body []byte) (CallbackEvent, error) {
	var cb struct {
		Payment struct {
			PayToken string `json:"payToken"`
			Status   string `json:"status"`
			OrderID  string `json:"orderId"`
		} `json:"payment"`
		PayToken string `json:"payToken"`
		Status   string `json:"status"`
	}
	_ = json.Unmarshal(body, &cb)
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = cb.Payment.OrderID
	}
	if ref == "" {
		return CallbackEvent{}, errors.New("callback without reference")
	}
	st := cb.Status
	if st == "" {
		st = cb.Payment.Status
	}
	return CallbackEvent{EventKey: "cb:" + ref + ":" + strings.ToUpper(st), Reference: ref, Payload: map[string]any{"status": st}}, nil
}
