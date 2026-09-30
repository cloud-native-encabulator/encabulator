package traffic

import (
	"math"
	"testing"
	"time"
)

func TestPatternValid(t *testing.T) {
	for _, p := range Patterns {
		if !p.Valid() {
			t.Errorf("%q should be valid", p)
		}
	}
	for _, p := range []Pattern{"", "chaos", "Steady"} {
		if p.Valid() {
			t.Errorf("%q should not be valid", p)
		}
	}
}

func TestRateAtSteady(t *testing.T) {
	for _, elapsed := range []time.Duration{0, time.Second, 90 * time.Second} {
		if got := RateAt(PatternSteady, 10, time.Minute, elapsed); got != 10 {
			t.Errorf("steady at %v = %v, want 10", elapsed, got)
		}
	}
}

func TestRateAtZeroBaseAndZeroPeriod(t *testing.T) {
	if got := RateAt(PatternSine, 0, time.Minute, 0); got != 0 {
		t.Errorf("zero base = %v, want 0", got)
	}
	if got := RateAt(PatternSine, 10, 0, time.Second); got != 10 {
		t.Errorf("zero period should degenerate to steady, got %v", got)
	}
	if got := RateAt(PatternRamp, 10, time.Minute, -time.Second); got != 0 {
		t.Errorf("negative elapsed = %v, want 0", got)
	}
}

func TestRateAtSine(t *testing.T) {
	const base, period = 10.0, 60 * time.Second
	cases := []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 10},                // sin 0
		{15 * time.Second, 18}, // sin pi/2  -> 1 + 0.8
		{30 * time.Second, 10}, // sin pi
		{45 * time.Second, 2},  // sin 3pi/2 -> 1 - 0.8
		{60 * time.Second, 10}, // full cycle
		{75 * time.Second, 18}, // repeats
	}
	for _, c := range cases {
		got := RateAt(PatternSine, base, period, c.elapsed)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("sine at %v = %v, want %v", c.elapsed, got, c.want)
		}
	}
}

// The sine must average out to the base rate over a whole period, or the
// operator's "rps" knob would lie.
func TestRateAtSineAveragesToBase(t *testing.T) {
	const base = 12.0
	const period = 20 * time.Second
	const steps = 20000
	sum := 0.0
	for i := range steps {
		elapsed := time.Duration(float64(i) / steps * float64(period))
		sum += RateAt(PatternSine, base, period, elapsed)
	}
	if avg := sum / steps; math.Abs(avg-base) > 0.05 {
		t.Errorf("sine average = %v, want ~%v", avg, base)
	}
}

func TestRateAtBurst(t *testing.T) {
	const base, period = 10.0, 100 * time.Second
	cases := []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 10},
		{79 * time.Second, 10},
		{80 * time.Second, 50},
		{99 * time.Second, 50},
		{100 * time.Second, 10}, // next period
		{180 * time.Second, 50},
	}
	for _, c := range cases {
		if got := RateAt(PatternBurst, base, period, c.elapsed); got != c.want {
			t.Errorf("burst at %v = %v, want %v", c.elapsed, got, c.want)
		}
	}
}

func TestRateAtBurstDeliversExpectedVolume(t *testing.T) {
	// 80% of the period at 1x plus 20% at 5x is 1.8x the base volume.
	const base = 10.0
	const period = 10 * time.Second
	const steps = 10000
	sum := 0.0
	for i := range steps {
		sum += RateAt(PatternBurst, base, period, time.Duration(float64(i)/steps*float64(period)))
	}
	if avg := sum / steps; math.Abs(avg-1.8*base) > 0.05 {
		t.Errorf("burst average = %v, want ~%v", avg, 1.8*base)
	}
}

func TestRateAtRamp(t *testing.T) {
	const base, period = 20.0, 40 * time.Second
	cases := []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 0},
		{10 * time.Second, 5},
		{20 * time.Second, 10},
		{39 * time.Second, 19.5},
		{40 * time.Second, 20}, // holds
		{400 * time.Second, 20},
	}
	for _, c := range cases {
		got := RateAt(PatternRamp, base, period, c.elapsed)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("ramp at %v = %v, want %v", c.elapsed, got, c.want)
		}
	}
}

func TestRateAtNeverNegative(t *testing.T) {
	for _, p := range Patterns {
		for i := range 1000 {
			elapsed := time.Duration(i) * 137 * time.Millisecond
			if got := RateAt(p, 3, 7*time.Second, elapsed); got < 0 {
				t.Fatalf("%s at %v = %v, want >= 0", p, elapsed, got)
			}
		}
	}
}
