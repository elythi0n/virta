// Package api serves the local control surface every frontend connects to: a small HTTP +
// WebSocket interface bound to loopback. It announces where it's listening and a bearer
// token via a discovery file, so a frontend on the same machine can find and authenticate
// to the daemon without any configuration.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/elythi0n/virta/internal/buildinfo"
	"github.com/elythi0n/virta/internal/pipeline"
)

// discoveryFileName is the file a frontend reads to find and authenticate to the daemon.
const discoveryFileName = "daemon.json"

// Discovery is the contents of the discovery file: how to reach the daemon and the token to
// authenticate with. It is written owner-only.
type Discovery struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
}

// Config configures the server. Token may be empty, in which case a random one is generated.
type Config struct {
	Addr       string // listen address, e.g. "127.0.0.1:0" for an ephemeral loopback port
	Token      string
	RuntimeDir string // where the discovery file is written
	Logger     *slog.Logger
}

// Server is the local HTTP/WebSocket API.
type Server struct {
	log      *slog.Logger
	ring     *logRing
	hub      *hub
	channels Channels // join/leave controller, installed via SetChannels
	profiles Profiles // profile controller, installed via SetProfiles
	authCtl  Auth     // account-auth controller, installed via SetAuth
	send     Send     // cross-posting controller, installed via SetSend

	token         string
	runtimeDir    string
	discoveryPath string

	ln      net.Listener
	httpSrv *http.Server

	baseCtx context.Context
	cancel  context.CancelFunc
}

// New builds a server. The returned server is not yet listening; call Start.
func New(cfg Config) (*Server, error) {
	token := cfg.Token
	if token == "" {
		var err error
		if token, err = newToken(); err != nil {
			return nil, err
		}
	}
	ring := newLogRing(200)
	base := cfg.Logger
	if base == nil {
		base = slog.New(slog.DiscardHandler)
	}
	// Wrap the logger so recent records are also captured for the diagnostics endpoint.
	log := slog.New(newRingHandler(base.Handler(), ring))

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		log:           log,
		ring:          ring,
		hub:           newHub(),
		token:         token,
		runtimeDir:    cfg.RuntimeDir,
		discoveryPath: filepath.Join(cfg.RuntimeDir, discoveryFileName),
		baseCtx:       ctx,
		cancel:        cancel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.Handle("GET /v1/diagnostics", s.auth(http.HandlerFunc(s.handleDiagnostics)))
	mux.Handle("GET /v1/stream", s.auth(http.HandlerFunc(s.handleStream)))
	mux.Handle("GET /v1/channels", s.auth(http.HandlerFunc(s.handleListChannels)))
	mux.Handle("POST /v1/channels", s.auth(http.HandlerFunc(s.handleJoinChannel)))
	mux.Handle("DELETE /v1/channels", s.auth(http.HandlerFunc(s.handleLeaveChannel)))
	mux.Handle("POST /v1/send", s.auth(http.HandlerFunc(s.handleSend)))
	mux.Handle("POST /v1/send/preview", s.auth(http.HandlerFunc(s.handleSendPreview)))
	mux.Handle("GET /v1/profiles", s.auth(http.HandlerFunc(s.handleListProfiles)))
	mux.Handle("POST /v1/profiles", s.auth(http.HandlerFunc(s.handleCreateProfile)))
	mux.Handle("POST /v1/profiles/{id}/activate", s.auth(http.HandlerFunc(s.handleActivateProfile)))
	mux.Handle("POST /v1/auth/twitch/device", s.auth(http.HandlerFunc(s.handleTwitchDeviceStart)))
	mux.Handle("GET /v1/auth/twitch/device/{id}", s.auth(http.HandlerFunc(s.handleTwitchDeviceStatus)))
	mux.Handle("POST /v1/auth/kick/start", s.auth(http.HandlerFunc(s.handleKickAuthStart)))
	mux.Handle("GET /v1/auth/kick/{id}", s.auth(http.HandlerFunc(s.handleKickAuthStatus)))
	mux.Handle("GET /dev", s.auth(http.HandlerFunc(s.handleDev)))

	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return s.baseCtx },
	}
	s.httpSrv.Addr = cfg.Addr
	return s, nil
}

// Sink exposes the WebSocket hub as a pipeline sink, so wiring can attach it to the pipeline
// and every event reaches connected clients.
func (s *Server) Sink() pipeline.Sink { return s.hub }

// Token returns the bearer token clients must present.
func (s *Server) Token() string { return s.token }

// Logger returns the server's logger (with diagnostics capture), so the rest of the daemon
// can log through the same ring buffer.
func (s *Server) Logger() *slog.Logger { return s.log }

// Addr returns the actual listen address (resolved port), valid after Start.
func (s *Server) Addr() string {
	if s.ln == nil {
		return s.httpSrv.Addr
	}
	return s.ln.Addr().String()
}

// Start binds the listener, writes the discovery file, and begins serving in the background.
func (s *Server) Start() error {
	addr := s.httpSrv.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	if err := s.writeDiscovery(); err != nil {
		_ = ln.Close()
		return err
	}
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("api server stopped", "err", err)
		}
	}()
	s.log.Info("api listening", "addr", s.Addr())
	s.log.Info("dev feed", "url", "http://"+s.Addr()+"/dev?token="+s.token)
	return nil
}

// Close stops serving, unblocks any streaming clients, and removes the discovery file.
func (s *Server) Close(ctx context.Context) error {
	s.cancel() // unblocks in-flight stream handlers so Shutdown can complete
	_ = os.Remove(s.discoveryPath)
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) writeDiscovery() error {
	d := Discovery{Addr: s.Addr(), Token: s.token}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return os.WriteFile(s.discoveryPath, b, 0o600)
}

// ReadDiscovery reads the discovery file from runtimeDir — how a frontend finds the daemon.
func ReadDiscovery(runtimeDir string) (Discovery, error) {
	b, err := os.ReadFile(filepath.Join(runtimeDir, discoveryFileName))
	if err != nil {
		return Discovery{}, err
	}
	var d Discovery
	if err := json.Unmarshal(b, &d); err != nil {
		return Discovery{}, err
	}
	return d, nil
}

// auth requires a valid bearer token, accepted either in the Authorization header or a
// "token" query parameter (browsers can't set headers on a WebSocket handshake).
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.tokenOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(r *http.Request) bool {
	presented := r.URL.Query().Get("token")
	if h := r.Header.Get("Authorization"); presented == "" && len(h) > 7 && h[:7] == "Bearer " {
		presented = h[7:]
	}
	// Constant-time compare to avoid leaking the token via timing.
	return presented != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"status": "ok", "version": buildinfo.String()})
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"clients": s.hub.clientCount(),
		"log":     s.ring.snapshot(),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
