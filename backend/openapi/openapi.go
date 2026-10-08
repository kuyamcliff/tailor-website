// Package openapi embeds the API description so the server can publish it at /api/v1/openapi.yaml.
// The document is kept in step with the router by backend/tests/openapi_test.go.
package openapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var Spec []byte

// Serve returns the OpenAPI document.
func Serve(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(Spec)
}
