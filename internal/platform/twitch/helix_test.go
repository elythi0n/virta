package twitch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

func TestHelixSend_Success(t *testing.T) {
	var gotBody map[string]string
	var gotAuth, gotClientID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotClientID = r.Header.Get("Client-Id")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"data":[{"message_id":"m1","is_sent":true}]}`))
	}))
	defer srv.Close()

	c := NewHelixClient("cid", srv.Client())
	c.SetSendURL(srv.URL)
	id, err := c.SendChat(context.Background(), "tok", "b1", "s1", "hello", "p1")
	if err != nil || id != "m1" {
		t.Fatalf("SendChat = %q, %v; want m1", id, err)
	}
	if gotAuth != "Bearer tok" || gotClientID != "cid" {
		t.Errorf("headers: auth=%q client-id=%q", gotAuth, gotClientID)
	}
	if gotBody["broadcaster_id"] != "b1" || gotBody["sender_id"] != "s1" || gotBody["message"] != "hello" || gotBody["reply_parent_message_id"] != "p1" {
		t.Errorf("request body = %+v", gotBody)
	}
}

func TestHelixSend_DroppedSurfacesReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"message_id":"","is_sent":false,"drop_reason":{"code":"msg_duplicate","message":"duplicate message"}}]}`))
	}))
	defer srv.Close()
	c := NewHelixClient("cid", srv.Client())
	c.SetSendURL(srv.URL)
	_, err := c.SendChat(context.Background(), "tok", "b", "s", "dupe", "")
	if err == nil || !strings.Contains(err.Error(), "duplicate message") {
		t.Errorf("dropped send err = %v, want the drop reason surfaced", err)
	}
}

func TestHelixSend_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer srv.Close()
	c := NewHelixClient("cid", srv.Client())
	c.SetSendURL(srv.URL)
	if _, err := c.SendChat(context.Background(), "tok", "b", "s", "hi", ""); err == nil {
		t.Error("401 returned nil error")
	}
}

func TestHelixUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("login") != "forsen" {
			t.Errorf("login query = %q, want forsen", r.URL.Query().Get("login"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"12345"}]}`))
	}))
	defer srv.Close()
	c := NewHelixClient("cid", srv.Client())
	c.SetUsersURL(srv.URL)
	id, err := c.UserID(context.Background(), "tok", "forsen")
	if err != nil || id != "12345" {
		t.Fatalf("UserID = %q, %v; want 12345", id, err)
	}
}

// TestAdapter_AuthenticatedSend covers the authenticated send path: Authenticate flips
// capabilities and routes Send through Helix with the resolved broadcaster id and the account's
// sender id; Deauthenticate reverts to read-only.
func TestAdapter_AuthenticatedSend(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"data":[{"message_id":"m1","is_sent":true}]}`))
	}))
	defer srv.Close()
	helix := NewHelixClient("cid", srv.Client())
	helix.SetSendURL(srv.URL)

	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })

	if a.Capabilities().Send {
		t.Fatal("anonymous adapter should not advertise Send")
	}
	a.Authenticate("sender-7",
		func(context.Context) (string, error) { return "tok", nil },
		helix,
		func(_ context.Context, login string) (string, error) {
			if login != "forsen" {
				t.Errorf("resolve login = %q, want forsen (lower-cased)", login)
			}
			return "b1", nil
		})
	if c := a.Capabilities(); !c.Send || !c.ReadAuthed {
		t.Fatalf("authenticated capabilities = %+v, want Send + ReadAuthed", c)
	}

	ch := platform.ChannelRef{Platform: platform.Twitch, Slug: "Forsen"}
	if err := a.Send(context.Background(), ch, "hello", platform.SendOpts{ReplyParentID: "p1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotBody["broadcaster_id"] != "b1" || gotBody["sender_id"] != "sender-7" || gotBody["message"] != "hello" || gotBody["reply_parent_message_id"] != "p1" {
		t.Errorf("send body = %+v", gotBody)
	}

	if err := a.Send(context.Background(), ch, "waves", platform.SendOpts{Action: true}); err != nil {
		t.Fatalf("action send: %v", err)
	}
	if gotBody["message"] != "/me waves" {
		t.Errorf("action message = %q, want /me waves", gotBody["message"])
	}

	a.Deauthenticate()
	if a.Capabilities().Send {
		t.Error("deauthenticated adapter should not advertise Send")
	}
}
