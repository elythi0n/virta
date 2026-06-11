package host

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

const testManifest = `{
	"id": "com.example.test",
	"name": "Test",
	"version": "1.0.0",
	"scopes": ["ui"],
	"main": {"gui": "gui/index.html"}
}`

func zipArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Both archive layouts must install with their relative structure intact: files at the
// archive root (hand-rolled plugin zips) and files wrapped in a single top-level directory
// (GitHub release zipballs).
func TestInstallArchiveLayouts(t *testing.T) {
	layouts := map[string]map[string]string{
		"root": {
			"virta-plugin.json": testManifest,
			"gui/index.html":    "<html></html>",
			"gui/app.js":        "// js",
		},
		"wrapped": {
			"repo-abc123/virta-plugin.json": testManifest,
			"repo-abc123/gui/index.html":    "<html></html>",
			"repo-abc123/gui/app.js":        "// js",
		},
	}
	for name, entries := range layouts {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "plugin.zip")
			if err := os.WriteFile(archive, zipArchive(t, entries), 0o600); err != nil {
				t.Fatal(err)
			}
			inst := NewInstaller(t.TempDir())
			res, err := inst.Install(context.Background(), archive)
			if err != nil {
				t.Fatalf("Install: %v", err)
			}
			if res.Manifest.ID != "com.example.test" {
				t.Fatalf("manifest id = %q", res.Manifest.ID)
			}
			for _, rel := range []string{"virta-plugin.json", "gui/index.html", "gui/app.js"} {
				if _, err := os.Stat(filepath.Join(res.InstallDir, filepath.FromSlash(rel))); err != nil {
					t.Errorf("expected %s in install dir: %v", rel, err)
				}
			}
		})
	}
}

// Archives with several top-level directories and no root manifest must fail cleanly rather
// than having a directory stripped away.
func TestInstallRejectsManifestlessArchive(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "plugin.zip")
	entries := map[string]string{
		"a/virta-plugin.json": testManifest,
		"b/readme.md":         "two top dirs, no root manifest",
	}
	if err := os.WriteFile(archive, zipArchive(t, entries), 0o600); err != nil {
		t.Fatal(err)
	}
	inst := NewInstaller(t.TempDir())
	if _, err := inst.Install(context.Background(), archive); err == nil {
		t.Fatal("expected install to fail without a root manifest")
	}
}
