package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elythi0n/virta/internal/hosted"
)

// HostedAuth is the user account surface for the hosted deployment. Injected via SetHostedAuth.
// When nil (the default — local/desktop), all /auth/user/* routes return 404.
type HostedAuth interface {
	Register(ctx context.Context, ip, email, displayName, password string) (HostedUser, string, error)
	Login(ctx context.Context, ip, email, password string) (HostedUser, string, error)
	Logout(ctx context.Context, r *http.Request) error
	Resolve(ctx context.Context, r *http.Request) (HostedUser, error)
}

// HostedUser is a registered account as returned to the client.
type HostedUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// SetHostedAuth installs the hosted account controller.
func (s *Server) SetHostedAuth(h HostedAuth) { s.hostedAuth = h }

// IsHosted reports whether this daemon is running in hosted multi-user mode.
func (s *Server) IsHosted() bool { return s.hostedAuth != nil }

func (s *Server) handleHostedRegister(w http.ResponseWriter, r *http.Request) {
	if s.hostedAuth == nil {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, "expected JSON with email and password", http.StatusBadRequest)
		return
	}
	ip := clientIP(r)
	user, token, err := s.hostedAuth.Register(r.Context(), ip, req.Email, req.DisplayName, req.Password)
	if err != nil {
		if errors.Is(err, hosted.ErrEmailTaken) {
			http.Error(w, "email already registered", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hosted.SetSessionCookie(w, token, isHTTPS(r))
	writeJSON(w, map[string]any{"user": user, "token": token})
}

func (s *Server) handleHostedLogin(w http.ResponseWriter, r *http.Request) {
	if s.hostedAuth == nil {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, "expected JSON with email and password", http.StatusBadRequest)
		return
	}
	user, token, err := s.hostedAuth.Login(r.Context(), clientIP(r), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, hosted.ErrUnauthorized) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	hosted.SetSessionCookie(w, token, isHTTPS(r))
	writeJSON(w, map[string]any{"user": user, "token": token})
}

func (s *Server) handleHostedLogout(w http.ResponseWriter, r *http.Request) {
	if s.hostedAuth == nil {
		http.NotFound(w, r)
		return
	}
	_ = s.hostedAuth.Logout(r.Context(), r)
	hosted.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHostedMe(w http.ResponseWriter, r *http.Request) {
	if s.hostedAuth == nil {
		http.NotFound(w, r)
		return
	}
	user, err := s.hostedAuth.Resolve(r.Context(), r)
	if err != nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	writeJSON(w, user)
}

// handleHostedStatus is a public endpoint that tells the frontend whether this is a
// hosted deployment (and therefore whether to show the login UI).
func (s *Server) handleHostedStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"hosted": s.hostedAuth != nil})
}

// clientIP extracts the real client IP, respecting X-Forwarded-For in hosted mode.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (leftmost) IP which is the original client.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
