// Package tests contains integration tests that run the real HTTP API against a real PostgreSQL
// database. Set TEST_DATABASE_URL to a disposable database; the schema is dropped and recreated.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/payments"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/seed"
	"github.com/kuyamcliff/tailor-website/backend/internal/server"
	"github.com/kuyamcliff/tailor-website/backend/migrations"
)

type env struct {
	t    *testing.T
	srv  *httptest.Server
	pool *pgxpool.Pool
	app  *server.App
}

var shared *env

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set; skipping integration tests")
		os.Exit(0)
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		fmt.Println("reset:", err)
		os.Exit(1)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		fmt.Println("migrate:", err)
		os.Exit(1)
	}
	os.Setenv("ASSET_MANIFEST_PATH", "/nonexistent")
	if err := seed.Reference(ctx, pool); err != nil {
		fmt.Println("seed:", err)
		os.Exit(1)
	}
	dir, _ := os.MkdirTemp("", "atelier-test-storage")
	cfg := config.Config{Env: config.EnvTest, DatabaseURL: url, SessionTTL: time.Hour, RateLimitMultiplier: 100, AllowedOrigins: []string{"http://localhost:3000"},
		Storage:  config.StorageConfig{Driver: "local", LocalDir: dir},
		Payments: config.PaymentsConfig{DevSimulator: true, Currency: "XAF"},
		Email:    config.EmailConfig{Provider: "disabled"}, SMS: config.SMSConfig{Provider: "disabled"},
		ReferenceAnalysisProvider: "disabled"}
	app, err := server.New(ctx, cfg, pool, log)
	if err != nil {
		fmt.Println("server:", err)
		os.Exit(1)
	}
	srv := httptest.NewServer(app.Router)
	app.Payments.APIURL = srv.URL
	for _, name := range []string{"mtn", "orange"} {
		app.Payments.UseProvider(payments.NewSimulator(name, 150*time.Millisecond), "test-callback-secret")
	}
	shared = &env{srv: srv, pool: pool, app: app}
	if err := seedTestCatalog(ctx, pool); err != nil {
		fmt.Println("catalog:", err)
		os.Exit(1)
	}
	code := m.Run()
	srv.Close()
	pool.Close()
	os.RemoveAll(dir)
	os.Exit(code)
}

// seedTestCatalog creates deterministic fabrics, a product and business settings for tests.
func seedTestCatalog(ctx context.Context, pool *pgxpool.Pool) error {
	stmts := []string{
		`UPDATE business_settings SET value = value || '{"name":"Test Atelier","taxRateBp":0,"delivery":[{"key":"pickup","method":"pickup","label":"Studio pickup","feeMinor":0,"active":true},{"key":"city","method":"local_delivery","label":"City delivery","feeMinor":2000,"active":true}]}'::jsonb WHERE key='business'`,
		`INSERT INTO fabrics (key, name, material_type, stock_status, price_impact_minor, suitable_garments) VALUES ('test-wool','Test wool','Wool','available',40000,'{suit,jacket,trousers}')`,
		`INSERT INTO fabric_colors (fabric_id, key, name, hex) SELECT id, 'navy', 'Navy', '#1f2a44' FROM fabrics WHERE key='test-wool'`,
		`INSERT INTO fabrics (key, name, material_type, stock_status, suitable_garments, active, archived_at) VALUES ('old-cloth','Old cloth','Wool','discontinued','{}', false, now())`,
		`INSERT INTO products (slug, name, visibility, price_minor, category_id) VALUES ('test-shirt','Test shirt','published',30000,(SELECT id FROM product_categories WHERE slug='shirts'))`,
		`INSERT INTO product_variants (product_id, sku, size_label, stock_qty) SELECT id, 'test-shirt-40', '40', 2 FROM products WHERE slug='test-shirt'`,
		`INSERT INTO product_variants (product_id, sku, size_label, stock_qty) SELECT id, 'test-shirt-42', '42', 1 FROM products WHERE slug='test-shirt'`,
		`UPDATE feature_flags SET enabled=true WHERE key IN ('online_payments','payments_mtn','payments_orange')`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", s[:40], err)
		}
	}
	return nil
}

// client is an HTTP client with a cookie jar, CSRF handling and an optional guest token.
type client struct {
	t     *testing.T
	http  *http.Client
	csrf  string
	guest string
	token string // access token for guest links
}

func newClient(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}, guest: strings.Repeat("g", 8) + uuid.NewString()}
}

type resp struct {
	status int
	body   []byte
}

func (r resp) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode %d %s: %v", r.status, r.body, err)
	}
}

func (r resp) m(t *testing.T) map[string]any {
	var out map[string]any
	r.json(t, &out)
	return out
}

func (c *client) do(method, path string, body any, headers ...string) resp {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, shared.srv.URL+"/api/v1"+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	req.Header.Set("X-Guest-Token", c.guest)
	if c.token != "" {
		req.Header.Set("X-Access-Token", c.token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{status: res.StatusCode, body: b}
}

func (c *client) idem(method, path string, body any) resp {
	return c.do(method, path, body, "Idempotency-Key", "test-"+uuid.NewString())
}

func expect(t *testing.T, r resp, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("expected %d, got %d: %s", status, r.status, r.body)
	}
}

func (c *client) signIn(identifier, password string) {
	c.t.Helper()
	r := c.do("POST", "/auth/signin", map[string]any{"identifier": identifier, "password": password})
	expect(c.t, r, 200)
	var me struct {
		User struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"user"`
	}
	r.json(c.t, &me)
	c.csrf = me.User.CSRFToken
}

func ownerClient(t *testing.T) *client {
	t.Helper()
	email := "owner-" + uuid.NewString()[:8] + "@atelier.test"
	_, err := shared.pool.Exec(context.Background(), `INSERT INTO users (email, password_hash, full_name, role) VALUES ($1, $2, 'Test Owner', 'owner')`,
		email, mustHash(t, "owner-password-123"))
	if err != nil {
		t.Fatal(err)
	}
	c := newClient(t)
	c.signIn(email, "owner-password-123")
	return c
}

func (c *client) upload(purpose string, img []byte, name string) resp {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(img)
	mw.Close()
	req, _ := http.NewRequest("POST", shared.srv.URL+"/api/v1/uploads?purpose="+purpose, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Guest-Token", c.guest)
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{status: res.StatusCode, body: b}
}

// waitFor polls fn until it returns true or the deadline passes.
func waitFor(t *testing.T, d time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

func contact(phone string) map[string]any {
	return map[string]any{"name": "Ada Customer", "phone": phone, "email": "ada@example.test", "preferredContact": "whatsapp"}
}
