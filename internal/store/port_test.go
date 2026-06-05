package store_test

import (
	"testing"
	"time"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/store"
	"github.com/elythi0n/virta/internal/store/storetest"
)

// The in-memory fake must satisfy the full store contract — the same suite SQLite and
// Postgres will run.
func TestMemory_Contract(t *testing.T) {
	storetest.RunContract(t, func(t *testing.T) store.Store {
		return store.NewMemory(clock.NewFake(time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)))
	})
}
