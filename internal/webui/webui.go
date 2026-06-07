// Package webui embeds the built web UI so the daemon can serve it itself: run virtad and open a
// browser, no desktop shell or dev server needed. `make web` stages frontends/web/dist here before
// the build; without it (e.g. a plain `go build`) only a placeholder is embedded and Built reports
// false.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var embedded embed.FS

// dist returns the embedded UI tree rooted at its index.
func dist() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return embedded
	}
	return sub
}

// Built reports whether a real UI (an index.html) was embedded into this binary.
func Built() bool {
	_, err := fs.Stat(dist(), "index.html")
	return err == nil
}

// IndexHTML returns the raw bytes of the embedded index.html.
// Callers that need to inject a script tag should use this instead of going through
// the file server, which redirects /index.html → / and would create an infinite loop.
func IndexHTML() ([]byte, error) {
	return fs.ReadFile(dist(), "index.html")
}

// Handler serves the embedded UI with SPA fallback: a missing path resolves to index.html so
// client-side routes load. Returns nil if no UI was built in (the daemon then skips the route).
func Handler() http.Handler {
	if !Built() {
		return nil
	}
	root := dist()
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" {
			p = "/index.html"
		}
		if _, err := fs.Stat(root, p[1:]); err != nil {
			// Unknown path: hand the SPA its entry point and let it route.
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
