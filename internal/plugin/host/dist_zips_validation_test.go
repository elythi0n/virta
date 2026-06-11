package host

import (
	"os"
	"path/filepath"
	"testing"
)

// Validates every packaged plugin archive in plugins/*/dist through the real install
// extraction path: the zip must parse, contain a valid manifest, and every entry point the
// manifest references must exist in the archive.
func TestDistZipsInstallable(t *testing.T) {
	root := "../../../plugins"
	zips, err := filepath.Glob(filepath.Join(root, "*", "dist", "*.zip"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(zips) == 0 {
		// dist/ archives are gitignored build artifacts; run plugins/*/build.sh to produce them.
		t.Skip("no dist zips present — nothing to validate")
	}

	inst := &Installer{}
	for _, z := range zips {
		t.Run(filepath.Base(z), func(t *testing.T) {
			data, err := os.ReadFile(z)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			m, files, err := inst.extractZip(data)
			if err != nil {
				t.Fatalf("extractZip: %v", err)
			}
			if m.Main.GUI == "" {
				t.Fatal("manifest declares no gui entry point")
			}
			if _, ok := files[m.Main.GUI]; !ok {
				t.Errorf("manifest gui entry %q missing from archive", m.Main.GUI)
			}
			for _, p := range m.Contributes.Panels {
				if p.Kind == "" || p.Title == "" {
					t.Errorf("panel contribution missing kind/title: %+v", p)
				}
			}
			// Nothing outside the manifest and gui/ should ship in the archive.
			for name := range files {
				if name != ManifestFileName && filepath.Dir(name) != "gui" {
					t.Errorf("unexpected file in archive: %s", name)
				}
			}
		})
	}
}
