package api

import "testing"

func TestIsLoopbackCORSOrigin(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"wails://wails.localhost", true},          // macOS/Linux webview custom scheme
		{"http://wails.localhost", true},           // Windows WebView2 origin — regression guard
		{"https://wails.localhost", true},          // same, https variant
		{"http://localhost", true},                 // bare localhost
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
