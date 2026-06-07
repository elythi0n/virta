package xbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBundle_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bundle.json"), `{
		"version": 1,
		"selectors": {"chatRow":".row","authorHandle":".author","messageText":".text"},
		"observer": "obs.js"
	}`)
	writeFile(t, filepath.Join(dir, "obs.js"), `console.log("ok")`)

	b, js, err := LoadBundle(dir)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if b.Version != 1 || b.Selectors["chatRow"] == "" {
		t.Errorf("bundle = %+v", b)
	}
	if string(js) != `console.log("ok")` {
		t.Errorf("js = %q", js)
	}
}

func TestLoadBundle_WrongVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bundle.json"), `{"version":99,"selectors":{},"observer":"x.js"}`)
	writeFile(t, filepath.Join(dir, "x.js"), ``)
	if _, _, err := LoadBundle(dir); err == nil {
		t.Error("expected error for wrong version")
	}
}

func TestLoadBundle_MissingSelector(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bundle.json"), `{"version":1,"selectors":{"chatRow":".r"},"observer":"x.js"}`)
	writeFile(t, filepath.Join(dir, "x.js"), ``)
	if _, _, err := LoadBundle(dir); err == nil {
		t.Error("expected error for missing required selector")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
