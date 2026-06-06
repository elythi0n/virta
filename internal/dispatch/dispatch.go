// Package dispatch is the one path outbound user input takes: parse it (slash commands → typed
// actions), capability-check it, then either queue a chat send through the rate governor or
// perform a moderation action. Mod buttons, the composer, and the held-message queue all call
// here, so capability checks and pacing live in one place (ADR-028).
package dispatch

import (
	"context"
	"time"

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
	// State reports a channel's queued send count and the time until its next send is permitted.
	State(key string) (queued int, nextIn time.Duration)
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

// TargetState is one channel's pre-send reachability: whether a message can be sent to it right
// now and, when it can't, a machine reason the frontend maps to copy. It backs the composer's
// reachable/excluded target chips before anything is sent.
type TargetState struct {
	Channel platform.ChannelRef
	CanSend bool
	Reason  platform.ReasonCode // set when CanSend is false
}

// canSend reports whether ch can receive a send right now, with a reason when it can't.
func (s *Sender) canSend(ch platform.ChannelRef) (bool, platform.ReasonCode) {
	a, ok := s.adapters[ch.Platform]
	if !ok {
		return false, platform.ReasonNone
	}
	if !a.Capabilities().Send {
		return false, platform.ReasonAuthRequired
	}
	return true, platform.ReasonNone
}

// Targets reports per-target send reachability without sending anything — the state a composer
// renders as reachable vs excluded chips before the user hits enter.
func (s *Sender) Targets(targets []platform.ChannelRef) []TargetState {
	out := make([]TargetState, 0, len(targets))
	for _, ch := range targets {
		ok, reason := s.canSend(ch)
		out = append(out, TargetState{Channel: ch, CanSend: ok, Reason: reason})
	}
	return out
}

// TargetSend is the disposition of a cross-posted message for one target.
type TargetSend struct {
	Channel   platform.ChannelRef
	Reachable bool
	Reason    platform.ReasonCode // set when Reachable is false
	// Sent receives the eventual send result (nil = sent) for a reachable target; nil otherwise.
	Sent <-chan error
}

// SendMany cross-posts text as a chat message to every reachable target, pacing each through the
// governor. Unreachable targets (signed out, no adapter) are excluded before anything is sent
// and reported — never errored — so one unreachable target can't fail the send to the others.
func (s *Sender) SendMany(ctx context.Context, targets []platform.ChannelRef, text string, opts platform.SendOpts) []TargetSend {
	out := make([]TargetSend, 0, len(targets))
	for _, ch := range targets {
		ok, reason := s.canSend(ch)
		if !ok {
			out = append(out, TargetSend{Channel: ch, Reachable: false, Reason: reason})
			continue
		}
		a := s.adapters[ch.Platform]
		res := s.gov.Submit(channelKey(ch), func() error {
			return a.Send(ctx, ch, text, opts)
		})
		out = append(out, TargetSend{Channel: ch, Reachable: true, Sent: res})
	}
	return out
}

// QueueInfo is one channel's outbound send-queue state: how many sends are waiting and how long
// until the next one is permitted (0 when a send can go right now).
type QueueInfo struct {
	Channel platform.ChannelRef
	Queued  int
	NextIn  time.Duration
}

// QueueState reports the send-queue depth and next-send countdown for each target, so a composer
// can show pacing instead of leaving the user guessing whether a send went out.
func (s *Sender) QueueState(targets []platform.ChannelRef) []QueueInfo {
	out := make([]QueueInfo, 0, len(targets))
	for _, ch := range targets {
		q, next := s.gov.State(channelKey(ch))
		out = append(out, QueueInfo{Channel: ch, Queued: q, NextIn: next})
	}
	return out
}

func channelKey(ch platform.ChannelRef) string { return ch.Key() }
