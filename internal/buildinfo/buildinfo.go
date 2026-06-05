// Package buildinfo exposes version metadata, injected at link time via -ldflags
// (see Makefile). Defaults make a `go run` / `go test` build self-describe as a dev build.
package buildinfo

// Set via -ldflags "-X .../buildinfo.Version=… -X .../buildinfo.Commit=… -X .../buildinfo.Date=…".
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a human-readable build identifier, e.g. "1.0.0 (a1b2c3d, 2026-06-05T…Z)".
func String() string {
	return Version + " (" + Commit + ", " + Date + ")"
}
