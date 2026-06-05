package filter

import (
	"bufio"
	"io"
	"strings"
)

// Profanity word lists are DATA, not source. This package deliberately ships no built-in list
// of slurs: baking offensive terms into the codebase surfaces them in code search/review and
// bloats a public repo, and word lists need updating without a recompile. Instead the matching
// engine lives here while the terms are loaded at runtime from a maintained external list
// (downloaded and cached like other remote-config data) and/or the user's own custom list —
// the same pattern used for other volatile defaults in this project.

// ParseList reads newline-delimited terms — the format of both a downloaded list and a user's
// custom list. Blank lines and #-comments are ignored; terms are trimmed and lowercased.
func ParseList(r io.Reader) []string {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, strings.ToLower(line))
	}
	return out
}

// ProfanityRule builds a mask rule from already-loaded terms (a parsed list merged with the
// user's custom terms). The caller supplies the words; this package never embeds them.
func ProfanityRule(id string, terms ...string) Rule {
	return Rule{ID: id, Action: ActionMask, Match: Match{Keywords: terms}}
}
