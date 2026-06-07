package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
)

type fakeProfiles struct {
	mu        sync.Mutex
	list      []ProfileInfo
	activated []string
	createErr error
}

func (f *fakeProfiles) List(context.Context) ([]ProfileInfo, error) { return f.list, nil }

func (f *fakeProfiles) Create(_ context.Context, name string) (ProfileInfo, error) {
	if f.createErr != nil {
		return ProfileInfo{}, f.createErr
	}
	return ProfileInfo{ID: "new", Name: name}, nil
}

func (f *fakeProfiles) Activate(_ context.Context, id string) error {
	f.mu.Lock()
	f.activated = append(f.activated, id)
	f.mu.Unlock()
	return nil
}

func (f *fakeProfiles) Delete(_ context.Context, _ string) error { return nil }

func TestProfiles_List(t *testing.T) {
	s := start(t)
	s.SetProfiles(&fakeProfiles{list: []ProfileInfo{{ID: "1", Name: "default", Default: true, Active: true}}})
	code, body := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/profiles?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("list status = %d", code)
	}
	var resp struct {
		Profiles []ProfileInfo `json:"profiles"`
	}
	_ = json.Unmarshal(body, &resp)
	if len(resp.Profiles) != 1 || !resp.Profiles[0].Active {
		t.Errorf("profiles = %+v", resp.Profiles)
	}
}

func TestProfiles_CreateThenActivate(t *testing.T) {
	s := start(t)
	fp := &fakeProfiles{}
	s.SetProfiles(fp)

	body, _ := json.Marshal(map[string]string{"name": "mod-duty"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/profiles?token="+s.Token(), body)
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", code)
	}

	code, _ = authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/profiles/abc/activate?token="+s.Token(), nil)
	if code != http.StatusNoContent {
		t.Fatalf("activate status = %d, want 204", code)
	}
	if len(fp.activated) != 1 || fp.activated[0] != "abc" {
		t.Errorf("activated = %v, want [abc]", fp.activated)
	}
}

func TestProfiles_CreateBadRequestAndConflict(t *testing.T) {
	s := start(t)
	s.SetProfiles(&fakeProfiles{createErr: errors.New("duplicate name")})

	// Missing name → 400.
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/profiles?token="+s.Token(), []byte(`{}`))
	if code != http.StatusBadRequest {
		t.Errorf("missing-name status = %d, want 400", code)
	}
	// Create error → 409.
	body, _ := json.Marshal(map[string]string{"name": "dupe"})
	code, _ = authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/profiles?token="+s.Token(), body)
	if code != http.StatusConflict {
		t.Errorf("conflict status = %d, want 409", code)
	}
}

func TestProfiles_UnavailableWithoutController(t *testing.T) {
	s := start(t)
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/profiles?token="+s.Token(), nil)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("no-controller status = %d, want 503", code)
	}
}
