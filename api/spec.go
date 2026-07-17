// Package api exposes the collector's machine-readable API contract.
package api

import _ "embed"

// OpenAPI is the canonical OpenAPI 3.1 document for the collector API.
//
//go:embed openapi.json
var OpenAPI []byte
