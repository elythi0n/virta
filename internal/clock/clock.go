// Package clock provides an injectable time source. Production code takes a Clock instead
// of calling time.Now directly, so time-dependent logic stays deterministic under test
// (inject a Fake, advance it explicitly). A lint rule forbids time.Now/Since/Until outside
// this package to keep that guarantee enforced rather than merely encouraged.
package clock

import (
	"sync"
	"time"
)

// Clock is a source of the current time.
type Clock interface {
	Now() time.Time
}

// System is the real clock, backed by the OS.
type System struct{}

// Now returns the current wall-clock time.
func (System) Now() time.Time { return time.Now() }

// Fake is a controllable clock for tests. The zero value is unusable; use NewFake.
// Safe for concurrent use.
type Fake struct {
	mu sync.Mutex
	t  time.Time
}

// NewFake returns a Fake clock fixed at t.
func NewFake(t time.Time) *Fake { return &Fake{t: t} }

// Now returns the fake's current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// Set jumps the fake clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t
}

// Compile-time assertions that both implement Clock.
var (
	_ Clock = System{}
	_ Clock = (*Fake)(nil)
)
