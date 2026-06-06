package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// fakeHeld records approve/deny calls and serves a canned queue.
type fakeHeld struct {
	list        []HeldMessage
	err         error
	notFound    bool
	approvedIDs []string
	deniedIDs   []string
}

func (f *fakeHeld) List() []HeldMessage { return f.list }

func (f *fakeHeld) Approve(_ context.Context, id string) error {
	if f.notFound {
		return ErrHeldNotFound
	}
	f.approvedIDs = append(f.approvedIDs, id)
	return f.err
}

func (f *fakeHeld) Deny(_ context.Context, id string) error {
	if f.notFound {
		return ErrHeldNotFound
	}
	f.deniedIDs = append(f.deniedIDs, id)
	return f.err
}

func TestHeld_ListsQueue(t *testing.T) {
	s := start(t)
	s.SetHeld(&fakeHeld{list: []HeldMessage{
		{ID: "h1", Channel: "twitch:forsen", Author: "bob", Text: "sus", Reason: "harassment"},
	}})

	code, resp := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/held?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (%s)", code, resp)
	}
	var got struct {
		Held []HeldMessage `json:"held"`
	}
	if err := json.Unmarshal(resp, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Held) != 1 || got.Held[0].ID != "h1" || got.Held[0].Reason != "harassment" {
		t.Errorf("held = %+v", got.Held)
	}
}

func TestHeld_Approve(t *testing.T) {
	s := start(t)
	fh := &fakeHeld{}
	s.SetHeld(fh)

	code, resp := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/held/h1/approve?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("approve status = %d, want 200 (%s)", code, resp)
	}
	if len(fh.approvedIDs) != 1 || fh.approvedIDs[0] != "h1" {
		t.Errorf("approved ids = %v, want [h1]", fh.approvedIDs)
	}
}

func TestHeld_Deny(t *testing.T) {
	s := start(t)
	fh := &fakeHeld{}
	s.SetHeld(fh)

	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/held/h2/deny?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("deny status = %d, want 200", code)
	}
	if len(fh.deniedIDs) != 1 || fh.deniedIDs[0] != "h2" {
		t.Errorf("denied ids = %v, want [h2]", fh.deniedIDs)
	}
}

func TestHeld_UnknownIDIs404(t *testing.T) {
	s := start(t)
	s.SetHeld(&fakeHeld{notFound: true})

	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/held/ghost/approve?token="+s.Token(), nil)
	if code != http.StatusNotFound {
		t.Errorf("approve unknown id status = %d, want 404", code)
	}
}

func TestHeld_UnavailableUntilSet(t *testing.T) {
	s := start(t) // SetHeld never called
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/held?token="+s.Token(), nil)
	if code != http.StatusServiceUnavailable {
		t.Errorf("list without controller status = %d, want 503", code)
	}
}
