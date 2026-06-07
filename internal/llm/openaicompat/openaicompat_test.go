package openaicompat

import (
	"testing"
)

func TestModelSupportsTools(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"gpt-4o", true},
		{"gpt-4o-mini", true},
		{"babbage-002", false},
		{"text-davinci-003", false},
		{"llama3.3:70b", true},
		{"claude-opus-4-8", true},
	}
	for _, c := range cases {
		if got := modelSupportsTools(c.id); got != c.want {
			t.Errorf("modelSupportsTools(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}
