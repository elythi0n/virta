package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

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
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
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
	writeJSON(w, map[string]any{"user": user})
}

func (s *Server) handleHostedLogin(w http.ResponseWriter, r *http.Request) {
	if s.hostedAuth == nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
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
	writeJSON(w, map[string]any{"user": user})
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

// clientIP extracts the real client IP. X-Forwarded-For is only trusted when the immediate
// peer is loopback (indicating a local reverse proxy). The rightmost non-empty entry from
// X-Forwarded-For is used to avoid trusting client-supplied headers.
func clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if clean := strings.TrimSpace(parts[len(parts)-1]); clean != "" {
				return clean
			}
		}
	}
	return host
}

// isHTTPS reports whether the request arrived over TLS. X-Forwarded-Proto is only
// trusted when the immediate peer is loopback (a local reverse proxy).
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return r.Header.Get("X-Forwarded-Proto") == "https"
	}
	return false
}
