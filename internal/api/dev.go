package api

import (
	_ "embed"
	"net/http"
)

// devPage is a deliberately minimal, dependency-free web client served at /dev. It exists to
// exercise the live API by hand — joining channels and watching the merged feed — not as a
// product surface. It connects to the stream, renders each event with its platform colour, and
// drives the channel join/leave endpoints, reusing the token from its own URL.
//
//go:embed dev.html
var devPage []byte

func (s *Server) handleDev(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(devPage)
}
