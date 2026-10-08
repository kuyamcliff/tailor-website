// Package httpx contains HTTP helpers shared by all API handlers: a typed error model,
// JSON encoding/decoding with size limits, and pagination parsing.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Error is an API error with a stable machine code and a safe customer-facing message.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.cause }

func NewError(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func BadRequest(msg string) *Error { return NewError(http.StatusBadRequest, "bad_request", msg) }
func NotFound(msg string) *Error   { return NewError(http.StatusNotFound, "not_found", msg) }
func Forbidden() *Error {
	return NewError(http.StatusForbidden, "forbidden", "You do not have access to this resource.")
}
func Unauthorized() *Error {
	return NewError(http.StatusUnauthorized, "unauthorized", "Please sign in to continue.")
}
func Conflict(code, msg string) *Error { return NewError(http.StatusConflict, code, msg) }

// Validation builds a 422 error carrying per-field messages.
func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed",
		Message: "Please check the highlighted fields.", Fields: fields}
}

func Internal(cause error) *Error {
	return &Error{Status: http.StatusInternalServerError, Code: "internal_error",
		Message: "Something went wrong on our side. Please try again.", cause: cause}
}

// Fields accumulates validation errors.
type Fields map[string]string

func (f Fields) Add(field, msg string) {
	if _, exists := f[field]; !exists {
		f[field] = msg
	}
}

func (f Fields) Err() error {
	if len(f) == 0 {
		return nil
	}
	return Validation(f)
}

type ctxKey int

const loggerKey ctxKey = iota

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

func Logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// JSON writes v as JSON with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError converts any error into a safe JSON response. Unknown errors never leak details.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if errors.Is(err, pgx.ErrNoRows) {
		apiErr = NotFound("We could not find what you were looking for.")
	} else if !errors.As(err, &apiErr) {
		apiErr = Internal(err)
	}
	if apiErr.Status >= 500 {
		Logger(r.Context()).Error("request failed", "error", err.Error(), "path", r.URL.Path)
	}
	JSON(w, apiErr.Status, map[string]any{"error": apiErr})
}

// Handler adapts error-returning handlers to http.HandlerFunc.
type Handler func(w http.ResponseWriter, r *http.Request) error

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err)
	}
}

const maxJSONBody = 1 << 20

// Decode reads a JSON body into dst, rejecting unknown fields and oversized bodies.
func Decode(r *http.Request, dst any) error {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		return NewError(http.StatusUnsupportedMediaType, "unsupported_media_type", "Expected a JSON request body.")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return BadRequest("The request body is empty.")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return NewError(http.StatusRequestEntityTooLarge, "too_large", "The request body is too large.")
		}
		return BadRequest("The request body is not valid: " + sanitizeDecodeErr(err))
	}
	if dec.More() {
		return BadRequest("The request body must contain a single JSON object.")
	}
	return nil
}

func sanitizeDecodeErr(err error) string {
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syn):
		return "malformed JSON"
	case errors.As(err, &typ):
		return fmt.Sprintf("field %q has the wrong type", typ.Field)
	case strings.HasPrefix(err.Error(), "json: unknown field"):
		return strings.TrimPrefix(err.Error(), "json: ")
	default:
		return "unreadable body"
	}
}

// PathUUID parses a UUID URL parameter.
func PathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, NotFound("We could not find what you were looking for.")
	}
	return id, nil
}

type Page struct {
	Limit  int
	Offset int
}

func ParsePage(r *http.Request, defLimit, maxLimit int) Page {
	p := Page{Limit: defLimit}
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		p.Limit = min(v, maxLimit)
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		p.Offset = v
	}
	return p
}

// List is the standard paginated response envelope.
type List[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func NewList[T any](items []T, total int, p Page) List[T] {
	if items == nil {
		items = []T{}
	}
	return List[T]{Items: items, Total: total, Limit: p.Limit, Offset: p.Offset}
}

// IdempotencyKey reads and validates the Idempotency-Key header.
func IdempotencyKey(r *http.Request) (string, error) {
	k := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(k) < 16 || len(k) > 128 {
		return "", BadRequest("A valid Idempotency-Key header is required.")
	}
	return k, nil
}
