package segment_test

import (
	"strings"
	"testing"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/segment"
)

func kinds(segs []platform.Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(string(s.Kind))
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}

func reassemble(segs []platform.Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestText(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantKinds string
	}{
		{"empty", "", ""},
		{"plain", "hello world", "text"},
		{"mention", "hi @alice", "text mention"},
		{"mention then text", "@alice hello", "mention text"},
		{"mention mid", "yo @bob nice", "text mention text"},
		{"two mentions", "@a @b", "mention text mention"},
		{"link", "see https://example.com now", "text link text"},
		{"mention trailing punct", "thanks @carol!", "text mention text"},
		{"bare at", "email@ x", "text"},
		{"only mention", "@dave", "mention"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segs := segment.Text(tt.in)
			if got := kinds(segs); got != tt.wantKinds {
				t.Errorf("kinds = %q, want %q (segs=%+v)", got, tt.wantKinds, segs)
			}
			// The segments must always reproduce the input exactly.
			if got := reassemble(segs); got != tt.in {
				t.Errorf("reassembled = %q, want %q", got, tt.in)
			}
		})
	}
}

func TestText_MentionTextIsExact(t *testing.T) {
	segs := segment.Text("thanks @carol! and @dave_99")
	var mentions []string
	for _, s := range segs {
		if s.Kind == platform.SegMention {
			mentions = append(mentions, s.Text)
		}
	}
	if len(mentions) != 2 || mentions[0] != "@carol" || mentions[1] != "@dave_99" {
		t.Errorf("mentions = %v, want [@carol @dave_99]", mentions)
	}
}
