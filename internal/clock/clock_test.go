package clock_test

import (
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

var base = time.Date(2026, time.June, 5, 12, 0, 0, 0, time.UTC)

func TestFake_StartsAtGivenTime(t *testing.T) {
	f := clock.NewFake(base)
	if got := f.Now(); !got.Equal(base) {
		t.Fatalf("Now() = %v, want %v", got, base)
	}
}

func TestFake_Advance(t *testing.T) {
	f := clock.NewFake(base)
	f.Advance(90 * time.Minute)
	if got, want := f.Now(), base.Add(90*time.Minute); !got.Equal(want) {
		t.Fatalf("after Advance: Now() = %v, want %v", got, want)
	}
}

func TestFake_Set(t *testing.T) {
	f := clock.NewFake(base)
	next := base.Add(24 * time.Hour)
	f.Set(next)
	if got := f.Now(); !got.Equal(next) {
		t.Fatalf("after Set: Now() = %v, want %v", got, next)
	}
}

func TestSystem_AdvancesMonotonically(t *testing.T) {
	var c clock.System
	t1 := c.Now()
	t2 := c.Now()
	if t2.Before(t1) {
		t.Fatalf("System clock went backwards: %v then %v", t1, t2)
	}
}
