package traffic

import (
	"math"
	"time"
)

// maxPerTick bounds how many requests a single tick may schedule, so a clock
// jump or an absurd rate cannot produce an unbounded burst.
const maxPerTick = 4096

// Scheduler converts a fractional request rate into whole requests per tick.
// It carries the remainder forward, so 0.3 rps on a 50 ms tick still yields
// exactly three requests every ten seconds instead of rounding to nothing.
//
// Scheduler is not safe for concurrent use; the engine drives it from one
// goroutine.
type Scheduler struct {
	accumulator float64
}

// Advance accumulates rate*dt and returns the number of whole requests due.
func (s *Scheduler) Advance(rate float64, dt time.Duration) int {
	if rate <= 0 || dt <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}
	s.accumulator += rate * dt.Seconds()
	if s.accumulator < 1 {
		return 0
	}
	whole := math.Floor(s.accumulator)
	n := int(math.Min(whole, maxPerTick))
	// Drop any remainder beyond the cap rather than letting it pile up.
	s.accumulator -= whole
	return n
}

// Pending returns the unspent fractional remainder. Exposed for tests.
func (s *Scheduler) Pending() float64 { return s.accumulator }

// Reset discards the accumulated remainder.
func (s *Scheduler) Reset() { s.accumulator = 0 }
