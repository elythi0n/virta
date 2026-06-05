package id_test

import (
	"bytes"
	"sort"
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/id"
)

func TestULID_FormatAndLength(t *testing.T) {
	g := id.NewULID(clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)))
	const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for range 100 {
		s := g.New()
		if len(s) != 26 {
			t.Fatalf("ULID length = %d, want 26: %q", len(s), s)
		}
		for _, c := range s {
			if !bytes.ContainsRune([]byte(crockford), c) {
				t.Fatalf("ULID %q has non-Crockford char %q", s, c)
			}
		}
	}
}

func TestULID_MonotonicWithinSameMillisecond(t *testing.T) {
	// Fixed clock ⇒ every id shares a timestamp; only the entropy increment keeps them
	// strictly increasing. This is what makes string-sorting equal mint order.
	g := id.NewULID(clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)))
	prev := g.New()
	for range 1000 {
		cur := g.New()
		if cur <= prev {
			t.Fatalf("not strictly increasing within a ms: %q then %q", prev, cur)
		}
		prev = cur
	}
}

func TestULID_SortsByTime(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	g := id.NewULID(clk)
	var ids []string
	for range 50 {
		ids = append(ids, g.New())
		clk.Advance(time.Millisecond)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("ULIDs minted over advancing time are not lexicographically sorted")
	}
}

func TestULID_Unique(t *testing.T) {
	g := id.NewULID(clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)))
	seen := make(map[string]struct{}, 10000)
	for range 10000 {
		s := g.New()
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate ULID: %q", s)
		}
		seen[s] = struct{}{}
	}
}

func TestULID_DeterministicWithInjectedEntropy(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC))
	mk := func() string {
		// Zero entropy ⇒ fully reproducible first id for a given timestamp.
		return id.NewULIDWithEntropy(clk, bytes.NewReader(make([]byte, 10))).New()
	}
	if a, b := mk(), mk(); a != b {
		t.Fatalf("same clock + zero entropy should be reproducible: %q != %q", a, b)
	}
}

func TestFake_DeterministicAndSorted(t *testing.T) {
	f := id.NewFake("msg")
	a, b, c := f.New(), f.New(), f.New()
	if a != "msg_00000000000000000001" {
		t.Errorf("first = %q", a)
	}
	if a >= b || b >= c {
		t.Errorf("fake ids not increasing: %q %q %q", a, b, c)
	}
}
