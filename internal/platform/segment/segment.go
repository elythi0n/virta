// Package segment splits a run of plain message text into typed pieces — text, @mentions,
// and links — so every platform's normalizer produces the same structured content and no
// frontend ever has to scan chat text itself. Platform-specific tokens (emotes, cheers) are
// carved out by the adapter first; this handles what's left, which is just words.
package segment

import (
	"strings"

	"github.com/elythi0n/virta/internal/platform"
)

// Text splits s into ordered segments. Adjacent plain words (and the spaces between them)
// coalesce into a single text segment; @mentions and http(s) links become their own
// segments. The concatenation of all segment Texts reproduces s exactly.
func Text(s string) []platform.Segment {
	if s == "" {
		return nil
	}
	var segs []platform.Segment
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			segs = append(segs, platform.Segment{Kind: platform.SegText, Text: buf.String()})
			buf.Reset()
		}
	}

	for i := 0; i < len(s); {
		if s[i] == ' ' {
			buf.WriteByte(' ')
			i++
			continue
		}
		// Read one whitespace-delimited token.
		j := i
		for j < len(s) && s[j] != ' ' {
			j++
		}
		tok := s[i:j]
		switch {
		case isLink(tok):
			flush()
			segs = append(segs, platform.Segment{Kind: platform.SegLink, Text: tok})
		case tok[0] == '@':
			// A mention is "@" plus a run of username characters; any trailing punctuation
			// (e.g. "@user," or "@user:") stays as text.
			n := mentionLen(tok)
			if n > 1 {
				flush()
				segs = append(segs, platform.Segment{Kind: platform.SegMention, Text: tok[:n]})
				buf.WriteString(tok[n:]) // trailing punctuation, if any
			} else {
				buf.WriteString(tok)
			}
		default:
			buf.WriteString(tok)
		}
		i = j
	}
	flush()
	return segs
}

func isLink(tok string) bool {
	return strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://")
}

// mentionLen returns the length of the "@username" prefix of tok (including the "@"), or 1
// if there are no username characters after the "@".
func mentionLen(tok string) int {
	n := 1 // the '@'
	for n < len(tok) && isUserChar(tok[n]) {
		n++
	}
	return n
}

func isUserChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}
