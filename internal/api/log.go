package api

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ringHandler is an slog.Handler that keeps the most recent log records in memory (for the
// diagnostics endpoint) while forwarding every record to a wrapped handler (normal output).
// The in-memory copy is what powers a "what just happened" view without writing logs to disk.
type ringHandler struct {
	base slog.Handler
	ring *logRing
}

// logRing is a fixed-capacity circular buffer of formatted log lines, safe for concurrent use.
type logRing struct {
	mu    sync.Mutex
	lines []LogLine
	size  int
	next  int
	full  bool
}

// LogLine is one captured log record, in a form ready to serialize for diagnostics.
type LogLine struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

func newLogRing(size int) *logRing {
	if size <= 0 {
		size = 200
	}
	return &logRing{lines: make([]LogLine, size), size: size}
}

func (r *logRing) add(l LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines[r.next] = l
	r.next = (r.next + 1) % r.size
	if r.next == 0 {
		r.full = true
	}
}

// snapshot returns the buffered lines oldest-first.
func (r *logRing) snapshot() []LogLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		out := make([]LogLine, r.next)
		copy(out, r.lines[:r.next])
		return out
	}
	out := make([]LogLine, 0, r.size)
	out = append(out, r.lines[r.next:]...)
	out = append(out, r.lines[:r.next]...)
	return out
}

// captureLevel is the lowest level kept in the diagnostics ring. Info and above are always
// captured (so the ring is useful regardless of where logs are written), while output to the
// wrapped handler still respects that handler's own level.
const captureLevel = slog.LevelInfo

func newRingHandler(base slog.Handler, ring *logRing) *ringHandler {
	return &ringHandler{base: base, ring: ring}
}

// Enabled is true when either the diagnostics ring wants the record or the wrapped handler
// does, so capturing for diagnostics never depends on the output handler's level.
func (h *ringHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= captureLevel || h.base.Enabled(ctx, l)
}

func (h *ringHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= captureLevel {
		line := LogLine{Time: r.Time, Level: r.Level.String(), Message: r.Message}
		if r.NumAttrs() > 0 {
			line.Attrs = make(map[string]any, r.NumAttrs())
			r.Attrs(func(a slog.Attr) bool {
				line.Attrs[a.Key] = a.Value.Any()
				return true
			})
		}
		h.ring.add(line)
	}
	// Forward to the real output only if it would accept the record itself.
	if h.base.Enabled(ctx, r.Level) {
		return h.base.Handle(ctx, r)
	}
	return nil
}

func (h *ringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ringHandler{base: h.base.WithAttrs(attrs), ring: h.ring}
}

func (h *ringHandler) WithGroup(name string) slog.Handler {
	return &ringHandler{base: h.base.WithGroup(name), ring: h.ring}
}
