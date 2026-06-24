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

// GetChannelInfo: round-trips the broadcaster id, decodes title/game/tags from the data array.
func TestHelix_GetChannelInfo_Success(t *testing.T) {
	var gotQuery, gotAuth, gotClientID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("broadcaster_id")
		gotAuth = r.Header.Get("Authorization")
		gotClientID = r.Header.Get("Client-Id")
		_, _ = w.Write([]byte(`{"data":[{
			"broadcaster_id":"42",
			"broadcaster_login":"streamer",
			"title":"Apex grind",
			"game_id":"509658",
			"game_name":"Just Chatting",
			"broadcaster_language":"en",
			"tags":["english","speedrun"]
		}]}`))
	}))
	defer srv.Close()

	c := NewHelixClient(func() string { return "cid" }, srv.Client())
	c.SetBroadcasterURLs(srv.URL, "")
	info, err := c.GetChannelInfo(context.Background(), "tok", "42")
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if gotQuery != "42" {
		t.Errorf("broadcaster_id = %q, want 42", gotQuery)
	}
	if gotAuth != "Bearer tok" || gotClientID != "cid" {
		t.Errorf("headers: auth=%q client-id=%q", gotAuth, gotClientID)
	}
	if info.Title != "Apex grind" || info.GameName != "Just Chatting" || info.GameID != "509658" {
		t.Errorf("info = %+v", info)
	}
	if len(info.Tags) != 2 || info.Tags[0] != "english" {
		t.Errorf("tags = %v", info.Tags)
	}
}

// A fresh broadcaster with no metadata row should return zero-value, not an error.
func TestHelix_GetChannelInfo_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	c := NewHelixClient(func() string { return "cid" }, srv.Client())
	c.SetBroadcasterURLs(srv.URL, "")
	info, err := c.GetChannelInfo(context.Background(), "tok", "42")
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if info.Title != "" || info.GameID != "" {
		t.Errorf("expected zero-value info, got %+v", info)
	}
}

// An empty broadcaster id is a programmer error, not a request — we don't want to fan out a
// global-scope query and get unexpected data back.
func TestHelix_GetChannelInfo_RequiresBroadcasterID(t *testing.T) {
	c := NewHelixClient(func() string { return "cid" }, http.DefaultClient)
	if _, err := c.GetChannelInfo(context.Background(), "tok", ""); err == nil {
		t.Error("empty broadcaster id returned nil error")
	}
}

// UpdateChannelInfo: PATCH /channels?broadcaster_id=... carrying only the non-empty patch fields.
func TestHelix_UpdateChannelInfo_Success(t *testing.T) {
	var gotMethod, gotQuery string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.Query().Get("broadcaster_id")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := NewHelixClient(func() string { return "cid" }, srv.Client())
	c.SetBroadcasterURLs(srv.URL, "")

	patch := ChannelInfoPatch{Title: "New title", GameID: "509658", Tags: []string{"english"}}
	if err := c.UpdateChannelInfo(context.Background(), "tok", "42", patch); err != nil {
		t.Fatalf("UpdateChannelInfo: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotQuery != "42" {
		t.Errorf("broadcaster_id = %q, want 42", gotQuery)
	}
	if gotBody["title"] != "New title" || gotBody["game_id"] != "509658" {
		t.Errorf("body = %+v", gotBody)
	}
	tags, _ := gotBody["tags"].([]any)
	if len(tags) != 1 || tags[0] != "english" {
		t.Errorf("tags in body = %v", gotBody["tags"])
	}
	// broadcaster_language was empty in the patch, so it must not appear in the body.
	if _, ok := gotBody["broadcaster_language"]; ok {
		t.Error("empty language was sent; omitempty broken?")
	}
}

func TestHelix_UpdateChannelInfo_RequiresBroadcasterID(t *testing.T) {
	c := NewHelixClient(func() string { return "cid" }, http.DefaultClient)
	if err := c.UpdateChannelInfo(context.Background(), "tok", "", ChannelInfoPatch{Title: "x"}); err == nil {
		t.Error("empty broadcaster id returned nil error")
	}
}

func TestHelix_UpdateChannelInfo_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"missing scope channel:manage:broadcast"}`))
	}))
	defer srv.Close()
	c := NewHelixClient(func() string { return "cid" }, srv.Client())
	c.SetBroadcasterURLs(srv.URL, "")
	err := c.UpdateChannelInfo(context.Background(), "tok", "42", ChannelInfoPatch{Title: "x"})
	if err == nil {
		t.Fatal("401 returned nil error")
	}
	if !strings.Contains(err.Error(), "channel:manage:broadcast") {
		t.Errorf("error %q should carry the scope message so the UI can detect a re-auth need", err)
	}
}

// SearchCategories: GET with the query string, returns the decoded data array.
func TestHelix_SearchCategories_Success(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"509658","name":"Just Chatting","box_art_url":"https://x/{width}x{height}.png"},
			{"id":"33214","name":"Just Dance"}
		]}`))
	}))
	defer srv.Close()
	c := NewHelixClient(func() string { return "cid" }, srv.Client())
	c.SetBroadcasterURLs("", srv.URL)

	cats, err := c.SearchCategories(context.Background(), "tok", "just")
	if err != nil {
		t.Fatalf("SearchCategories: %v", err)
	}
	if gotQuery != "just" {
		t.Errorf("query = %q, want just", gotQuery)
	}
	if len(cats) != 2 || cats[0].ID != "509658" || cats[0].Name != "Just Chatting" {
		t.Errorf("cats = %+v", cats)
	}
	if !strings.Contains(cats[0].BoxArtURL, "{width}") {
		t.Errorf("box art url didn't survive: %q", cats[0].BoxArtURL)
	}
}

// An empty query short-circuits without an HTTP round trip — keeps the typeahead's "type to
// search" UI from spraying empty searches when the user clears the input.
func TestHelix_SearchCategories_EmptyQuery(t *testing.T) {
	c := NewHelixClient(func() string { return "cid" }, http.DefaultClient)
	cats, err := c.SearchCategories(context.Background(), "tok", "")
	if err != nil {
		t.Fatalf("SearchCategories: %v", err)
	}
	if cats != nil {
		t.Errorf("empty query should return nil slice, got %v", cats)
	}
}
