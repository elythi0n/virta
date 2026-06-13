package api

import "testing"

func TestIsLoopbackCORSOrigin(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"http://localhost", true},                 // Electron desktop shell origin
		{"http://localhost:5173", true},            // Vite dev server
		{"http://127.0.0.1:8080", true},            // loopback IPv4
		{"http://[::1]:8080", true},                // loopback IPv6
		{"http://app.localhost:3000", true},        // any *.localhost is loopback per RFC 6761
		{"https://example.com", false},             // external origin
		{"http://wails.localhost.evil.com", false}, // suffix spoof must not match
		{"", false},
	}
	for _, c := range cases {
		if got := isLoopbackCORSOrigin(c.origin); got != c.want {
			t.Errorf("isLoopbackCORSOrigin(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
}

func TestIsLoopbackOrigin(t *testing.T) {
	const host = "127.0.0.1:55603"
	cases := []struct {
		origin string
		want   bool
	}{
		{"http://" + host, true},   // exact same-origin
		{"https://" + host, true},  // exact same-origin, https
		{"http://localhost", true}, // Electron desktop shell origin
		{"http://127.0.0.1:9000", true},
		{"http://[::1]:9000", true},
		{"http://app.localhost", true},         // any *.localhost is loopback per RFC 6761
		{"https://twitch.tv", false},           // external origin
		{"http://wails.localhost.evil", false}, // suffix spoof must not match
		{"", false},
	}
	for _, c := range cases {
		if got := isLoopbackOrigin(c.origin, host); got != c.want {
			t.Errorf("isLoopbackOrigin(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
}
