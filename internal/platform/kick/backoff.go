package kick

import "time"

// backoff computes reconnect delays that grow exponentially up to a ceiling with jitter, so a
// reconnect storm doesn't hammer Pusher in lockstep. Equal jitter: half the delay is fixed,
// half is spread across [0, half] from a caller-supplied random value.
type backoff struct {
	base time.Duration
	max  time.Duration
}

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
