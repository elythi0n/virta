package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestAPIContract_PathsComplete verifies that every route in the table has a handler, summary,
// and scope, and that the table hasn't shrunk below the expected minimum — a developer removing a
// route by mistake fails here before the OpenAPI spec drifts.
func TestAPIContract_PathsComplete(t *testing.T) {
	s := start(t)
	rt := s.routes()
	if len(rt) < 30 {
		t.Errorf("routes() returned only %d entries; expected ≥30 (route may have been removed)", len(rt))
	}
	for _, r := range rt {
		if r.handler == nil {
			t.Errorf("route %s %s has a nil handler", r.method, r.path)
		}
		if r.summary == "" {
			t.Errorf("route %s %s has no summary (required for OpenAPI generation)", r.method, r.path)
		}
		if r.scope == "" {
			t.Errorf("route %s %s has no scope", r.method, r.path)
		}
	}
}

// TestAPIContract_OpenAPIPathsMatch verifies every route in the table appears in the generated
// OpenAPI spec — this is the additive-only gate: routes can be added but not silently removed.
func TestAPIContract_OpenAPIPathsMatch(t *testing.T) {
	s := start(t)
	code, body := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/openapi.json?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("GET /v1/openapi.json status = %d", code)
	}
	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, required := range []string{"openapi", "info", "paths", "components", "tags"} {
		if spec[required] == nil {
			t.Errorf("OpenAPI spec missing field %q", required)
		}
	}
	paths, _ := spec["paths"].(map[string]any)
	if len(paths) == 0 {
		t.Fatal("OpenAPI spec has no paths")
	}
	for _, r := range s.routes() {
		if paths[r.path] == nil {
			t.Errorf("route %s %s appears in routes() but not in /v1/openapi.json", r.method, r.path)
		}
	}
}

// TestAPIContract_ScopesExhaustive ensures every route scope is a declared scope.
func TestAPIContract_ScopesExhaustive(t *testing.T) {
	valid := map[Scope]bool{}
	for _, s := range AllScopes {
		valid[s] = true
	}
	s := start(t)
	for _, r := range s.routes() {
		if !valid[r.scope] {
			t.Errorf("route %s %s has unknown scope %q", r.method, r.path, r.scope)
		}
	}
}
