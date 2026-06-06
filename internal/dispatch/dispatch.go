// Package dispatch is the one path outbound user input takes: parse it (slash commands → typed
// actions), capability-check it, then either queue a chat send through the rate governor or
// perform a moderation action. Mod buttons, the composer, and the held-message queue all call
// here, so capability checks and pacing live in one place (ADR-028).
package dispatch

import (
	"context"

	"github.com/elythi0n/virta/internal/command"
	"github.com/elythi0n/virta/internal/platform"
)

// Adapter is the slice of a platform adapter dispatch needs.
type Adapter interface {
	Capabilities() platform.Capabilities
	Send(ctx context.Context, ch platform.ChannelRef, text string, opts platform.SendOpts) error
	Moderate(ctx context.Context, action platform.ModAction) error
}

// Governor paces sends per channel key.
type Governor interface {
	Submit(key string, send func() error) <-chan error
}

// Outcome is what a dispatched input resulted in.
type Outcome struct {
	Kind command.Kind // Send / Mod / Help / Hint
	Hint string       // for Hint/Help: text to show, never sent
	// Sent receives the eventual send result (nil = sent) for Kind==Send; nil for other kinds.
	Sent <-chan error
}

// Sender ties the parser, governor, and adapters together.
type Sender struct {
	adapters map[platform.Platform]Adapter
	gov      Governor
	help     string
}

// New builds a Sender over the given per-platform adapters and rate governor.
func New(adapters map[platform.Platform]Adapter, gov Governor, helpText string) *Sender {
	return &Sender{adapters: adapters, gov: gov, help: helpText}
}

// Do parses input for channel ch and executes it: a chat send is queued through the governor;
// a recognized moderation action runs immediately; an unknown/unavailable command returns a
// hint and is never sent.
func (s *Sender) Do(ctx context.Context, ch platform.ChannelRef, input string) (Outcome, error) {
	a, ok := s.adapters[ch.Platform]
	if !ok {
		return Outcome{Kind: command.KindHint, Hint: "platform not available"}, nil
	}
	caps := a.Capabilities()
	p := command.Parse(input, ch, caps)

	switch p.Kind {
	case command.KindHelp:
		return Outcome{Kind: command.KindHelp, Hint: s.help}, nil
	case command.KindHint:
		return Outcome{Kind: command.KindHint, Hint: p.Hint}, nil
	case command.KindMod:
		return Outcome{Kind: command.KindMod}, a.Moderate(ctx, p.Action)
	default: // KindSend
		if !caps.Send {
			return Outcome{Kind: command.KindHint, Hint: "you can't send to this channel"}, nil
		}
		opts := platform.SendOpts{Action: p.IsAction}
		text := p.Text
		res := s.gov.Submit(channelKey(ch), func() error { return a.Send(ctx, ch, text, opts) })
		return Outcome{Kind: command.KindSend, Sent: res}, nil
	}
}

func channelKey(ch platform.ChannelRef) string { return ch.Key() }
