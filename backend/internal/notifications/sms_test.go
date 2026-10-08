package notifications

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
)

func TestSMSText(t *testing.T) {
	got := smsText("Your quote Q-12", "Your quote is ready.\n\nhttps://atelier.example/quotes/1")
	if got != "Your quote Q-12. Your quote is ready. https://atelier.example/quotes/1" {
		t.Errorf("smsText = %q", got)
	}
	long := smsText("Brodé", strings.Repeat("é", 600))
	if utf8.RuneCountInString(long) != maxSMSChars || !utf8.ValidString(long) || !strings.HasSuffix(long, "...") {
		t.Errorf("long message %d characters", utf8.RuneCountInString(long))
	}
}

func TestSMSSenderSelection(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := []struct {
		cfg  config.SMSConfig
		want string
	}{
		{config.SMSConfig{Provider: "disabled"}, "disabled"},
		{config.SMSConfig{Provider: "log"}, "log"},
		{config.SMSConfig{Provider: "orange", OrangeAPIURL: "https://api.orange.com"}, "disabled"}, // no credentials
		{config.SMSConfig{Provider: "twilio", TwilioAPIURL: "https://api.twilio.com", TwilioAccountSID: "AC1", TwilioAuthToken: "t"}, "disabled"},
		{config.SMSConfig{Provider: "orange", OrangeAPIURL: "http://insecure", OrangeClientID: "a", OrangeClientSecret: "b", OrangeSenderAddress: "2370000"}, "disabled"},
		{config.SMSConfig{Provider: "orange", OrangeAPIURL: "https://api.orange.com", OrangeClientID: "a", OrangeClientSecret: "b", OrangeSenderAddress: "2370000"}, "orange"},
		{config.SMSConfig{Provider: "twilio", TwilioAPIURL: "https://api.twilio.com", TwilioAccountSID: "AC1", TwilioAuthToken: "t", TwilioFrom: "Atelier"}, "twilio"},
	}
	for i, c := range cases {
		if got := newSMSSender(c.cfg, log).Name(); got != c.want {
			t.Errorf("case %d: sender %s, want %s", i, got, c.want)
		}
	}
}

func TestOrangeSMS(t *testing.T) {
	var tokens, sends atomic.Int32
	var gotPath string
	var got map[string]map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth/v3/token":
			u, p, _ := r.BasicAuth()
			b, _ := io.ReadAll(r.Body)
			if u != "client" || p != "secret" || string(b) != "grant_type=client_credentials" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			tokens.Add(1)
			_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"sms-token","expires_in":"3600"}`))
		case strings.HasPrefix(r.URL.Path, "/smsmessaging/v1/outbound/"):
			if r.Header.Get("Authorization") != "Bearer sms-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			sends.Add(1)
			gotPath = r.URL.EscapedPath()
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	o := &orangeSMS{cfg: config.SMSConfig{OrangeAPIURL: srv.URL, OrangeClientID: "client", OrangeClientSecret: "secret",
		OrangeSenderAddress: "2370000", OrangeSenderName: "Atelier"}, client: srv.Client()}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := o.Send(ctx, "237677000001", "Ready for pickup", "Order A-7 is ready."); err != nil {
			t.Fatal(err)
		}
	}
	if tokens.Load() != 1 || sends.Load() != 2 {
		t.Errorf("tokens %d sends %d; the token should be reused", tokens.Load(), sends.Load())
	}
	if gotPath != "/smsmessaging/v1/outbound/tel%3A%2B2370000/requests" {
		t.Errorf("send path %q", gotPath)
	}
	req := got["outboundSMSMessageRequest"]
	if req["address"] != "tel:+237677000001" || req["senderAddress"] != "tel:+2370000" || req["senderName"] != "Atelier" {
		t.Errorf("request %+v", req)
	}
	if msg := req["outboundSMSTextMessage"].(map[string]any)["message"]; msg != "Ready for pickup. Order A-7 is ready." {
		t.Errorf("message %q", msg)
	}
	if err := o.Send(ctx, "not-a-number", "x", "y"); err == nil {
		t.Error("invalid number accepted")
	}
}

func TestTwilioSMS(t *testing.T) {
	var form url.Values
	status := http.StatusCreated
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if r.URL.Path != "/2010-04-01/Accounts/AC123/Messages.json" || u != "AC123" || p != "auth" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = r.ParseForm()
		form = r.PostForm
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"code":21211}`))
	}))
	defer srv.Close()
	tw := &twilioSMS{cfg: config.SMSConfig{TwilioAPIURL: srv.URL, TwilioAccountSID: "AC123", TwilioAuthToken: "auth", TwilioFrom: "Atelier"}, client: srv.Client()}
	if err := tw.Send(context.Background(), "237677000001", "Fitting booked", "Thursday at 10:00."); err != nil {
		t.Fatal(err)
	}
	if form.Get("To") != "+237677000001" || form.Get("From") != "Atelier" || form.Get("Body") != "Fitting booked. Thursday at 10:00." {
		t.Errorf("form %v", form)
	}
	tw.cfg.TwilioMessagingServiceSID = "MG1"
	_ = tw.Send(context.Background(), "237677000001", "a", "b")
	if form.Get("MessagingServiceSid") != "MG1" || form.Get("From") != "" {
		t.Errorf("messaging service form %v", form)
	}
	status = http.StatusBadRequest
	if err := tw.Send(context.Background(), "237677000001", "a", "b"); err == nil || !strings.Contains(err.Error(), "21211") {
		t.Errorf("provider error not reported: %v", err)
	}
}
