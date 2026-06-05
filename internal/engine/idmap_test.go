package engine

import "testing"

func TestIDMap_PutGet(t *testing.T) {
	m := newIDMap(4)
	m.put("a", "ulid-a")
	if got := m.get("a"); got != "ulid-a" {
		t.Errorf("get(a) = %q, want ulid-a", got)
	}
	if got := m.get("missing"); got != "" {
		t.Errorf("get(missing) = %q, want empty", got)
	}
}

func TestIDMap_EvictsOldestWhenFull(t *testing.T) {
	m := newIDMap(2)
	m.put("a", "1")
	m.put("b", "2")
	m.put("c", "3") // evicts "a", the oldest
	if got := m.get("a"); got != "" {
		t.Errorf("get(a) = %q, want evicted (empty)", got)
	}
	if m.get("b") != "2" || m.get("c") != "3" {
		t.Errorf("b/c should survive: b=%q c=%q", m.get("b"), m.get("c"))
	}
}

func TestIDMap_UpdateInPlaceDoesNotEvict(t *testing.T) {
	m := newIDMap(2)
	m.put("a", "1")
	m.put("b", "2")
	m.put("a", "1b") // update existing key — must not consume a slot or evict "b"
	if m.get("a") != "1b" {
		t.Errorf("get(a) = %q, want updated 1b", m.get("a"))
	}
	if m.get("b") != "2" {
		t.Errorf("get(b) = %q, want b to survive an in-place update", m.get("b"))
	}
}

func TestIDMap_CapacityFloor(t *testing.T) {
	m := newIDMap(0) // clamped to 1
	m.put("a", "1")
	m.put("b", "2") // evicts "a"
	if m.get("a") != "" || m.get("b") != "2" {
		t.Errorf("capacity-1 map mishandled eviction: a=%q b=%q", m.get("a"), m.get("b"))
	}
}
