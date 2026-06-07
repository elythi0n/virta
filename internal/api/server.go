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
	RuntimeDir string   // where the discovery file is written
	CORSOrigins []string // opt-in CORS allowlist for local web tools (empty = CORS off)
	Logger     *slog.Logger
}

// Server is the local HTTP/WebSocket API.
type Server struct {
	log         *slog.Logger
	ring        *logRing
	hub         *hub
	channels    Channels          // join/leave controller, installed via SetChannels
	profiles    Profiles          // profile controller, installed via SetProfiles
	filters     Filters           // filter-ruleset controller, installed via SetFilters
	connections Connections       // per-platform connection-method controller, installed via SetConnections
	accounts    Accounts          // connected-accounts controller, installed via SetAccounts
	authConfig  AuthConfigControl // OAuth-credentials controller, installed via SetAuthConfig
	authCtl     Auth              // account-auth controller, installed via SetAuth
	send              Send              // cross-posting controller, installed via SetSend
	held              Held              // AutoMod hold-queue controller, installed via SetHeld
	history           History           // message-log search/scrollback controller, installed via SetHistory
	tokens            Tokens            // scoped API-token controller, installed via SetTokens
	portability       Portability       // profile import/export controller, installed via SetPortability
	themes            Themes            // custom theme management, installed via SetThemes
	webhooks          Webhooks          // outbound webhook management, installed via SetWebhooks
	mcpHandler        http.Handler      // MCP server, installed via SetMCPHandler (nil = not available)
	intel             Intel             // intelligence controller, installed via SetIntel
	hostedAuth        HostedAuth        // multi-user account surface (nil in local/desktop mode)
	webui             http.Handler      // embedded web UI, installed via SetWebUI (nil = not served)
	corsOrigins       []string          // opt-in CORS allowlist for local web tools (empty = CORS off)
	integrationReport any               // native-integration report forwarded from the desktop shell

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
		corsOrigins:   cfg.CORSOrigins,
		baseCtx:       ctx,
		cancel:        cancel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth) // public: liveness, no token
	mux.HandleFunc("GET /v1/openapi.json", s.handleOpenAPI)
	mux.HandleFunc("GET /v1/asyncapi.json", s.handleAsyncAPI)
	mux.HandleFunc("GET /docs", s.handleDocs)
	// Hosted multi-user auth — public (no bearer token required; users log in here).
	mux.HandleFunc("GET /auth/status", s.handleHostedStatus)
	mux.HandleFunc("POST /auth/register", s.handleHostedRegister)
	mux.HandleFunc("POST /auth/login", s.handleHostedLogin)
	mux.HandleFunc("POST /auth/logout", s.handleHostedLogout)
	mux.HandleFunc("GET /auth/me", s.handleHostedMe)
	// MCP server (Model Context Protocol): bearer-token gated, served at /mcp.
	// Mounted outside the declarative route table so it can accept any HTTP method.
	mux.Handle("/mcp", s.scoped(ScopeRead, http.HandlerFunc(s.handleMCP)))
	mux.Handle("/mcp/", s.scoped(ScopeRead, http.HandlerFunc(s.handleMCP)))
	// Every authenticated endpoint is declared once in routes(), with the scope a non-root token
	// needs; the same table drives the generated OpenAPI doc, so the contract can't drift.
	for _, rt := range s.routes() {
		mux.Handle(rt.method+" "+rt.path, s.scoped(rt.scope, http.HandlerFunc(rt.handler)))
	}
	// Bootstrap for a same-machine browser: hand a loopback client the token so a virtad-served
	// SPA can authenticate. Empty addr means "this origin". Remote clients are refused; serving a
	// UI beyond loopback needs the hosted auth layer (ADR-031, deferred).
	mux.HandleFunc("GET /__discovery", s.handleDiscovery)
	mux.HandleFunc("GET /__integration", s.handleDesktopIntegration)
	// /overlay: the transparent feed-only build for an OBS browser source (zero-install).
	// Served from the embedded web assets. The token must be passed as a query param:
	// http://127.0.0.1:<port>/overlay?token=<token>&channels=twitch:forsen
	mux.HandleFunc("GET /overlay", s.handleOverlay)
	// SPA fallback: anything not matched above is served from the embedded web UI (if present).
	mux.HandleFunc("/", s.handleWebUI)

	s.httpSrv = &http.Server{
		Handler:           s.withCORS(mux),
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

// SetIntegrationReport installs the native-integration report (resolved by the desktop shell) so
// the web UI can read the active rungs from /__integration without coupling to the shell.
func (s *Server) SetIntegrationReport(report any) { s.integrationReport = report }

// SetMCPHandler installs the MCP server handler at /mcp. Until called, /mcp returns 503.
func (s *Server) SetMCPHandler(h http.Handler) { s.mcpHandler = h }

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcpHandler == nil {
		http.Error(w, "MCP server unavailable (enable logging to use intelligence tools)", http.StatusServiceUnavailable)
		return
	}
	s.mcpHandler.ServeHTTP(w, r)
}

// SetWebUI installs the embedded web UI handler so the daemon serves the app itself. Passing nil
// (no UI compiled into the binary) leaves the SPA route answering "not built".
func (s *Server) SetWebUI(h http.Handler) { s.webui = h }

// handleDiscovery hands a loopback client the token (and an empty address meaning "this origin"),
// so a browser on this machine can authenticate to a virtad-served SPA with no configuration.
// Remote clients are refused: serving the UI beyond loopback needs the hosted auth layer (ADR-031).
func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, Discovery{Addr: "", Token: s.token})
}

// handleWebUI serves the embedded SPA (with its own fallback), or a short notice if this binary
// was built without a UI.
func (s *Server) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if s.webui == nil {
		http.Error(w, "web UI not built into this binary (run `make web`)", http.StatusNotFound)
		return
	}
	s.webui.ServeHTTP(w, r)
}

// handleOverlay rewrites to overlay.html so the token-gated transparent feed renders at /overlay.
func (s *Server) handleOverlay(w http.ResponseWriter, r *http.Request) {
	if !s.tokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.webui == nil {
		http.Error(w, "overlay not built", http.StatusNotFound)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/overlay.html"
	s.webui.ServeHTTP(w, r2)
}

// handleDesktopIntegration serves the shell's native-integration report (resolved by the desktop
// shell and forwarded here). When no report is installed (the daemon runs standalone) this returns
// a minimal "web fallback" document.
func (s *Server) handleDesktopIntegration(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		http.NotFound(w, r)
		return
	}
	if s.integrationReport != nil {
		writeJSON(w, s.integrationReport)
		return
	}
	writeJSON(w, map[string]any{
		"os": "unknown",
		"features": []map[string]any{
			{"id": "window", "rung": "browser"},
			{"id": "theme", "rung": "native"},
			{"id": "quicklaunch", "rung": "in_app"},
			{"id": "hotkeys", "rung": "in_app", "detail": "browser"},
			{"id": "notifications", "rung": "in_app"},
			{"id": "tray", "rung": "none"},
			{"id": "sounds", "rung": "visual"},
		},
	})
}

// tokenOK checks whether a request presents the root token (used internally by overlay/integration).
func (s *Server) tokenOK(r *http.Request) bool {
	tok := presentedToken(r)
	return tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) == 1
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

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
	// The token is intentionally not logged (it would leak into stderr/journald and the
	// diagnostics ring); it lives in the owner-only discovery file.
	s.log.Info("dev feed", "url", "http://"+s.Addr()+"/dev", "token", "in "+s.discoveryPath)
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

// route is one authenticated endpoint: method, path pattern, the scope a non-root token needs, the
// handler, and a one-line summary for the generated OpenAPI doc. One table = scope enforcement and
// the published contract from a single source.
type route struct {
	method  string
	path    string
	scope   Scope
	handler http.HandlerFunc
	summary string
}

func (s *Server) routes() []route {
	return []route{
		{"GET", "/v1/diagnostics", ScopeRead, s.handleDiagnostics, "Server diagnostics (clients, log ring)"},
		{"GET", "/v1/stream", ScopeRead, s.handleStream, "Live event stream (WebSocket)"},
		{"GET", "/v1/channels", ScopeRead, s.handleListChannels, "List joined channels"},
		{"GET", "/v1/capabilities", ScopeRead, s.handleCapabilities, "Per-platform capabilities"},
		{"GET", "/v1/streams", ScopeRead, s.handleListStreams, "Live stream metadata per channel"},
		{"GET", "/v1/emotes", ScopeRead, s.handleListEmotes, "Resolved emote sets"},
		{"GET", "/v1/filters", ScopeRead, s.handleListFilters, "Current filter ruleset"},
		{"GET", "/v1/search", ScopeRead, s.handleSearch, "Full-text search over the message log"},
		{"GET", "/v1/history", ScopeRead, s.handleHistory, "Per-channel scrollback"},
		{"GET", "/v1/held", ScopeRead, s.handleListHeld, "AutoMod hold queue"},
		{"GET", "/v1/accounts", ScopeRead, s.handleListAccounts, "Connected accounts"},
		{"GET", "/v1/connections/methods", ScopeRead, s.handleListMethods, "Per-platform connection method"},
		{"GET", "/v1/profiles", ScopeRead, s.handleListProfiles, "List workspace profiles"},
		{"GET", "/v1/auth/config", ScopeRead, s.handleGetAuthConfig, "Which platforms have OAuth credentials configured"},

		{"POST", "/v1/send", ScopeSend, s.handleSend, "Cross-post a message to channels"},
		{"POST", "/v1/send/preview", ScopeSend, s.handleSendPreview, "Preview per-target send reachability"},
		{"POST", "/v1/send/queue", ScopeSend, s.handleSendQueue, "Per-channel send-queue state"},

		{"POST", "/v1/held/{id}/approve", ScopeModerate, s.handleApproveHeld, "Approve a held message"},
		{"POST", "/v1/held/{id}/deny", ScopeModerate, s.handleDenyHeld, "Deny a held message"},

		{"PUT", "/v1/filters", ScopeControl, s.handleSetFilters, "Replace the filter ruleset"},
		{"PUT", "/v1/connections/method", ScopeControl, s.handleSetMethod, "Set a platform's connection method"},
		{"POST", "/v1/channels", ScopeControl, s.handleJoinChannel, "Join a channel"},
		{"DELETE", "/v1/channels", ScopeControl, s.handleLeaveChannel, "Leave a channel"},
		{"POST", "/v1/profiles", ScopeControl, s.handleCreateProfile, "Create a workspace profile"},
		{"POST", "/v1/profiles/{id}/activate", ScopeControl, s.handleActivateProfile, "Activate a profile"},

		{"PUT", "/v1/auth/config", ScopeAdmin, s.handleSetAuthConfig, "Set OAuth app credentials"},
		{"DELETE", "/v1/accounts/{id}", ScopeAdmin, s.handleDisconnectAccount, "Disconnect an account"},
		{"POST", "/v1/auth/twitch/device", ScopeAdmin, s.handleTwitchDeviceStart, "Begin Twitch device sign-in"},
		{"GET", "/v1/auth/twitch/device/{id}", ScopeAdmin, s.handleTwitchDeviceStatus, "Twitch device sign-in status"},
		{"POST", "/v1/auth/kick/start", ScopeAdmin, s.handleKickAuthStart, "Begin Kick sign-in"},
		{"GET", "/v1/auth/kick/{id}", ScopeAdmin, s.handleKickAuthStatus, "Kick sign-in status"},
		{"GET", "/v1/tokens", ScopeAdmin, s.handleListTokens, "List API tokens"},
		{"POST", "/v1/tokens", ScopeAdmin, s.handleMintToken, "Mint a scoped API token"},
		{"DELETE", "/v1/tokens/{id}", ScopeAdmin, s.handleRevokeToken, "Revoke an API token"},
		{"GET", "/v1/profiles/{id}/export", ScopeControl, s.handleExportProfile, "Export a profile to a portable JSON"},
		{"POST", "/v1/profiles/import", ScopeControl, s.handleImportProfile, "Import a profile from a portable JSON"},
		{"GET", "/v1/webhooks", ScopeControl, s.handleListWebhooks, "List webhook endpoints + event catalog"},
		{"POST", "/v1/webhooks", ScopeControl, s.handleCreateWebhook, "Create a webhook endpoint"},
		{"DELETE", "/v1/webhooks/{id}", ScopeControl, s.handleDeleteWebhook, "Delete a webhook endpoint"},
		{"GET", "/v1/webhooks/{id}/log", ScopeControl, s.handleWebhookLog, "Webhook delivery log (last 100)"},
		{"POST", "/v1/webhooks/{id}/resume", ScopeControl, s.handleResumeWebhook, "Resume a paused webhook"},
		{"GET", "/v1/themes", ScopeRead, s.handleListThemes, "List built-in and custom themes"},
		{"POST", "/v1/themes", ScopeControl, s.handleImportTheme, "Import a .vtheme JSON"},
		{"GET", "/v1/themes/{id}/export", ScopeRead, s.handleExportTheme, "Export a theme as .vtheme JSON"},
		{"DELETE", "/v1/themes/{id}", ScopeControl, s.handleDeleteTheme, "Delete a custom theme"},
		{"GET", "/v1/intel/models", ScopeRead, s.handleListModels, "List available AI models"},
		{"POST", "/v1/intel/ask", ScopeRead, s.handleAsk, "Ask a question over logged chat (agent loop, NDJSON stream)"},
		{"GET", "/v1/intel/config", ScopeRead, s.handleGetIntelConfig, "Get LLM configuration"},
		{"PUT", "/v1/intel/config", ScopeControl, s.handleSetIntelConfig, "Update LLM configuration"},
		{"GET", "/dev", ScopeRead, s.handleDev, "Developer event probe page"},
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"status": "ok", "version": buildinfo.String()})
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"clients":            s.hub.clientCount(),
		"unforwarded_events": s.hub.unforwardedCount(),
		"log":                s.ring.snapshot(),
		"crash_dumps":        crashDumps(s.runtimeDir),
	})
}

func crashDumps(runtimeDir string) []string {
	if runtimeDir == "" {
		return nil
	}
	// Avoid importing crash directly so the api package stays light; use a path glob.
	d, err := os.ReadDir(filepath.Join(runtimeDir, "crashes"))
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(d))
	for _, e := range d {
		if !e.IsDir() {
			paths = append(paths, filepath.Join(runtimeDir, "crashes", e.Name()))
		}
	}
	return paths
}

// withCORS adds CORS headers for an opt-in allowlist of origins (local web tools), and answers
// preflight requests. Off by default (empty allowlist): a same-origin SPA and the desktop webview
// never need it, so cross-origin access is something the user deliberately enables (ADR-017). A
// "*" entry allows any origin (credentials still ride the bearer token, not cookies).
func (s *Server) withCORS(next http.Handler) http.Handler {
	if len(s.corsOrigins) == 0 {
		return next
	}
	allowAll := false
	allowed := map[string]bool{}
	for _, o := range s.corsOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
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
