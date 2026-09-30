package traffic

import (
	"math"
	"testing"
	"time"
)

const tick = 50 * time.Millisecond

func TestSchedulerIgnoresNonsense(t *testing.T) {
	var s Scheduler
	for _, rate := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := s.Advance(rate, tick); got != 0 {
			t.Fatalf("Advance(%v) = %d, want 0", rate, got)
		}
	}
	if got := s.Advance(100, 0); got != 0 {
		t.Fatalf("Advance with zero dt = %d, want 0", got)
	}
	if s.Pending() != 0 {
		t.Fatalf("Pending() = %v, want 0", s.Pending())
	}
}

// A rate far below one request per tick must still come out exactly right over
// time: that is the whole point of the fractional accumulator.
func TestSchedulerFractionalRateIsExactOverTime(t *testing.T) {
	cases := []struct {
		rate    float64
		seconds int
		want    int
	}{
		{rate: 0.1, seconds: 60, want: 6},
		{rate: 0.3, seconds: 10, want: 3},
		{rate: 1, seconds: 10, want: 10},
		{rate: 7.5, seconds: 4, want: 30},
		{rate: 19, seconds: 3, want: 57},
		{rate: 500, seconds: 2, want: 1000},
	}
	for _, c := range cases {
		var s Scheduler
		total := 0
		ticks := c.seconds * int(time.Second/tick)
		for range ticks {
			total += s.Advance(c.rate, tick)
		}
		// The accumulator may hold a fraction of one request at the end.
		if total != c.want && total != c.want-1 {
			t.Errorf("rate %v over %ds produced %d, want %d", c.rate, c.seconds, total, c.want)
		}
		if s.Pending() < 0 || s.Pending() >= 1 {
			t.Errorf("rate %v left Pending() = %v, want [0,1)", c.rate, s.Pending())
		}
	}
}

func TestSchedulerCarriesRemainderAcrossRateChanges(t *testing.T) {
	var s Scheduler
	// 0.6 of a request accumulated at 12 rps for one tick.
	if got := s.Advance(12, tick); got != 0 {
		t.Fatalf("first tick = %d, want 0", got)
	}
	if math.Abs(s.Pending()-0.6) > 1e-9 {
		t.Fatalf("Pending() = %v, want 0.6", s.Pending())
	}
	// Raise the rate: the carried 0.6 must count toward the next request.
	if got := s.Advance(8, tick); got != 1 {
		t.Fatalf("second tick = %d, want 1", got)
	}
	if math.Abs(s.Pending()) > 1e-9 {
		t.Fatalf("Pending() = %v, want 0", s.Pending())
	}
}

func TestSchedulerLongTickProducesMany(t *testing.T) {
	var s Scheduler
	if got := s.Advance(10, time.Second); got != 10 {
		t.Fatalf("one second at 10 rps = %d, want 10", got)
	}
}

func TestSchedulerCapsRunawayTicks(t *testing.T) {
	var s Scheduler
	// A clock jump of an hour at 500 rps would ask for 1.8M requests.
	got := s.Advance(500, time.Hour)
	if got != maxPerTick {
		t.Fatalf("Advance = %d, want the cap %d", got, maxPerTick)
	}
	if s.Pending() != 0 {
		t.Fatalf("Pending() = %v, want the excess discarded", s.Pending())
	}
}

func TestSchedulerReset(t *testing.T) {
	var s Scheduler
	s.Advance(12, tick)
	s.Reset()
	if s.Pending() != 0 {
		t.Fatalf("Pending() after Reset = %v, want 0", s.Pending())
	}
}
