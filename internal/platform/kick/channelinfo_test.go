package kick

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
)

// authedAdapter wires an adapter to a scripted official-API server with a fixed broadcaster-id
// resolver, returning the adapter and the server so tests can assert on what it received.
func authedAdapter(t *testing.T, h http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	api := NewAPIClient(srv.Client())
	api.SetBaseURL(srv.URL)

	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })
	a.Authenticate(
		func(context.Context) (string, error) { return "tok", nil },
		api,
		func(_ context.Context, slug string) (string, error) {
			if slug != "xqc" {
				t.Errorf("resolve slug = %q, want lower-cased xqc", slug)
			}
			return "777", nil
		})
	return a
}

func TestAdapter_ResolveID(t *testing.T) {
	a := authedAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	id, err := a.ResolveID(context.Background(), "xqc")
	if err != nil || id != "777" {
		t.Fatalf("ResolveID = %q, %v; want 777", id, err)
	}
}

func TestAdapter_ResolveIDUnauthenticated(t *testing.T) {
	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })
	if _, err := a.ResolveID(context.Background(), "xqc"); err == nil {
		t.Fatal("ResolveID without auth returned nil error")
	}
}

func TestAdapter_GetChannelInfo(t *testing.T) {
	var gotPath, gotQuery string
	a := authedAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query().Get("broadcaster_user_id")
		_, _ = w.Write([]byte(`{"data":[{"broadcaster_user_id":777,"slug":"xqc","stream_title":"live now","category":{"id":7,"name":"Just Chatting"}}]}`))
	})
	info, err := a.GetChannelInfo(context.Background(), "xQc")
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if gotPath != "/channels" || gotQuery != "777" {
		t.Errorf("request = %s?broadcaster_user_id=%s, want /channels?...=777", gotPath, gotQuery)
	}
	if info.StreamTitle != "live now" || info.Category.Name != "Just Chatting" {
		t.Errorf("info = %+v", info)
	}
}

func TestAdapter_UpdateChannelInfo(t *testing.T) {
	var body map[string]any
	var method string
	a := authedAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		_ = json.NewDecoder(r.Body).Decode(&body)
	})
	if err := a.UpdateChannelInfo(context.Background(), "xqc", ChannelInfoPatch{Title: "new title", CategoryID: 42}); err != nil {
		t.Fatalf("UpdateChannelInfo: %v", err)
	}
	if method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", method)
	}
	if body["stream_title"] != "new title" || body["category_id"] != float64(42) {
		t.Errorf("patch body = %+v", body)
	}
}

func TestAdapter_SearchCategories(t *testing.T) {
	var gotQuery string
	a := authedAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"Counter-Strike"},{"id":2,"name":"Chess"}]}`))
	})
	cats, err := a.SearchCategories(context.Background(), "chess")
	if err != nil {
		t.Fatalf("SearchCategories: %v", err)
	}
	if gotQuery != "chess" {
		t.Errorf("query = %q, want chess", gotQuery)
	}
	if len(cats) != 2 || cats[0].Name != "Counter-Strike" {
		t.Errorf("categories = %+v", cats)
	}
}

// The Deck calls these on an anonymous adapter before sign-in; each must report unsupported
// rather than panic on the nil auth pointer.
func TestAdapter_ChannelInfoUnauthenticated(t *testing.T) {
	a := New(Options{Dial: dialFake(newFakeTransport())})
	t.Cleanup(func() { _ = a.Close() })
	ctx := context.Background()
	if _, err := a.GetChannelInfo(ctx, "xqc"); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("GetChannelInfo = %v, want ErrUnsupported", err)
	}
	if err := a.UpdateChannelInfo(ctx, "xqc", ChannelInfoPatch{Title: "x"}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("UpdateChannelInfo = %v, want ErrUnsupported", err)
	}
	if _, err := a.SearchCategories(ctx, "chess"); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SearchCategories = %v, want ErrUnsupported", err)
	}
}
