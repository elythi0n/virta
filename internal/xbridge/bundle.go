package xbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Bundle is the versioned selector + observer configuration for a scrape target. It is bundled
// with the binary (bundle/ in the repo) and can be updated without a binary release: a remote
// update channel delivers a new bundle.json + observer.js, which the loader verifies before
// accepting. See docs/14 for the update channel design.
type Bundle struct {
	Version   int               `json:"version"`
	CreatedAt time.Time         `json:"created_at"`
	Selectors map[string]string `json:"selectors"`
	Observer  string            `json:"observer"` // path to the observer JS, relative to bundle.json
}

// ErrBundleIncompatible is returned when the bundle's version is not supported.
var ErrBundleIncompatible = errors.New("bundle: incompatible version")

// LoadBundle reads bundle.json + the referenced observer JS from dir. It validates the version
// and required selectors before returning — a malformed or too-old bundle returns an error rather
// than being silently ignored.
func LoadBundle(dir string) (bundle Bundle, observerJS []byte, err error) {
	meta, err := os.ReadFile(filepath.Join(dir, "bundle.json"))
	if err != nil {
		return bundle, nil, fmt.Errorf("bundle: read metadata: %w", err)
	}
	if err := json.Unmarshal(meta, &bundle); err != nil {
		return bundle, nil, fmt.Errorf("bundle: parse: %w", err)
	}
	if bundle.Version != 1 {
		return bundle, nil, ErrBundleIncompatible
	}
	for _, required := range []string{"chatRow", "authorHandle", "messageText"} {
		if bundle.Selectors[required] == "" {
			return bundle, nil, fmt.Errorf("bundle: missing required selector %q", required)
		}
	}
	jsPath := bundle.Observer
	if !filepath.IsAbs(jsPath) {
		jsPath = filepath.Join(dir, jsPath)
	}
	observerJS, err = os.ReadFile(jsPath)
	if err != nil {
		return bundle, nil, fmt.Errorf("bundle: read observer: %w", err)
	}
	return bundle, observerJS, nil
}
