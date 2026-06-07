package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// PlatformAuthConfig is one platform's OAuth app credential state. The client id is not secret and
// is returned for display/editing; the client secret is write-only (only whether one is set).
type PlatformAuthConfig struct {
	ClientID   string `json:"client_id"`
	HasSecret  bool   `json:"has_secret"`
	Configured bool   `json:"configured"`
}

// AuthConfig is the OAuth app credentials, as served by GET /v1/auth/config.
type AuthConfig struct {
	Twitch PlatformAuthConfig `json:"twitch"`
	Kick   PlatformAuthConfig `json:"kick"`
}

// AuthConfigControl is the OAuth-credentials control, implemented by the wiring layer and injected
// via SetAuthConfig. Setting credentials persists them (vault) and applies them to the live clients.
type AuthConfigControl interface {
	AuthConfig() AuthConfig
	SetAuthConfig(ctx context.Context, platform, clientID, clientSecret string) error
}

// SetAuthConfig installs the OAuth-credentials controller.
func (s *Server) SetAuthConfig(c AuthConfigControl) { s.authConfig = c }

func (s *Server) handleGetAuthConfig(w http.ResponseWriter, _ *http.Request) {
	if s.hostedAuth != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if s.authConfig == nil {
		http.Error(w, "auth config unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.authConfig.AuthConfig())
}

func (s *Server) handleSetAuthConfig(w http.ResponseWriter, r *http.Request) {
	if s.hostedAuth != nil {
		http.Error(w, "OAuth credentials are operator-only in hosted mode", http.StatusForbidden)
		return
	}
	if s.authConfig == nil {
		http.Error(w, "auth config unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		Platform     string `json:"platform"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Platform == "" {
		http.Error(w, "expected JSON body with platform and client_id", http.StatusBadRequest)
		return
	}
	if err := s.authConfig.SetAuthConfig(r.Context(), req.Platform, req.ClientID, req.ClientSecret); err != nil {
		if errors.Is(err, ErrUnknownPlatform) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.authConfig.AuthConfig())
}
