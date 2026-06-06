package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
)

// fakeChannels records join/leave calls and serves a canned list.
type fakeChannels struct {
	mu      sync.Mutex
	joins   [][3]string // {platform, slug, mode}
	leaves  [][2]string // {platform, slug}
	joinErr error
	list    []ChannelInfo
	caps    map[string]Capabilities
}

func (f *fakeChannels) Join(_ context.Context, platform, slug, mode string) error {
	if f.joinErr != nil {
		return f.joinErr
	}
	f.mu.Lock()
	f.joins = append(f.joins, [3]string{platform, slug, mode})
	f.mu.Unlock()
	return nil
}

func (f *fakeChannels) Leave(platform, slug string) error {
	f.mu.Lock()
	f.leaves = append(f.leaves, [2]string{platform, slug})
	f.mu.Unlock()
	return nil
}

func (f *fakeChannels) List() []ChannelInfo { return f.list }

func (f *fakeChannels) Capabilities() map[string]Capabilities { return f.caps }

func (f *fakeChannels) joinCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.joins)
}

// authedReq issues a request with the bearer token and returns the status code and body.
func authedReq(t *testing.T, method, url string, body []byte) (int, []byte) {
	t.Helper()
	var r *http.Request
	var err error
	if body != nil {
		r, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		r, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

func TestChannels_JoinRoutesToController(t *testing.T) {
	s := start(t)
	fc := &fakeChannels{}
	s.SetChannels(fc)

	body, _ := json.Marshal(channelRequest{Platform: "twitch", Slug: "forsen", Mode: "anonymous"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/channels?token="+s.Token(), body)
	if code != http.StatusNoContent {
		t.Fatalf("join status = %d, want 204", code)
	}
	if fc.joinCount() != 1 || fc.joins[0] != [3]string{"twitch", "forsen", "anonymous"} {
		t.Errorf("controller joins = %v", fc.joins)
	}
}

func TestChannels_RequiresAuth(t *testing.T) {
	s := start(t)
	s.SetChannels(&fakeChannels{})
	body, _ := json.Marshal(channelRequest{Platform: "twitch", Slug: "forsen"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/channels", body) // no token
	if code != http.StatusUnauthorized {
		t.Fatalf("no-token join status = %d, want 401", code)
	}
}

func TestChannels_BadRequestWithoutSlug(t *testing.T) {
	s := start(t)
	s.SetChannels(&fakeChannels{})
	body, _ := json.Marshal(channelRequest{Platform: "twitch"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/channels?token="+s.Token(), body)
	if code != http.StatusBadRequest {
		t.Fatalf("missing-slug status = %d, want 400", code)
	}
}

func TestChannels_UnknownPlatformIs400(t *testing.T) {
	s := start(t)
	s.SetChannels(&fakeChannels{joinErr: ErrUnknownPlatform})
	body, _ := json.Marshal(channelRequest{Platform: "myspace", Slug: "x"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/channels?token="+s.Token(), body)
	if code != http.StatusBadRequest {
		t.Fatalf("unknown-platform status = %d, want 400", code)
	}
}

func TestChannels_JoinErrorIsBadGateway(t *testing.T) {
	s := start(t)
	s.SetChannels(&fakeChannels{joinErr: errors.New("dial refused")})
	body, _ := json.Marshal(channelRequest{Platform: "twitch", Slug: "forsen"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/channels?token="+s.Token(), body)
	if code != http.StatusBadGateway {
		t.Fatalf("connection-failure status = %d, want 502", code)
	}
}

func TestChannels_LeaveByQuery(t *testing.T) {
	s := start(t)
	fc := &fakeChannels{}
	s.SetChannels(fc)
	code, _ := authedReq(t, http.MethodDelete, "http://"+s.Addr()+"/v1/channels?platform=twitch&slug=forsen&token="+s.Token(), nil)
	if code != http.StatusNoContent {
		t.Fatalf("leave status = %d, want 204", code)
	}
	if len(fc.leaves) != 1 || fc.leaves[0] != [2]string{"twitch", "forsen"} {
		t.Errorf("controller leaves = %v", fc.leaves)
	}
}

func TestChannels_ListReturnsJoined(t *testing.T) {
	s := start(t)
	s.SetChannels(&fakeChannels{list: []ChannelInfo{{Platform: "twitch", Slug: "forsen", State: "ok"}}})
	code, body := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/channels?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", code)
	}
	var resp struct {
		Channels []ChannelInfo `json:"channels"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Channels) != 1 || resp.Channels[0].Slug != "forsen" {
		t.Errorf("channels = %+v", resp.Channels)
	}
}

func TestCapabilities_ReturnsPerPlatform(t *testing.T) {
	s := start(t)
	s.SetChannels(&fakeChannels{caps: map[string]Capabilities{
		"twitch": {ReadAnonymous: true, Send: true, Moderation: true, Stability: "official"},
		"x":      {Stability: "besteffort"},
	}})
	code, body := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/capabilities?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("capabilities status = %d, want 200", code)
	}
	var resp struct {
		Capabilities map[string]Capabilities `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Capabilities["twitch"].Send || resp.Capabilities["x"].Stability != "besteffort" {
		t.Errorf("capabilities = %+v", resp.Capabilities)
	}
}

func TestChannels_UnavailableWithoutController(t *testing.T) {
	s := start(t) // SetChannels never called
	code, _ := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/v1/channels?token="+s.Token(), nil)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("no-controller status = %d, want 503", code)
	}
}
