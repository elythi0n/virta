package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// fakeHistory records the params it was called with and serves canned rows.
type fakeHistory struct {
	rows       []LoggedMessage
	gotSearch  SearchParams
	gotHistory HistoryParams
}

func (f *fakeHistory) Search(_ context.Context, p SearchParams) ([]LoggedMessage, error) {
	f.gotSearch = p
	return f.rows, nil
}

func (f *fakeHistory) History(_ context.Context, p HistoryParams) ([]LoggedMessage, error) {
	f.gotHistory = p
	return f.rows, nil
}

func TestSearch_QueryAndFilters(t *testing.T) {
	s := start(t)
	fh := &fakeHistory{rows: []LoggedMessage{{ID: "m1", Channel: "twitch:forsen", Author: "bob", Body: "hello world"}}}
	s.SetHistory(fh)

	code, resp := authedReq(t, http.MethodGet,
		"http://"+s.Addr()+"/v1/search?q=hello&channel=twitch:forsen&author=bob&limit=20&token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("search status = %d (%s)", code, resp)
	}
	if fh.gotSearch.Text != "hello" || fh.gotSearch.Channel != "twitch:forsen" || fh.gotSearch.Author != "bob" || fh.gotSearch.Limit != 20 {
		t.Errorf("search params = %+v", fh.gotSearch)
	}
	var got struct {
		Messages []LoggedMessage `json:"messages"`
	}
	if err := json.Unmarshal(resp, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].ID != "m1" {
		t.Errorf("messages = %+v", got.Messages)
	}
}

func TestSearch_MissingQueryIs400(t *testing.T) {
	s := start(t)
	s.SetHistory(&fakeHistory{})
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/search?token="+s.Token(), nil)
	if code != http.StatusBadRequest {
		t.Errorf("missing q status = %d, want 400", code)
	}
}

func TestSearch_LimitDefaultsAndCaps(t *testing.T) {
	s := start(t)
	fh := &fakeHistory{}
	s.SetHistory(fh)

	authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/search?q=x&token="+s.Token(), nil)
	if fh.gotSearch.Limit != 100 {
		t.Errorf("default limit = %d, want 100", fh.gotSearch.Limit)
	}
	authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/search?q=x&limit=9999&token="+s.Token(), nil)
	if fh.gotSearch.Limit != 500 {
		t.Errorf("capped limit = %d, want 500", fh.gotSearch.Limit)
	}
}

func TestHistory_RequiresChannel(t *testing.T) {
	s := start(t)
	fh := &fakeHistory{rows: []LoggedMessage{{ID: "m1"}}}
	s.SetHistory(fh)

	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/history?token="+s.Token(), nil)
	if code != http.StatusBadRequest {
		t.Errorf("history without channel = %d, want 400", code)
	}

	code, _ = authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/history?channel=twitch:forsen&before=msg-9&token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("history status = %d", code)
	}
	if fh.gotHistory.Channel != "twitch:forsen" || fh.gotHistory.Before != "msg-9" {
		t.Errorf("history params = %+v", fh.gotHistory)
	}
}

func TestSearch_UnavailableUntilSet(t *testing.T) {
	s := start(t) // SetHistory never called
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/search?q=x&token="+s.Token(), nil)
	if code != http.StatusServiceUnavailable {
		t.Errorf("search without controller = %d, want 503", code)
	}
}
