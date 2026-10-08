package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
)

// SMS senders. Phones are stored as E.164 digits without the plus (237677000001).
//
//	orange  Orange SMS API (developer.orange.com, SMS Africa and Middle East): OAuth client
//	        credentials, then POST /smsmessaging/v1/outbound/tel%3A%2B{sender}/requests.
//	twilio  Twilio Messaging: POST /2010-04-01/Accounts/{sid}/Messages.json with basic auth.
//
// Each stays disabled until its credentials are set; SMS_PROVIDER=log is for development only.

// maxSMSChars keeps a message to three concatenated segments.
const maxSMSChars = 459

// smsText turns a notification into one short message: title, body and link, without the email
// footer, cut at a character boundary.
func smsText(subject, body string) string {
	t := strings.TrimSpace(subject)
	if b := strings.TrimSpace(body); b != "" {
		t += ". " + b
	}
	t = strings.Join(strings.Fields(t), " ")
	if utf8.RuneCountInString(t) <= maxSMSChars {
		return t
	}
	r := []rune(t)
	return string(r[:maxSMSChars-3]) + "..."
}

func newSMSSender(cfg config.SMSConfig, log *slog.Logger) Sender {
	switch cfg.Provider {
	case "log":
		return logSender{log: log, kind: "sms"}
	case "orange":
		if ok, why := cfg.OrangeConfigured(); !ok {
			log.Warn("SMS_PROVIDER=orange but " + why + "; SMS disabled")
			return disabledSender{}
		}
		return &orangeSMS{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
	case "twilio":
		if ok, why := cfg.TwilioConfigured(); !ok {
			log.Warn("SMS_PROVIDER=twilio but " + why + "; SMS disabled")
			return disabledSender{}
		}
		return &twilioSMS{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
	default:
		return disabledSender{}
	}
}

// validMSISDN accepts 8 to 15 digits, as E.164 allows.
func validMSISDN(s string) bool {
	if len(s) < 8 || len(s) > 15 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

type orangeSMS struct {
	cfg    config.SMSConfig
	client *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func (o *orangeSMS) Name() string { return "orange" }

func (o *orangeSMS) accessToken(ctx context.Context) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.token != "" && time.Now().Before(o.tokenExp) {
		return o.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.OrangeAPIURL+"/oauth/v3/token", strings.NewReader(form.Encode()))
	req.SetBasicAuth(o.cfg.OrangeClientID, o.cfg.OrangeClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("orange sms token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("orange sms token: status %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string          `json:"access_token"`
		ExpiresIn   json.RawMessage `json:"expires_in"` // documented as a string, sometimes a number
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("orange sms token: unexpected response")
	}
	secs := 3600
	_, _ = fmt.Sscan(strings.Trim(string(tok.ExpiresIn), `"`), &secs)
	o.token = tok.AccessToken
	o.tokenExp = time.Now().Add(time.Duration(max(secs-60, 30)) * time.Second)
	return o.token, nil
}

func (o *orangeSMS) Send(ctx context.Context, to, subject, body string) error {
	if !validMSISDN(to) {
		return errors.New("invalid phone number")
	}
	tok, err := o.accessToken(ctx)
	if err != nil {
		return err
	}
	sender := "tel:+" + o.cfg.OrangeSenderAddress
	msg := map[string]any{"address": "tel:+" + to, "senderAddress": sender,
		"outboundSMSTextMessage": map[string]string{"message": smsText(subject, body)}}
	if o.cfg.OrangeSenderName != "" {
		msg["senderName"] = o.cfg.OrangeSenderName
	}
	payload, _ := json.Marshal(map[string]any{"outboundSMSMessageRequest": msg})
	// The sender goes in the path fully encoded, as Orange documents it: tel%3A%2B2370000.
	endpoint := o.cfg.OrangeAPIURL + "/smsmessaging/v1/outbound/" + url.QueryEscape(sender) + "/requests"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("orange sms: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == http.StatusUnauthorized {
		o.mu.Lock()
		o.token = ""
		o.mu.Unlock()
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("orange sms: status %d", resp.StatusCode)
	}
	return nil
}

type twilioSMS struct {
	cfg    config.SMSConfig
	client *http.Client
}

func (t *twilioSMS) Name() string { return "twilio" }

func (t *twilioSMS) Send(ctx context.Context, to, subject, body string) error {
	if !validMSISDN(to) {
		return errors.New("invalid phone number")
	}
	form := url.Values{"To": {"+" + to}, "Body": {smsText(subject, body)}}
	if t.cfg.TwilioMessagingServiceSID != "" {
		form.Set("MessagingServiceSid", t.cfg.TwilioMessagingServiceSID)
	} else {
		form.Set("From", t.cfg.TwilioFrom)
	}
	endpoint := t.cfg.TwilioAPIURL + "/2010-04-01/Accounts/" + url.PathEscape(t.cfg.TwilioAccountSID) + "/Messages.json"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.SetBasicAuth(t.cfg.TwilioAccountSID, t.cfg.TwilioAuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio sms: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var e struct {
			Code int `json:"code"`
		}
		_ = json.Unmarshal(b, &e)
		return fmt.Errorf("twilio sms: status %d code %d", resp.StatusCode, e.Code)
	}
	return nil
}
