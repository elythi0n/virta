package twitch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// twitchEventSubWS is Twitch's EventSub WebSocket endpoint.
const twitchEventSubWS = "wss://eventsub.wss.twitch.tv/ws"

// ESDialFunc opens an EventSub WebSocket to url. Injected so tests drive the supervisor with
// scripted frames and no network.
type ESDialFunc func(ctx context.Context, url string) (esConn, error)

// esChannelState tracks one channel on the EventSub side.
type esChannelState struct {
	slug    string
	bid     string            // broadcaster id, resolved once
	subIDs  map[string]string // subscription type → id ("" = active but id unknown, e.g. after a 409)
	up      bool              // chat.message subscription is live → reads come from EventSub
	automod bool              // automod hold/update subscriptions are live (we mod there)
}

// esSupervisor owns the authenticated EventSub read path for one account: it dials the WS,
// creates per-channel subscriptions within the 10 s welcome window, re-creates them after a
// fresh reconnect, follows session_reconnect URLs (where Twitch carries subscriptions over),
// resubscribes once on revocation, and reports per-channel up/down so the adapter can migrate
// reads between IRC and EventSub in both directions.
//
// One WebSocket connection is used. At 3 subscriptions per channel that supports ~100 channels
// (Twitch caps 300 subscriptions per connection); beyond that the supervisor reports degraded
// and leaves the overflow channels on IRC rather than opening more connections.
type esSupervisor struct {
	dial    ESDialFunc
	helix   *HelixClient
	tokens  TokenFunc
	userID  string // the authenticated account: reader for chat, moderator for automod
	resolve func(ctx context.Context, login string) (string, error)
	emit    func(platform.Event)
	onState func(slug string, up bool) // adapter migration hook; never called with locks held
	clk     clock.Clock
	backoff backoff

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	rng    uint64 // splitmix64 state for backoff jitter; only the run goroutine touches it

	mu        sync.Mutex
	channels  map[string]*esChannelState
	sessionID string
	budget    subBudget
	health    platform.HealthStatus
}

func newESSupervisor(parent context.Context, dial ESDialFunc, helix *HelixClient, tokens TokenFunc,
	userID string, resolve func(ctx context.Context, login string) (string, error),
	emit func(platform.Event), onState func(slug string, up bool), clk clock.Clock, bo backoff) *esSupervisor {
	ctx, cancel := context.WithCancel(parent)
	s := &esSupervisor{
		dial:     dial,
		helix:    helix,
		tokens:   tokens,
		userID:   userID,
		resolve:  resolve,
		emit:     emit,
		onState:  onState,
		clk:      clk,
		backoff:  bo,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		rng:      uint64(clk.Now().UnixNano()),
		channels: map[string]*esChannelState{},
		health:   platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonUpstreamDown, Detail: "eventsub connecting"},
	}
	go s.run()
	return s
}

// join registers a channel; it is subscribed immediately when a session is live, or on the next
// welcome otherwise.
func (s *esSupervisor) join(slug string) {
	slug = strings.ToLower(slug)
	s.mu.Lock()
	if _, ok := s.channels[slug]; ok {
		s.mu.Unlock()
		return
	}
	ch := &esChannelState{slug: slug, subIDs: map[string]string{}}
	s.channels[slug] = ch
	sessionID := s.sessionID
	s.mu.Unlock()
	if sessionID != "" {
		go s.subscribeChannel(ch, sessionID)
	}
}

// leave drops a channel: its subscriptions are deleted best-effort and the state removed. No
// onState fires — the adapter initiated this and owns the IRC side.
func (s *esSupervisor) leave(slug string) {
	slug = strings.ToLower(slug)
	s.mu.Lock()
	ch, ok := s.channels[slug]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.channels, slug)
	ids := make([]string, 0, len(ch.subIDs))
	for _, id := range ch.subIDs {
		if id != "" {
			ids = append(ids, id)
		}
		s.budget.remove(0)
	}
	s.mu.Unlock()
	go func() {
		tok, err := s.tokens(s.ctx)
		if err != nil {
			return // session teardown will clean up server-side
		}
		for _, id := range ids {
			_ = s.helix.DeleteEventSubSubscription(s.ctx, tok, id)
		}
	}()
}

// channelUp reports whether slug's reads currently come from EventSub.
func (s *esSupervisor) channelUp(slug string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[strings.ToLower(slug)]
	return ok && ch.up
}

func (s *esSupervisor) healthStatus() platform.HealthStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}

func (s *esSupervisor) setHealth(h platform.HealthStatus) {
	s.mu.Lock()
	s.health = h
	s.mu.Unlock()
}

// close stops the supervisor. Twitch disables websocket-transport subscriptions when their
// connection closes, so no explicit cleanup call is needed.
func (s *esSupervisor) close() {
	s.cancel()
	<-s.done
}

// run is the connection loop: dial, run the session, and on exit either follow the reconnect
// URL (subscriptions carry over) or back off and start fresh (subscriptions re-created on the
// new welcome). Every fresh start marks all channels down first so the adapter falls back to
// IRC while EventSub is gone.
func (s *esSupervisor) run() {
	defer close(s.done)
	url := twitchEventSubWS
	carried := false // true when dialing a session_reconnect URL: subs survive, don't recreate
	attempt := 0
	for s.ctx.Err() == nil {
		conn, err := s.dial(s.ctx, url)
		if err != nil {
			s.markAllDown("eventsub dial failed: " + err.Error())
			url, carried = twitchEventSubWS, false
			attempt++
			if !s.sleep(attempt) {
				return
			}
			continue
		}

		sess := &session{
			conn: conn,
			emit: s.emit,
			onWelcome: func(sessionID string) {
				attempt = 0
				recreate := !carried
				carried = false
				go s.onWelcome(sessionID, recreate)
			},
			onRevoke: func(subID, subType, broadcasterID string) {
				go s.onRevoke(subID, subType, broadcasterID)
			},
		}
		reconnect, runErr := sess.run(s.ctx)
		_ = conn.Close()
		if s.ctx.Err() != nil {
			return
		}
		if runErr == nil && reconnect != "" {
			// Twitch-directed move: the new session keeps our subscriptions; channels stay up.
			url, carried = reconnect, true
			continue
		}
		s.markAllDown("eventsub connection lost")
		url, carried = twitchEventSubWS, false
		attempt++
		if !s.sleep(attempt) {
			return
		}
	}
}

// nextRand advances a splitmix64 sequence for backoff jitter — same scheme as the IRC shards,
// independent per supervisor, no shared/global RNG.
func (s *esSupervisor) nextRand() uint64 {
	s.rng += 0x9e3779b97f4a7c15
	z := s.rng
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// sleep waits for the attempt's backoff delay, returning false when the supervisor is closing.
func (s *esSupervisor) sleep(attempt int) bool {
	t := time.NewTimer(s.backoff.delay(attempt, s.nextRand()))
	defer t.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// markAllDown flips every channel to the IRC side and degrades health.
func (s *esSupervisor) markAllDown(detail string) {
	s.mu.Lock()
	s.sessionID = ""
	s.budget = subBudget{}
	var flipped []string
	for _, ch := range s.channels {
		if ch.up {
			ch.up = false
			flipped = append(flipped, ch.slug)
		}
		ch.subIDs = map[string]string{}
		ch.automod = false
	}
	s.health = platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonUpstreamDown, Detail: detail}
	s.mu.Unlock()
	for _, slug := range flipped {
		s.onState(slug, false)
	}
}

// onWelcome records the session and (on a fresh session) creates every channel's subscriptions.
// Twitch closes the socket if no subscription lands within 10 s of the welcome, so this starts
// immediately on the session goroutine's signal.
func (s *esSupervisor) onWelcome(sessionID string, recreate bool) {
	s.mu.Lock()
	s.sessionID = sessionID
	s.health = platform.HealthStatus{State: platform.HealthOK}
	var pending []*esChannelState
	if recreate {
		s.budget = subBudget{}
		for _, ch := range s.channels {
			pending = append(pending, ch)
		}
	}
	s.mu.Unlock()
	for _, ch := range pending {
		s.subscribeChannel(ch, sessionID)
	}
}

// subTypesFor returns the subscriptions a channel wants: chat reads always, automod hold/update
// stacked on top (they simply fail with 403 where the account isn't a moderator).
func (s *esSupervisor) subTypesFor(bid string) []esSubRequest {
	return []esSubRequest{
		{Type: subChatMessage, Version: "1", Condition: map[string]string{"broadcaster_user_id": bid, "user_id": s.userID}},
		{Type: subAutomodHold, Version: "1", Condition: map[string]string{"broadcaster_user_id": bid, "moderator_user_id": s.userID}},
		{Type: subAutomodUpd, Version: "1", Condition: map[string]string{"broadcaster_user_id": bid, "moderator_user_id": s.userID}},
	}
}

// subscribeChannel resolves the broadcaster and creates the channel's subscriptions on the
// session. chat.message success flips the channel to EventSub reads; automod failures only cost
// the held queue (403 = not a moderator there — expected, not an error).
func (s *esSupervisor) subscribeChannel(ch *esChannelState, sessionID string) {
	tok, err := s.tokens(s.ctx)
	if err != nil {
		return // token unavailable; channel stays on IRC, next welcome retries
	}
	if ch.bid == "" {
		bid, err := s.resolve(s.ctx, ch.slug)
		if err != nil {
			return // unresolvable now; stays on IRC, next welcome retries
		}
		s.mu.Lock()
		ch.bid = bid
		s.mu.Unlock()
	}

	chatUp := false
	automodUp := 0
	for _, sub := range s.subTypesFor(ch.bid) {
		s.mu.Lock()
		if s.sessionID != sessionID { // session changed under us; the new welcome resubscribes
			s.mu.Unlock()
			return
		}
		_, ok := s.budget.add()
		s.mu.Unlock()
		if !ok {
			s.setHealth(platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonRateLimited, Detail: "eventsub subscription cap reached; overflow channels stay on IRC"})
			break
		}
		sub.SessionID = sessionID
		id, err := s.helix.CreateEventSubSubscription(s.ctx, tok, sub)
		if err != nil {
			s.mu.Lock()
			s.budget.remove(0)
			s.mu.Unlock()
			if errors.Is(err, errESForbidden) {
				continue // not a mod there — chat reads still fine
			}
			continue
		}
		s.mu.Lock()
		ch.subIDs[sub.Type] = id
		s.mu.Unlock()
		switch sub.Type {
		case subChatMessage:
			chatUp = true
		case subAutomodHold, subAutomodUpd:
			automodUp++
		}
	}

	s.mu.Lock()
	wasUp := ch.up
	ch.up = chatUp
	ch.automod = automodUp == 2
	s.mu.Unlock()
	if chatUp && !wasUp {
		s.onState(ch.slug, true)
	}
}

// onRevoke handles Twitch turning off one subscription: drop the bookkeeping and retry the
// channel once. If the retry can't restore chat reads, the adapter falls back to IRC.
func (s *esSupervisor) onRevoke(subID, subType, broadcasterID string) {
	s.mu.Lock()
	var target *esChannelState
	for _, ch := range s.channels {
		if (subID != "" && ch.subIDs[subType] == subID) || (ch.bid != "" && ch.bid == broadcasterID) {
			target = ch
			break
		}
	}
	if target == nil {
		s.mu.Unlock()
		return
	}
	delete(target.subIDs, subType)
	s.budget.remove(0)
	wasUp := target.up
	if subType == subChatMessage {
		target.up = false
	}
	target.automod = false
	sessionID := s.sessionID
	s.mu.Unlock()

	if subType == subChatMessage && wasUp {
		s.onState(target.slug, false)
	}
	if sessionID != "" {
		s.subscribeChannel(target, sessionID)
	}
}
