package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

type fakeAuth struct {
	device     DeviceSession
	deviceErr  error
	deviceOK   bool
	kick       AuthSession
	kickErr    error
	kickStatOK bool
}

func (f *fakeAuth) StartTwitchDevice(context.Context) (DeviceSession, error) {
	return f.device, f.deviceErr
}

func (f *fakeAuth) TwitchDeviceStatus(string) (DeviceSession, bool) {
	return f.device, f.deviceOK
}

func (f *fakeAuth) StartKickAuth(context.Context) (AuthSession, error) {
	return f.kick, f.kickErr
}

func (f *fakeAuth) KickAuthStatus(string) (AuthSession, bool) {
	return f.kick, f.kickStatOK
}

func TestAuth_TwitchDeviceStartAndStatus(t *testing.T) {
	s := start(t)
	s.SetAuth(&fakeAuth{
		device:   DeviceSession{ID: "d1", UserCode: "ABCD-EFGH", State: "pending"},
		deviceOK: true,
	})
	base := "http://" + s.Addr()

	code, body := authedReq(t, http.MethodPost, base+"/v1/auth/twitch/device?token="+s.Token(), nil)
	if code != http.StatusCreated {
		t.Fatalf("start status = %d, want 201", code)
	}
	var got DeviceSession
	_ = json.Unmarshal(body, &got)
	if got.UserCode != "ABCD-EFGH" {
		t.Errorf("user code = %q", got.UserCode)
	}

	code, _ = authedReq(t, http.MethodGet, base+"/v1/auth/twitch/device/d1?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
}

func TestAuth_TwitchStartErrorAndUnknownStatus(t *testing.T) {
	s := start(t)
	s.SetAuth(&fakeAuth{deviceErr: errors.New("provider down"), deviceOK: false})
	base := "http://" + s.Addr()

	code, _ := authedReq(t, http.MethodPost, base+"/v1/auth/twitch/device?token="+s.Token(), nil)
	if code != http.StatusBadGateway {
		t.Fatalf("start-error status = %d, want 502", code)
	}
	code, _ = authedReq(t, http.MethodGet, base+"/v1/auth/twitch/device/nope?token="+s.Token(), nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown-status code = %d, want 404", code)
	}
}

func TestAuth_KickStartAndStatus(t *testing.T) {
	s := start(t)
	s.SetAuth(&fakeAuth{
		kick:       AuthSession{ID: "k1", AuthorizeURL: "https://id.kick.com/oauth/authorize?x=1", State: "st"},
		kickStatOK: true,
	})
	base := "http://" + s.Addr()

	code, body := authedReq(t, http.MethodPost, base+"/v1/auth/kick/start?token="+s.Token(), nil)
	if code != http.StatusCreated {
		t.Fatalf("start status = %d, want 201", code)
	}
	var got AuthSession
	_ = json.Unmarshal(body, &got)
	if got.AuthorizeURL == "" {
		t.Errorf("authorize url empty: %+v", got)
	}

	code, _ = authedReq(t, http.MethodGet, base+"/v1/auth/kick/k1?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
}

func TestAuth_KickStartErrorAndUnknownStatus(t *testing.T) {
	s := start(t)
	s.SetAuth(&fakeAuth{kickErr: errors.New("not configured"), kickStatOK: false})
	base := "http://" + s.Addr()

	code, _ := authedReq(t, http.MethodPost, base+"/v1/auth/kick/start?token="+s.Token(), nil)
	if code != http.StatusBadGateway {
		t.Fatalf("start-error status = %d, want 502", code)
	}
	code, _ = authedReq(t, http.MethodGet, base+"/v1/auth/kick/nope?token="+s.Token(), nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown-status code = %d, want 404", code)
	}
}

func TestAuth_Unavailable(t *testing.T) {
	s := start(t) // no SetAuth → controller nil
	base := "http://" + s.Addr()

	for _, path := range []string{
		"/v1/auth/twitch/device",
		"/v1/auth/kick/start",
	} {
		code, _ := authedReq(t, http.MethodPost, base+path+"?token="+s.Token(), nil)
		if code != http.StatusServiceUnavailable {
			t.Errorf("POST %s with no controller = %d, want 503", path, code)
		}
	}
	for _, path := range []string{
		"/v1/auth/twitch/device/x",
		"/v1/auth/kick/x",
	} {
		code, _ := authedReq(t, http.MethodGet, base+path+"?token="+s.Token(), nil)
		if code != http.StatusServiceUnavailable {
			t.Errorf("GET %s with no controller = %d, want 503", path, code)
		}
	}
}
