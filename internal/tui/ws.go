package tui

import (
	"context"

	"github.com/coder/websocket"
)

// websocketConn wraps a coder/websocket connection in the Read/Write/Close interface the model uses.
type websocketConn struct{ c *websocket.Conn }

func (w *websocketConn) Read(ctx context.Context) (msgType websocket.MessageType, p []byte, err error) {
	return w.c.Read(ctx)
}

func (w *websocketConn) Write(ctx context.Context, msgType websocket.MessageType, p []byte) error {
	return w.c.Write(ctx, msgType, p)
}

func (w *websocketConn) Close(code websocket.StatusCode, reason string) error {
	return w.c.Close(code, reason)
}

// websocketConnect dials a WebSocket URL and returns a conn.
func websocketConnect(wsURL string) (*websocketConn, *websocket.Conn, error) {
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		return nil, nil, err
	}
	return &websocketConn{c: c}, c, nil
}
