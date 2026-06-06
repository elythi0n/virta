// The desktop shell is its own Go module so its Wails -> CGO -> WebKit dependency never enters the
// core module's build (`go build ./...` and `make ci` skip a directory that has its own go.mod).
// Build it with the Wails CLI on a machine that has the WebKit dev libraries; run `go mod tidy`
// here first to resolve Wails' transitive dependencies and write go.sum.
module github.com/elythi0n/virta/frontends/desktop

go 1.26

require (
	github.com/elythi0n/virta v0.0.0
	github.com/wailsapp/wails/v2 v2.10.1
)

replace github.com/elythi0n/virta => ../..
