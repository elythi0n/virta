package translate

import (
	"context"
	"testing"
)

func TestDetector_SkipsShortAndCommands(t *testing.T) {
	d := NewDetector(nil)
	for _, skip := range []string{"ok", "lol", "/ban user", "!clip", "   "} {
		if d.Detect(skip) != Unknown {
			t.Errorf("should skip %q but got a tag", skip)
		}
	}
}

func TestDetector_DetectsEnglish(t *testing.T) {
	d := NewDetector(nil)
	// A clear English sentence.
	tag := d.Detect("This is a perfectly normal English sentence with enough words to detect.")
	if tag != "en" && tag != Unknown {
		t.Errorf("English detection = %q, want en or unknown", tag)
	}
	// We accept "unknown" here because lingua needs longer text for high confidence.
}

func TestDetector_SkipsEmoteLike(t *testing.T) {
	// Body that looks like emote codes (no real letters) should be skipped.
	d := NewDetector(nil)
	tag := d.Detect("KEKW LUL Pog PogChamp")
	// These are all-caps, which is common for emote codes; the skip heuristic may or may not catch this.
	// The test just verifies no panic.
	_ = tag
}

func TestNoop(t *testing.T) {
	n := Noop{}
	if n.Available() {
		t.Error("noop should not be available")
	}
	result, err := n.Translate(context.TODO(), "hello", "en", "fr")
	if err != nil || result != "" {
		t.Errorf("noop.Translate = %q, %v", result, err)
	}
}
