package tests

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Still renders for the no-WebGL fallback are uploaded with an asset version. They are validated
// and re-encoded like every other image and served from the content-addressed asset store.
func TestAssetRenderUpload(t *testing.T) {
	owner := ownerClient(t)

	r := owner.uploadTo("/owner/assets/files", testJPEG(t, 660, 860, 7), "suit-front.jpg")
	expect(t, r, http.StatusCreated)
	var up struct {
		File struct {
			URL    string `json:"url"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"file"`
	}
	r.json(t, &up)
	if !strings.HasPrefix(up.File.URL, "/api/v1/asset-files/") || !strings.HasSuffix(up.File.URL, ".jpg") || len(up.File.SHA256) != 64 {
		t.Fatalf("unexpected file descriptor %+v", up.File)
	}

	res, err := http.Get(shared.srv.URL + up.File.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" || int64(len(body)) != up.File.Bytes {
		t.Fatalf("serve: %d %s %d bytes", res.StatusCode, res.Header.Get("Content-Type"), len(body))
	}
	if !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset files must be cached as immutable, got %q", res.Header.Get("Cache-Control"))
	}

	// Not a model, texture or image.
	expect(t, owner.uploadTo("/owner/assets/files", []byte("hello, this is not a picture at all"), "notes.txt"), http.StatusUnprocessableEntity)

	// Customers cannot upload asset files.
	guest := newClient(t)
	if r := guest.uploadTo("/owner/assets/files", testJPEG(t, 660, 860, 9), "x.jpg"); r.status < 400 {
		t.Fatalf("guest upload allowed: %d", r.status)
	}

	// Unknown file types are not served.
	res, err = http.Get(shared.srv.URL + strings.TrimSuffix(up.File.URL, ".jpg") + ".exe")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown extension served: %d", res.StatusCode)
	}
}
