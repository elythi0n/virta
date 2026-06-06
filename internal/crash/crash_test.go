package crash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHandle_WritesDump(t *testing.T) {
	dir := t.TempDir()
	var written string
	func() {
		defer func() {
			// Catch the re-panic so the test doesn't fail.
			_ = recover()
		}()
		defer Handle(dir)
		panic("test crash message")
	}()
	dumps := ListDumps(dir)
	if len(dumps) == 0 {
		t.Fatal("no crash dump written")
	}
	written = dumps[0]
	b, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	var d Dump
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("parse dump: %v", err)
	}
	if d.Panic != "test crash message" {
		t.Errorf("panic = %q, want test crash message", d.Panic)
	}
	if d.Stack == "" {
		t.Error("stack is empty")
	}
}

func TestPruneOld(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "crashes")
	_ = os.MkdirAll(dir, 0o700)
	for i := 0; i < MaxDumps+5; i++ {
		name := filepath.Join(dir, "crash-2026010"+string(rune('0'+i))+".json")
		_ = os.WriteFile(name, []byte(`{}`), 0o600)
	}
	pruneOld(dir)
	entries, _ := os.ReadDir(dir)
	if len(entries) != MaxDumps {
		t.Errorf("after prune: %d files, want %d", len(entries), MaxDumps)
	}
}
