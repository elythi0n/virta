package wordlist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func serve(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLoadFromFreshDiskCache(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, cacheFile), []byte("foo\nbar\n# comment\n\nbaz"), 0o600)

	l := &Loader{dir: dir, ttl: DefaultTTL, client: &http.Client{Timeout: time.Second}, RemoteURL: "http://127.0.0.1:0/unreachable"}
	terms := l.Load(context.Background())
	want := []string{"foo", "bar", "baz"}
	if len(terms) != len(want) {
		t.Fatalf("terms = %v, want %v", terms, want)
	}
	for i, w := range want {
		if terms[i] != w {
			t.Errorf("terms[%d] = %q, want %q", i, terms[i], w)
		}
	}
}

func TestLoadFromRemoteAndWritesCache(t *testing.T) {
	dir := t.TempDir()
	srv := serve(t, "alpha\nbeta\ngamma", 200)
	l := &Loader{dir: dir, ttl: DefaultTTL, client: &http.Client{Timeout: time.Second}, RemoteURL: srv.URL}

	terms := l.loadBuiltin(context.Background())
	if len(terms) != 3 {
		t.Fatalf("expected 3 terms, got %v", terms)
	}
	// Cache file should now exist.
	if _, err := os.Stat(filepath.Join(dir, cacheFile)); err != nil {
		t.Errorf("cache file not written: %v", err)
	}
}

func TestFallsBackToStaleCacheOnRemoteFailure(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, cacheFile)
	_ = os.WriteFile(stale, []byte("stale_word"), 0o600)
	past := time.Now().Add(-8 * 24 * time.Hour)
	_ = os.Chtimes(stale, past, past)

	srv := serve(t, "", 503)
	l := &Loader{dir: dir, ttl: DefaultTTL, client: &http.Client{Timeout: time.Second}, RemoteURL: srv.URL}
	terms := l.loadBuiltin(context.Background())
	if len(terms) != 1 || terms[0] != "stale_word" {
		t.Fatalf("expected stale fallback, got %v", terms)
	}
}

func TestCustomListMerged(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, cacheFile), []byte("remote_term"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, customFile), []byte("custom_term\nremote_term"), 0o600)

	l := &Loader{dir: dir, ttl: DefaultTTL, client: &http.Client{Timeout: time.Second}, RemoteURL: "http://127.0.0.1:0/unreachable"}
	terms := l.Load(context.Background())
	// Dedup should produce exactly 2 terms: remote_term (first), custom_term (added from custom).
	if len(terms) != 2 {
		t.Fatalf("expected 2 terms (deduped), got %v", terms)
	}
}

func TestDedup(t *testing.T) {
	a := []string{"cat", "dog", "cat"}
	b := []string{"dog", "fish"}
	got := dedup(a, b)
	want := []string{"cat", "dog", "fish"}
	if len(got) != len(want) {
		t.Fatalf("dedup = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}
