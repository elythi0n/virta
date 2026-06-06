package twitch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// capture is one recorded moderation request: method, path, query, and decoded JSON body.
type capture struct {
	method string
	path   string
	query  url.Values
	body   map[string]any
}

// modServer records every request and replies 204, returning the captures slice and a configured
// Helix client pointed at it.
func modServer(t *testing.T) (*[]capture, *HelixClient) {
	t.Helper()
	caps := &[]capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := capture{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &c.body)
		}
		*caps = append(*caps, c)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	hc := NewHelixClient(func() string { return "cid" }, srv.Client())
	hc.SetModerationURLs(srv.URL+"/bans", srv.URL+"/chat", srv.URL+"/settings", srv.URL+"/automod")
	return caps, hc
}

func TestHelixBan_PermanentAndTimeout(t *testing.T) {
	caps, hc := modServer(t)

	if err := hc.Ban(context.Background(), "tok", "b1", "mod1", "u1", 0, "spam"); err != nil {
		t.Fatalf("ban: %v", err)
	}
	c := (*caps)[0]
	if c.method != http.MethodPost || c.query.Get("broadcaster_id") != "b1" || c.query.Get("moderator_id") != "mod1" {
		t.Errorf("ban request = %+v", c)
	}
	data, _ := c.body["data"].(map[string]any)
	if data["user_id"] != "u1" || data["reason"] != "spam" {
		t.Errorf("ban data = %+v", data)
	}
	if _, hasDur := data["duration"]; hasDur {
		t.Error("permanent ban must not carry a duration")
	}

	if err := hc.Ban(context.Background(), "tok", "b1", "mod1", "u1", 600, ""); err != nil {
		t.Fatalf("timeout: %v", err)
	}
	data2, _ := (*caps)[1].body["data"].(map[string]any)
	if data2["duration"] != float64(600) { // JSON numbers decode to float64
		t.Errorf("timeout duration = %v, want 600", data2["duration"])
	}
}

func TestHelixUnbanAndDelete(t *testing.T) {
	caps, hc := modServer(t)

	if err := hc.Unban(context.Background(), "tok", "b1", "mod1", "u1"); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if c := (*caps)[0]; c.method != http.MethodDelete || c.query.Get("user_id") != "u1" {
		t.Errorf("unban request = %+v", c)
	}

	if err := hc.DeleteMessage(context.Background(), "tok", "b1", "mod1", "msg9"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if c := (*caps)[1]; c.method != http.MethodDelete || c.query.Get("message_id") != "msg9" {
		t.Errorf("delete request = %+v", c)
	}

	if err := hc.ClearChat(context.Background(), "tok", "b1", "mod1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if c := (*caps)[2]; c.method != http.MethodDelete || c.query.Has("message_id") {
		t.Errorf("clear request = %+v (must omit message_id)", c)
	}
}

func TestHelixChatSettingsAndAutomod(t *testing.T) {
	caps, hc := modServer(t)

	if err := hc.UpdateChatSettings(context.Background(), "tok", "b1", "mod1", map[string]any{"emote_mode": true}); err != nil {
		t.Fatalf("chat settings: %v", err)
	}
	if c := (*caps)[0]; c.method != http.MethodPatch || c.body["emote_mode"] != true {
		t.Errorf("chat settings request = %+v", c)
	}

	if err := hc.ManageHeldMessage(context.Background(), "tok", "mod1", "h7", true); err != nil {
		t.Fatalf("automod allow: %v", err)
	}
	c := (*caps)[1]
	if c.method != http.MethodPost || c.body["action"] != "ALLOW" || c.body["msg_id"] != "h7" || c.body["user_id"] != "mod1" {
		t.Errorf("automod request = %+v", c)
	}

	if err := hc.ManageHeldMessage(context.Background(), "tok", "mod1", "h7", false); err != nil {
		t.Fatalf("automod deny: %v", err)
	}
	if (*caps)[2].body["action"] != "DENY" {
		t.Errorf("deny action = %v", (*caps)[2].body["action"])
	}
}

// TestAdapter_Moderate covers the adapter's typed-action routing: it resolves the broadcaster and
// (for ban) the target login, sends the right Helix call, and serves held approve/deny without a
// channel. An anonymous adapter reports the action unsupported.
func TestAdapter_Moderate(t *testing.T) {
	caps, hc := modServer(t)
	// The target-login lookup hits the users endpoint; point it at a server returning a fixed id.
	users := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"u-resolved"}]}`))
	}))
	t.Cleanup(users.Close)
	hc.SetUsersURL(users.URL)

	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })

	ch := platform.ChannelRef{Platform: platform.Twitch, Slug: "Forsen"}
	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModBan, Channel: ch, TargetUserID: "baddie"}); err != platform.ErrUnsupported {
		t.Fatalf("anonymous Moderate = %v, want ErrUnsupported", err)
	}

	a.Authenticate("mod-self",
		func(context.Context) (string, error) { return "tok", nil },
		hc,
		func(_ context.Context, login string) (string, error) {
			if login != "forsen" {
				t.Errorf("broadcaster resolve login = %q, want forsen", login)
			}
			return "b1", nil
		})
	if c := a.Capabilities(); !c.Moderation || !c.HeldQueue {
		t.Fatalf("authenticated caps = %+v, want Moderation + HeldQueue", c)
	}

	// Timeout: resolves the target login to its id and carries the duration.
	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModTimeout, Channel: ch, TargetUserID: "baddie", Duration: 90 * time.Second}); err != nil {
		t.Fatalf("timeout moderate: %v", err)
	}
	last := (*caps)[len(*caps)-1]
	data, _ := last.body["data"].(map[string]any)
	if data["user_id"] != "u-resolved" || data["duration"] != float64(90) || last.query.Get("moderator_id") != "mod-self" || last.query.Get("broadcaster_id") != "b1" {
		t.Errorf("timeout request = %+v", last)
	}

	// A numeric target id is used directly (no users lookup needed).
	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModBan, Channel: ch, TargetUserID: "12345"}); err != nil {
		t.Fatalf("ban by id: %v", err)
	}
	banData, _ := (*caps)[len(*caps)-1].body["data"].(map[string]any)
	if banData["user_id"] != "12345" {
		t.Errorf("ban by numeric id used %v, want 12345 verbatim", banData["user_id"])
	}

	// Held approve needs no channel; it manages by message id as the moderator.
	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModApproveHeld, TargetMessageID: "h1"}); err != nil {
		t.Fatalf("approve held: %v", err)
	}
	held := (*caps)[len(*caps)-1]
	if held.path != "/automod" || held.body["action"] != "ALLOW" || held.body["msg_id"] != "h1" {
		t.Errorf("approve held request = %+v", held)
	}

	// Slow-mode toggle patches chat settings with the wait time.
	if err := a.Moderate(context.Background(), platform.ModAction{Type: platform.ModSetSlow, Channel: ch, Enabled: true, Duration: 30 * time.Second}); err != nil {
		t.Fatalf("slow mode: %v", err)
	}
	slow := (*caps)[len(*caps)-1]
	if slow.method != http.MethodPatch || slow.body["slow_mode"] != true || slow.body["slow_mode_wait_time"] != float64(30) {
		t.Errorf("slow mode request = %+v", slow)
	}
}
