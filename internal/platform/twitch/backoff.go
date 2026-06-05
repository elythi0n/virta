package twitch

import "time"

// backoff computes reconnect delays that grow exponentially up to a ceiling and carry
// jitter, so a fleet of connections dropped by the same upstream blip doesn't reconnect in
// lockstep (a thundering herd that would hammer the server and get throttled). It uses
// "equal jitter": half of each delay is fixed and half is spread across [0, half] using a
// caller-supplied random value.
type backoff struct {
	base time.Duration // delay before the first retry
	max  time.Duration // ceiling regardless of how many retries have failed
}

// delay returns the wait before retry number attempt (1-based). rnd is a random 64-bit value
// supplied by the caller (each shard owns an independent generator), which is what actually
// desynchronizes a fleet — two shards retrying in the same instant still draw different
// jitter.
func (b backoff) delay(attempt int, rnd uint64) time.Duration {
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
	if half <= 0 {
		return d
	}
	return half + time.Duration(rnd%uint64(half+1))
}
