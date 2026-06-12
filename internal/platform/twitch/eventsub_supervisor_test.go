package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// ── fakes ────────────────────────────────────────────────────────────────────

// esScriptDial hands out scripted conns in order; a nil entry means "dial fails".
type esScriptDial struct {
	mu    sync.Mutex
	conns []*esFakeConn
	urls  []string
}

func (d *esScriptDial) dial(_ context.Context, url string) (esConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.urls = append(d.urls, url)
	if len(d.conns) == 0 {
		return nil, errors.New("no more scripted conns")
	}
	c := d.conns[0]
	d.conns = d.conns[1:]
	if c == nil {
		return nil, errors.New("scripted dial failure")
	}
	return c, nil
}

func (d *esScriptDial) dialedURLs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.urls...)
}

func welcomeFrame(sessionID string) []byte {
	return []byte(fmt.Sprintf(`{"metadata":{"message_type":"session_welcome"},"payload":{"session":{"id":%q,"status":"connected"}}}`, sessionID))
}

func reconnectFrame(url string) []byte {
	return []byte(fmt.Sprintf(`{"metadata":{"message_type":"session_reconnect"},"payload":{"session":{"id":"s-next","reconnect_url":%q}}}`, url))
}

func revocationFrame(subID, subType, bid string) []byte {
	return []byte(fmt.Sprintf(`{"metadata":{"message_type":"revocation","subscription_type":%q},"payload":{"subscription":{"id":%q,"type":%q,"status":"authorization_revoked","condition":{"broadcaster_user_id":%q}}}}`, subType, subID, subType, bid))
}

func automodHoldFrame(msgID string) []byte {
	return []byte(fmt.Sprintf(`{"metadata":{"message_type":"notification","subscription_type":"automod.message.hold","message_timestamp":"2026-06-11T12:00:00Z"},"payload":{"event":{"broadcaster_user_login":"forsen","user_login":"baddie","message_id":%q,"message":{"text":"sus"},"category":"harassment"}}}`, msgID))
}

// esHelixServer fakes the Helix subscriptions endpoint: it records creates/deletes and can be
// told to 403 specific subscription types (automod topics on channels the account doesn't mod).
type esHelixServer struct {
	mu      sync.Mutex
	creates []esSubRequest
	deletes []string
	forbid  map[string]bool // subscription type → respond 403
	nextID  int
	srv     *httptest.Server
}

func newESHelixServer() *esHelixServer {
	s := &esHelixServer{forbid: map[string]bool{}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var body struct {
				Type      string            `json:"type"`
				Version   string            `json:"version"`
				Condition map[string]string `json:"condition"`
				Transport struct {
					SessionID string `json:"session_id"`
				} `json:"transport"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.forbid[body.Type] {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			s.nextID++
			id := fmt.Sprintf("sub-%d", s.nextID)
			s.creates = append(s.creates, esSubRequest{Type: body.Type, Version: body.Version, Condition: body.Condition, SessionID: body.Transport.SessionID})
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"data":[{"id":%q,"status":"enabled"}],"total":1}`, id)
		case http.MethodDelete:
			s.mu.Lock()
			s.deletes = append(s.deletes, r.URL.Query().Get("id"))
			s.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	return s
}

func (s *esHelixServer) created() []esSubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]esSubRequest(nil), s.creates...)
}

func (s *esHelixServer) client() *HelixClient {
	c := NewHelixClient(func() string { return "cid" }, s.srv.Client())
	c.SetEventSubURL(s.srv.URL)
	return c
}

// stateRecorder collects onState callbacks for assertion.
type stateRecorder struct {
	mu     sync.Mutex
	states []string // "slug:up" / "slug:down"
}

func (r *stateRecorder) record(slug string, up bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	dir := "down"
	if up {
		dir = "up"
	}
	r.states = append(r.states, slug+":"+dir)
}

func (r *stateRecorder) wait(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, s := range r.states {
			if s == want {
				r.mu.Unlock()
				return
			}
		}
		r.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("state %q never recorded; got %v", want, r.states)
}

func waitCreates(t *testing.T, hs *esHelixServer, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(hs.created()) >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("never saw %d creates; got %d: %+v", n, len(hs.created()), hs.created())
}

func testSupervisor(t *testing.T, dial ESDialFunc, hs *esHelixServer, rec *stateRecorder, emit func(platform.Event)) *esSupervisor {
	t.Helper()
	if emit == nil {
		emit = func(platform.Event) {}
	}
	tokens := func(context.Context) (string, error) { return "tok", nil }
	resolve := func(_ context.Context, login string) (string, error) { return "bid-" + login, nil }
	s := newESSupervisor(context.Background(), dial, hs.client(), tokens, "u-1", resolve,
		emit, rec.record, clock.System{}, backoff{base: time.Millisecond, max: 5 * time.Millisecond})
	t.Cleanup(s.close)
	return s
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestESSupervisor_SubscribesOnWelcomeAndEmitsHeld(t *testing.T) {
	conn := newESConn()
	dial := &esScriptDial{conns: []*esFakeConn{conn}}
	hs := newESHelixServer()
	t.Cleanup(hs.srv.Close)
	rec := &stateRecorder{}
	events := make(chan platform.Event, 16)

	s := testSupervisor(t, dial.dial, hs, rec, func(ev platform.Event) { events <- ev })
	s.join("Forsen")
	conn.frames <- welcomeFrame("s-1")

	rec.wait(t, "forsen:up")
	waitCreates(t, hs, 3)
	byType := map[string]esSubRequest{}
	for _, c := range hs.created() {
		byType[c.Type] = c
	}
	chat := byType[subChatMessage]
	if chat.Condition["broadcaster_user_id"] != "bid-forsen" || chat.Condition["user_id"] != "u-1" || chat.SessionID != "s-1" {
		t.Errorf("chat sub = %+v", chat)
	}
	hold := byType[subAutomodHold]
	if hold.Condition["moderator_user_id"] != "u-1" || hold.Condition["broadcaster_user_id"] != "bid-forsen" {
		t.Errorf("automod hold sub = %+v", hold)
	}
	if _, ok := byType[subAutomodUpd]; !ok {
		t.Error("automod update subscription missing")
	}
	if !s.channelUp("forsen") {
		t.Error("channel should read via EventSub")
	}
	if h := s.healthStatus(); h.State != platform.HealthOK {
		t.Errorf("health = %+v, want OK", h)
	}

	// A held notification on the session reaches the event stream as a MessageHeldEvent —
	// the AutoMod queue's producer, end to end minus the real network.
	conn.frames <- automodHoldFrame("h-9")
	select {
	case ev := <-events:
		h, ok := ev.(platform.MessageHeldEvent)
		if !ok || h.Held.ID != "h-9" || h.Held.Channel.Slug != "forsen" {
			t.Fatalf("event = %#v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no held event")
	}
}

func TestESSupervisor_AutomodForbiddenKeepsChatUp(t *testing.T) {
	conn := newESConn()
	dial := &esScriptDial{conns: []*esFakeConn{conn}}
	hs := newESHelixServer()
	t.Cleanup(hs.srv.Close)
	hs.forbid[subAutomodHold] = true
	hs.forbid[subAutomodUpd] = true
	rec := &stateRecorder{}

	s := testSupervisor(t, dial.dial, hs, rec, nil)
	s.join("forsen")
	conn.frames <- welcomeFrame("s-1")

	rec.wait(t, "forsen:up")
	s.mu.Lock()
	automod := s.channels["forsen"].automod
	s.mu.Unlock()
	if automod {
		t.Error("automod should be off where the account isn't a moderator")
	}
	if !s.channelUp("forsen") {
		t.Error("chat reads should still be up")
	}
}

func TestESSupervisor_DropRecreatesOnFreshSession(t *testing.T) {
	conn1, conn2 := newESConn(), newESConn()
	dial := &esScriptDial{conns: []*esFakeConn{conn1, conn2}}
	hs := newESHelixServer()
	t.Cleanup(hs.srv.Close)
	rec := &stateRecorder{}

	s := testSupervisor(t, dial.dial, hs, rec, nil)
	s.join("forsen")
	conn1.frames <- welcomeFrame("s-1")
	rec.wait(t, "forsen:up")
	waitCreates(t, hs, 3)

	_ = conn1.Close() // connection drops → channels fall back, supervisor redials fresh
	rec.wait(t, "forsen:down")
	conn2.frames <- welcomeFrame("s-2")
	rec.wait(t, "forsen:up")
	waitCreates(t, hs, 6) // everything re-created on the new session
	for _, c := range hs.created()[3:] {
		if c.SessionID != "s-2" {
			t.Errorf("re-created sub bound to %q, want s-2", c.SessionID)
		}
	}
}

func TestESSupervisor_SessionReconnectCarriesSubscriptions(t *testing.T) {
	conn1, conn2 := newESConn(), newESConn()
	dial := &esScriptDial{conns: []*esFakeConn{conn1, conn2}}
	hs := newESHelixServer()
	t.Cleanup(hs.srv.Close)
	rec := &stateRecorder{}

	s := testSupervisor(t, dial.dial, hs, rec, nil)
	s.join("forsen")
	conn1.frames <- welcomeFrame("s-1")
	rec.wait(t, "forsen:up")
	waitCreates(t, hs, 3)

	conn1.frames <- reconnectFrame("wss://example/reconnect")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		urls := dial.dialedURLs()
		if len(urls) >= 2 && urls[1] == "wss://example/reconnect" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if urls := dial.dialedURLs(); len(urls) < 2 || urls[1] != "wss://example/reconnect" {
		t.Fatalf("dialed = %v, want the reconnect URL second", urls)
	}
	conn2.frames <- welcomeFrame("s-1b")
	time.Sleep(50 * time.Millisecond) // would-be re-creates land within this window
	if n := len(hs.created()); n != 3 {
		t.Errorf("creates after carried reconnect = %d, want 3 (no re-create)", n)
	}
	if !s.channelUp("forsen") {
		t.Error("channel should stay up across a carried reconnect")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, st := range rec.states {
		if st == "forsen:down" {
			t.Error("carried reconnect must not flap the channel down")
		}
	}
}

func TestESSupervisor_RevocationResubscribes(t *testing.T) {
	conn := newESConn()
	dial := &esScriptDial{conns: []*esFakeConn{conn}}
	hs := newESHelixServer()
	t.Cleanup(hs.srv.Close)
	rec := &stateRecorder{}

	s := testSupervisor(t, dial.dial, hs, rec, nil)
	s.join("forsen")
	conn.frames <- welcomeFrame("s-1")
	rec.wait(t, "forsen:up")
	waitCreates(t, hs, 3)

	var chatSubID string
	s.mu.Lock()
	chatSubID = s.channels["forsen"].subIDs[subChatMessage]
	s.mu.Unlock()

	conn.frames <- revocationFrame(chatSubID, subChatMessage, "bid-forsen")
	rec.wait(t, "forsen:down") // IRC takes over while we retry
	rec.wait(t, "forsen:up")   // and the retry restores EventSub
	waitCreates(t, hs, 6)      // a full channel re-subscribe
}

func TestAdapter_MigratesBetweenIRCAndEventSub(t *testing.T) {
	ft := newFakeTransport()
	conn := newESConn()
	dial := &esScriptDial{conns: []*esFakeConn{conn}} // one conn; after it drops, dials fail
	hs := newESHelixServer()
	t.Cleanup(hs.srv.Close)

	a := New(Options{Dial: dialFake(ft), ESDial: dial.dial, BackoffBase: time.Millisecond, BackoffMax: 5 * time.Millisecond})
	t.Cleanup(func() { _ = a.Close() })

	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatal(err)
	}
	waitForWrite(t, ft, "JOIN #forsen")

	tokens := func(context.Context) (string, error) { return "tok", nil }
	resolve := func(_ context.Context, login string) (string, error) { return "bid-" + login, nil }
	a.Authenticate("u-1", tokens, hs.client(), resolve)

	conn.frames <- welcomeFrame("s-1")
	// EventSub reads come up → the channel's IRC membership is retired.
	waitForWrite(t, ft, "PART #forsen")

	// The EventSub connection dies and can't come back (no scripted conns left) → the adapter
	// rejoins the channel over IRC, the other migration direction.
	_ = conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		joins := 0
		for _, w := range ft.writes() {
			if w == "JOIN #forsen" {
				joins++
			}
		}
		if joins >= 2 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("channel never fell back to IRC; writes %v", ft.writes())
}

func TestAdapter_DedupesAcrossTransports(t *testing.T) {
	ft := newFakeTransport()
	a := New(Options{Dial: dialFake(ft)})
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Join(context.Background(), platform.ChannelRef{Slug: "forsen"}, platform.ModeAnonymous); err != nil {
		t.Fatal(err)
	}

	// The same platform message id arriving twice (e.g. IRC and EventSub overlap during
	// migration) must reach the feed once.
	line := `@display-name=Alice;id=dup-1 :alice!alice@alice.tmi.twitch.tv PRIVMSG #forsen :hello`
	ft.feed(line)
	ft.feed(line)
	ft.feed(strings.Replace(line, "dup-1", "dup-2", 1))

	var ids []string
	timeout := time.After(2 * time.Second)
	for len(ids) < 2 {
		select {
		case ev := <-a.Events():
			if me, ok := ev.(platform.MessageEvent); ok {
				ids = append(ids, me.Message.PlatformMessageID)
			}
		case <-timeout:
			t.Fatalf("events = %v", ids)
		}
	}
	if ids[0] != "dup-1" || ids[1] != "dup-2" {
		t.Errorf("ids = %v, want [dup-1 dup-2] (duplicate suppressed)", ids)
	}
	select {
	case ev := <-a.Events():
		if me, ok := ev.(platform.MessageEvent); ok {
			t.Errorf("unexpected extra message %q", me.Message.PlatformMessageID)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestESHelix_CreateAndDelete(t *testing.T) {
	hs := newESHelixServer()
	t.Cleanup(hs.srv.Close)
	c := hs.client()

	id, err := c.CreateEventSubSubscription(context.Background(), "tok", esSubRequest{
		Type: subChatMessage, Version: "1",
		Condition: map[string]string{"broadcaster_user_id": "b", "user_id": "u"},
		SessionID: "s-1",
	})
	if err != nil || id == "" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	if err := c.DeleteEventSubSubscription(context.Background(), "tok", id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	hs.forbid[subAutomodHold] = true
	_, err = c.CreateEventSubSubscription(context.Background(), "tok", esSubRequest{
		Type: subAutomodHold, Version: "1",
		Condition: map[string]string{"broadcaster_user_id": "b", "moderator_user_id": "u"},
		SessionID: "s-1",
	})
	if !errors.Is(err, errESForbidden) {
		t.Errorf("403 = %v, want errESForbidden", err)
	}

	// 409 (already exists) and DELETE 404 (already gone) are both "desired state holds".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	c2 := NewHelixClient(func() string { return "cid" }, srv.Client())
	c2.SetEventSubURL(srv.URL)
	if id, err := c2.CreateEventSubSubscription(context.Background(), "tok", esSubRequest{Type: subChatMessage, Version: "1", SessionID: "s"}); err != nil || id != "" {
		t.Errorf("409 = (%q, %v), want (\"\", nil)", id, err)
	}
	if err := c2.DeleteEventSubSubscription(context.Background(), "tok", "gone"); err != nil {
		t.Errorf("delete 404 = %v, want nil", err)
	}
}
