package api

import (
	"context"
	"errors"
	"net/http"
)

// ErrAuthNotConfigured means a platform's sign-in credentials (client id) aren't set, so the flow
// can't start. It's a server-config issue, not an upstream failure — the API answers 503, not 502.
var ErrAuthNotConfigured = errors.New("sign-in not configured")

// authStartError maps an auth-start failure to a status: a missing configuration is 503 (with the
// actionable message), anything else is an upstream/network failure (502).
func (s *Server) authStartError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrAuthNotConfigured) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

// Auth is the account-authorization control surface, implemented by the auth managers and
// injected via SetAuth. Decoupled from the auth packages via plain structs.
type Auth interface {
	// StartTwitchDevice begins a Twitch device-code authorization, returning the code to show.
	StartTwitchDevice(ctx context.Context) (DeviceSession, error)
	// TwitchDeviceStatus reports a session's progress (ok=false if unknown).
	TwitchDeviceStatus(id string) (DeviceSession, bool)
	// StartKickAuth begins a Kick OAuth (PKCE) authorization, returning the URL to open.
	StartKickAuth(ctx context.Context) (AuthSession, error)
	// KickAuthStatus reports a Kick session's progress (ok=false if unknown).
	KickAuthStatus(id string) (AuthSession, bool)
}

// DeviceSession is a device-flow session as served to clients.
type DeviceSession struct {
	ID              string `json:"id"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	State           string `json:"state"`
	Login           string `json:"login,omitempty"`
	Error           string `json:"error,omitempty"`
}

// AuthSession is a redirect-flow (Kick) authorization session as served to clients.
type AuthSession struct {
	ID           string `json:"id"`
	AuthorizeURL string `json:"authorize_url"`
	State        string `json:"state"`
	Login        string `json:"login,omitempty"`
	Error        string `json:"error,omitempty"`
}

// SetAuth installs the auth controller (wiring layer, after the managers exist).
func (s *Server) SetAuth(a Auth) { s.authCtl = a }

func (s *Server) handleTwitchDeviceStart(w http.ResponseWriter, r *http.Request) {
	if s.authCtl == nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	sess, err := s.authCtl.StartTwitchDevice(r.Context())
	if err != nil {
		s.authStartError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, sess)
}

func (s *Server) handleTwitchDeviceStatus(w http.ResponseWriter, r *http.Request) {
	if s.authCtl == nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	sess, ok := s.authCtl.TwitchDeviceStatus(r.PathValue("id"))
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	writeJSON(w, sess)
}

func (s *Server) handleKickAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.authCtl == nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	sess, err := s.authCtl.StartKickAuth(r.Context())
	if err != nil {
		s.authStartError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, sess)
}

func (s *Server) handleKickAuthStatus(w http.ResponseWriter, r *http.Request) {
	if s.authCtl == nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	sess, ok := s.authCtl.KickAuthStatus(r.PathValue("id"))
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	writeJSON(w, sess)
}
