package twitch

import (
	"slices"
	"testing"
)

func TestDecodeFrame(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"single line", "PING :tmi\r\n", []string{"PING :tmi"}},
		{"no crlf", "PING :tmi", []string{"PING :tmi"}},
		{"multi line", "A\r\nB\r\nC\r\n", []string{"A", "B", "C"}},
		{"empty", "\r\n", nil},
		{"blank between", "A\r\n\r\nB\r\n", []string{"A", "B"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeFrame([]byte(tt.in))
			if !slices.Equal(got, tt.want) {
				t.Errorf("decodeFrame(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
