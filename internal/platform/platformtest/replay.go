package platformtest

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

// updateGolden, when set via `go test -update`, rewrites golden files instead of comparing
// against them — the normal way to regenerate expectations after an intended change.
var updateGolden = flag.Bool("update", false, "rewrite golden files instead of comparing")

// Normalizer turns one raw platform payload (an IRC line, a websocket frame, a JSON event)
// into a UnifiedMessage. Each real adapter exposes one; the replay harness drives it.
type Normalizer func(raw []byte) (platform.UnifiedMessage, error)

// Replay runs each raw payload through normalize and returns the resulting messages. A
// payload that fails to normalize fails the test, naming the offending line — so a fixture
// that the adapter can't parse is caught loudly.
func Replay(t *testing.T, raw [][]byte, normalize Normalizer) []platform.UnifiedMessage {
	t.Helper()
	out := make([]platform.UnifiedMessage, 0, len(raw))
	for i, line := range raw {
		msg, err := normalize(line)
		if err != nil {
			t.Fatalf("normalize line %d (%q): %v", i, line, err)
		}
		out = append(out, msg)
	}
	return out
}

// AssertGolden compares the JSON encoding of v against testdata/<name>. With `-update` it
// writes the file instead. This is how adapter normalization is pinned: record real
// payloads, normalize them, and freeze the UnifiedMessage output as golden.
func AssertGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden %q: %v", name, err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %q: %v", name, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %q (run with -update to create): %v", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
