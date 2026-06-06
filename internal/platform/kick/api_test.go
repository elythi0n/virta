package kick

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// apiServer scripts the official API endpoints and records what it was asked.
type apiServer struct {
	srv *httptest.Server

	lastMethod string
	lastPath   string
	lastAuth   string
	lastBody   map[string]any

	status     int    // forced status (0 = behave normally)
	retryAfter string // Retry-After header when status is 429
	sendUnsent bool   // answer is_sent=false
}

func newAPIServer(t *testing.T) (*apiServer, *APIClient) {
	t.Helper()
	a := &apiServer{}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.lastMethod, a.lastPath = r.Method, r.URL.Path
		a.lastAuth = r.Header.Get("Authorization")
		a.lastBody = nil
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&a.lastBody)
		}
		if a.status != 0 {
			if a.retryAfter != "" {
				w.Header().Set("Retry-After", a.retryAfter)
			}
			w.WriteHeader(a.status)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/chat":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":    map[string]any{"is_sent": !a.sendUnsent, "message_id": "msg-uuid-1"},
				"message": "OK",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/chat/msg-uuid-1":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/moderation/bans":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}, "message": "OK"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(a.srv.Close)
	c := NewAPIClient(a.srv.Client())
	c.SetBaseURL(a.srv.URL)
	return a, c
}

func TestSendChat_Success(t *testing.T) {
	a, c := newAPIServer(t)
	id, err := c.SendChat(context.Background(), "tok", "123", "hello chat", "")
	if err != nil || id != "msg-uuid-1" {
		t.Fatalf("SendChat = %q, %v; want msg-uuid-1", id, err)
	}
	if a.lastAuth != "Bearer tok" {
		t.Errorf("auth header = %q", a.lastAuth)
	}
	if a.lastBody["type"] != "user" || a.lastBody["broadcaster_user_id"] != float64(123) || a.lastBody["content"] != "hello chat" {
		t.Errorf("body = %v", a.lastBody)
	}
	if _, ok := a.lastBody["reply_to_message_id"]; ok {
		t.Error("reply id sent for a non-reply")
	}
}

func TestSendChat_Reply(t *testing.T) {
	a, c := newAPIServer(t)
	if _, err := c.SendChat(context.Background(), "tok", "123", "re", "parent-uuid"); err != nil {
		t.Fatal(err)
	}
	if a.lastBody["reply_to_message_id"] != "parent-uuid" {
		t.Errorf("reply id = %v", a.lastBody["reply_to_message_id"])
	}
}

func TestSendChat_NotSentSurfaced(t *testing.T) {
	a, c := newAPIServer(t)
	a.sendUnsent = true
	if _, err := c.SendChat(context.Background(), "tok", "123", "x", ""); err == nil {
		t.Error("accepted-but-unsent returned nil error")
	}
}

func TestSendChat_BadBroadcasterID(t *testing.T) {
	_, c := newAPIServer(t)
	if _, err := c.SendChat(context.Background(), "tok", "xqc", "x", ""); err == nil {
		t.Error("non-numeric broadcaster id returned nil error")
	}
}

func TestSendChat_RateLimited(t *testing.T) {
	a, c := newAPIServer(t)
	a.status, a.retryAfter = http.StatusTooManyRequests, "12"
	_, err := c.SendChat(context.Background(), "tok", "123", "x", "")
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v, want RateLimitError", err)
	}
	if rl.RetryAfter != 12*time.Second || rl.RateLimited() != 12*time.Second {
		t.Errorf("RetryAfter = %s, want 12s", rl.RetryAfter)
	}
}

func TestSendChat_RateLimitedNoHint(t *testing.T) {
	a, c := newAPIServer(t)
	a.status = http.StatusTooManyRequests
	_, err := c.SendChat(context.Background(), "tok", "123", "x", "")
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 0 {
		t.Fatalf("err = %v, want hint-less RateLimitError", err)
	}
	if rl.Error() == "" {
		t.Error("empty error text")
	}
}

func TestSendChat_ServerError(t *testing.T) {
	a, c := newAPIServer(t)
	a.status = http.StatusInternalServerError
	if _, err := c.SendChat(context.Background(), "tok", "123", "x", ""); err == nil {
		t.Error("500 returned nil error")
	}
}

func TestDeleteMessage(t *testing.T) {
	a, c := newAPIServer(t)
	if err := c.DeleteMessage(context.Background(), "tok", "msg-uuid-1"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if a.lastMethod != http.MethodDelete || a.lastPath != "/chat/msg-uuid-1" {
		t.Errorf("request = %s %s", a.lastMethod, a.lastPath)
	}
}

func TestBan_Permanent(t *testing.T) {
	a, c := newAPIServer(t)
	if err := c.Ban(context.Background(), "tok", "123", "55", 0, "spam"); err != nil {
		t.Fatal(err)
	}
	if a.lastMethod != http.MethodPost || a.lastPath != "/moderation/bans" {
		t.Errorf("request = %s %s", a.lastMethod, a.lastPath)
	}
	if _, ok := a.lastBody["duration"]; ok {
		t.Error("permanent ban carried a duration")
	}
	if a.lastBody["user_id"] != float64(55) || a.lastBody["reason"] != "spam" {
		t.Errorf("body = %v", a.lastBody)
	}
}

func TestBan_TimeoutRoundsUpToMinutes(t *testing.T) {
	a, c := newAPIServer(t)
	if err := c.Ban(context.Background(), "tok", "123", "55", 30*time.Second, ""); err != nil {
		t.Fatal(err)
	}
	if a.lastBody["duration"] != float64(1) {
		t.Errorf("30s timeout → duration %v, want 1 minute", a.lastBody["duration"])
	}
	if _, ok := a.lastBody["reason"]; ok {
		t.Error("empty reason was sent")
	}
}

func TestBan_TimeoutTooLongRefused(t *testing.T) {
	_, c := newAPIServer(t)
	if err := c.Ban(context.Background(), "tok", "123", "55", 8*24*time.Hour, ""); err == nil {
		t.Error("8-day timeout returned nil error")
	}
}

func TestUnban(t *testing.T) {
	a, c := newAPIServer(t)
	if err := c.Unban(context.Background(), "tok", "123", "55"); err != nil {
		t.Fatal(err)
	}
	if a.lastMethod != http.MethodDelete || a.lastPath != "/moderation/bans" {
		t.Errorf("request = %s %s", a.lastMethod, a.lastPath)
	}
	if a.lastBody["broadcaster_user_id"] != float64(123) || a.lastBody["user_id"] != float64(55) {
		t.Errorf("body = %v", a.lastBody)
	}
}

func TestModerate_MapsActions(t *testing.T) {
	cases := []struct {
		action     platform.ModAction
		wantMethod string
		wantPath   string
		wantDur    any // expected duration field, nil = absent
	}{
		{platform.ModAction{Type: platform.ModBan, TargetUserID: "55"}, http.MethodPost, "/moderation/bans", nil},
		{platform.ModAction{Type: platform.ModTimeout, TargetUserID: "55", Duration: 10 * time.Minute}, http.MethodPost, "/moderation/bans", float64(10)},
		{platform.ModAction{Type: platform.ModTimeout, TargetUserID: "55"}, http.MethodPost, "/moderation/bans", float64(1)}, // no duration → minimum
		{platform.ModAction{Type: platform.ModUnban, TargetUserID: "55"}, http.MethodDelete, "/moderation/bans", nil},
		{platform.ModAction{Type: platform.ModUntimeout, TargetUserID: "55"}, http.MethodDelete, "/moderation/bans", nil},
		{platform.ModAction{Type: platform.ModDeleteMessage, TargetMessageID: "msg-uuid-1"}, http.MethodDelete, "/chat/msg-uuid-1", nil},
	}
	for _, tc := range cases {
		a, c := newAPIServer(t)
		if err := c.Moderate(context.Background(), "tok", "123", tc.action); err != nil {
			t.Errorf("%s: %v", tc.action.Type, err)
			continue
		}
		if a.lastMethod != tc.wantMethod || a.lastPath != tc.wantPath {
			t.Errorf("%s → %s %s, want %s %s", tc.action.Type, a.lastMethod, a.lastPath, tc.wantMethod, tc.wantPath)
		}
		got, ok := a.lastBody["duration"]
		if tc.wantDur == nil && ok {
			t.Errorf("%s carried duration %v", tc.action.Type, got)
		}
		if tc.wantDur != nil && got != tc.wantDur {
			t.Errorf("%s duration = %v, want %v", tc.action.Type, got, tc.wantDur)
		}
	}
}

func TestModerate_UnsupportedActions(t *testing.T) {
	_, c := newAPIServer(t)
	for _, typ := range []platform.ModActionType{platform.ModClear, platform.ModSetSlow, platform.ModSetEmoteOnly} {
		err := c.Moderate(context.Background(), "tok", "123", platform.ModAction{Type: typ})
		if !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("%s = %v, want ErrUnsupported", typ, err)
		}
	}
}

// TestAdapter_AuthenticatedSendAndModerate covers the Kick adapter's authenticated path:
// Authenticate flips capabilities and routes Send/Moderate through the official API with the
// resolved broadcaster user id; Deauthenticate reverts to read-only.
func TestAdapter_AuthenticatedSendAndModerate(t *testing.T) {
	var sendBody map[string]any
	var sawBan bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/chat":
			_ = json.NewDecoder(r.Body).Decode(&sendBody)
			_, _ = w.Write([]byte(`{"data":{"is_sent":true,"message_id":"m1"}}`))
		case strings.HasSuffix(r.URL.Path, "/moderation/bans"):
			sawBan = true
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	api := NewAPIClient(srv.Client())
	api.SetBaseURL(srv.URL)

	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })

	if a.Capabilities().Send {
		t.Fatal("anonymous adapter should not advertise Send")
	}
	a.Authenticate(
		func(context.Context) (string, error) { return "tok", nil },
		api,
		func(_ context.Context, slug string) (string, error) {
			if slug != "xqc" {
				t.Errorf("resolve slug = %q, want xqc (lower-cased)", slug)
			}
			return "777", nil
		})
	if c := a.Capabilities(); !c.Send || !c.Moderation {
		t.Fatalf("authenticated capabilities = %+v, want Send + Moderation", c)
	}

	ch := platform.ChannelRef{Platform: platform.Kick, Slug: "xQc"}
	if err := a.Send(context.Background(), ch, "gg", platform.SendOpts{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sendBody["content"] != "gg" || sendBody["broadcaster_user_id"] != float64(777) {
		t.Errorf("send body = %+v", sendBody)
	}

	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModBan, Channel: ch, TargetUserID: "9"}); err != nil {
		t.Fatalf("Moderate: %v", err)
	}
	if !sawBan {
		t.Error("moderation did not reach the bans endpoint")
	}

	a.Deauthenticate()
	if a.Capabilities().Send {
		t.Error("deauthenticated adapter should not advertise Send")
	}
}

func TestBroadcasterID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slug") != "xqc" {
			t.Errorf("slug query = %q, want xqc", r.URL.Query().Get("slug"))
		}
		_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":777}]}`))
	}))
	defer srv.Close()
	c := NewAPIClient(srv.Client())
	c.SetBaseURL(srv.URL)
	id, err := c.BroadcasterID(context.Background(), "tok", "xqc")
	if err != nil || id != "777" {
		t.Fatalf("BroadcasterID = %q, %v; want 777", id, err)
	}
}
