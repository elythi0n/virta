// reply-bot is a minimal Go bot that connects to a running virtad, listens for "!ping" in any
// chat, and replies with "pong!". Run it with:
//
//	VIRTA_TOKEN=vk_... VIRTA_ADDR=127.0.0.1:50432 go run .
//
// The token needs the `read` and `send` scopes (mint one in Settings → Integrations).
// This example requires a Go installation but no other dependencies beyond the standard library.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/websocket"
)

// WireEvent is a subset of the daemon's event envelope (see /v1/openapi.json for the full spec).
type WireEvent struct {
	Type    string   `json:"type"`
	Seq     int64    `json:"seq"`
	Message *Message `json:"message,omitempty"`
}

type Message struct {
	Platform string   `json:"platform"`
	Channel  Channel  `json:"channel"`
	Author   Author   `json:"author"`
	Segments []Seg    `json:"segments"`
}

type Channel struct{ Slug string `json:"slug"` }
type Author struct{ DisplayName, Login string }
type Seg struct{ Type, Text string }

func main() {
	addr := os.Getenv("VIRTA_ADDR")
	token := os.Getenv("VIRTA_TOKEN")
	if addr == "" || token == "" {
		log.Fatal("Set VIRTA_ADDR and VIRTA_TOKEN")
	}

	wsURL := "ws://" + addr + "/v1/stream?token=" + url.QueryEscape(token)
	origin := "http://" + addr
	ws, err := websocket.Dial(wsURL, "", origin)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer ws.Close()

	// Subscribe to all channels.
	sub, _ := json.Marshal(map[string]any{"action": "subscribe"})
	if _, err := ws.Write(sub); err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	log.Printf("Connected to %s — watching for !ping", addr)

	var lastSeq int64
	for {
		var ev WireEvent
		if err := websocket.JSON.Receive(ws, &ev); err != nil {
			log.Fatalf("receive: %v", err)
		}
		if ev.Seq <= lastSeq {
			continue // replay dedup
		}
		lastSeq = ev.Seq
		if ev.Type != "message" || ev.Message == nil {
			continue
		}
		body := msgBody(ev.Message)
		if !strings.Contains(strings.ToLower(body), "!ping") {
			continue
		}
		channel := ev.Message.Message.Channel.Slug // simplified
		go reply(addr, token, "twitch:"+channel, "pong!")
	}
}

func msgBody(m *Message) string {
	var b strings.Builder
	for _, s := range m.Segments {
		if s.Type == "text" {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func reply(addr, token, channel, text string) {
	body, _ := json.Marshal(map[string]any{"channels": []string{channel}, "text": text})
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/send", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "send error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Fprintf(os.Stderr, "replied in %s -> %d\n", channel, resp.StatusCode)
}
