// Package filter is the pipeline's rule stage: ordered, composable rules that hide, highlight,
// or mask messages. Rules are evaluated in the core so every frontend agrees on the result.
// The stage is pure and hot-path-safe — it reads an immutable compiled ruleset via a lock-free
// atomic load and never does I/O; word lists are recompiled off the hot path and swapped in.
//
// hide/highlight are display annotations (the message is still logged and counted); mask
// rewrites profane spans to a mask token, keeping the original only in Segment.Reveal for the
// local feed's click-to-reveal — PlainText (and therefore TTS, webhooks, logging) stays masked.
package filter

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/elythi0n/virta/internal/platform"
)

// maskToken is the visible replacement for masked text (U+2217 asterisk operators).
const maskToken = "∗∗∗"

// Action is what a matching rule does.
type Action string

const (
	ActionHide      Action = "hide"
	ActionHighlight Action = "highlight"
	ActionMask      Action = "mask"
)

// Match scopes a rule. Empty categories don't constrain; within a category the options are
// OR'd; across categories they're AND'd. Keywords are whole-word, case-insensitive.
type Match struct {
	Platforms []platform.Platform
	Channels  []string // slugs
	Authors   []string // logins
	Types     []platform.MessageType
	Keywords  []string
	Regexes   []string
}

// Rule is one ordered filter rule.
type Rule struct {
	ID     string
	Action Action
	Match  Match
}

type compiledRule struct {
	id        string
	action    Action
	platforms map[platform.Platform]struct{}
	channels  map[string]struct{}
	authors   map[string]struct{}
	types     map[platform.MessageType]struct{}
	keywords  map[string]struct{} // lowercased whole words
	regexes   []*regexp.Regexp
}

// Ruleset is a compiled, immutable set of rules ready for hot-path evaluation.
type Ruleset struct{ rules []compiledRule }

func set[T comparable](xs []T) map[T]struct{} {
	if len(xs) == 0 {
		return nil
	}
	m := make(map[T]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

// Compile validates and compiles rules (regexes, lowercased keyword sets) once, off the hot
// path. A bad regex fails the whole compile so the problem surfaces at config time.
func Compile(rules []Rule) (*Ruleset, error) {
	rs := &Ruleset{rules: make([]compiledRule, 0, len(rules))}
	for _, r := range rules {
		cr := compiledRule{
			id:        r.ID,
			action:    r.Action,
			platforms: set(r.Match.Platforms),
			channels:  set(r.Match.Channels),
			types:     set(r.Match.Types),
		}
		if len(r.Match.Authors) > 0 {
			cr.authors = make(map[string]struct{}, len(r.Match.Authors))
			for _, a := range r.Match.Authors {
				cr.authors[strings.ToLower(a)] = struct{}{}
			}
		}
		if len(r.Match.Keywords) > 0 {
			cr.keywords = make(map[string]struct{}, len(r.Match.Keywords))
			for _, k := range r.Match.Keywords {
				cr.keywords[strings.ToLower(k)] = struct{}{}
			}
		}
		for _, expr := range r.Match.Regexes {
			re, err := regexp.Compile(expr)
			if err != nil {
				return nil, fmt.Errorf("filter: rule %q: bad regex %q: %w", r.ID, expr, err)
			}
			cr.regexes = append(cr.regexes, re)
		}
		rs.rules = append(rs.rules, cr)
	}
	return rs, nil
}

// scopeMatches checks the platform/channel/author/type constraints (not text).
func (r compiledRule) scopeMatches(msg *platform.UnifiedMessage) bool {
	if r.platforms != nil {
		if _, ok := r.platforms[msg.Platform]; !ok {
			return false
		}
	}
	if r.channels != nil {
		if _, ok := r.channels[msg.Channel.Slug]; !ok {
			return false
		}
	}
	if r.authors != nil {
		if _, ok := r.authors[strings.ToLower(msg.Author.Login)]; !ok {
			return false
		}
	}
	if r.types != nil {
		if _, ok := r.types[msg.Type]; !ok {
			return false
		}
	}
	return true
}

// textHit reports whether the rule's keyword/regex terms appear in the text, for hide/highlight
// (which use case-insensitive substring matching — you highlight your name wherever it
// appears). A rule with no text terms "hits" on scope alone (e.g. hide-by-author). Masking, by
// contrast, matches whole words only (see collectSpans).
func (r compiledRule) textHit(text string) bool {
	if len(r.keywords) == 0 && len(r.regexes) == 0 {
		return true
	}
	lower := strings.ToLower(text)
	for kw := range r.keywords { // keys are already lowercased by Compile
		if strings.Contains(lower, kw) {
			return true
		}
	}
	for _, re := range r.regexes {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// Stage applies the current ruleset to each message.
type Stage struct{ rs atomic.Pointer[Ruleset] }

// NewStage builds the filter stage with an initial ruleset (may be an empty *Ruleset).
func NewStage(rs *Ruleset) *Stage {
	s := &Stage{}
	if rs == nil {
		rs = &Ruleset{}
	}
	s.rs.Store(rs)
	return s
}

// SetRuleset hot-swaps the ruleset (called off the hot path when a profile/settings change).
func (s *Stage) SetRuleset(rs *Ruleset) {
	if rs == nil {
		rs = &Ruleset{}
	}
	s.rs.Store(rs)
}

func (s *Stage) Name() string { return "filter" }

// Annotate evaluates every rule in order, applying hide/highlight annotations and masking
// profane spans in place.
func (s *Stage) Annotate(_ context.Context, msg *platform.UnifiedMessage) error {
	rs := s.rs.Load()
	if rs == nil || len(rs.rules) == 0 {
		return nil
	}
	text := msg.PlainText()
	for i := range rs.rules {
		r := rs.rules[i]
		if !r.scopeMatches(msg) {
			continue
		}
		switch r.action {
		case ActionHide:
			if r.textHit(text) {
				msg.Annotate().Hidden = true
			}
		case ActionHighlight:
			if r.textHit(text) {
				msg.Annotate().Highlight = r.id
			}
		case ActionMask:
			if maskSegments(msg, r.keywords, r.regexes) {
				msg.Annotate().Masked = true
			}
		}
	}
	return nil
}

var _ interface {
	Name() string
	Annotate(context.Context, *platform.UnifiedMessage) error
} = (*Stage)(nil)

// maskSegments rewrites the message's text segments, replacing keyword/regex spans with masked
// segments. Returns true if anything was masked. Non-text segments are left untouched.
func maskSegments(msg *platform.UnifiedMessage, kw map[string]struct{}, regexes []*regexp.Regexp) bool {
	if len(kw) == 0 && len(regexes) == 0 {
		return false
	}
	masked := false
	out := make([]platform.Segment, 0, len(msg.Segments))
	for _, seg := range msg.Segments {
		if seg.Kind != platform.SegText {
			out = append(out, seg)
			continue
		}
		spans := collectSpans(seg.Text, kw, regexes)
		if len(spans) == 0 {
			out = append(out, seg)
			continue
		}
		masked = true
		out = append(out, splitMasked(seg.Text, spans)...)
	}
	if masked {
		msg.Segments = out
	}
	return masked
}

// splitMasked breaks text into text/masked segments at the given (byte) spans.
func splitMasked(text string, spans [][2]int) []platform.Segment {
	var segs []platform.Segment
	cursor := 0
	for _, sp := range spans {
		if sp[0] > cursor {
			segs = append(segs, platform.Segment{Kind: platform.SegText, Text: text[cursor:sp[0]]})
		}
		segs = append(segs, platform.Segment{Kind: platform.SegMasked, Text: maskToken, Reveal: text[sp[0]:sp[1]]})
		cursor = sp[1]
	}
	if cursor < len(text) {
		segs = append(segs, platform.Segment{Kind: platform.SegText, Text: text[cursor:]})
	}
	return segs
}

// collectSpans returns merged byte spans in text matched by whole-word keywords (case-
// insensitive, Unicode-aware) or regexes. Each candidate word is lowercased individually so
// byte offsets stay aligned with text even when lowering changes a rune's byte length.
func collectSpans(text string, kw map[string]struct{}, regexes []*regexp.Regexp) [][2]int {
	var spans [][2]int
	if len(kw) > 0 {
		i := 0
		for i < len(text) {
			r, size := utf8.DecodeRuneInString(text[i:])
			if !isWordRune(r) {
				i += size
				continue
			}
			start := i
			for i < len(text) {
				r, size := utf8.DecodeRuneInString(text[i:])
				if !isWordRune(r) {
					break
				}
				i += size
			}
			if _, ok := kw[strings.ToLower(text[start:i])]; ok {
				spans = append(spans, [2]int{start, i})
			}
		}
	}
	for _, re := range regexes {
		for _, m := range re.FindAllStringIndex(text, -1) {
			if m[0] != m[1] { // ignore zero-width matches
				spans = append(spans, [2]int{m[0], m[1]})
			}
		}
	}
	return mergeSpans(spans)
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// mergeSpans sorts and coalesces overlapping/adjacent spans so masking never double-splits.
func mergeSpans(spans [][2]int) [][2]int {
	if len(spans) < 2 {
		return spans
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	merged := spans[:1]
	for _, sp := range spans[1:] {
		last := &merged[len(merged)-1]
		if sp[0] <= last[1] {
			if sp[1] > last[1] {
				last[1] = sp[1]
			}
			continue
		}
		merged = append(merged, sp)
	}
	return merged
}
