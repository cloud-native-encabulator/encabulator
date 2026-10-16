package ratelimit_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/ratelimit"
)

// clock is a manual clock so the bucket maths is exact rather than flaky.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock { return &clock{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestBurstIsTwiceTheRate(t *testing.T) {
	c := newClock()
	l := ratelimit.New(func() float64 { return 10 }, 100).WithClock(c.Now)

	// A fresh bucket starts full, and full is 2x the rate.
	for i := range 20 {
		if ok, _ := l.Allow("wendy"); !ok {
			t.Fatalf("request %d was rejected inside the burst", i+1)
		}
	}
	ok, retryAfter := l.Allow("wendy")
	if ok {
		t.Fatal("the 21st request should exceed a burst of 20")
	}
	if retryAfter < time.Second {
		t.Errorf("Retry-After = %v; it must never be less than a second", retryAfter)
	}
}

func TestRefillAtTheConfiguredRate(t *testing.T) {
	c := newClock()
	l := ratelimit.New(func() float64 { return 10 }, 100).WithClock(c.Now)

	for range 20 {
		l.Allow("wendy")
	}
	if ok, _ := l.Allow("wendy"); ok {
		t.Fatal("bucket should be empty")
	}

	// 10 rps means one token per 100ms.
	c.advance(100 * time.Millisecond)
	if ok, _ := l.Allow("wendy"); !ok {
		t.Fatal("one token should have refilled after 100ms")
	}
	if ok, _ := l.Allow("wendy"); ok {
		t.Fatal("only one token should have refilled")
	}

	// A long wait refills to the burst ceiling and no further.
	c.advance(time.Hour)
	allowed := 0
	for range 100 {
		if ok, _ := l.Allow("wendy"); ok {
			allowed++
		}
	}
	if allowed != 20 {
		t.Errorf("after a long idle period %d requests were allowed, want the burst of 20", allowed)
	}
}

func TestBucketsAreIndependentPerKey(t *testing.T) {
	c := newClock()
	l := ratelimit.New(func() float64 { return 1 }, 100).WithClock(c.Now)

	for range 2 {
		if ok, _ := l.Allow("wendy"); !ok {
			t.Fatal("wendy should be inside her burst")
		}
	}
	if ok, _ := l.Allow("wendy"); ok {
		t.Fatal("wendy should be limited")
	}
	// otto is untouched.
	if ok, _ := l.Allow("otto"); !ok {
		t.Fatal("one subject's limit leaked into another's")
	}
}

func TestRateOfZeroClosesTheGate(t *testing.T) {
	c := newClock()
	l := ratelimit.New(func() float64 { return 0 }, 100).WithClock(c.Now)

	ok, retryAfter := l.Allow("wendy")
	if ok {
		t.Error("a rate of zero must reject everything")
	}
	if retryAfter <= 0 {
		t.Errorf("Retry-After = %v", retryAfter)
	}
}

func TestRateChangeResetsTheBucket(t *testing.T) {
	c := newClock()
	var rate atomic.Value
	rate.Store(1.0)
	l := ratelimit.New(func() float64 { return rate.Load().(float64) }, 100).WithClock(c.Now)

	for range 2 {
		l.Allow("wendy")
	}
	if ok, _ := l.Allow("wendy"); ok {
		t.Fatal("bucket should be empty at 1 rps")
	}

	// An operator raises the limit mid-demo; the effect must be immediate.
	rate.Store(50.0)
	allowed := 0
	for range 100 {
		if ok, _ := l.Allow("wendy"); ok {
			allowed++
		}
	}
	if allowed != 100 {
		t.Errorf("after raising the rate %d of 100 requests were allowed", allowed)
	}
}

func TestLRUIsBounded(t *testing.T) {
	c := newClock()
	const capacity = 50
	l := ratelimit.New(func() float64 { return 1 }, capacity).WithClock(c.Now)

	for i := range capacity * 10 {
		l.Allow(fmt.Sprintf("client-%d", i))
	}
	if got := l.Len(); got != capacity {
		t.Errorf("bucket count = %d, want the capacity %d", got, capacity)
	}
}

func TestLRUEvictsTheLeastRecentlyUsed(t *testing.T) {
	c := newClock()
	l := ratelimit.New(func() float64 { return 1 }, 2).WithClock(c.Now)

	// Fill "a" and "b", then keep "a" warm while "c" arrives; "b" should go.
	l.Allow("a")
	l.Allow("a")
	l.Allow("b")
	l.Allow("b")
	l.Allow("a") // touches a, now over its burst but most recently used
	l.Allow("c")

	if got := l.Len(); got != 2 {
		t.Fatalf("bucket count = %d", got)
	}
	// "b" was evicted, so it gets a fresh full bucket.
	if ok, _ := l.Allow("b"); !ok {
		t.Error("the evicted key did not get a fresh bucket")
	}
}

func TestConcurrentUseIsSafe(t *testing.T) {
	l := ratelimit.New(func() float64 { return 1000 }, 1000)

	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 200 {
				key := fmt.Sprintf("client-%d", (i+j)%100)
				if ok, _ := l.Allow(key); ok {
					allowed.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()

	if allowed.Load() == 0 {
		t.Error("nothing was allowed")
	}
	if got := l.Len(); got > 1000 {
		t.Errorf("bucket count = %d, over capacity", got)
	}
}

func TestDefaultCapacity(t *testing.T) {
	l := ratelimit.New(func() float64 { return 1 }, 0)
	for i := range 100 {
		l.Allow(fmt.Sprintf("client-%d", i))
	}
	if l.Len() != 100 {
		t.Errorf("bucket count = %d; the default capacity should be %d",
			l.Len(), ratelimit.DefaultCapacity)
	}
}
