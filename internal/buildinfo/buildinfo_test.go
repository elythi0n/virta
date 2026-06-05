package buildinfo_test

import (
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/buildinfo"
)

func TestString_IncludesAllFields(t *testing.T) {
	got := buildinfo.String()
	for _, want := range []string{buildinfo.Version, buildinfo.Commit, buildinfo.Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}

func TestString_DefaultsToDevBuild(t *testing.T) {
	// Without -ldflags injection (i.e. under `go test`), it must self-describe as dev.
	if buildinfo.Version != "dev" {
		t.Skipf("Version was injected (%q); default-build assertion not applicable", buildinfo.Version)
	}
	if got := buildinfo.String(); !strings.HasPrefix(got, "dev (") {
		t.Errorf("default String() = %q, want prefix %q", got, "dev (")
	}
}
