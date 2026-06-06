package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDelivery_HMACAndHeaders(t *testing.T) {
	const secret = "s3cr3t"
	type capture struct {
		method, sig, ts, id string
		body                 []byte
	}
	received := make(chan capture, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- capture{
			method: r.Method,
			sig:    r.Header.Get("X-Virta-Signature"),
			ts:     r.Header.Get("X-Virta-Timestamp"),
			id:     r.Header.Get("X-Virta-Delivery-Id"),
			body:   body,
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	mgr := NewManager(nil, srv.Client())
	mgr.Register(Endpoint{ID: "e1", Name: "test", URL: srv.URL, Events: []string{"event.raid"}, Active: true}, secret)
	defer mgr.Close()

	d := Delivery{ID: "d1", Type: "event.raid", CreatedAt: time.Now(), Data: []byte(`{"platform":"twitch"}`)}
	mgr.Dispatch(d)

	select {
	case got := <-received:
		if got.method != http.MethodPost {
			t.Errorf("method = %q, want POST", got.method)
		}
		if got.id != "d1" {
			t.Errorf("delivery id = %q, want d1", got.id)
		}
		// Verify HMAC: sha256(timestamp + body)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(got.ts))
		mac.Write(got.body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if got.sig != expected {
			t.Errorf("HMAC sig mismatch: got %q, want %q", got.sig, expected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no delivery within 2s")
	}
}

func TestDelivery_EventFiltering(t *testing.T) {
	delivered := make(chan string, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d Delivery
		_ = json.NewDecoder(r.Body).Decode(&d)
		delivered <- d.Type
		w.WriteHeader(200)
	}))
	defer srv.Close()

	mgr := NewManager(nil, srv.Client())
	mgr.Register(Endpoint{ID: "e1", Name: "raids only", URL: srv.URL, Events: []string{"event.raid"}, Active: true}, "")
	defer mgr.Close()

	// Raid should match.
	mgr.Dispatch(Delivery{ID: "1", Type: "event.raid", CreatedAt: time.Now(), Data: []byte(`{}`)})
	// Follow should not.
	mgr.Dispatch(Delivery{ID: "2", Type: "event.follow", CreatedAt: time.Now(), Data: []byte(`{}`)})

	select {
	case t0 := <-delivered:
		if t0 != "event.raid" {
			t.Errorf("got %q, want event.raid", t0)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("no delivery within 300ms")
	}
	// No second delivery.
	select {
	case t1 := <-delivered:
		t.Errorf("unexpected second delivery: %q", t1)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAutoPauseAndResume(t *testing.T) {
	orig := initialBackoff
	initialBackoff = 5 * time.Millisecond
	t.Cleanup(func() { initialBackoff = orig })
	// Simulate sustained failure by pointing at a closed port (immediate connection refused).
	mgr := NewManager(nil, &http.Client{Timeout: 50 * time.Millisecond})
	mgr.Register(Endpoint{ID: "e1", Name: "fail", URL: "http://127.0.0.1:1", Events: []string{"event.raid"}, Active: true}, "")
	defer mgr.Close()

	// Feed 10 deliveries; each fails immediately (connection refused, no retry sleep for very
	// short timeouts). Give 1s for the worker to drain them and accumulate 5 failures.
	for i := range 10 {
		mgr.Dispatch(Delivery{ID: string(rune('a' + i)), Type: "event.raid", CreatedAt: time.Now(), Data: []byte(`{}`)})
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if mgr.IsPaused("e1") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !mgr.IsPaused("e1") {
		t.Error("endpoint should be auto-paused after sustained failures")
	}
	mgr.Resume("e1")
	if mgr.IsPaused("e1") {
		t.Error("endpoint should not be paused after Resume")
	}
}

func TestEventCatalog(t *testing.T) {
	cat := EventCatalog()
	if len(cat) == 0 {
		t.Error("EventCatalog is empty")
	}
	seen := map[string]bool{}
	for _, e := range cat {
		if seen[e] {
			t.Errorf("duplicate catalog entry: %q", e)
		}
		seen[e] = true
	}
}
