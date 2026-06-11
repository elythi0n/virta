package twitch

import (
	"context"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

// twitchIRCWebSocket is Twitch's IRC-over-WebSocket endpoint.
const twitchIRCWebSocket = "wss://irc-ws.chat.twitch.tv:443"

// wsTransport is the production transport: Twitch IRC carried over a WebSocket. A single
// frame can contain several CRLF-separated IRC lines, so reads buffer the extra lines.
type wsTransport struct {
	conn *websocket.Conn

	writeMu sync.Mutex // coder/websocket permits only one concurrent writer

	pending []string // lines decoded from a frame but not yet returned
}

// dialWebSocket opens the Twitch IRC WebSocket. It is the default DialFunc.
func dialWebSocket(ctx context.Context) (transport, error) {
	conn, resp, err := websocket.Dial(ctx, twitchIRCWebSocket, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	// Twitch IRC frames can be larger than the default limit during bursts; raise it.
	conn.SetReadLimit(1 << 20)
	return &wsTransport{conn: conn}, nil
}

func (w *wsTransport) WriteLine(ctx context.Context, line string) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.conn.Write(ctx, websocket.MessageText, []byte(line+"\r\n"))
}

func (w *wsTransport) ReadLine(ctx context.Context) (string, error) {
	for len(w.pending) == 0 {
		_, data, err := w.conn.Read(ctx)
		if err != nil {
			return "", err
		}
		w.pending = append(w.pending, decodeFrame(data)...)
	}
	line := w.pending[0]
	w.pending = w.pending[1:]
	return line, nil
}

// decodeFrame splits one WebSocket frame into the IRC lines it carries. A Twitch IRC frame
// holds one or more complete CRLF-separated lines; empty lines are dropped.
func decodeFrame(data []byte) []string {
	trimmed := strings.TrimRight(string(data), "\r\n")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "\r\n")
	out := make([]string, 0, len(parts))
	for _, l := range parts {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (w *wsTransport) Close() error { return w.conn.CloseNow() }

// esWSConn carries EventSub's JSON frames over a WebSocket — one frame, one message; no line
// splitting like IRC.
type esWSConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// DialEventSub opens an EventSub WebSocket. It is the production ESDialFunc, passed to Options
// by the daemon wiring; tests use scripted fakes instead.
func DialEventSub(ctx context.Context, url string) (esConn, error) {
	conn, resp, err := websocket.Dial(ctx, url, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(1 << 20)
	return &esWSConn{conn: conn}, nil
}

func (w *esWSConn) Read(ctx context.Context) ([]byte, error) {
	_, data, err := w.conn.Read(ctx)
	return data, err
}

func (w *esWSConn) Write(ctx context.Context, b []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.conn.Write(ctx, websocket.MessageText, b)
}

func (w *esWSConn) Close() error { return w.conn.CloseNow() }
