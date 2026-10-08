// Package uploads validates, sanitizes and stores customer and staff files, and serves them
// with per-object authorization. Customer references are private by default.
package uploads

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register WebP decoder

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/tokens"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
)

const (
	MaxImageBytes  = 15 << 20
	MaxAssetBytes  = 80 << 20
	minImageSide   = 200
	maxImageSide   = 12000
	maxImagePixels = 60_000_000
	GuestHeader    = "X-Guest-Token"
)

var allowedImageMIME = map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/webp": "webp"}

// customerPurposes may be uploaded by customers and guests; everything else needs staff permissions.
var customerPurposes = map[string]bool{"reference": true, "support": true, "body_photo": true}

var staffPurposePerm = map[string]string{
	"portfolio": "portfolio.write", "product": "products.write", "fabric": "fabrics.write",
	"content": "content.write", "asset": "assets.write",
}

type Service struct {
	Pool     *pgxpool.Pool
	Store    Storage
	Settings *settings.Service
	Log      *slog.Logger
}

type Upload struct {
	ID           uuid.UUID         `json:"id"`
	Purpose      string            `json:"purpose"`
	Visibility   string            `json:"visibility"`
	OriginalName string            `json:"originalName"`
	MIME         string            `json:"mime"`
	Bytes        int64             `json:"bytes"`
	Width        *int              `json:"width"`
	Height       *int              `json:"height"`
	SHA256       string            `json:"sha256"`
	URL          string            `json:"url"`
	Variants     map[string]string `json:"variants"`
	CreatedAt    time.Time         `json:"createdAt"`
	Duplicate    bool              `json:"duplicate,omitempty"`
}

// GuestToken extracts and validates the anonymous ownership token.
func GuestToken(r *http.Request) string {
	t := strings.TrimSpace(r.Header.Get(GuestHeader))
	if len(t) < 32 || len(t) > 128 {
		return ""
	}
	return t
}

// Processed is a validated, metadata-free image with its display derivatives.
type Processed struct {
	Main     []byte
	MIME     string
	Width    int
	Height   int
	Variants map[string][]byte
}

// ProcessImage validates actual image content (not the extension or declared type), rejects
// decompression bombs, strips all metadata by re-encoding, and produces display derivatives.
func ProcessImage(data []byte) (*Processed, error) {
	sniffed := http.DetectContentType(data)
	if _, ok := allowedImageMIME[sniffed]; !ok {
		return nil, httpx.NewError(http.StatusUnsupportedMediaType, "unsupported_file",
			"This file type is not supported. Upload a JPEG, PNG or WebP image.")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || allowedImageMIME[sniffed] != format {
		return nil, httpx.NewError(http.StatusUnprocessableEntity, "invalid_image", "This image appears to be damaged. Try another file.")
	}
	if cfg.Width < minImageSide || cfg.Height < minImageSide {
		return nil, httpx.NewError(http.StatusUnprocessableEntity, "image_too_small",
			fmt.Sprintf("Images must be at least %d by %d pixels.", minImageSide, minImageSide))
	}
	if cfg.Width > maxImageSide || cfg.Height > maxImageSide || cfg.Width*cfg.Height > maxImagePixels {
		return nil, httpx.NewError(http.StatusUnprocessableEntity, "image_too_large", "This image has too many pixels. Resize it and try again.")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, httpx.NewError(http.StatusUnprocessableEntity, "invalid_image", "This image appears to be damaged. Try another file.")
	}
	out := &Processed{Width: cfg.Width, Height: cfg.Height, Variants: map[string][]byte{}}
	// PNG keeps transparency; everything else becomes a clean JPEG with no EXIF/GPS metadata.
	if format == "png" && hasAlpha(img) {
		var buf bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
			return nil, err
		}
		out.Main, out.MIME = buf.Bytes(), "image/png"
	} else {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, flatten(img), &jpeg.Options{Quality: 90}); err != nil {
			return nil, err
		}
		out.Main, out.MIME = buf.Bytes(), "image/jpeg"
	}
	for name, side := range map[string]int{"preview": 1600, "thumb": 480} {
		v, err := resizeJPEG(img, side)
		if err != nil {
			return nil, err
		}
		out.Variants[name] = v
	}
	return out, nil
}

func hasAlpha(img image.Image) bool {
	switch i := img.(type) {
	case *image.NRGBA:
		return !i.Opaque()
	case *image.RGBA:
		return !i.Opaque()
	case *image.Paletted:
		return !i.Opaque()
	}
	return false
}

func flatten(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, image.White, image.Point{}, draw.Src)
	draw.Draw(dst, b, img, b.Min, draw.Over)
	return dst
}

func resizeJPEG(img image.Image, maxSide int) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxSide || h > maxSide {
		if w >= h {
			h = h * maxSide / w
			w = maxSide
		} else {
			w = w * maxSide / h
			h = maxSide
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sanitizeName(n string) string {
	n = path.Base(strings.ReplaceAll(n, "\\", "/"))
	var b strings.Builder
	for _, r := range n {
		if r >= 32 && r != 127 && r != '"' && r != '<' && r != '>' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	// Cut at a character boundary so a long accented name stays valid UTF-8.
	for len(s) > 120 {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

func (s *Service) urlFor(id uuid.UUID, variant string) string {
	if variant == "" || variant == "original" {
		return "/api/v1/uploads/" + id.String()
	}
	return "/api/v1/uploads/" + id.String() + "/" + variant
}

// Create handles multipart uploads for images.
func (s *Service) Create(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := auth.FromContext(ctx)
	purpose := r.URL.Query().Get("purpose")
	if purpose == "" {
		purpose = "reference"
	}
	if customerPurposes[purpose] {
		if purpose == "body_photo" && !s.Settings.Flag(ctx, "photo_body_estimation") {
			return httpx.NewError(http.StatusServiceUnavailable, "feature_disabled", "Photo measurement estimation is not available.")
		}
		if purpose == "body_photo" && (p == nil || p.CustomerID == nil) {
			return httpx.NewError(http.StatusUnauthorized, "sign_in_required", "Sign in to measure from photos.")
		}
	} else if perm, ok := staffPurposePerm[purpose]; !ok || purpose == "asset" {
		return httpx.BadRequest("Unknown upload purpose.")
	} else if !p.Can(perm) {
		return httpx.Forbidden()
	}
	guest := GuestToken(r)
	if p == nil && guest == "" {
		return httpx.BadRequest("Missing guest session. Refresh the page and try again.")
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxImageBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "Images must be 15 MB or smaller.")
		}
		return httpx.BadRequest("The upload could not be read. Please try again.")
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck
	file, header, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest("Choose a file to upload.")
	}
	defer file.Close()
	if header.Size > MaxImageBytes {
		return httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "Images must be 15 MB or smaller.")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxImageBytes+1))
	if err != nil {
		return httpx.BadRequest("The upload was interrupted. Please try again.")
	}
	if int64(len(data)) > MaxImageBytes {
		return httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "Images must be 15 MB or smaller.")
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])

	// Duplicate detection: the same owner uploading the same bytes gets the existing record back.
	var customerID *uuid.UUID
	var guestHash []byte
	if p != nil && !p.IsStaff {
		customerID = p.CustomerID
	}
	if p == nil {
		guestHash = tokens.Hash(guest)
	}
	if existing, err := s.findDuplicate(ctx, digest, purpose, customerID, guestHash, p); err == nil && existing != nil {
		existing.Duplicate = true
		httpx.JSON(w, http.StatusOK, existing)
		return nil
	}

	img, err := ProcessImage(data)
	if err != nil {
		return err
	}
	id := uuid.New()
	base := fmt.Sprintf("%s/%s/%s", purpose, time.Now().UTC().Format("2006/01"), id)
	ext := ".jpg"
	if img.MIME == "image/png" {
		ext = ".png"
	}
	mainKey := base + "/original" + ext
	if err := s.Store.Put(ctx, mainKey, bytes.NewReader(img.Main), int64(len(img.Main)), img.MIME); err != nil {
		return fmt.Errorf("store original: %w", err)
	}
	derivatives := map[string]string{}
	for name, b := range img.Variants {
		k := base + "/" + name + ".jpg"
		if err := s.Store.Put(ctx, k, bytes.NewReader(b), int64(len(b)), "image/jpeg"); err != nil {
			s.cleanup(ctx, append([]string{mainKey}, values(derivatives)...))
			return fmt.Errorf("store %s: %w", name, err)
		}
		derivatives[name] = k
	}
	visibility := "private"
	if !customerPurposes[purpose] {
		visibility = "public"
	}
	var retention *time.Time
	if customerPurposes[purpose] {
		biz, err := s.Settings.Business(ctx, s.Pool)
		if err == nil {
			t := time.Now().AddDate(0, 0, biz.RetentionDays)
			if purpose == "body_photo" {
				// Body photos are deleted as soon as they are measured; unused ones go after a day.
				t = time.Now().Add(24 * time.Hour)
			}
			retention = &t
		}
	}
	var uploadedBy *uuid.UUID
	if p != nil {
		uploadedBy = &p.UserID
	}
	derivJSON, _ := json.Marshal(derivatives)
	u := &Upload{ID: id, Purpose: purpose, Visibility: visibility, OriginalName: sanitizeName(header.Filename), MIME: img.MIME,
		Bytes: int64(len(img.Main)), Width: &img.Width, Height: &img.Height, SHA256: digest}
	err = s.Pool.QueryRow(ctx, `INSERT INTO uploads (id, purpose, visibility, customer_id, uploaded_by, guest_token_hash, storage_key,
		original_name, mime, bytes, width, height, sha256, derivatives, retention_until)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING created_at`,
		id, purpose, visibility, customerID, uploadedBy, guestHash, mainKey, u.OriginalName, img.MIME, u.Bytes,
		img.Width, img.Height, digest, derivJSON, retention).Scan(&u.CreatedAt)
	if err != nil {
		s.cleanup(ctx, append([]string{mainKey}, values(derivatives)...))
		return err
	}
	s.Log.Info("upload stored", "upload_id", id, "purpose", purpose, "bytes", u.Bytes, "storage", s.Store.Name())
	s.decorate(u, derivatives)
	httpx.JSON(w, http.StatusCreated, u)
	return nil
}

func values(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func (s *Service) cleanup(ctx context.Context, keys []string) {
	for _, k := range keys {
		if err := s.Store.Delete(ctx, k); err != nil {
			s.Log.Warn("cleanup failed", "key", k, "error", err)
		}
	}
}

func (s *Service) decorate(u *Upload, derivatives map[string]string) {
	u.URL = s.urlFor(u.ID, "")
	u.Variants = map[string]string{}
	for name := range derivatives {
		u.Variants[name] = s.urlFor(u.ID, name)
	}
}

func (s *Service) findDuplicate(ctx context.Context, digest, purpose string, customerID *uuid.UUID, guestHash []byte, p *auth.Principal) (*Upload, error) {
	var uploadedBy *uuid.UUID
	if p != nil && p.IsStaff {
		uploadedBy = &p.UserID
	}
	u := &Upload{}
	var deriv []byte
	err := s.Pool.QueryRow(ctx, `SELECT id, purpose, visibility, original_name, mime, bytes, width, height, sha256, derivatives, created_at
		FROM uploads WHERE sha256=$1 AND purpose=$2 AND deleted_at IS NULL
		  AND ((customer_id IS NOT NULL AND customer_id = $3) OR (guest_token_hash IS NOT NULL AND guest_token_hash = $4)
		       OR ($5::uuid IS NOT NULL AND uploaded_by = $5))
		LIMIT 1`, digest, purpose, customerID, guestHash, uploadedBy).
		Scan(&u.ID, &u.Purpose, &u.Visibility, &u.OriginalName, &u.MIME, &u.Bytes, &u.Width, &u.Height, &u.SHA256, &deriv, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	var d map[string]string
	_ = json.Unmarshal(deriv, &d)
	s.decorate(u, d)
	return u, nil
}

type record struct {
	id          uuid.UUID
	purpose     string
	visibility  string
	customerID  *uuid.UUID
	guestHash   []byte
	storageKey  string
	mime        string
	derivatives map[string]string
	deleted     bool
}

func (s *Service) load(ctx context.Context, id uuid.UUID) (*record, error) {
	rec := &record{id: id}
	var deriv []byte
	var deletedAt *time.Time
	err := s.Pool.QueryRow(ctx, `SELECT purpose, visibility, customer_id, guest_token_hash, storage_key, mime, derivatives, deleted_at
		FROM uploads WHERE id=$1`, id).Scan(&rec.purpose, &rec.visibility, &rec.customerID, &rec.guestHash, &rec.storageKey, &rec.mime, &deriv, &deletedAt)
	if err != nil {
		return nil, err
	}
	rec.deleted = deletedAt != nil
	_ = json.Unmarshal(deriv, &rec.derivatives)
	return rec, nil
}

// CanAccess reports whether the requester may read a private upload: its owner (account or guest
// token), staff with the matching read permission, or a holder of an access token for a request or
// support thread the upload is attached to.
func (s *Service) CanAccess(r *http.Request, rec *record) bool {
	if rec.visibility == "public" {
		return true
	}
	p := auth.FromContext(r.Context())
	if p != nil && p.IsStaff {
		switch rec.purpose {
		case "support":
			return p.Can("support.read")
		default:
			return p.Can("requests.read") || p.Can("orders.read") || p.Can("customers.read")
		}
	}
	if p != nil && p.CustomerID != nil && rec.customerID != nil && *p.CustomerID == *rec.customerID {
		return true
	}
	if g := GuestToken(r); g != "" && rec.guestHash != nil && tokens.Matches(g, rec.guestHash) {
		return true
	}
	if at := r.URL.Query().Get("access"); at != "" {
		var ok bool
		_ = s.Pool.QueryRow(r.Context(), `SELECT EXISTS (
			SELECT 1 FROM request_references rr JOIN quote_requests q ON q.id = rr.request_id
			WHERE rr.upload_id = $1 AND q.access_token_hash = $2
			UNION ALL
			SELECT 1 FROM support_messages m JOIN support_threads t ON t.id = m.thread_id
			WHERE $1 = ANY(m.attachment_ids) AND t.access_token_hash = $2)`, rec.id, tokens.Hash(at)).Scan(&ok)
		return ok
	}
	return false
}

// Serve streams an upload or one of its derivatives after an authorization check.
func (s *Service) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	rec, err := s.load(r.Context(), id)
	if err != nil {
		return err
	}
	if rec.deleted {
		return httpx.NewError(http.StatusGone, "deleted", "This file has been removed.")
	}
	if !s.CanAccess(r, rec) {
		return httpx.NotFound("We could not find this file.") // do not confirm existence
	}
	variant := chi.URLParam(r, "variant")
	key, mime := rec.storageKey, rec.mime
	if variant != "" {
		k, ok := rec.derivatives[variant]
		if !ok {
			return httpx.NotFound("We could not find this file.")
		}
		key, mime = k, "image/jpeg"
		if strings.HasSuffix(k, ".webp") {
			mime = "image/webp"
		}
	}
	etag := `"` + id.String() + "-" + variant + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	obj, err := s.Store.Get(r.Context(), key)
	if errors.Is(err, ErrObjectNotFound) {
		return httpx.NotFound("We could not find this file.")
	}
	if err != nil {
		return err
	}
	defer obj.Close()
	h := w.Header()
	h.Set("Content-Type", mime)
	h.Set("ETag", etag)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if rec.visibility == "public" {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	} else {
		h.Set("Cache-Control", "private, no-store")
	}
	_, err = io.Copy(w, obj)
	return err
}

// Delete lets the owner of a private upload remove it (bytes are deleted immediately).
func (s *Service) Delete(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	rec, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	p := auth.FromContext(ctx)
	owner := (p != nil && p.CustomerID != nil && rec.customerID != nil && *p.CustomerID == *rec.customerID) ||
		(GuestToken(r) != "" && rec.guestHash != nil && tokens.Matches(GuestToken(r), rec.guestHash))
	staff := p != nil && p.IsStaff && (p.Can(staffPurposePerm[rec.purpose]) || p.Can("customers.update"))
	if !owner && !staff {
		return httpx.NotFound("We could not find this file.")
	}
	if rec.deleted {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	if err := s.purge(ctx, rec); err != nil {
		return err
	}
	if staff && !owner {
		_ = audit.Write(ctx, s.Pool, audit.Entry{Action: "upload.delete", ObjectType: "upload", ObjectID: id.String()})
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ReadOwned returns the display-size copy (metadata already stripped) of a customer's own upload
// with the given purpose. Used to hand body photos to the estimation provider.
func (s *Service) ReadOwned(ctx context.Context, id, customerID uuid.UUID, purpose string) ([]byte, error) {
	rec, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec.deleted || rec.purpose != purpose || rec.customerID == nil || *rec.customerID != customerID {
		return nil, ErrObjectNotFound
	}
	key := rec.derivatives["preview"]
	if key == "" {
		key = rec.storageKey
	}
	obj, err := s.Store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(io.LimitReader(obj, MaxImageBytes))
}

// PurgeOwned deletes a customer's own upload with the given purpose from storage at once.
func (s *Service) PurgeOwned(ctx context.Context, id, customerID uuid.UUID, purpose string) error {
	rec, err := s.load(ctx, id)
	if err != nil || rec.deleted || rec.purpose != purpose || rec.customerID == nil || *rec.customerID != customerID {
		return err
	}
	return s.purge(ctx, rec)
}

func (s *Service) purge(ctx context.Context, rec *record) error {
	keys := append([]string{rec.storageKey}, values(rec.derivatives)...)
	for _, k := range keys {
		if err := s.Store.Delete(ctx, k); err != nil {
			return fmt.Errorf("delete object: %w", err)
		}
	}
	_, err := s.Pool.Exec(ctx, `UPDATE uploads SET status='deleted', deleted_at=now() WHERE id=$1`, rec.id)
	return err
}

// PurgeExpired applies the retention policy: expired private uploads and abandoned guest uploads
// (never attached to a request within 7 days) are deleted from storage and marked deleted.
func (s *Service) PurgeExpired(ctx context.Context) (int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM uploads u WHERE deleted_at IS NULL AND (
			(retention_until IS NOT NULL AND retention_until < now())
			OR (guest_token_hash IS NOT NULL AND customer_id IS NULL AND created_at < now() - interval '7 days'
			    AND NOT EXISTS (SELECT 1 FROM request_references rr WHERE rr.upload_id = u.id))
		) LIMIT 200`)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		rec, err := s.load(ctx, id)
		if err != nil {
			continue
		}
		if err := s.purge(ctx, rec); err != nil {
			s.Log.Warn("retention purge failed", "upload_id", id, "error", err)
			continue
		}
		n++
	}
	return n, nil
}
