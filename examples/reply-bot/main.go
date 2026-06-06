// reply-bot is a minimal Go bot that connects to a running virtad, listens for "!ping" in any
// chat, and replies with "pong!". Uses only the standard library.
//
// Run: VIRTA_TOKEN=vk_... VIRTA_ADDR=127.0.0.1:50432 go run main.go
//
// Needs `read` + `send` scopes. Mint a token in Settings → Integrations.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

type WireEvent struct {
	Type    string   `json:"type"`
	Seq     int64    `json:"seq"`
	Message *Message `json:"message,omitempty"`
}

type Message struct {
	Platform string  `json:"platform"`
	Channel  Channel `json:"channel"`
	Segments []Seg   `json:"segments"`
}

type Channel struct{ Slug string `json:"slug"` }
type Seg struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func main() {
	addr := os.Getenv("VIRTA_ADDR")
	token := os.Getenv("VIRTA_TOKEN")
	if addr == "" || token == "" {
		log.Fatal("Set VIRTA_ADDR and VIRTA_TOKEN")
	}
	conn, err := wsConnect(addr, token)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	sub, _ := json.Marshal(map[string]any{"action": "subscribe"})
	if err := wsWriteText(conn, sub); err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	log.Printf("Connected — watching for !ping")
	var lastSeq int64
	for {
		msg, err := wsReadText(conn)
		if err != nil {
			log.Fatalf("receive: %v", err)
		}
		var ev WireEvent
		if err := json.Unmarshal(msg, &ev); err != nil {
			continue
		}
		if ev.Seq <= lastSeq {
			continue
		}
		lastSeq = ev.Seq
		if ev.Type != "message" || ev.Message == nil {
			continue
		}
		var body strings.Builder
		for _, s := range ev.Message.Segments {
			if s.Type == "text" {
				body.WriteString(s.Text)
			}
		}
		if !strings.Contains(strings.ToLower(body.String()), "!ping") {
			continue
		}
		ch := ev.Message.Platform + ":" + ev.Message.Channel.Slug
		go reply(addr, token, ch, "pong!")
	}
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

func wsConnect(addr, token string) (net.Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	hs := fmt.Sprintf(
		"GET /v1/stream?token=%s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n",
		token, addr)
	if _, err := conn.Write([]byte(hs)); err != nil {
		return nil, err
	}
	rd := bufio.NewReader(conn)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" {
			break
		}
	}
	return conn, nil
}

func wsWriteText(conn net.Conn, msg []byte) error {
	l := len(msg)
	frame := []byte{0x81}
	switch {
	case l < 126:
		frame = append(frame, byte(l)|0x80)
	case l < 65536:
		frame = append(frame, 126|0x80, byte(l>>8), byte(l))
	}
	mask := []byte{0x37, 0xfa, 0x21, 0x3d}
	frame = append(frame, mask...)
	for i, b := range msg {
		frame = append(frame, b^mask[i%4])
	}
	_, err := conn.Write(frame)
	return err
}

func wsReadText(conn net.Conn) ([]byte, error) {
	rd := bufio.NewReader(conn)
	_, _ = rd.ReadByte()
	b1, _ := rd.ReadByte()
	paylen := int(b1 & 0x7f)
	if paylen == 126 {
		hi, _ := rd.ReadByte()
		lo, _ := rd.ReadByte()
		paylen = int(hi)<<8 | int(lo)
	}
	buf := make([]byte, paylen)
	for i := range buf {
		buf[i], _ = rd.ReadByte()
	}
	return buf, nil
}
