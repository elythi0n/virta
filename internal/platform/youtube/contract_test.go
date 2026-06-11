package youtube

import (
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/platform"
	"github.com/elythi0n/virta/internal/platform/platformtest"
)

func TestAdapter_Contract(t *testing.T) {
	platformtest.RunAdapterContract(t, func(t *testing.T) platform.Adapter {
		// The contract never joins a channel, so the bases just need to be non-default to
		// guarantee no accidental network egress.
		return New(Options{
			WebBase:         "http://127.0.0.1:0",
			APIBase:         "http://127.0.0.1:0",
			BackoffBase:     time.Millisecond,
			BackoffMax:      2 * time.Millisecond,
			ResolveRetryMin: time.Millisecond,
			ResolveRetryMax: 2 * time.Millisecond,
		})
	})
}
