// Package config loads runtime configuration from environment variables.
// Secrets are only ever read from the environment; nothing here has a secret default.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvProduction  Env = "production"
)

type Config struct {
	Env            Env
	HTTPAddr       string
	DatabaseURL    string
	PublicSiteURL  string // e.g. https://atelier.example.com, used for links and callbacks
	PublicAPIURL   string // externally reachable API base, used for provider callbacks
	AllowedOrigins []string
	TrustedProxies int
	SessionTTL     time.Duration
	CookieSecure   bool
	CookieDomain   string
	AutoMigrate    bool
	InternalAPIKey string // shared secret for server-side rendering requests from the frontend
	// RateLimitMultiplier scales every per-IP rate limit (tests and load tests raise it).
	RateLimitMultiplier int

	Storage  StorageConfig
	Payments PaymentsConfig
	Email    EmailConfig
	SMS      SMSConfig

	ReferenceAnalysisProvider string
	ReferenceAnalysisAPIKey   string
	ReferenceAnalysisModel    string
	BodyEstimationProvider    string
	BodyEstimationAPIKey      string

	BootstrapOwnerEmail    string
	BootstrapOwnerPassword string
	BootstrapOwnerName     string
}

type StorageConfig struct {
	Driver        string // "local" or "s3"
	LocalDir      string
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	PublicBaseURL string // CDN base for public objects, optional
}

type PaymentsConfig struct {
	// DevSimulator enables deterministic simulated providers. Refused in production.
	DevSimulator bool
	Currency     string

	MTNBaseURL         string
	MTNTargetEnv       string
	MTNSubscriptionKey string
	MTNAPIUser         string
	MTNAPIKey          string
	MTNCallbackSecret  string

	OrangeBaseURL        string
	OrangeClientID       string
	OrangeClientSecret   string
	OrangeAuthToken      string // X-AUTH-TOKEN merchant header
	OrangeChannelMSISDN  string
	OrangePIN            string
	OrangeCallbackSecret string
}

type EmailConfig struct {
	Provider string // "disabled", "log", or "smtp"
	From     string
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
}

type SMSConfig struct {
	Provider string // "disabled" or "log" until a real SMS contract is chosen
	APIKey   string
}

func Load() (Config, error) {
	c := Config{
		Env:                 Env(get("APP_ENV", "development")),
		HTTPAddr:            get("HTTP_ADDR", ":8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		PublicSiteURL:       strings.TrimRight(get("PUBLIC_SITE_URL", "http://localhost:3000"), "/"),
		PublicAPIURL:        strings.TrimRight(get("PUBLIC_API_URL", "http://localhost:8080"), "/"),
		AllowedOrigins:      splitList(get("ALLOWED_ORIGINS", "http://localhost:3000")),
		TrustedProxies:      getInt("TRUSTED_PROXY_HOPS", 0),
		SessionTTL:          time.Duration(getInt("SESSION_TTL_HOURS", 24*30)) * time.Hour,
		CookieDomain:        os.Getenv("COOKIE_DOMAIN"),
		AutoMigrate:         getBool("AUTO_MIGRATE", false),
		InternalAPIKey:      os.Getenv("INTERNAL_API_KEY"),
		RateLimitMultiplier: getInt("RATE_LIMIT_MULTIPLIER", 1),
		Storage: StorageConfig{
			Driver:        get("STORAGE_DRIVER", "local"),
			LocalDir:      get("STORAGE_LOCAL_DIR", "./data/storage"),
			Endpoint:      os.Getenv("STORAGE_ENDPOINT"),
			Region:        get("STORAGE_REGION", "us-east-1"),
			Bucket:        os.Getenv("STORAGE_BUCKET"),
			AccessKey:     os.Getenv("STORAGE_ACCESS_KEY"),
			SecretKey:     os.Getenv("STORAGE_SECRET_KEY"),
			UseSSL:        getBool("STORAGE_USE_SSL", true),
			PublicBaseURL: strings.TrimRight(os.Getenv("STORAGE_PUBLIC_BASE_URL"), "/"),
		},
		Payments: PaymentsConfig{
			DevSimulator:         getBool("PAYMENTS_DEV_SIMULATOR", false),
			Currency:             get("CURRENCY", "XAF"),
			MTNBaseURL:           strings.TrimRight(os.Getenv("MTN_MOMO_API_URL"), "/"),
			MTNTargetEnv:         os.Getenv("MTN_MOMO_TARGET_ENVIRONMENT"),
			MTNSubscriptionKey:   os.Getenv("MTN_MOMO_SUBSCRIPTION_KEY"),
			MTNAPIUser:           os.Getenv("MTN_MOMO_API_USER"),
			MTNAPIKey:            os.Getenv("MTN_MOMO_API_KEY"),
			MTNCallbackSecret:    os.Getenv("MTN_MOMO_CALLBACK_SECRET"),
			OrangeBaseURL:        strings.TrimRight(os.Getenv("ORANGE_MONEY_API_URL"), "/"),
			OrangeClientID:       os.Getenv("ORANGE_MONEY_CLIENT_ID"),
			OrangeClientSecret:   os.Getenv("ORANGE_MONEY_CLIENT_SECRET"),
			OrangeAuthToken:      os.Getenv("ORANGE_MONEY_AUTH_TOKEN"),
			OrangeChannelMSISDN:  os.Getenv("ORANGE_MONEY_CHANNEL_MSISDN"),
			OrangePIN:            os.Getenv("ORANGE_MONEY_PIN"),
			OrangeCallbackSecret: os.Getenv("ORANGE_MONEY_CALLBACK_SECRET"),
		},
		Email: EmailConfig{
			Provider: get("EMAIL_PROVIDER", "log"),
			From:     get("EMAIL_FROM", ""),
			SMTPHost: os.Getenv("SMTP_HOST"),
			SMTPPort: getInt("SMTP_PORT", 587),
			SMTPUser: os.Getenv("SMTP_USER"),
			SMTPPass: os.Getenv("SMTP_PASSWORD"),
		},
		SMS: SMSConfig{
			Provider: get("SMS_PROVIDER", "log"),
			APIKey:   os.Getenv("SMS_PROVIDER_API_KEY"),
		},
		ReferenceAnalysisProvider: get("AI_REFERENCE_PROVIDER", "disabled"),
		ReferenceAnalysisAPIKey:   os.Getenv("AI_REFERENCE_API_KEY"),
		ReferenceAnalysisModel:    os.Getenv("AI_REFERENCE_MODEL"),
		BodyEstimationProvider:    get("BODY_ESTIMATION_PROVIDER", "disabled"),
		BodyEstimationAPIKey:      os.Getenv("BODY_ESTIMATION_API_KEY"),
		BootstrapOwnerEmail:       os.Getenv("BOOTSTRAP_OWNER_EMAIL"),
		BootstrapOwnerPassword:    os.Getenv("BOOTSTRAP_OWNER_PASSWORD"),
		BootstrapOwnerName:        get("BOOTSTRAP_OWNER_NAME", "Owner"),
	}
	c.CookieSecure = getBool("COOKIE_SECURE", c.Env == EnvProduction)
	return c, c.Validate()
}

// Validate refuses unsafe production configurations instead of silently degrading.
func (c Config) Validate() error {
	var errs []error
	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test or production, got %q", c.Env))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.Storage.Driver != "local" && c.Storage.Driver != "s3" {
		errs = append(errs, fmt.Errorf("STORAGE_DRIVER must be local or s3"))
	}
	if c.Env == EnvProduction {
		if c.Payments.DevSimulator {
			errs = append(errs, errors.New("PAYMENTS_DEV_SIMULATOR must not be enabled in production"))
		}
		if !c.CookieSecure {
			errs = append(errs, errors.New("COOKIE_SECURE must be true in production"))
		}
		if !strings.HasPrefix(c.PublicSiteURL, "https://") || !strings.HasPrefix(c.PublicAPIURL, "https://") {
			errs = append(errs, errors.New("PUBLIC_SITE_URL and PUBLIC_API_URL must use https in production"))
		}
		if len(c.InternalAPIKey) < 32 {
			errs = append(errs, errors.New("INTERNAL_API_KEY must be at least 32 characters in production"))
		}
		if c.Email.Provider == "log" {
			errs = append(errs, errors.New("EMAIL_PROVIDER=log is not allowed in production; use smtp or disabled"))
		}
	}
	return errors.Join(errs...)
}

func (c Config) IsProduction() bool { return c.Env == EnvProduction }

func get(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}
