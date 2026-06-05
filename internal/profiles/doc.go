// Package profiles owns the ProfileDoc — a named, versioned workspace snapshot — and the
// manager that activates one: diffing the channel set so only the difference is joined/left
// (no feed gap for channels common to both), swapping the filter ruleset, and announcing the
// switch. The store persists the doc as opaque JSON; this package gives it meaning.
package profiles

import (
	"encoding/json"

	"github.com/elythi0n/virta/internal/filter"
	"github.com/elythi0n/virta/internal/platform"
)

// CurrentVersion is the ProfileDoc schema version. Bump it and extend Migrate when the shape
// changes.
const CurrentVersion = 1

// Doc is a workspace: the channels to join, the filter rules, logging policy, and per-frontend
// layout sections (opaque to the core, carried through untouched).
type Doc struct {
	Version  int             `json:"version"`
	Channels []ChannelSpec   `json:"channels,omitempty"`
	Filters  []filter.Rule   `json:"filters,omitempty"`
	Logging  Logging         `json:"logging"`
	Layouts  json.RawMessage `json:"layouts,omitempty"`
}

// ChannelSpec is one channel a profile joins.
type ChannelSpec struct {
	Platform platform.Platform `json:"platform"`
	Slug     string            `json:"slug"`
	Mode     platform.ConnMode `json:"mode,omitempty"`
}

// Logging is the per-profile persistence policy (off by default — ADR-014).
type Logging struct {
	Enabled   bool   `json:"enabled"`
	Retention string `json:"retention,omitempty"`
}

// Ref is the ChannelRef a spec joins as.
func (c ChannelSpec) Ref() platform.ChannelRef {
	return platform.ChannelRef{Platform: c.Platform, Slug: c.Slug}
}

func (c ChannelSpec) mode() platform.ConnMode {
	if c.Mode == "" {
		return platform.ModeAutomatic
	}
	return c.Mode
}

// NewDoc is an empty current-version document.
func NewDoc() Doc { return Doc{Version: CurrentVersion} }

// Marshal serializes the doc for storage.
func (d Doc) Marshal() (json.RawMessage, error) { return json.Marshal(d) }

// Migrate decodes a stored doc and upgrades it to CurrentVersion. v1 is the base; this is the
// seam where future version upgrades slot in (each step bumping Version as it transforms).
func Migrate(raw json.RawMessage) (Doc, error) {
	if len(raw) == 0 {
		return NewDoc(), nil
	}
	var d Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		return Doc{}, err
	}
	if d.Version == 0 {
		d.Version = 1 // pre-versioned docs are treated as v1
	}
	// Future: for d.Version < CurrentVersion, apply ordered upgrade steps here.
	d.Version = CurrentVersion
	return d, nil
}

// channelKey identifies a channel for set diffing (platform:slug).
func channelKey(p platform.Platform, slug string) string { return string(p) + ":" + slug }
