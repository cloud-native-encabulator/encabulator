// Package ratelimit is the gateway's per-subject token bucket, held in a bounded
// LRU so that a flood of distinct keys cannot grow without limit (design 8.1).
package ratelimit

import (
	"container/list"
	"math"
	"sync"
	"time"
)

// DefaultCapacity is the number of buckets the LRU holds, per design 8.1.
const DefaultCapacity = 10_000

// BurstFactor is the multiple of the rate a bucket may hold.
const BurstFactor = 2

// Limiter is a set of token buckets keyed by subject or client address. The rate
// is read from a function on every call, because it comes from a feature flag
// that an operator can change at any moment.
type Limiter struct {
	rate     func() float64
	capacity int
	now      func() time.Time

	mu    sync.Mutex
	order *list.List               // front is most recently used
	index map[string]*list.Element // key -> element holding *bucket
}

type bucket struct {
	key    string
	tokens float64
	last   time.Time
	// rate is the rate the bucket was last refilled at; a flag change resets the
	// bucket rather than letting a stale fill rate leak into the new limit.
	rate float64
}

// New builds a Limiter. rate returns the current requests-per-second limit;
// capacity of 0 uses DefaultCapacity.
func New(rate func() float64, capacity int) *Limiter {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Limiter{
		rate:     rate,
		capacity: capacity,
		now:      time.Now,
		order:    list.New(),
		index:    make(map[string]*list.Element, capacity),
	}
}

// WithClock replaces the clock. Tests use it; nothing else should.
func (l *Limiter) WithClock(now func() time.Time) *Limiter {
	l.now = now
	return l
}

// Allow reports whether the key may make a request now. When it may not, the
// second return value is how long the caller should wait, for Retry-After.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	rate := l.rate()
	// A rate of zero is a closed gate: the flag is the demo's congestion lever, so
	// zero means zero, not unlimited.
	if rate <= 0 {
		return false, time.Second
	}
	burst := rate * BurstFactor
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.touch(key, now, rate, burst)
	if b.rate != rate {
		// The operator moved the limit; restart this bucket at the new rate.
		b.rate = rate
		b.tokens = burst
		b.last = now
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(burst, b.tokens+elapsed*rate)
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens -= 1
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / rate * float64(time.Second))
	if wait < time.Second {
		// Retry-After is whole seconds on the wire; never advertise zero.
		wait = time.Second
	}
	return false, wait
}

// touch returns the key's bucket, creating it and evicting the least recently
// used key when the LRU is full.
func (l *Limiter) touch(key string, now time.Time, rate, burst float64) *bucket {
	if el, ok := l.index[key]; ok {
		l.order.MoveToFront(el)
		return el.Value.(*bucket)
	}
	if l.order.Len() >= l.capacity {
		if oldest := l.order.Back(); oldest != nil {
			l.order.Remove(oldest)
			delete(l.index, oldest.Value.(*bucket).key)
		}
	}
	b := &bucket{key: key, tokens: burst, last: now, rate: rate}
	l.index[key] = l.order.PushFront(b)
	return b
}

// Len reports how many buckets are held, for tests and for a gauge.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}
