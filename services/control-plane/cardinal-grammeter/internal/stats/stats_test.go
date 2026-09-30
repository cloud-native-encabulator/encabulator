package stats

import (
	"math"
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestStatusLabel(t *testing.T) {
	cases := map[int]string{0: "error", -1: "error", 200: "200", 503: "503"}
	for status, want := range cases {
		if got := StatusLabel(status); got != want {
			t.Errorf("StatusLabel(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestRecorderCountsAndPercentiles(t *testing.T) {
	clock := newFakeClock()
	r := NewRecorder(clock.Now)

	// 100 journeys with durations 1ms..100ms; every tenth one failed.
	for i := 1; i <= 100; i++ {
		r.RecordJourney("browse", time.Duration(i)*time.Millisecond, i%10 == 0)
	}

	snap := r.Snapshot()
	js := snap.Journeys["browse"]
	if js.Count != 100 {
		t.Errorf("count = %d, want 100", js.Count)
	}
	if js.Errors != 10 {
		t.Errorf("errors = %d, want 10", js.Errors)
	}
	// Nearest-rank percentiles over 1..100 ms.
	if js.P50Ms != 50 {
		t.Errorf("p50 = %v, want 50", js.P50Ms)
	}
	if js.P99Ms != 99 {
		t.Errorf("p99 = %v, want 99", js.P99Ms)
	}
}

func TestRecorderWindowExpiry(t *testing.T) {
	clock := newFakeClock()
	r := NewRecorder(clock.Now)

	r.RecordJourney("order", 10*time.Millisecond, false)
	r.RecordRequest(202)

	clock.advance(Window + time.Second)
	r.RecordJourney("order", 20*time.Millisecond, false)
	r.RecordRequest(503)

	snap := r.Snapshot()
	if got := snap.Journeys["order"].Count; got != 1 {
		t.Errorf("count = %d, want 1 after the old sample aged out", got)
	}
	if snap.Journeys["order"].P50Ms != 20 {
		t.Errorf("p50 = %v, want 20", snap.Journeys["order"].P50Ms)
	}
	if snap.Statuses["202"] != 0 {
		t.Errorf("statuses = %v, want the 202 aged out", snap.Statuses)
	}
	if snap.Statuses["503"] != 1 {
		t.Errorf("statuses = %v, want one 503", snap.Statuses)
	}
}

func TestRecorderActualRPSUsesTheRecentWindow(t *testing.T) {
	clock := newFakeClock()
	r := NewRecorder(clock.Now)

	// 50 journeys spread evenly over the recent window: 5 per second.
	for range 50 {
		r.RecordJourney("quote", time.Millisecond, false)
		clock.advance(RecentWindow / 50)
	}
	snap := r.Snapshot()
	if math.Abs(snap.ActualRps-5) > 0.2 {
		t.Errorf("actual rps = %v, want ~5", snap.ActualRps)
	}

	// Ten seconds of silence drains it, while the 60s journey counts stay.
	clock.advance(RecentWindow + time.Second)
	snap = r.Snapshot()
	if snap.ActualRps != 0 {
		t.Errorf("actual rps = %v, want 0 after a quiet window", snap.ActualRps)
	}
	if snap.Journeys["quote"].Count != 50 {
		t.Errorf("count = %d, want 50 still inside the 60s window", snap.Journeys["quote"].Count)
	}
}

func TestRecorderEmptySnapshot(t *testing.T) {
	r := NewRecorder(newFakeClock().Now)
	snap := r.Snapshot()
	if len(snap.Journeys) != 0 || len(snap.Statuses) != 0 || snap.ActualRps != 0 {
		t.Errorf("empty recorder produced %+v", snap)
	}
}

func TestRecorderSeparatesJourneys(t *testing.T) {
	r := NewRecorder(newFakeClock().Now)
	r.RecordJourney("browse", 5*time.Millisecond, false)
	r.RecordJourney("order", 500*time.Millisecond, true)

	snap := r.Snapshot()
	if snap.Journeys["browse"].P99Ms != 5 {
		t.Errorf("browse p99 = %v, want 5", snap.Journeys["browse"].P99Ms)
	}
	if snap.Journeys["order"].Errors != 1 {
		t.Errorf("order errors = %d, want 1", snap.Journeys["order"].Errors)
	}
	if snap.Journeys["browse"].Errors != 0 {
		t.Errorf("browse picked up order's error")
	}
}

func TestRecorderIsConcurrencySafe(t *testing.T) {
	r := NewRecorder(time.Now)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 200 {
				r.RecordJourney("browse", time.Duration(j)*time.Millisecond, j%7 == 0)
				r.RecordRequest(200 + i)
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			_ = r.Snapshot()
		}
	}()
	wg.Wait()

	if got := r.Snapshot().Journeys["browse"].Count; got != 16*200 {
		t.Errorf("count = %d, want %d", got, 16*200)
	}
}

func TestPercentileEdges(t *testing.T) {
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("percentile of nothing = %v, want 0", got)
	}
	if got := percentile([]float64{7}, 0.99); got != 7 {
		t.Errorf("percentile of one sample = %v, want 7", got)
	}
	if got := percentile([]float64{1, 2}, 0); got != 1 {
		t.Errorf("zeroth percentile = %v, want 1", got)
	}
	if got := percentile([]float64{1.238, 2}, 0.5); got != 1.24 {
		t.Errorf("percentile rounding = %v, want 1.24", got)
	}
}
