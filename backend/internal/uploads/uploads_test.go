package uploads

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

func photo(w, h int, alpha bool) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && x < w/2 {
				a = 0
			}
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 120, a})
		}
	}
	return img
}

func encJPEG(t *testing.T, img image.Image) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encPNG(t *testing.T, img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withEXIF inserts an APP1 Exif segment carrying a fake GPS note right after the JPEG SOI marker.
func withEXIF(jpg []byte) []byte {
	payload := append([]byte("Exif\x00\x00"), []byte("GPS 4.0511N 9.7679E camera serial 12345")...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

func code(err error) string {
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Code
	}
	return ""
}

func TestProcessImageStripsMetadata(t *testing.T) {
	src := withEXIF(encJPEG(t, photo(400, 300, false)))
	if !bytes.Contains(src, []byte("GPS")) {
		t.Fatal("test image should carry metadata")
	}
	out, err := ProcessImage(src)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Main, []byte("Exif")) || bytes.Contains(out.Main, []byte("GPS")) {
		t.Fatal("metadata survived re-encoding")
	}
	if out.MIME != "image/jpeg" || out.Width != 400 || out.Height != 300 {
		t.Fatalf("unexpected result %s %dx%d", out.MIME, out.Width, out.Height)
	}
	for _, v := range []string{"preview", "thumb"} {
		if len(out.Variants[v]) == 0 {
			t.Errorf("missing %s derivative", v)
		}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out.Variants["thumb"]))
	if err != nil || cfg.Width > 480 || cfg.Height > 480 {
		t.Errorf("thumb %dx%d %v", cfg.Width, cfg.Height, err)
	}
}

func TestProcessImageKeepsTransparency(t *testing.T) {
	out, err := ProcessImage(encPNG(t, photo(300, 300, true)))
	if err != nil {
		t.Fatal(err)
	}
	if out.MIME != "image/png" {
		t.Fatalf("transparent PNG became %s", out.MIME)
	}
	opaque, err := ProcessImage(encPNG(t, photo(300, 300, false)))
	if err != nil || opaque.MIME != "image/jpeg" {
		t.Fatalf("opaque PNG should become JPEG: %v %v", opaque, err)
	}
}

func TestProcessImageRejects(t *testing.T) {
	small := encJPEG(t, photo(100, 100, false))
	// A PNG header that claims 20000 x 20000 pixels: refused before decoding (decompression bomb).
	bomb := encPNG(t, photo(300, 300, false))
	binary.BigEndian.PutUint32(bomb[16:], 20000)
	binary.BigEndian.PutUint32(bomb[20:], 20000)
	binary.BigEndian.PutUint32(bomb[29:], crc32.ChecksumIEEE(bomb[12:29])) // keep the IHDR checksum valid
	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00!\xf9\x04\x00\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	truncated := encJPEG(t, photo(400, 400, false))[:600]
	cases := map[string]struct {
		data []byte
		code string
	}{
		"text pretending to be an image": {[]byte("<svg onload=alert(1)>"), "unsupported_file"},
		"gif":                            {gif, "unsupported_file"},
		"too small":                      {small, "image_too_small"},
		"pixel bomb":                     {bomb, "image_too_large"},
		"truncated":                      {truncated, "invalid_image"},
	}
	for name, c := range cases {
		_, err := ProcessImage(c.data)
		if code(err) != c.code {
			t.Errorf("%s: got %v (%s), want %s", name, err, code(err), c.code)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":          "passwd",
		`C:\Users\ada\robe.jpg`:     "robe.jpg",
		"<script>\"x\".png":         "scriptx.png",
		"tab\there\x00.jpg":         "tabhere.jpg",
		"Robe de soirée brodée.jpg": "Robe de soirée brodée.jpg",
	} {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
	long := sanitizeName(strings.Repeat("é", 100) + ".jpg")
	if len(long) > 120 || !utf8.ValidString(long) {
		t.Errorf("long name %d bytes, valid UTF-8 %v", len(long), utf8.ValidString(long))
	}
}

func TestGuestToken(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if GuestToken(r) != "" {
		t.Error("missing token accepted")
	}
	r.Header.Set(GuestHeader, "short")
	if GuestToken(r) != "" {
		t.Error("short token accepted")
	}
	r.Header.Set(GuestHeader, strings.Repeat("a", 40))
	if GuestToken(r) == "" {
		t.Error("valid token refused")
	}
}

func TestLocalStorage(t *testing.T) {
	s, err := NewStorage(config.StorageConfig{Driver: "local", LocalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "uploads/ab/file.jpg", strings.NewReader("hello"), 5, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	rc, err := s.Get(ctx, "uploads/ab/file.jpg")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello" {
		t.Fatalf("read %q", b)
	}
	for _, bad := range []string{"../outside.jpg", "uploads/../../x", ""} {
		if err := s.Put(ctx, bad, strings.NewReader("x"), 1, ""); err == nil {
			t.Errorf("key %q accepted", bad)
		}
	}
	if err := s.Delete(ctx, "uploads/ab/file.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "uploads/ab/file.jpg"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("deleted object still readable: %v", err)
	}
	if err := s.Delete(ctx, "uploads/ab/file.jpg"); err != nil {
		t.Fatalf("deleting twice should be harmless: %v", err)
	}
	if _, err := NewStorage(config.StorageConfig{Driver: "s3"}); err == nil {
		t.Error("s3 without credentials accepted")
	}
}
