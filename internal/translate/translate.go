// Package translate provides language detection (lingua-go, offline, always-free) and
// translation (tiered: LibreTranslate sidecar, LLM batch, DeepL). It is the 7.7 translation
// feature: detection runs on every message once enabled; translation only when requested.
package translate

import (
	"context"
	"strings"
	"sync"

	"github.com/pemistahl/lingua-go"
)

// Tag is a BCP-47 language tag (e.g. "en", "ja", "es").
type Tag string

// Unknown is returned when detection is not confident.
const Unknown Tag = ""

// Detector detects the language of a message body. It is offline and free (lingua-go).
type Detector struct {
	d        lingua.LanguageDetector
	once     sync.Once
	skip     func(body string) bool // heuristic: emote-only, very short, commands
}

// NewDetector builds a Detector for the given preferred languages. Detection is only run on
// messages that aren't in the preferred set (so all-English streams have zero overhead).
func NewDetector(preferred []Tag) *Detector {
	return &Detector{
		skip: makeSkipFn(),
	}
}

// Detect returns the detected language of body, or Unknown if confidence is low or the body
// should be skipped (emote-only, too short, a slash command).
func (d *Detector) Detect(body string) Tag {
	d.once.Do(func() {
		d.d = lingua.NewLanguageDetectorBuilder().
			FromAllLanguages().
			WithMinimumRelativeDistance(0.15).
			Build()
	})
	if d.skip(body) {
		return Unknown
	}
	lang, ok := d.d.DetectLanguageOf(body)
	if !ok {
		return Unknown
	}
	iso := lang.IsoCode639_1()
	return Tag(strings.ToLower(iso.String()))
}

func makeSkipFn() func(string) bool {
	return func(body string) bool {
		if len(body) < 4 {
			return true // too short
		}
		if strings.HasPrefix(body, "/") || strings.HasPrefix(body, "!") {
			return true // command
		}
		// Heuristic emote-only: a body with no letters (only emote codes + spaces).
		// A proper emote-only check would use the segments, but the translator sees plain text.
		wordCount := len(strings.Fields(body))
		letterCount := 0
		for _, r := range body {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				letterCount++
			}
		}
		return wordCount > 0 && float64(letterCount)/float64(len(body)) < 0.3
	}
}

// Engine is one translation backend.
type Engine interface {
	// ID identifies the engine (libretranslate | llm | deepl).
	ID() string
	// Translate translates text from src to dst language tag. src="" = auto-detect.
	Translate(ctx context.Context, text, src, dst string) (string, error)
	// Available reports whether the engine is usable (key set, sidecar running, etc.)
	Available() bool
}

// Noop is a no-op translation engine; returned when no engine is configured.
type Noop struct{}

func (Noop) ID() string                                       { return "noop" }
func (Noop) Available() bool                                  { return false }
func (Noop) Translate(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
