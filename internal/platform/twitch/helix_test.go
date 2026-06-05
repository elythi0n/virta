package twitch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
