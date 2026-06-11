package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

const testVideoID = "dQw4w9WgXcQ"

// fakeYouTube serves the /@slug/live page and the two InnerTube endpoints from canned state,
// so adapter tests drive the full resolve → continuation → poll lifecycle with zero network.
type fakeYouTube struct {
	mu        sync.Mutex
	live      bool
	notFound  bool
	chat      map[string]string // continuation token → get_live_chat response body
	liveHits  int
	nextHits  int
	pollHits  int
	chatToken string // token /next hands out
}

func newFakeYouTube() *fakeYouTube {
	return &fakeYouTube{live: true, chatToken: "tok-1", chat: map[string]string{}}
}

func (f *fakeYouTube) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/youtubei/v1/next", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.nextHits++
		token := f.chatToken
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"contents":{"twoColumnWatchNextResults":{"conversationBar":{"liveChatRenderer":{"continuations":[{"reloadContinuationData":{"continuation":"` + token + `","timeoutMs":0}}]}}}}}`))
	})
	mux.HandleFunc("/youtubei/v1/live_chat/get_live_chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Continuation string `json:"continuation"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.pollHits++
		resp, ok := f.chat[body.Continuation]
		f.mu.Unlock()
		if !ok {
			resp = `{}` // unknown token → stream ended
		}
		_, _ = w.Write([]byte(resp))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		live, notFound := f.live, f.notFound
		f.liveHits++
		f.mu.Unlock()
		switch {
		case notFound:
			http.NotFound(w, r)
		case live:
			_, _ = w.Write([]byte(`<html><script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"` + testVideoID + `","isLive":true},"microformat":{"liveBroadcastDetails":{"isLiveNow":true}}};</script></html>`))
		default:
			_, _ = w.Write([]byte(`<html><body>channel home, nothing live here</body></html>`))
		}
	})
	return mux
}

func (f *fakeYouTube) set(fn func(*fakeYouTube)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeYouTube) counts() (live, next, poll int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.liveHits, f.nextHits, f.pollHits
}

// chatResponse builds a get_live_chat body with the given actions and next token.
func chatResponse(next string, actions ...string) string {
	acts := "[]"
	if len(actions) > 0 {
		acts = "["
		for i, a := range actions {
			if i > 0 {
				acts += ","
			}
			acts += a
		}
		acts += "]"
	}
	return `{"continuationContents":{"liveChatContinuation":{"continuations":[{"timedContinuationData":{"continuation":"` + next + `","timeoutMs":1}}],"actions":` + acts + `}}}`
}

const textAction = `{"addChatItemAction":{"item":{"liveChatTextMessageRenderer":{"id":"live-1","timestampUsec":"1717599600000000","authorName":{"simpleText":"Fan"},"authorExternalChannelId":"UCfan","message":{"runs":[{"text":"hi there"}]}}}}}`
const deleteAction = `{"markChatItemAsDeletedAction":{"targetItemId":"live-1"}}`

// newTestAdapter builds an adapter pointed at the fake with timings shrunk for tests.
func newTestAdapter(t *testing.T, f *fakeYouTube) *Adapter {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	a := New(Options{
		WebBase:         srv.URL,
		APIBase:         srv.URL,
		HTTPClient:      srv.Client(),
		BackoffBase:     time.Millisecond,
		BackoffMax:      2 * time.Millisecond,
		PollFloor:       time.Millisecond,
		PollCeil:        2 * time.Millisecond,
		ResolveRetryMin: time.Millisecond,
		ResolveRetryMax: 2 * time.Millisecond,
	})
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func join(t *testing.T, a *Adapter, slug string) platform.ChannelRef {
	t.Helper()
	ch := platform.ChannelRef{Platform: platform.YouTube, Slug: slug}
	if err := a.Join(context.Background(), ch, platform.ModeAnonymous); err != nil {
		t.Fatalf("Join: %v", err)
	}
	return ch
}

func TestAdapter_EmitsNormalizedMessagesAndDeletions(t *testing.T) {
	f := newFakeYouTube()
	f.set(func(f *fakeYouTube) {
		f.chat["tok-1"] = chatResponse("tok-2", textAction, deleteAction)
		// tok-2 is unknown → the next poll reports the stream ended.
	})
	a := newTestAdapter(t, f)
	join(t, a, "somecreator")

	var got platform.UnifiedMessage
	var deleted platform.MessageDeletedEvent
	timeout := time.After(2 * time.Second)
	for got.PlatformMessageID == "" || deleted.PlatformMessageID == "" {
		select {
		case ev := <-a.Events():
			switch e := ev.(type) {
			case platform.MessageEvent:
				got = e.Message
			case platform.MessageDeletedEvent:
				deleted = e
			}
		case <-timeout:
			t.Fatalf("timed out; message=%+v deleted=%+v", got, deleted)
		}
	}
	if got.PlainText() != "hi there" || got.Author.DisplayName != "Fan" || got.Platform != platform.YouTube {
		t.Errorf("message = %+v", got)
	}
	if got.Channel.Slug != "somecreator" {
		t.Errorf("channel = %+v, want the joined channel", got.Channel)
	}
	if deleted.PlatformMessageID != "live-1" || deleted.Channel.Slug != "somecreator" {
		t.Errorf("deletion = %+v", deleted)
	}
}

func TestAdapter_StreamEndDegradesAndReResolves(t *testing.T) {
	f := newFakeYouTube()
	f.set(func(f *fakeYouTube) {
		f.chat["tok-1"] = chatResponse("tok-2", textAction)
		// tok-2 unknown → stream end → the worker must degrade (not_live) and re-resolve.
	})
	a := newTestAdapter(t, f)
	ch := join(t, a, "somecreator")

	sawDegraded := false
	timeout := time.After(2 * time.Second)
	for !sawDegraded {
		select {
		case ev := <-a.Events():
			if he, ok := ev.(platform.HealthEvent); ok {
				if he.Channel == nil || he.Channel.Slug != ch.Slug {
					t.Errorf("health event channel = %+v, want per-channel", he.Channel)
				}
				if he.Status.State == platform.HealthDegraded && he.Status.Reason == platform.ReasonNotLive {
					sawDegraded = true
				}
			}
		case <-timeout:
			t.Fatal("no degraded/not_live health event after stream end")
		}
	}

	// The page is still live, so the wait loop resolves again and reconnects: /live and /next
	// must both be hit a second time.
	deadline := time.Now().Add(2 * time.Second)
	for {
		live, next, _ := f.counts()
		if live >= 2 && next >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no re-resolve after stream end: live=%d next=%d", live, next)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAdapter_JoinChannelNotFound(t *testing.T) {
	f := newFakeYouTube()
	f.set(func(f *fakeYouTube) { f.notFound = true })
	a := newTestAdapter(t, f)

	err := a.Join(context.Background(), platform.ChannelRef{Platform: platform.YouTube, Slug: "ghost"}, platform.ModeAnonymous)
	var re *ResolveError
	if !errors.As(err, &re) || re.Reason != platform.ReasonChannelNotFound {
		t.Fatalf("Join = %v, want a ResolveError with channel_not_found", err)
	}
}

func TestAdapter_NotLiveWaitsThenAttaches(t *testing.T) {
	f := newFakeYouTube()
	f.set(func(f *fakeYouTube) {
		f.live = false
		f.chat["tok-1"] = chatResponse("tok-2", textAction)
	})
	a := newTestAdapter(t, f)
	join(t, a, "somecreator") // offline channel still joins; the worker waits

	// First the waiting state is reported…
	select {
	case ev := <-a.Events():
		he, ok := ev.(platform.HealthEvent)
		if !ok || he.Status.Reason != platform.ReasonNotLive {
			t.Fatalf("first event = %+v, want degraded/not_live", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no waiting health event for an offline channel")
	}

	// …then the channel goes live and chat starts flowing without a re-join.
	f.set(func(f *fakeYouTube) { f.live = true })
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-a.Events():
			if me, ok := ev.(platform.MessageEvent); ok {
				if me.Message.PlainText() != "hi there" {
					t.Errorf("message = %+v", me.Message)
				}
				return
			}
		case <-timeout:
			t.Fatal("no message after the channel went live")
		}
	}
}

func TestAdapter_LeaveStopsPolling(t *testing.T) {
	f := newFakeYouTube()
	f.set(func(f *fakeYouTube) { f.live = false }) // keep the worker in the cheap wait loop
	a := newTestAdapter(t, f)
	ch := join(t, a, "somecreator")

	if err := a.Leave(ch); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	// After Leave the worker stops; request counts must settle (allow one in-flight resolve).
	time.Sleep(20 * time.Millisecond)
	before, _, _ := f.counts()
	time.Sleep(50 * time.Millisecond)
	after, _, _ := f.counts()
	if after > before+1 {
		t.Errorf("still resolving after Leave: %d → %d", before, after)
	}
	// Leaving an unknown channel is a no-op, not an error.
	if err := a.Leave(platform.ChannelRef{Platform: platform.YouTube, Slug: "never-joined"}); err != nil {
		t.Errorf("Leave unknown = %v, want nil", err)
	}
}

func TestAdapter_JoinIsIdempotent(t *testing.T) {
	f := newFakeYouTube()
	f.set(func(f *fakeYouTube) {
		f.chat["tok-1"] = chatResponse("tok-1") // steady empty polls keep the worker attached
	})
	a := newTestAdapter(t, f)
	ch := join(t, a, "somecreator")
	if err := a.Join(context.Background(), ch, platform.ModeAnonymous); err != nil {
		t.Fatalf("second Join: %v", err)
	}
	a.mu.Lock()
	workers := len(a.workers)
	a.mu.Unlock()
	if workers != 1 {
		t.Errorf("workers = %d, want 1 after a duplicate join", workers)
	}
}

func TestAdapter_CloseStopsWorkersAndClosesEvents(t *testing.T) {
	f := newFakeYouTube()
	f.set(func(f *fakeYouTube) {
		f.chat["tok-1"] = chatResponse("tok-1")
	})
	a := newTestAdapter(t, f)
	join(t, a, "somecreator")

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, open := <-a.Events():
			if !open {
				return
			}
		case <-deadline:
			t.Fatal("Events not closed after Close")
		}
	}
}
