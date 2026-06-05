package kick

import (
	"context"
	"sync"

	"github.com/coder/websocket"
)

// pusherHost is Kick's Pusher cluster. The URL carries the app key and protocol version as
// query parameters, mirroring the official pusher-js client.
const pusherHost = "wss://ws-us2.pusher.com/app/"

func pusherURL(appKey string) string {
	return pusherHost + appKey + "?protocol=7&client=virta&version=8.4.0"
}

// wsTransport is the production transport: Pusher carried over a WebSocket. Each frame is one
// JSON message.
type wsTransport struct {
	conn    *websocket.Conn
	writeMu sync.Mutex // coder/websocket permits only one concurrent writer
}

// dialPusher opens the Kick Pusher WebSocket for the given app key.
func dialPusher(ctx context.Context, appKey string) (transport, error) {
	conn, resp, err := websocket.Dial(ctx, pusherURL(appKey), nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(1 << 20) // chat bursts can exceed the default frame limit
	return &wsTransport{conn: conn}, nil
}

func (w *wsTransport) Write(ctx context.Context, b []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.conn.Write(ctx, websocket.MessageText, b)
}

func (w *wsTransport) Read(ctx context.Context) ([]byte, error) {
	_, data, err := w.conn.Read(ctx)
	return data, err
}

func (w *wsTransport) Close() error { return w.conn.CloseNow() }
