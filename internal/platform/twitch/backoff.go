package twitch

import (
	"time"

	"github.com/elythi0n/virta/internal/clock"
)

// backoff computes reconnect delays that grow exponentially up to a ceiling and carry
// jitter, so a fleet of connections dropped by the same upstream blip doesn't reconnect in
// lockstep (a thundering herd that would hammer the server and get throttled). It uses
// "equal jitter": half of each delay is fixed and half is spread out, keeping waits bounded
// yet desynchronized.
type backoff struct {
	base time.Duration // delay before the first retry
	max  time.Duration // ceiling regardless of how many retries have failed
}

// delay returns the wait before retry number attempt (1-based). The jitter fraction is read
// from the injected clock rather than a global RNG: the whole adapter stays deterministic
// under a fake clock, and there's no second, unsynchronized source of randomness to reason
// about.
func (b backoff) delay(attempt int, clk clock.Clock) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := b.base
	for i := 1; i < attempt && d < b.max; i++ {
		d *= 2
	}
	if d <= 0 || d > b.max {
		d = b.max
	}
	half := d / 2
	// Nanosecond remainder → fraction in [0,1) → jitter in [0, half].
	frac := clk.Now().UnixNano() % 1000
	if frac < 0 {
		frac = -frac
	}
	return half + time.Duration(frac)*half/1000
}
