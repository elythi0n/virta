package kick

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPFetcher_StatusMapping(t *testing.T) {
	var status int
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	f := &httpFetcher{
		client:  srv.Client(),
		url:     func(slug string) string { return srv.URL + "/" + slug },
		extract: extractChatroomID,
	}

	cases := []struct {
		name    string
		status  int
		body    string
		wantID  string
		wantErr error
	}{
		{"ok", http.StatusOK, `{"chatroom":{"id":12345}}`, "12345", nil},
		{"blocked", http.StatusForbidden, "blocked", "", errBlocked},
		{"rate limited", http.StatusTooManyRequests, "", "", errBlocked},
		{"not found", http.StatusNotFound, "", "", errNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body = c.status, c.body
			id, err := f.Fetch(context.Background(), "xqc")
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Errorf("err = %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil || id != c.wantID {
				t.Errorf("Fetch = %q, %v; want %q", id, err, c.wantID)
			}
		})
	}

	t.Run("unexpected status is an error", func(t *testing.T) {
		status, body = http.StatusInternalServerError, ""
		if _, err := f.Fetch(context.Background(), "x"); err == nil {
			t.Error("status 500 returned nil error")
		}
	})
}

func TestExtractChatroomID(t *testing.T) {
	if id, err := extractChatroomID([]byte(`{"chatroom":{"id":98765}}`)); err != nil || id != "98765" {
		t.Errorf("valid = %q, %v", id, err)
	}
	if _, err := extractChatroomID([]byte(`{"chatroom":{}}`)); !errors.Is(err, errNotFound) {
		t.Errorf("missing id should be errNotFound, got %v", err)
	}
	if _, err := extractChatroomID([]byte(`not json`)); err == nil {
		t.Error("bad json returned nil error")
	}
}

func TestExtractOfficialChatroomID(t *testing.T) {
	if id, err := extractOfficialChatroomID([]byte(`{"data":[{"chatroom_id":555}]}`)); err != nil || id != "555" {
		t.Errorf("chatroom_id form = %q, %v", id, err)
	}
	if id, err := extractOfficialChatroomID([]byte(`{"data":[{"chatroom":{"id":777}}]}`)); err != nil || id != "777" {
		t.Errorf("nested form = %q, %v", id, err)
	}
	if _, err := extractOfficialChatroomID([]byte(`{"data":[]}`)); !errors.Is(err, errNotFound) {
		t.Errorf("empty data should be errNotFound, got %v", err)
	}
}

func TestNewFetchersConstruct(t *testing.T) {
	if NewDirectFetcher(nil) == nil || NewOfficialFetcher(nil) == nil {
		t.Error("fetcher constructors returned nil")
	}
}
