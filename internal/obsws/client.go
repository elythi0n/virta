package obsws

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const requestTimeout = 10 * time.Second

// client is a low-level obs-websocket v5 connection.
type client struct {
	conn    *websocket.Conn
	mu      sync.Mutex
	pending map[string]chan responseData
	seq     atomic.Uint64
}

// dial connects to an obs-websocket server at ws://host:port, authenticates if required,
// and returns the ready client along with the Hello payload (containing version strings).
func dial(ctx context.Context, host, port, password string) (*client, *helloData, error) {
	addr := "ws://" + host + ":" + port
	conn, resp, err := websocket.Dial(ctx, addr, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("obsws dial: %w", err)
	}
	conn.SetReadLimit(4 << 20)

	c := &client{
		conn:    conn,
		pending: make(map[string]chan responseData),
	}

	// Read Hello (op=0).
	var hello message
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		_ = conn.CloseNow()
		return nil, nil, fmt.Errorf("obsws hello: %w", err)
	}
	if hello.Op != opHello {
		_ = conn.CloseNow()
		return nil, nil, fmt.Errorf("obsws: expected op=0 (hello), got op=%d", hello.Op)
	}
	var hd helloData
	if err := json.Unmarshal(hello.Data, &hd); err != nil {
		_ = conn.CloseNow()
		return nil, nil, fmt.Errorf("obsws: decode hello: %w", err)
	}

	// Build Identify payload with optional auth.
	id := identifyData{RPCVersion: 1}
	if hd.Authentication != nil && password != "" {
		auth, err := computeAuth(password, hd.Authentication.Salt, hd.Authentication.Challenge)
		if err != nil {
			_ = conn.CloseNow()
			return nil, nil, fmt.Errorf("obsws: compute auth: %w", err)
		}
		id.Authentication = auth
	}

	// Send Identify (op=1).
	if err := wsjson.Write(ctx, conn, message{Op: opIdentify, Data: mustMarshal(id)}); err != nil {
		_ = conn.CloseNow()
		return nil, nil, fmt.Errorf("obsws: send identify: %w", err)
	}

	// Read Identified (op=2).
	var identified message
	if err := wsjson.Read(ctx, conn, &identified); err != nil {
		_ = conn.CloseNow()
		return nil, nil, fmt.Errorf("obsws: identified: %w", err)
	}
	if identified.Op != opIdentified {
		_ = conn.CloseNow()
		return nil, nil, fmt.Errorf("obsws: expected op=2 (identified), got op=%d (wrong password?)", identified.Op)
	}

	return c, &hd, nil
}

// computeAuth implements the obs-websocket SHA-256 auth formula:
//
//	secret     = base64( sha256( password + salt ) )
//	authString = base64( sha256( secret + challenge ) )
func computeAuth(password, salt, challenge string) (string, error) {
	h1 := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(h1[:])
	h2 := sha256.Sum256([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(h2[:]), nil
}

// readLoop reads frames until the connection closes, routing RequestResponse frames to
// waiting request callers. It exits when an error occurs; the caller selects on a done
// channel to detect disconnection.
func (c *client) readLoop(ctx context.Context) {
	for {
		var msg message
		if err := wsjson.Read(ctx, c.conn, &msg); err != nil {
			return
		}
		if msg.Op != opRequestResponse {
			continue
		}
		var rd responseData
		if err := json.Unmarshal(msg.Data, &rd); err != nil {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[rd.ID]
		if ok {
			delete(c.pending, rd.ID)
		}
		c.mu.Unlock()
		if ok {
			select {
			case ch <- rd:
			default:
			}
		}
	}
}

// request sends a Request frame (op=6) and waits for the correlated RequestResponse.
func (c *client) request(ctx context.Context, reqType string, payload any) (responseData, error) {
	id := fmt.Sprintf("%d", c.seq.Add(1))

	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return responseData{}, err
		}
		raw = b
	}

	ch := make(chan responseData, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	frame := message{Op: opRequest, Data: mustMarshal(requestData{Type: reqType, ID: id, Payload: raw})}
	if err := wsjson.Write(ctx, c.conn, frame); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return responseData{}, err
	}

	tctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	select {
	case <-tctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return responseData{}, fmt.Errorf("obsws: request %q timed out", reqType)
	case rd := <-ch:
		return rd, nil
	}
}

// close terminates the connection.
func (c *client) close() {
	_ = c.conn.CloseNow()
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
