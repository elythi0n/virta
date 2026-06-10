// Package pluginhost implements the Phase 8 plugin platform:
// manifest parsing, remote installation (Git URL), sandboxed execution (WASM + GUI),
// host API surface, and lifecycle management.
package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// validID matches plugin IDs: reverse-domain or short identifier.
var validID = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9][a-z0-9-]*)*$`)

// Scope is a capability a plugin must declare to access a host API surface.
type Scope string

const (
	ScopeRead     Scope = "read"     // read chat history, channels, accounts
	ScopeEvents   Scope = "events"   // subscribe to live message/event stream
	ScopeSend     Scope = "send"     // send chat messages
	ScopeModerate Scope = "moderate" // approve/deny held messages, ban, timeout
	ScopeStorage  Scope = "storage"  // per-plugin key-value storage
	ScopeHTTP     Scope = "http"     // outbound HTTP (explicitly declared URLs only)
	ScopeUI       Scope = "ui"       // contribute panels, commands, and palette entries
)

// PanelContrib describes a panel the plugin contributes to the dock.
type PanelContrib struct {
	Kind  string `json:"kind"` // must be globally unique; namespaced under plugin id at registration
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"` // icon name from the host icon set
}

// CommandContrib describes a slash-command the plugin contributes.
type CommandContrib struct {
	Name  string `json:"name"`  // e.g. "markets" → triggers /markets
	Title string `json:"title"` // palette label
	Scope string `json:"scope"` // "chat" | "palette" | "both"
}

// DataSourceContrib describes a daemon-side DataSource the plugin contributes.
type DataSourceContrib struct {
	ID string `json:"id"` // e.g. "tick" → published as plugin.<plugin-id>.tick
}

// Contributes groups all contribution kinds a plugin declares.
type Contributes struct {
	Panels      []PanelContrib      `json:"panels,omitempty"`
	Commands    []CommandContrib    `json:"commands,omitempty"`
	DataSources []DataSourceContrib `json:"data_sources,omitempty"`
}

// EngineConstraint specifies the minimum virta daemon version required.
type EngineConstraint struct {
	Virta string `json:"virta,omitempty"` // semver range e.g. ">=1.0.0"
}

// Entry points for the plugin executable.
type Main struct {
	// WASM is the relative path to the WASM module for hook execution.
	WASM string `json:"wasm,omitempty"`
	// GUI is the relative path to the entry HTML for sandboxed GUI panels.
	GUI string `json:"gui,omitempty"`
}

// Manifest is the parsed content of a plugin's virta-plugin.json.
type Manifest struct {
	// ID is the stable reverse-domain plugin identifier (e.g. "com.virta.markets").
	// For built-in plugins this is the short form ("markets").
	ID string `json:"id"`
	// Name is the human-readable display name.
	Name string `json:"name"`
	// Version is a semantic version string (e.g. "1.0.0").
	Version string `json:"version"`
	// Publisher is the author or organisation name.
	Publisher string `json:"publisher,omitempty"`
	// Description is a one-line summary shown in the catalog.
	Description string `json:"description,omitempty"`
	// Tags are free-form labels for catalog filtering.
	Tags []string `json:"tags,omitempty"`
	// Engines specifies minimum host version requirements.
	Engines EngineConstraint `json:"engines,omitempty"`
	// Scopes is the list of host API capabilities the plugin requests.
	// The user must consent to these during installation.
	Scopes []Scope `json:"scopes"`
	// HTTPEndpoints lists the URL prefixes the plugin may reach through the host's HTTP bridge
	// ('http' scope). Requests outside these prefixes (or origins found in the plugin's saved
	// config values) are refused by the daemon.
	HTTPEndpoints []string `json:"http_endpoints,omitempty"`
	// Contributes declares what the plugin adds to the host.
	Contributes Contributes `json:"contributes"`
	// Main specifies the executable entry points.
	Main Main `json:"main,omitempty"`
	// Config is the JSON Schema for the plugin's user-editable configuration.
	Config json.RawMessage `json:"config,omitempty"`
	// BuiltIn marks a plugin that ships inside the daemon binary (no remote install).
	BuiltIn bool `json:"built_in,omitempty"`
}

// Validate checks that the manifest is well-formed.
func (m *Manifest) Validate() error {
	if m.ID == "" {
		return errors.New("manifest: id is required")
	}
	if !validID.MatchString(m.ID) {
		return fmt.Errorf("manifest: invalid id %q (must be lower-case reverse-domain or short identifier)", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("manifest: name is required")
	}
	if m.Version == "" {
		return errors.New("manifest: version is required")
	}
	// Validate declared scopes are recognised.
	known := map[Scope]bool{
		ScopeRead: true, ScopeEvents: true, ScopeSend: true,
		ScopeModerate: true, ScopeStorage: true, ScopeHTTP: true, ScopeUI: true,
	}
	for _, s := range m.Scopes {
		if !known[s] {
			return fmt.Errorf("manifest: unknown scope %q", s)
		}
	}
	for _, ep := range m.HTTPEndpoints {
		if !strings.HasPrefix(ep, "https://") && !isLoopbackHTTP(ep) {
			return fmt.Errorf("manifest: http_endpoints entry %q must be https:// (http:// is allowed for loopback only)", ep)
		}
	}
	if len(m.HTTPEndpoints) > 0 && !m.HasScope(ScopeHTTP) {
		return errors.New("manifest: http_endpoints declared without the 'http' scope")
	}
	return nil
}

// isLoopbackHTTP reports whether ep is a plain-HTTP URL pointing at localhost (dev convenience).
func isLoopbackHTTP(ep string) bool {
	return strings.HasPrefix(ep, "http://localhost") || strings.HasPrefix(ep, "http://127.0.0.1")
}

// HasScope reports whether the manifest declares the given scope.
func (m *Manifest) HasScope(s Scope) bool {
	for _, decl := range m.Scopes {
		if decl == s {
			return true
		}
	}
	return false
}

// ParseManifest parses and validates a virta-plugin.json byte slice.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest: invalid JSON: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
