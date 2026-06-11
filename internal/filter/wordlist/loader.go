// Package wordlist manages the profanity word list: downloads a maintained external list,
// caches it to disk, and merges it with the user's custom terms. The filter engine lives in
// the parent filter package; this package only supplies the terms — it never embeds them, so
// offensive words never appear in the source tree or compiled binary.
//
// Remote list: LDNOOBW (List of Dirty, Naughty, Obscene, and Otherwise Bad Words),
// newline-delimited English terms maintained at github.com/LDNOOBW. The format matches
// filter.ParseList exactly (blank lines and # comments ignored, terms trimmed + lowercased).
//
// Cache: <datadir>/en_profanity.txt — refreshed when older than TTL (default 7 days). A
// network failure falls back to the stale cache so masking survives offline restarts.
//
// Custom list: <datadir>/custom.txt — same format; merged after the remote list (duplicates
// dropped). Users can add domain-specific terms without touching the remote list.
package wordlist

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultRemoteURL is the LDNOOBW English profanity list — maintained, newline-delimited,
	// same format as filter.ParseList.
	DefaultRemoteURL = "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/en"
	cacheFile        = "en_profanity.txt"
	customFile       = "custom.txt"
	// DefaultTTL is how long the cached list is considered fresh before a remote refresh.
	DefaultTTL = 7 * 24 * time.Hour
)

// Loader downloads, caches, and merges profanity word lists.
type Loader struct {
	dir       string
	ttl       time.Duration
	client    *http.Client
	RemoteURL string // overridable for tests; defaults to DefaultRemoteURL
}

// New creates a Loader that stores its cache in dir.
func New(dir string) *Loader {
	return &Loader{dir: dir, ttl: DefaultTTL, client: &http.Client{Timeout: 15 * time.Second}, RemoteURL: DefaultRemoteURL}
}

// Load returns the merged term list: cached/fresh remote list + user custom list. It never
// returns an error — a remote failure falls back to whatever is cached, and a total absence
// returns nil (no terms, masking is a no-op).
func (l *Loader) Load(ctx context.Context) []string {
	builtin := l.loadBuiltin(ctx)
	custom := l.loadCustom()
	return dedup(builtin, custom)
}

func (l *Loader) loadBuiltin(ctx context.Context) []string {
	path := filepath.Join(l.dir, cacheFile)
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < l.ttl {
		if data, err := os.ReadFile(path); err == nil {
			return parseLines(data)
		}
	}
	// Stale or missing — try a remote fetch.
	terms, err := l.fetchRemote(ctx)
	if err == nil && len(terms) > 0 {
		_ = os.MkdirAll(l.dir, 0o700)
		_ = os.WriteFile(path, []byte(strings.Join(terms, "\n")), 0o600)
		return terms
	}
	// Remote failed — serve the stale cache if it exists.
	if data, err := os.ReadFile(path); err == nil {
		return parseLines(data)
	}
	return nil
}

func (l *Loader) fetchRemote(ctx context.Context) ([]string, error) {
	url := l.RemoteURL
	if url == "" {
		url = DefaultRemoteURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wordlist: remote HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return nil, err
	}
	return parseLines(b), nil
}

func (l *Loader) loadCustom() []string {
	data, err := os.ReadFile(filepath.Join(l.dir, customFile))
	if err != nil {
		return nil
	}
	return parseLines(data)
}

func parseLines(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, strings.ToLower(line))
	}
	return out
}

func dedup(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, t := range append(a, b...) {
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}
