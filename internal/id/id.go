// Package id generates ULIDs: 128-bit identifiers that sort lexicographically by creation
// time. A ULID is a 48-bit millisecond timestamp followed by 80 bits of randomness, encoded
// as 26 Crockford base32 characters. Because the time component leads and same-millisecond
// ids increment monotonically, sorting ids as strings sorts them by creation order — which
// is exactly what the message store needs for stable, cursor-based pagination.
//
// The time source and entropy source are both injected so tests are deterministic. This is
// the one package allowed to read randomness directly; everything else takes a Generator.
package id

import (
	"crypto/rand"
	"io"
	"sync"

	"github.com/elythi0n/virta/internal/clock"
)

// crockford is the base32 alphabet used by ULID (no I, L, O, U to avoid ambiguity).
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Generator produces unique, time-sortable ids.
type Generator interface {
	New() string
}

// ULID generates monotonic ULIDs. Safe for concurrent use.
type ULID struct {
	clk     clock.Clock
	entropy io.Reader

	mu      sync.Mutex
	lastMs  uint64
	lastEnt [10]byte // last 80-bit entropy, incremented within the same millisecond
}

// NewULID returns a generator using clk for timestamps and crypto/rand for entropy.
func NewULID(clk clock.Clock) *ULID {
	return &ULID{clk: clk, entropy: rand.Reader}
}

// NewULIDWithEntropy returns a generator with an explicit entropy source — used in tests to
// make ids reproducible.
func NewULIDWithEntropy(clk clock.Clock, entropy io.Reader) *ULID {
	return &ULID{clk: clk, entropy: entropy}
}

// New returns the next ULID. Within a single millisecond the entropy is incremented rather
// than re-drawn, so two ids minted in the same millisecond still compare strictly in mint
// order.
func (g *ULID) New() string {
	ms := uint64(g.clk.Now().UnixMilli())

	g.mu.Lock()
	if ms == g.lastMs {
		incr(&g.lastEnt)
	} else {
		g.lastMs = ms
		if _, err := io.ReadFull(g.entropy, g.lastEnt[:]); err != nil {
			// crypto/rand should never fail; if a test entropy source is exhausted, fall
			// back to incrementing so New never panics and ids stay monotonic.
			incr(&g.lastEnt)
		}
	}
	ent := g.lastEnt
	g.mu.Unlock()

	var b [16]byte
	// 48-bit timestamp, big-endian, in the high 6 bytes.
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	copy(b[6:], ent[:])
	return encode(b)
}

// incr adds one to an 80-bit big-endian counter, wrapping at overflow.
func incr(e *[10]byte) {
	for i := len(e) - 1; i >= 0; i-- {
		e[i]++
		if e[i] != 0 {
			return
		}
	}
}

// encode renders 16 bytes as 26 Crockford base32 characters (the canonical ULID encoding).
func encode(b [16]byte) string {
	var d [26]byte
	d[0] = crockford[(b[0]&224)>>5]
	d[1] = crockford[b[0]&31]
	d[2] = crockford[(b[1]&248)>>3]
	d[3] = crockford[((b[1]&7)<<2)|((b[2]&192)>>6)]
	d[4] = crockford[(b[2]&62)>>1]
	d[5] = crockford[((b[2]&1)<<4)|((b[3]&240)>>4)]
	d[6] = crockford[((b[3]&15)<<1)|((b[4]&128)>>7)]
	d[7] = crockford[(b[4]&124)>>2]
	d[8] = crockford[((b[4]&3)<<3)|((b[5]&224)>>5)]
	d[9] = crockford[b[5]&31]
	d[10] = crockford[(b[6]&248)>>3]
	d[11] = crockford[((b[6]&7)<<2)|((b[7]&192)>>6)]
	d[12] = crockford[(b[7]&62)>>1]
	d[13] = crockford[((b[7]&1)<<4)|((b[8]&240)>>4)]
	d[14] = crockford[((b[8]&15)<<1)|((b[9]&128)>>7)]
	d[15] = crockford[(b[9]&124)>>2]
	d[16] = crockford[((b[9]&3)<<3)|((b[10]&224)>>5)]
	d[17] = crockford[b[10]&31]
	d[18] = crockford[(b[11]&248)>>3]
	d[19] = crockford[((b[11]&7)<<2)|((b[12]&192)>>6)]
	d[20] = crockford[(b[12]&62)>>1]
	d[21] = crockford[((b[12]&1)<<4)|((b[13]&240)>>4)]
	d[22] = crockford[((b[13]&15)<<1)|((b[14]&128)>>7)]
	d[23] = crockford[(b[14]&124)>>2]
	d[24] = crockford[((b[14]&3)<<3)|((b[15]&224)>>5)]
	d[25] = crockford[b[15]&31]
	return string(d[:])
}

// Fake is a deterministic Generator for tests: it returns sequential, sortable ids without
// any clock or randomness.
type Fake struct {
	mu     sync.Mutex
	seq    uint64
	prefix string
}

// NewFake returns a deterministic generator. ids are zero-padded so they sort in mint order.
func NewFake(prefix string) *Fake { return &Fake{prefix: prefix} }

// New returns the next deterministic id, e.g. "id_00000000000000000001".
func (f *Fake) New() string {
	f.mu.Lock()
	f.seq++
	n := f.seq
	f.mu.Unlock()
	var buf [20]byte
	for i := len(buf) - 1; i >= 0; i-- {
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	p := f.prefix
	if p == "" {
		p = "id"
	}
	return p + "_" + string(buf[:])
}

var (
	_ Generator = (*ULID)(nil)
	_ Generator = (*Fake)(nil)
)
