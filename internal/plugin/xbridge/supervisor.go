package xbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elythi0n/virta/internal/platform"
)

// BridgeState is the supervisor's view of the bridge, surfaced as reason codes.
const (
	StateConnecting   = "x_bridge_connecting"
	StateConnected    = "x_bridge_connected"
	StateDegraded     = "x_bridge_degraded"
	StateDisabled     = "x_bridge_disabled"
	StateNotFound     = "x_bridge_not_found"     // bridge binary not on PATH
	StateAuthRequired = "x_bridge_auth_required" // X requires a sign-in for this broadcast
)

// Options configures the Supervisor.
type Options struct {
	// BinaryPath is the x-bridge binary. Empty = search PATH for "x-bridge".
	BinaryPath string
	// SocketPath is the UNIX socket the bridge connects back on. Empty = auto.
	SocketPath string
	Logger     *slog.Logger
}

// Supervisor spawns and manages the x-bridge process, reconnects on crash with circuit-breaker
// backoff, and surfaces platform events for every message and health transition it receives.
type Supervisor struct {
	opts   Options
	log    *slog.Logger
	events chan platform.Event
	state  atomic.Value // string reason code
	quit   chan struct{}
	wg     sync.WaitGroup
}

// New builds a Supervisor. Call Start to begin.
func New(opts Options) *Supervisor {
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Supervisor{opts: opts, log: log, events: make(chan platform.Event, 256), quit: make(chan struct{})}
	s.state.Store(StateConnecting)
	return s
}

// Events returns the channel of platform events (messages + health) the supervisor emits.
func (s *Supervisor) Events() <-chan platform.Event { return s.events }

// State returns the current supervisor state as a reason code.
func (s *Supervisor) State() string { return s.state.Load().(string) }

// Start begins supervising. Returns immediately; the supervisor runs in the background.
func (s *Supervisor) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.loop(ctx)
}

// Close stops the supervisor and waits for cleanup.
func (s *Supervisor) Close() {
	close(s.quit)
	s.wg.Wait()
}

func (s *Supervisor) loop(ctx context.Context) {
	defer s.wg.Done()
	bin := s.opts.BinaryPath
	if bin == "" {
		var err error
		bin, err = exec.LookPath("x-bridge")
		if err != nil {
			s.setState(StateNotFound)
			s.emit(platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonCode(StateNotFound)}})
			s.log.Warn("x-bridge binary not found; X adapter disabled")
			return
		}
	}
	const maxBackoff = 30 * time.Second
	attempt := 0
	for {
		select {
		case <-s.quit:
			return
		case <-ctx.Done():
			return
		default:
		}
		if err := s.run(ctx, bin); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			backoff := time.Duration(math.Min(float64(maxBackoff), float64(500*time.Millisecond)*math.Pow(2, float64(attempt))))
			s.log.Warn("x-bridge exited, retrying", "attempt", attempt, "backoff", backoff, "err", err)
			s.setState(StateDegraded)
			attempt++
			select {
			case <-time.After(backoff):
			case <-s.quit:
				return
			case <-ctx.Done():
				return
			}
		} else {
			attempt = 0
		}
	}
}

func (s *Supervisor) run(ctx context.Context, bin string) error {
	// The bridge connects back on a local socket; we listen, launch the bridge with the path,
	// then accept one connection.
	sockPath := s.opts.SocketPath
	if sockPath == "" {
		sockPath = os.TempDir() + "/virta-xbridge.sock"
		_ = os.Remove(sockPath)
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close(); _ = os.Remove(sockPath) }()

	cmd := exec.CommandContext(ctx, bin, "--socket", sockPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() { _ = cmd.Wait() }()

	s.setState(StateConnecting)
	s.emit(platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthOK, Reason: platform.ReasonCode(StateConnecting)}})

	// Accept the bridge's connection (with a generous timeout so slow browser launches work).
	_ = ln.(*net.UnixListener).SetDeadline(time.Now().Add(30 * time.Second))
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	s.setState(StateConnected)
	s.emit(platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthOK, Reason: platform.ReasonCode(StateConnected)}})

	dec := NewDecoder(conn)
	for {
		f, err := dec.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		s.handle(f)
	}
}

func (s *Supervisor) handle(f Frame) {
	switch f.Type {
	case FrameMessage:
		var p MessagePayload
		if err := json.Unmarshal(f.Payload, &p); err == nil {
			s.emit(xMessageToEvent(p))
		}
	case FrameStatus:
		var p StatusPayload
		if err := json.Unmarshal(f.Payload, &p); err == nil {
			s.setState(p.State)
			reason := StateConnected
			state := platform.HealthOK
			if p.State != StateConnected && p.State != StateConnecting {
				state = platform.HealthDegraded
				reason = p.State
			}
			s.emit(platform.HealthEvent{Status: platform.HealthStatus{State: state, Reason: platform.ReasonCode(reason)}})
		}
	case FrameError:
		var p ErrorPayload
		if err := json.Unmarshal(f.Payload, &p); err == nil {
			s.log.Warn("x-bridge error", "code", p.Code, "detail", p.Detail)
			s.setState(p.Code)
			s.emit(platform.HealthEvent{Status: platform.HealthStatus{State: platform.HealthDegraded, Reason: platform.ReasonCode(p.Code)}})
		}
	}
}

func (s *Supervisor) setState(state string) { s.state.Store(state) }

func (s *Supervisor) emit(ev platform.Event) {
	select {
	case s.events <- ev:
	default:
		// The consumer is slow; drop rather than block.
	}
}

// xMessageToEvent converts a bridge message into a platform event. X messages are best-effort
// (no numeric user id, no platform message id for moderation) so capabilities stay limited.
func xMessageToEvent(p MessagePayload) platform.Event {
	return platform.MessageEvent{Message: platform.UnifiedMessage{
		ID:       p.ContentHash, // content-hash makes these deduplicable across reconnects
		Platform: platform.X,
		Channel:  platform.ChannelRef{Platform: platform.X, Slug: p.BroadcastID},
		Type:     platform.TypeChat,
		Author:   platform.Author{Login: p.Author, DisplayName: p.Author},
		Segments: []platform.Segment{{Kind: platform.SegText, Text: p.Text}},
	}}
}
