package api

import (
	"encoding/json"
	"testing"
)

func TestOpenAPIIsValidJSONAndDocumentsRoutes(t *testing.T) {
	var document struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPI, &document); err != nil {
		t.Fatalf("OpenAPI document is not valid JSON: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("expected OpenAPI 3.1.0, got %q", document.OpenAPI)
	}
	for _, path := range []string{"/openapi.json", "/collect", "/batch", "/healthz", "/readyz"} {
		if _, ok := document.Paths[path]; !ok {
			t.Errorf("OpenAPI document is missing %s", path)
		}
	}
}
