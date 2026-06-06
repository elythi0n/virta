package twitch

import (
	"context"

	"github.com/elythi0n/virta/internal/platform"
)

// EventSub connection limits (Twitch): up to 3 WebSocket connections per user, up to 300
// enabled subscriptions per connection.
const (
	maxConnsPerUser = 3
	maxSubsPerConn  = 300
)

// subBudget tracks how many EventSub subscriptions sit on each of a user's connections, so the
// adapter knows when to open another connection and when it has hit the hard cap. Pure
// bookkeeping — the actual subscribe/connect is done by the (live) transport layer.
type subBudget struct {
	conns []int // subscription count per connection index
}

// add reserves a slot, returning the connection index to use. ok is false at the hard cap
// (maxConnsPerUser × maxSubsPerConn).
func (b *subBudget) add() (conn int, ok bool) {
	for i, n := range b.conns {
		if n < maxSubsPerConn {
			b.conns[i]++
			return i, true
		}
	}
	if len(b.conns) < maxConnsPerUser {
		b.conns = append(b.conns, 1)
		return len(b.conns) - 1, true
	}
	return -1, false
}

// remove frees a slot on a connection.
func (b *subBudget) remove(conn int) {
	if conn >= 0 && conn < len(b.conns) && b.conns[conn] > 0 {
		b.conns[conn]--
	}
}

// total is the number of active subscriptions across all connections.
func (b *subBudget) total() int {
	n := 0
	for _, c := range b.conns {
		n += c
	}
	return n
}

// esConn is the transport for one EventSub WebSocket. The real implementation runs over a
// WebSocket; tests inject a fake.
type esConn interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, b []byte) error
	Close() error
}

// session runs one EventSub WebSocket connection: it dispatches frames until the connection
// drops or Twitch asks us to reconnect. onWelcome fires when the session id arrives (the cue to
// create subscriptions, within Twitch's 10 s window); notifications become platform events.
type session struct {
	conn      esConn
	emit      func(platform.Event)
	onWelcome func(sessionID string)
}

// run reads frames until the connection errors (returns reconnect="") or Twitch sends a
// session_reconnect (returns the new URL to dial). A nil error with a non-empty reconnect URL
// means "redial there, seamlessly".
func (s *session) run(ctx context.Context) (reconnect string, err error) {
	for {
		b, err := s.conn.Read(ctx)
		if err != nil {
			return "", err
		}
		env, perr := parseEnvelope(b)
		if perr != nil {
			continue // ignore unparseable frames
		}
		switch env.Metadata.MessageType {
		case esWelcome:
			if sess, e := sessionFromPayload(env.Payload); e == nil && s.onWelcome != nil {
				s.onWelcome(sess.ID)
			}
		case esKeepalive:
			// Liveness only — the read deadline (set by the transport) covers the timeout rule.
		case esNotification:
			if ev, ok, e := eventFromNotification(env.Metadata.SubscriptionType, env.Payload, parseESTimestamp(env.Metadata.MessageTimestamp)); e == nil && ok {
				s.emit(ev)
			}
		case esReconnect:
			if sess, e := sessionFromPayload(env.Payload); e == nil && sess.ReconnectURL != "" {
				return sess.ReconnectURL, nil
			}
		case esRevocation:
			// A subscription was revoked (e.g. token scope lost); the supervisor re-subscribes
			// or surfaces it. Nothing to emit here.
		}
	}
}
