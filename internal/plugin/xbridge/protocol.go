// Package xbridge defines the virtad ↔ x-bridge protocol and the supervisor that manages the
// bridge process. The bridge is a separate binary (cmd/x-bridge) that scrapes X.com broadcast
// chat via a local Chromium instance; it connects back to virtad over a loopback socket and
// forwards events in the same wire-event envelope the other adapters use. This package is the
// daemon's half: the protocol, the connection handler, and the supervisor with circuit-breaker
// backoff so a bridge crash or a selector change degrades gracefully.
package xbridge

import (
	"bufio"
	"encoding/json"
	"io"
)

// FrameType identifies a message from the bridge.
type FrameType string

const (
	// FrameMessage carries a chat message from a broadcast.
	FrameMessage FrameType = "message"
	// FrameStatus reports the bridge's own state (connecting, live, ended, degraded).
	FrameStatus FrameType = "status"
	// FrameError carries a machine reason code — never prose; the UI maps codes to its own copy.
	FrameError FrameType = "error"
)

// Frame is one newline-delimited JSON frame on the bridge socket.
type Frame struct {
	Type    FrameType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// StatusPayload is the FrameStatus payload.
type StatusPayload struct {
	// State is the bridge's current condition as a machine code. Matches the reason-code enum
	// used everywhere else in the daemon so the settings UI can render it from the same table.
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"` // optional human-visible detail (for diagnostics)
}

// ErrorPayload is the FrameError payload.
type ErrorPayload struct {
	Code   string `json:"code"`   // machine reason code, e.g. "selector_missing", "auth_required"
	Detail string `json:"detail"` // what selector/URL/etc was involved (diagnostics only)
}

// MessagePayload is a chat message forwarded from a broadcast page.
type MessagePayload struct {
	BroadcastID string `json:"broadcast_id"`
	Author      string `json:"author"`
	Verified    bool   `json:"verified"`
	Text        string `json:"text"`
	ContentHash string `json:"content_hash"` // sha256(author+text) for dedup
	TimestampMs int64  `json:"timestamp_ms"` // best-effort page timestamp; 0 if unavailable
}

// Decoder wraps a reader with newline-delimited JSON frame decoding.
type Decoder struct {
	sc *bufio.Scanner
}

// NewDecoder wraps r. Each call to Next() returns one decoded frame.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{sc: bufio.NewScanner(r)}
}

// Next reads the next frame. Returns io.EOF when the connection closes.
func (d *Decoder) Next() (Frame, error) {
	if !d.sc.Scan() {
		if err := d.sc.Err(); err != nil {
			return Frame{}, err
		}
		return Frame{}, io.EOF
	}
	var f Frame
	err := json.Unmarshal(d.sc.Bytes(), &f)
	return f, err
}

// Encoder writes newline-delimited JSON frames to w.
type Encoder struct {
	w io.Writer
}

// NewEncoder wraps w.
func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

// Write encodes and writes a frame.
func (e *Encoder) Write(f Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = e.w.Write(b)
	return err
}

// MarshalPayload encodes a typed payload as the Frame.Payload field.
func MarshalPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
