// Package rng provides the small slice of randomness the grammeter needs,
// behind an interface so tests can pin the sequence.
package rng

import (
	mathrand "math/rand/v2"
	"sync"
)

// Source is the subset of a random number generator used by the grammeter.
// Implementations must be safe for concurrent use.
type Source interface {
	// Float64 returns a pseudo-random number in [0.0, 1.0).
	Float64() float64
	// IntN returns a pseudo-random int in [0, n). It panics if n <= 0.
	IntN(n int) int
}

type globalSource struct{}

func (globalSource) Float64() float64 { return mathrand.Float64() }
func (globalSource) IntN(n int) int   { return mathrand.IntN(n) }

// Global returns the process-wide source. math/rand/v2's top-level functions
// are safe for concurrent use, so no locking is needed.
func Global() Source { return globalSource{} }

type seededSource struct {
	mu sync.Mutex
	r  *mathrand.Rand
}

func (s *seededSource) Float64() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Float64()
}

func (s *seededSource) IntN(n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.IntN(n)
}

// Seeded returns a deterministic source for tests and reproducible runs.
func Seeded(seed uint64) Source {
	return &seededSource{r: mathrand.New(mathrand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}
