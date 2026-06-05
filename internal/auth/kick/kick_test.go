package kick

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/store"
)

// kickOAuth scripts Kick's token + users endpoints.
type kickOAuth struct {
	srv          *httptest.Server
	access       string
	refresh      string
	refreshFail  bool
	exchangeFail bool
}

func newKickOAuth(t *testing.T) *kickOAuth {
	o := &kickOAuth{access: "access-1", refresh: "refresh-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		grant := r.Form.Get("grant_type")
		if (grant == "refresh_token" && o.refreshFail) || (grant == "authorization_code" && o.exchangeFail) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"access_token": o.access, "refresh_token": o.refresh, "expires_in": 3600,
			"scope": "user:read chat:write",
		})
	})
	mux.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"data": []map[string]any{{"user_id": 55, "slug": "xqc", "name": "xQc"}}})
	})
	o.srv = httptest.NewServer(mux)
	t.Cleanup(o.srv.Close)
	return o
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newManager(t *testing.T, o *kickOAuth, clk clock.Clock) (*Manager, *secrets.Memory, *store.Memory) {
	t.Helper()
	c := NewClient("client-id", "", o.srv.Client(), clk)
	c.SetEndpoints("https://id.kick.com/oauth/authorize", o.srv.URL+"/token", o.srv.URL+"/users")
	vault := secrets.NewMemory()
	st := store.NewMemory(clk)
	m := NewManager(c, vault, st.Accounts(), id.NewFake("acc"), clk)
	t.Cleanup(func() { _ = m.Close() })
	return m, vault, st
}

func TestPKCEChallenge(t *testing.T) {
	sum := sha256.Sum256([]byte("verifier-abc"))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if got := pkceChallenge("verifier-abc"); got != want {
		t.Errorf("pkceChallenge = %q, want %q", got, want)
	}
}

func TestAuthFlow_EndToEnd(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newKickOAuth(t)
	m, vault, st := newManager(t, o, clk)
	ctx := context.Background()

	s, err := m.StartAuth(ctx)
	if err != nil {
		t.Fatalf("StartAuth: %v", err)
	}
	// The authorize URL carries the loopback redirect, the PKCE challenge, and state.
	u, _ := url.Parse(s.AuthorizeURL)
	q := u.Query()
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL missing PKCE: %s", s.AuthorizeURL)
	}
	redirect := q.Get("redirect_uri")
	state := q.Get("state")
	if redirect == "" || state == "" {
		t.Fatalf("authorize URL missing redirect/state: %s", s.AuthorizeURL)
	}

	// Play the browser: Kick redirects to the loopback callback with code + state.
	resp, err := http.Get(redirect + "?code=auth-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	_ = resp.Body.Close()

	got, ok := m.Status(s.ID)
	if !ok || got.State != StateAuthorized || got.Login != "xqc" {
		t.Fatalf("status = %+v (ok %v), want authorized/xqc", got, ok)
	}
	accs, _ := st.Accounts().List(ctx)
	if len(accs) != 1 || accs[0].Platform != platform.Kick || accs[0].PlatformUID != "55" || accs[0].SecretRef != SecretRef("55") {
		t.Fatalf("account = %+v", accs)
	}
	if v, err := vault.Get(ctx, SecretRef("55")); err != nil || v == "" {
		t.Errorf("token not stored: %v", err)
	}
}

func TestAuthFlow_StateMismatch(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	m, _, _ := newManager(t, newKickOAuth(t), clk)
	s, _ := m.StartAuth(context.Background())
	redirect := mustRedirect(t, s.AuthorizeURL)

	resp, err := http.Get(redirect + "?code=x&state=WRONG")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got, _ := m.Status(s.ID); got.State != StateError {
		t.Errorf("state = %q, want error on state mismatch", got.State)
	}
}

func TestAuthFlow_ProviderError(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	m, _, _ := newManager(t, newKickOAuth(t), clk)
	s, _ := m.StartAuth(context.Background())
	redirect := mustRedirect(t, s.AuthorizeURL)
	state := mustState(t, s.AuthorizeURL)

	resp, err := http.Get(redirect + "?error=access_denied&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got, _ := m.Status(s.ID); got.State != StateError {
		t.Errorf("state = %q, want error on provider error", got.State)
	}
}

func TestAuthFlow_ExchangeError(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newKickOAuth(t)
	o.exchangeFail = true
	m, _, _ := newManager(t, o, clk)
	s, _ := m.StartAuth(context.Background())
	resp, err := http.Get(mustRedirect(t, s.AuthorizeURL) + "?code=x&state=" + url.QueryEscape(mustState(t, s.AuthorizeURL)))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got, _ := m.Status(s.ID); got.State != StateError {
		t.Errorf("state = %q, want error on failed exchange", got.State)
	}
}

func TestAccessToken_RefreshError(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newKickOAuth(t)
	o.refreshFail = true
	m, _, _ := newManager(t, o, clk)
	ctx := context.Background()
	ref := SecretRef("55")
	_ = m.writeToken(ctx, ref, Token{Access: "old", Refresh: "r", ExpiresAt: clk.Now().Add(-time.Minute)})
	if _, err := m.AccessToken(ctx, ref); err == nil {
		t.Error("AccessToken with a failing refresh returned nil error")
	}
	// And a missing token errors too.
	if _, err := m.AccessToken(ctx, SecretRef("999")); err == nil {
		t.Error("AccessToken with no stored token returned nil error")
	}
}

func TestAccessToken_RefreshesAndRotates(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newKickOAuth(t)
	m, _, _ := newManager(t, o, clk)
	ctx := context.Background()
	ref := SecretRef("55")
	_ = m.writeToken(ctx, ref, Token{Access: "old", Refresh: "r", ExpiresAt: clk.Now().Add(-time.Minute)})

	o.access, o.refresh = "access-2", "refresh-2"
	got, err := m.AccessToken(ctx, ref)
	if err != nil || got != "access-2" {
		t.Fatalf("AccessToken = %q, %v; want access-2", got, err)
	}
	if stored, _ := m.readToken(ctx, ref); stored.Refresh != "refresh-2" {
		t.Errorf("stored refresh = %q, want rotated", stored.Refresh)
	}
}

func TestRestartSurvival(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newKickOAuth(t)
	vault := secrets.NewMemory()
	st := store.NewMemory(clk)
	ctx := context.Background()
	ref := SecretRef("55")
	b, _ := json.Marshal(Token{Access: "persisted", Refresh: "r", ExpiresAt: clk.Now().Add(time.Hour)})
	_ = vault.Set(ctx, ref, string(b))

	c := NewClient("client-id", "", o.srv.Client(), clk)
	m := NewManager(c, vault, st.Accounts(), id.NewFake("acc"), clk)
	t.Cleanup(func() { _ = m.Close() })
	if got, err := m.AccessToken(ctx, ref); err != nil || got != "persisted" {
		t.Errorf("after restart AccessToken = %q, %v", got, err)
	}
}

func mustRedirect(t *testing.T, authURL string) string {
	t.Helper()
	u, _ := url.Parse(authURL)
	return u.Query().Get("redirect_uri")
}

func mustState(t *testing.T, authURL string) string {
	t.Helper()
	u, _ := url.Parse(authURL)
	return u.Query().Get("state")
}
