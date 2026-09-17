package engine

// Fixed is a fixed-point number with scale 1000 (permille).
//
// All simulation arithmetic goes through Fixed — never floats — so that
// replays are bit-for-bit reproducible (docs/world_genesis.md §7).
type Fixed int64

const (
	// One is 1.0 in permille scale.
	One Fixed = 1000
	// Scale is the fixed-point denominator.
	Scale Fixed = 1000
)

// Fmul returns a*b in fixed-point.
func Fmul(a, b Fixed) Fixed { return a * b / Scale }

// Fdiv returns a/b in fixed-point. Caller must guarantee b != 0.
func Fdiv(a, b Fixed) Fixed { return a * Scale / b }

// Fclamp clamps x into [lo, hi].
func Fclamp(x, lo, hi Fixed) Fixed {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// Rng is a deterministic xorshift64 PRNG driven by the scenario seed
// (world_genesis §7: no unversioned randomness inside a match).
type Rng struct{ s uint64 }

// NewRng creates a PRNG from a seed.
func NewRng(seed uint64) *Rng { return &Rng{s: seed} }

// Next returns the next raw uint64.
func (r *Rng) Next() uint64 {
	r.s ^= r.s << 13
	r.s ^= r.s >> 7
	r.s ^= r.s << 17
	return r.s
}

// Intn returns a value in [0, n). n must be > 0.
func (r *Rng) Intn(n int) int { return int(r.Next() % uint64(n)) }
