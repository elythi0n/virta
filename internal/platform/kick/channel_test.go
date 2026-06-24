package kick

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// GetChannelInfo: GET /channels?broadcaster_user_id=… decodes the first row's title + category.
func TestKick_GetChannelInfo_Success(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotQuery = r.URL.Query().Get("broadcaster_user_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{
			"broadcaster_user_id":42,
			"slug":"streamer",
			"stream_title":"Apex grind",
			"category":{"id":12345,"name":"Apex Legends","slug":"apex-legends"}
		}]}`))
	}))
	defer srv.Close()

	c := NewAPIClient(srv.Client())
	c.SetBaseURL(srv.URL)
	info, err := c.GetChannelInfo(context.Background(), "tok", "42")
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/channels" {
		t.Errorf("called %s %s", gotMethod, gotPath)
	}
	if gotQuery != "42" {
		t.Errorf("broadcaster_user_id query = %q", gotQuery)
	}
	if info.StreamTitle != "Apex grind" || info.Category.Name != "Apex Legends" || info.Category.ID != 12345 {
		t.Errorf("info = %+v", info)
	}
}

// An empty data array returns zero-value, not an error — same fresh-account contract as Twitch.
func TestKick_GetChannelInfo_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	c := NewAPIClient(srv.Client())
	c.SetBaseURL(srv.URL)
	info, err := c.GetChannelInfo(context.Background(), "tok", "42")
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if info.StreamTitle != "" || info.Category.ID != 0 {
		t.Errorf("expected zero-value info, got %+v", info)
	}
}

// A non-numeric broadcaster id is rejected before any HTTP call — protects against accidentally
// hitting the API with garbage like a login string when the upstream resolver fails.
func TestKick_GetChannelInfo_RejectsBadBroadcasterID(t *testing.T) {
	c := NewAPIClient(http.DefaultClient)
	if _, err := c.GetChannelInfo(context.Background(), "tok", "not-a-number"); err == nil {
		t.Error("non-numeric broadcaster id returned nil error")
	}
}

// UpdateChannelInfo: PATCH /channels with stream_title + category_id, only the non-empty fields.
func TestKick_UpdateChannelInfo_Success(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := NewAPIClient(srv.Client())
	c.SetBaseURL(srv.URL)

	patch := ChannelInfoPatch{Title: "New title", CategoryID: 12345}
	if err := c.UpdateChannelInfo(context.Background(), "tok", patch); err != nil {
		t.Fatalf("UpdateChannelInfo: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/channels" {
		t.Errorf("called %s %s", gotMethod, gotPath)
	}
	if gotBody["stream_title"] != "New title" {
		t.Errorf("stream_title = %v", gotBody["stream_title"])
	}
	// JSON numbers decode as float64 in untyped maps.
	if v, _ := gotBody["category_id"].(float64); v != 12345 {
		t.Errorf("category_id = %v", gotBody["category_id"])
	}
}

// An empty patch is a no-op — we don't bother hitting the API with nothing to set, since Kick
// would 400 a body without fields anyway.
func TestKick_UpdateChannelInfo_EmptyPatch(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()
	c := NewAPIClient(srv.Client())
	c.SetBaseURL(srv.URL)

	if err := c.UpdateChannelInfo(context.Background(), "tok", ChannelInfoPatch{}); err != nil {
		t.Fatalf("UpdateChannelInfo: %v", err)
	}
	if called {
		t.Error("empty patch should not have hit the server")
	}
}

// SearchCategories: GET /categories?q=…, decodes the data array.
func TestKick_SearchCategories_Success(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":12345,"name":"Apex Legends","slug":"apex-legends"}]}`))
	}))
	defer srv.Close()
	c := NewAPIClient(srv.Client())
	c.SetBaseURL(srv.URL)

	cats, err := c.SearchCategories(context.Background(), "tok", "apex")
	if err != nil {
		t.Fatalf("SearchCategories: %v", err)
	}
	if gotQuery != "apex" {
		t.Errorf("q = %q, want apex", gotQuery)
	}
	if len(cats) != 1 || cats[0].ID != 12345 || cats[0].Name != "Apex Legends" {
		t.Errorf("cats = %+v", cats)
	}
}

// Empty query short-circuits without HTTP — same contract as Twitch.
func TestKick_SearchCategories_EmptyQuery(t *testing.T) {
	c := NewAPIClient(http.DefaultClient)
	cats, err := c.SearchCategories(context.Background(), "tok", "")
	if err != nil {
		t.Fatalf("SearchCategories: %v", err)
	}
	if cats != nil {
		t.Errorf("empty query should return nil slice, got %v", cats)
	}
}

// 401 from the patch endpoint carries the missing-scope text so the UI can show the re-auth
// banner. A test pinning the substring guards against the platform-message detection drifting.
func TestKick_UpdateChannelInfo_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"missing required scope channel:write"}`))
	}))
	defer srv.Close()
	c := NewAPIClient(srv.Client())
	c.SetBaseURL(srv.URL)
	err := c.UpdateChannelInfo(context.Background(), "tok", ChannelInfoPatch{Title: "x"})
	if err == nil {
		t.Fatal("401 returned nil error")
	}
	if !strings.Contains(err.Error(), "channel:write") {
		t.Errorf("error should carry the scope message, got %q", err)
	}
}
