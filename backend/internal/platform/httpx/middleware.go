package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const requestIDKey ctxKey = 100
const clientIPKey ctxKey = 101

func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func ClientIP(ctx context.Context) string {
	if v, ok := ctx.Value(clientIPKey).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RequestContext assigns a request ID, resolves the client IP and attaches a request-scoped logger.
// trustedHops is the number of reverse proxies in front of the API whose X-Forwarded-For entries are trusted.
func RequestContext(base *slog.Logger, trustedHops int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if len(id) < 8 || len(id) > 64 || strings.ContainsAny(id, " \n\r\t") {
				id = newRequestID()
			}
			ip := resolveClientIP(r, trustedHops)
			w.Header().Set("X-Request-ID", id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			ctx = context.WithValue(ctx, clientIPKey, ip)
			ctx = WithLogger(ctx, base.With("request_id", id))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveClientIP(r *http.Request, trustedHops int) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if trustedHops <= 0 {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	// The right-most entries were added by trusted proxies; the client is trustedHops from the right.
	idx := len(parts) - trustedHops
	if idx >= 0 && idx < len(parts) {
		if ip := strings.TrimSpace(parts[idx]); net.ParseIP(ip) != nil {
			return ip
		}
	}
	return host
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog writes one structured log line per request and records metrics.
func AccessLog(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			dur := time.Since(start)
			if m != nil {
				m.Observe(r.Method, route, rec.status, dur)
			}
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			} else if route == "/healthz" || route == "/readyz" || route == "/metrics" {
				level = slog.LevelDebug
			}
			Logger(r.Context()).Log(r.Context(), level, "http request",
				"method", r.Method, "route", route, "status", rec.status,
				"duration_ms", dur.Milliseconds(), "bytes", rec.bytes, "ip", ClientIP(r.Context()))
		})
	}
}

// Recover converts panics into 500 responses without exposing stack traces to clients.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				Logger(r.Context()).Error("panic", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
				WriteError(w, r, Internal(fmt.Errorf("panic: %v", p)))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders sets conservative headers for an API that only returns JSON and files.
func SecurityHeaders(production bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-site")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			if production {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS allows credentialed requests only from an explicit origin allow-list.
func CORS(allowed []string) func(http.Handler) http.Handler {
	set := map[string]bool{}
	for _, o := range allowed {
		set[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && set[origin] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
					h.Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, Idempotency-Key, X-Request-ID, X-Access-Token, X-Guest-Token")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			} else if r.Method == http.MethodOptions && origin != "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter is an in-memory token bucket keyed by client IP and bucket name.
// For multi-instance deployments, place a shared limiter (edge or Redis) in front; this remains a safety net.
type RateLimiter struct {
	Multiplier int
	mu         sync.Mutex
	buckets    map[string]*bucket
	now        func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{buckets: map[string]*bucket{}, now: time.Now}
	go rl.sweep()
	return rl
}

func (rl *RateLimiter) sweep() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		rl.mu.Lock()
		cutoff := rl.now().Add(-30 * time.Minute)
		for k, b := range rl.buckets {
			if b.last.Before(cutoff) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow consumes one token from the bucket identified by key. burst tokens refill over per.
func (rl *RateLimiter) Allow(key string, burst int, per time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(burst), last: now}
		rl.buckets[key] = b
	}
	rate := float64(burst) / per.Seconds()
	b.tokens = min(float64(burst), b.tokens+now.Sub(b.last).Seconds()*rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Limit returns middleware applying a named per-IP limit.
func (rl *RateLimiter) Limit(name string, burst int, per time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.Allow(name+"|"+ClientIP(r.Context()), burst*max(rl.Multiplier, 1), per) {
				w.Header().Set("Retry-After", fmt.Sprint(int(per.Seconds()/float64(burst))+1))
				WriteError(w, r, NewError(http.StatusTooManyRequests, "rate_limited",
					"Too many requests. Please wait a moment and try again."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
