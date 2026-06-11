package youtube

import "time"

// backoff computes retry delays that grow exponentially up to a ceiling with jitter. Equal
// jitter: half the delay is fixed, half is spread across [0, half] from a caller-supplied
// random value — so per-channel pollers don't retry in lockstep.
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
