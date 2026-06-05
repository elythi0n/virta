package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestDev_RequiresAuth(t *testing.T) {
	s := start(t)
	resp, err := http.Get("http://" + s.Addr() + "/dev")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token /dev status = %d, want 401", resp.StatusCode)
	}
}

func TestDev_ServesPageWithToken(t *testing.T) {
	s := start(t)
	code, body := authedReq(t, http.MethodGet, "http://"+s.Addr()+"/dev?token="+s.Token(), nil)
	if code != http.StatusOK {
		t.Fatalf("/dev status = %d, want 200", code)
	}
	page := string(body)
	if !strings.Contains(page, "<title>Virta — dev feed</title>") {
		t.Error("served page is not the dev feed")
	}
	if !strings.Contains(page, "/v1/stream") || !strings.Contains(page, "/v1/channels") {
		t.Error("dev page should drive the stream and channels endpoints")
	}
	// The M2 catch-up surfaces: profile switcher, stats line, and mask/highlight handling.
	for _, want := range []string{"/v1/profiles", "id=\"statsbar\"", "profile_changed", "'stats'", "masked"} {
		if !strings.Contains(page, want) {
			t.Errorf("dev page missing %q (M2 catch-up)", want)
		}
	}
}
