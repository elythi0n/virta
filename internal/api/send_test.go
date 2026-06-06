package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// fakeSend records calls and serves canned results.
type fakeSend struct {
	targets     []SendTarget
	results     []SendResult
	queue       []QueueState
	err         error
	gotChannels []string
	gotText     string
}

func (f *fakeSend) Queue(targets []string) ([]QueueState, error) {
	f.gotChannels = targets
	if f.err != nil {
		return nil, f.err
	}
	return f.queue, nil
}

func (f *fakeSend) Preview(targets []string) ([]SendTarget, error) {
	f.gotChannels = targets
	if f.err != nil {
		return nil, f.err
	}
	return f.targets, nil
}

func (f *fakeSend) Send(_ context.Context, targets []string, text string) ([]SendResult, error) {
	f.gotChannels, f.gotText = targets, text
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func TestSend_CrossPostsToTargets(t *testing.T) {
	s := start(t)
	fs := &fakeSend{results: []SendResult{
		{Channel: "twitch:forsen", Status: SendSent},
		{Channel: "kick:xqc", Status: SendExcluded, Reason: "auth_required"},
	}}
	s.SetSend(fs)

	body, _ := json.Marshal(sendRequest{Channels: []string{"twitch:forsen", "kick:xqc"}, Text: "gg"})
	code, resp := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send?token="+s.Token(), body)
	if code != http.StatusOK {
		t.Fatalf("send status = %d, want 200 (%s)", code, resp)
	}
	if fs.gotText != "gg" || len(fs.gotChannels) != 2 {
		t.Errorf("controller got text=%q channels=%v", fs.gotText, fs.gotChannels)
	}
	var got struct {
		Results []SendResult `json:"results"`
	}
	if err := json.Unmarshal(resp, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Results) != 2 || got.Results[0].Status != SendSent || got.Results[1].Status != SendExcluded || got.Results[1].Reason != "auth_required" {
		t.Errorf("results = %+v", got.Results)
	}
}

func TestSend_PreviewReportsReachability(t *testing.T) {
	s := start(t)
	s.SetSend(&fakeSend{targets: []SendTarget{
		{Channel: "twitch:forsen", CanSend: true},
		{Channel: "kick:xqc", CanSend: false, Reason: "auth_required"},
	}})
	body, _ := json.Marshal(previewRequest{Channels: []string{"twitch:forsen", "kick:xqc"}})
	code, resp := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send/preview?token="+s.Token(), body)
	if code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", code)
	}
	var got struct {
		Targets []SendTarget `json:"targets"`
	}
	_ = json.Unmarshal(resp, &got)
	if len(got.Targets) != 2 || !got.Targets[0].CanSend || got.Targets[1].CanSend || got.Targets[1].Reason != "auth_required" {
		t.Errorf("targets = %+v", got.Targets)
	}
}

func TestSend_QueueReportsDepthAndCountdown(t *testing.T) {
	s := start(t)
	s.SetSend(&fakeSend{queue: []QueueState{
		{Channel: "twitch:forsen", Queued: 0, NextInMs: 0},
		{Channel: "kick:xqc", Queued: 2, NextInMs: 1200},
	}})
	body, _ := json.Marshal(previewRequest{Channels: []string{"twitch:forsen", "kick:xqc"}})
	code, resp := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send/queue?token="+s.Token(), body)
	if code != http.StatusOK {
		t.Fatalf("queue status = %d, want 200", code)
	}
	var got struct {
		Queue []QueueState `json:"queue"`
	}
	_ = json.Unmarshal(resp, &got)
	if len(got.Queue) != 2 || got.Queue[1].Queued != 2 || got.Queue[1].NextInMs != 1200 {
		t.Errorf("queue = %+v", got.Queue)
	}
}

func TestSend_RequiresAuth(t *testing.T) {
	s := start(t)
	s.SetSend(&fakeSend{})
	body, _ := json.Marshal(sendRequest{Channels: []string{"twitch:forsen"}, Text: "hi"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send", body) // no token
	if code != http.StatusUnauthorized {
		t.Fatalf("no-token send status = %d, want 401", code)
	}
}

func TestSend_BadRequest(t *testing.T) {
	s := start(t)
	s.SetSend(&fakeSend{})
	// Missing text.
	body, _ := json.Marshal(sendRequest{Channels: []string{"twitch:forsen"}})
	if code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send?token="+s.Token(), body); code != http.StatusBadRequest {
		t.Errorf("missing-text status = %d, want 400", code)
	}
	// Empty channels.
	body, _ = json.Marshal(sendRequest{Text: "hi"})
	if code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send?token="+s.Token(), body); code != http.StatusBadRequest {
		t.Errorf("empty-channels status = %d, want 400", code)
	}
}

func TestSend_UnknownPlatformIs400(t *testing.T) {
	s := start(t)
	s.SetSend(&fakeSend{err: ErrUnknownPlatform})
	body, _ := json.Marshal(sendRequest{Channels: []string{"myspace:bob"}, Text: "hi"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send?token="+s.Token(), body)
	if code != http.StatusBadRequest {
		t.Fatalf("unknown-platform status = %d, want 400", code)
	}
}

func TestSend_UnavailableWithoutController(t *testing.T) {
	s := start(t) // SetSend never called
	body, _ := json.Marshal(sendRequest{Channels: []string{"twitch:forsen"}, Text: "hi"})
	code, _ := authedReq(t, http.MethodPost, "http://"+s.Addr()+"/v1/send?token="+s.Token(), body)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("no-controller status = %d, want 503", code)
	}
}
