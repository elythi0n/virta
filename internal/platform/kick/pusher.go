package kick

import (
	"encoding/json"
	"strings"
)

// Kick chat is delivered over Pusher (protocol 7). The handful of frames we care about:
// the connection-established handshake, keepalive ping/pong, our outgoing subscribe, and the
// chat-message event. Pusher double-encodes event payloads — the `data` field is a JSON
// string that itself contains JSON — which pusherPayload unwraps.
const (
	eventConnEstablished = "pusher:connection_established"
	eventPing            = "pusher:ping"
	eventSubscribe       = "pusher:subscribe"
	eventChatMessage     = `App\Events\ChatMessageEvent`
)

// frame is one Pusher message. Data stays raw because its shape depends on Event and is
// usually a quoted JSON string (see pusherPayload).
type frame struct {
	Event   string          `json:"event"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func parseFrame(b []byte) (frame, error) {
	var f frame
	err := json.Unmarshal(b, &f)
	return f, err
}

// pusherPayload returns the inner JSON of a frame's data, unwrapping Pusher's double encoding
// (a JSON string containing JSON) but tolerating a plain object too.
func pusherPayload(f frame) ([]byte, error) {
	if len(f.Data) == 0 {
		return nil, nil
	}
	if f.Data[0] == '"' {
		var s string
		if err := json.Unmarshal(f.Data, &s); err != nil {
			return nil, err
		}
		return []byte(s), nil
	}
	return f.Data, nil
}

// subscribeFrame builds the subscribe message for a chatroom channel.
func subscribeFrame(channel string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"event": eventSubscribe,
		"data":  map[string]string{"channel": channel},
	})
}

// pongFrame answers a server ping to keep the connection alive.
func pongFrame() []byte { return []byte(`{"event":"pusher:pong","data":"{}"}`) }

// chatroomChannel is the Pusher channel name for a chatroom id.
func chatroomChannel(chatroomID string) string { return "chatrooms." + chatroomID + ".v2" }

// chatroomIDFromChannel reverses chatroomChannel ("chatrooms.123.v2" → "123"), or "" if the
// channel name isn't a chatroom subscription.
func chatroomIDFromChannel(channel string) string {
	const prefix, suffix = "chatrooms.", ".v2"
	if !strings.HasPrefix(channel, prefix) || !strings.HasSuffix(channel, suffix) {
		return ""
	}
	return channel[len(prefix) : len(channel)-len(suffix)]
}
