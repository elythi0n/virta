package pluginhost_test

import (
	"testing"

	"github.com/elythi0n/virta/internal/pluginhost"
)

func TestParseManifest_Valid(t *testing.T) {
	raw := []byte(`{
		"id": "com.example.test",
		"name": "Test Plugin",
		"version": "1.0.0",
		"scopes": ["read", "http"],
		"contributes": {}
	}`)
	m, err := pluginhost.ParseManifest(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.HasScope(pluginhost.ScopeRead) {
		t.Error("expected ScopeRead")
	}
	if !m.HasScope(pluginhost.ScopeHTTP) {
		t.Error("expected ScopeHTTP")
	}
}

func TestParseManifest_MissingID(t *testing.T) {
	raw := []byte(`{"name":"x","version":"1.0.0","scopes":[]}`)
	_, err := pluginhost.ParseManifest(raw)
	if err == nil {
		t.Error("expected error for missing id")
	}
}

func TestParseManifest_InvalidScope(t *testing.T) {
	raw := []byte(`{"id":"com.x.y","name":"x","version":"1.0.0","scopes":["superpower"]}`)
	_, err := pluginhost.ParseManifest(raw)
	if err == nil {
		t.Error("expected error for unknown scope")
	}
}

func TestRegistry_ScopeEnforcementOnEnable(t *testing.T) {
	// A plugin that contributes a DataSource without ScopeHTTP must be rejected.
	host := pluginhost.NewHostAPI(nil, nil)
	reg := pluginhost.New(host, nil, nil)

	raw := []byte(`{
		"id": "com.test.no-http",
		"name": "No HTTP",
		"version": "1.0.0",
		"scopes": ["ui"],
		"contributes": {"data_sources": [{"id": "tick"}]}
	}`)
	m, err := pluginhost.ParseManifest(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := reg.RegisterBuiltIn(m); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Enable(t.Context(), "com.test.no-http"); err == nil {
		t.Error("expected scope enforcement error for DataSource without ScopeHTTP")
	}
}

func TestRegistry_ScopeEnforcementPasses_WithHTTP(t *testing.T) {
	host := pluginhost.NewHostAPI(nil, nil)
	reg := pluginhost.New(host, nil, nil)

	raw := []byte(`{
		"id": "com.test.with-http",
		"name": "With HTTP",
		"version": "1.0.0",
		"scopes": ["ui", "http"],
		"contributes": {"data_sources": [{"id": "tick"}]}
	}`)
	m, err := pluginhost.ParseManifest(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := reg.RegisterBuiltIn(m); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Enable(t.Context(), "com.test.with-http"); err != nil {
		t.Errorf("expected enable to succeed with ScopeHTTP declared: %v", err)
	}
}
