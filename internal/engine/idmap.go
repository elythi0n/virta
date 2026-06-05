package engine

import "sync"

// idMap is a bounded platform-id → engine-ULID map with FIFO eviction. Deletions reference
// recent messages, so keeping only the most recent N mappings is enough: an id that has
// aged out resolves to "" and the frontend falls back to matching on the platform id. The
// fixed-size ring makes memory constant regardless of stream volume — no unbounded growth.
type idMap struct {
	mu   sync.Mutex
	cap  int
	m    map[string]string
	ring []string // insertion order; ring[next] is the oldest entry when full
	next int
}

func newIDMap(capacity int) *idMap {
	if capacity < 1 {
		capacity = 1
	}
	return &idMap{
		cap:  capacity,
		m:    make(map[string]string, capacity),
		ring: make([]string, capacity),
	}
}

// put records key→ulid, evicting the oldest entry when the map is full. Re-inserting an
// existing key updates its value in place without consuming a new slot.
func (im *idMap) put(key, ulid string) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if _, ok := im.m[key]; ok {
		im.m[key] = ulid
		return
	}
	if old := im.ring[im.next]; old != "" {
		delete(im.m, old)
	}
	im.ring[im.next] = key
	im.m[key] = ulid
	im.next = (im.next + 1) % im.cap
}

// get returns the ULID mapped to key, or "" if absent or evicted.
func (im *idMap) get(key string) string {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.m[key]
}
