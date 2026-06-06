package twitch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/secrets"
	"github.com/elythi0n/virta/internal/store"
)

// oauthServer scripts the Twitch OAuth endpoints. tokenCalls counts /token hits so the device
// poll can return "pending" first and then succeed.
type oauthServer struct {
	srv          *httptest.Server
	tokenCalls   atomic.Int32
	pendUntil    int32  // return authorization_pending until this many token calls
	slowUntil    int32  // return slow_down until this many token calls
	access       string // current access token handed out
	refresh      string // current refresh token handed out
	denied       bool   // make the device poll report access_denied
	expired      bool   // make the device poll report expired
	deviceFail   bool   // make /device fail
	validateFail bool   // make /validate fail
	refreshFail  bool   // make refresh-grant /token fail
}

func newOAuthServer(t *testing.T) *oauthServer {
	o := &oauthServer{pendUntil: 1, access: "access-1", refresh: "refresh-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		if o.deviceFail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"device_code": "dev-code", "user_code": "ABCD-1234",
			"verification_uri": "https://twitch.tv/activate", "expires_in": 300, "interval": 1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "refresh_token" {
			if o.refreshFail {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, map[string]any{"message": "server"})
				return
			}
			writeJSON(w, map[string]any{
				"access_token": o.access, "refresh_token": o.refresh, "expires_in": 3600,
				"scope": []string{"user:read:chat", "user:write:chat"},
			})
			return
		}
		// device grant
		n := o.tokenCalls.Add(1)
		bad := func(msg string) { w.WriteHeader(http.StatusBadRequest); writeJSON(w, map[string]any{"message": msg}) }
		switch {
		case o.denied:
			bad("access_denied")
		case o.expired:
			bad("expired_token")
		case n <= o.slowUntil:
			bad("slow_down")
		case n <= o.slowUntil+o.pendUntil:
			bad("authorization_pending")
		default:
			writeJSON(w, map[string]any{
				"access_token": o.access, "refresh_token": o.refresh, "expires_in": 3600,
				"scope": []string{"user:read:chat", "user:write:chat"},
			})
		}
	})
	mux.HandleFunc("/validate", func(w http.ResponseWriter, _ *http.Request) {
		if o.validateFail {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]any{"user_id": "42", "login": "streamer", "scopes": []string{"user:read:chat"}})
	})
	o.srv = httptest.NewServer(mux)
	t.Cleanup(o.srv.Close)
	return o
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (o *oauthServer) client(clk clock.Clock) *Client {
	c := NewClient("test-client-id", o.srv.Client(), clk)
	c.SetEndpoints(o.srv.URL+"/device", o.srv.URL+"/token", o.srv.URL+"/validate")
	return c
}

func newManager(t *testing.T, o *oauthServer, clk clock.Clock) (*Manager, *secrets.Memory, *store.Memory) {
	t.Helper()
	vault := secrets.NewMemory()
	st := store.NewMemory(clk)
	m := NewManager(o.client(clk), vault, st.Accounts(), id.NewFake("acc"), clk)
	m.unit = time.Millisecond // poll fast in tests
	t.Cleanup(func() { _ = m.Close() })
	return m, vault, st
}

func waitState(t *testing.T, m *Manager, id string, want State) Session {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s, ok := m.Status(id); ok && s.State != StatePending {
			if s.State != want {
				t.Fatalf("session state = %q (%s), want %q", s.State, s.Error, want)
			}
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("session %s never left pending", id)
	return Session{}
}

func TestDeviceFlow_EndToEnd(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	m, vault, st := newManager(t, o, clk)
	authed := make(chan store.Account, 1)
	m.SetOnAuthorized(func(a store.Account) { authed <- a })

	s, err := m.StartDevice(context.Background())
	if err != nil {
		t.Fatalf("StartDevice: %v", err)
	}
	if s.UserCode != "ABCD-1234" || s.VerificationURI == "" {
		t.Fatalf("device session = %+v", s)
	}

	done := waitState(t, m, s.ID, StateAuthorized)
	if done.Login != "streamer" {
		t.Errorf("login = %q", done.Login)
	}
	// Account row created with the right secret ref.
	accs, _ := st.Accounts().List(context.Background())
	if len(accs) != 1 || accs[0].PlatformUID != "42" || accs[0].SecretRef != SecretRef("42") || accs[0].Platform != platform.Twitch {
		t.Fatalf("account = %+v", accs)
	}
	// Token actually stored in the vault.
	if v, err := vault.Get(context.Background(), SecretRef("42")); err != nil || v == "" {
		t.Errorf("token not stored: %v", err)
	}
	// The wiring hook fired with the authorized account.
	select {
	case acc := <-authed:
		if acc.PlatformUID != "42" {
			t.Errorf("OnAuthorized account = %+v, want uid 42", acc)
		}
	case <-time.After(2 * time.Second):
		t.Error("OnAuthorized did not fire")
	}
}

func TestDeviceFlow_Denied(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	o.denied = true
	m, _, _ := newManager(t, o, clk)
	s, _ := m.StartDevice(context.Background())
	waitState(t, m, s.ID, StateDenied)
}

func TestAccessToken_RefreshesAndRotates(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	m, _, _ := newManager(t, o, clk)
	ctx := context.Background()

	// Seed an expired token directly in the vault.
	ref := SecretRef("42")
	_ = m.writeToken(ctx, ref, Token{Access: "old", Refresh: "old-refresh", ExpiresAt: clk.Now().Add(-time.Minute)})

	o.access, o.refresh = "access-2", "refresh-2"
	got, err := m.AccessToken(ctx, ref)
	if err != nil || got != "access-2" {
		t.Fatalf("AccessToken = %q, %v; want refreshed access-2", got, err)
	}
	// The new (rotated) refresh token is persisted.
	stored, _ := m.readToken(ctx, ref)
	if stored.Refresh != "refresh-2" {
		t.Errorf("stored refresh = %q, want rotated refresh-2", stored.Refresh)
	}
}

func TestAccessToken_FreshSkipsRefresh(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	m, _, _ := newManager(t, o, clk)
	ctx := context.Background()
	ref := SecretRef("42")
	_ = m.writeToken(ctx, ref, Token{Access: "still-good", Refresh: "r", ExpiresAt: clk.Now().Add(time.Hour)})
	if got, err := m.AccessToken(ctx, ref); err != nil || got != "still-good" {
		t.Errorf("AccessToken = %q, %v; want the unexpired token", got, err)
	}
}

func TestRestartSurvival(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	vault := secrets.NewMemory()
	st := store.NewMemory(clk)
	ctx := context.Background()
	ref := SecretRef("42")
	_ = vault.Set(ctx, ref, mustJSON(Token{Access: "persisted", Refresh: "r", ExpiresAt: clk.Now().Add(time.Hour)}))

	// A fresh Manager (as after a restart) reads the token straight from the vault.
	m := NewManager(o.client(clk), vault, st.Accounts(), id.NewFake("acc"), clk)
	t.Cleanup(func() { _ = m.Close() })
	if got, err := m.AccessToken(ctx, ref); err != nil || got != "persisted" {
		t.Errorf("after restart AccessToken = %q, %v; want persisted", got, err)
	}
}

func mustJSON(t Token) string {
	b, _ := json.Marshal(t)
	return string(b)
}

func TestClient_DefaultHTTPClient(t *testing.T) {
	if NewClient("id", nil, clock.NewFake(time.Unix(0, 0))) == nil {
		t.Error("NewClient returned nil")
	}
}

func TestPoll_SlowDownThenSucceeds(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	o.slowUntil, o.pendUntil = 1, 1 // slow_down, then pending, then success
	m, _, _ := newManager(t, o, clk)
	s, _ := m.StartDevice(context.Background())
	waitState(t, m, s.ID, StateAuthorized)
}

func TestPoll_Expired(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	o.expired = true
	m, _, _ := newManager(t, o, clk)
	s, _ := m.StartDevice(context.Background())
	waitState(t, m, s.ID, StateExpired)
}

func TestPoll_ValidateErrorIsError(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	o.pendUntil, o.validateFail = 0, true // succeed the token poll, fail validate
	m, _, _ := newManager(t, o, clk)
	s, _ := m.StartDevice(context.Background())
	waitState(t, m, s.ID, StateError)
}

func TestStartDevice_ServerError(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	o.deviceFail = true
	m, _, _ := newManager(t, o, clk)
	if _, err := m.StartDevice(context.Background()); err == nil {
		t.Error("StartDevice with a failing /device returned nil error")
	}
}

func TestStatus_Unknown(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	m, _, _ := newManager(t, newOAuthServer(t), clk)
	if _, ok := m.Status("nope"); ok {
		t.Error("Status of an unknown session returned ok=true")
	}
}

func TestAccessToken_Errors(t *testing.T) {
	clk := clock.NewFake(time.Unix(1_000_000, 0))
	o := newOAuthServer(t)
	m, _, _ := newManager(t, o, clk)
	ctx := context.Background()
	// No stored token → error.
	if _, err := m.AccessToken(ctx, SecretRef("999")); err == nil {
		t.Error("AccessToken with no stored token returned nil error")
	}
	// Expired token + failing refresh → error.
	o.refreshFail = true
	ref := SecretRef("42")
	_ = m.writeToken(ctx, ref, Token{Access: "old", Refresh: "r", ExpiresAt: clk.Now().Add(-time.Minute)})
	if _, err := m.AccessToken(ctx, ref); err == nil {
		t.Error("AccessToken with a failing refresh returned nil error")
	}
}
