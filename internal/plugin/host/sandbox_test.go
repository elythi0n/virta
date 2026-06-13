package host

import "testing"

func TestLoopbackEmbedOrigin(t *testing.T) {
	cases := []struct {
		origin string
		want   string
	}{
		{"http://localhost:35833", "http://localhost:35833"}, // Electron desktop shell origin
		{"http://localhost", "http://localhost"},
		{"http://127.0.0.1:8344", "http://127.0.0.1:8344"},
		{"http://[::1]:9000", "http://[::1]:9000"},
		{"http://app.localhost:3000", "http://app.localhost:3000"}, // any *.localhost is loopback
		{"https://localhost:5173", "https://localhost:5173"},
		{"https://evil.com", ""},          // remote origin must not be allowed to embed
		{"http://localhost.evil.com", ""}, // suffix spoof must not match
		{"ftp://localhost", ""},           // non-HTTP scheme rejected
		{"not a url", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := loopbackEmbedOrigin(c.origin); got != c.want {
			t.Errorf("loopbackEmbedOrigin(%q) = %q, want %q", c.origin, got, c.want)
		}
	}
}
