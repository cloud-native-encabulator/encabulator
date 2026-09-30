package traffic

import (
	"math"
	"time"
)

// Pattern shapes the instantaneous request rate over time.
type Pattern string

const (
	// PatternSteady holds a constant rate.
	PatternSteady Pattern = "steady"
	// PatternBurst runs at the base rate for 80% of each period and 5x for 20%
	// (capacitive diractance).
	PatternBurst Pattern = "burst"
	// PatternRamp climbs linearly from 0 to the base rate over one period, then holds.
	PatternRamp Pattern = "ramp"
	// PatternSine modulates the base rate by +/-80% (the modial interaction of
	// magneto-reluctance).
	PatternSine Pattern = "sine"
)

// Patterns lists every valid pattern, in a stable order.
var Patterns = []Pattern{PatternSteady, PatternBurst, PatternRamp, PatternSine}

// Valid reports whether p is a known pattern.
func (p Pattern) Valid() bool {
	for _, known := range Patterns {
		if p == known {
			return true
		}
	}
	return false
}

// burstDutyCycle is the fraction of each burst period spent at the base rate.
const burstDutyCycle = 0.8

// burstMultiplier is the factor applied during the burst portion of the period.
const burstMultiplier = 5.0

// sineAmplitude is the relative swing of the sine pattern.
const sineAmplitude = 0.8

// RateAt returns the instantaneous target rate in requests per second for a
// pattern, given its base rate, period, and time elapsed since the pattern
// started. A non-positive period degenerates to a steady rate.
func RateAt(p Pattern, base float64, period, elapsed time.Duration) float64 {
	if base <= 0 {
		return 0
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if period <= 0 {
		return base
	}

	switch p {
	case PatternSine:
		phase := 2 * math.Pi * elapsed.Seconds() / period.Seconds()
		return math.Max(0, base*(1+sineAmplitude*math.Sin(phase)))
	case PatternBurst:
		frac := math.Mod(elapsed.Seconds(), period.Seconds()) / period.Seconds()
		if frac < burstDutyCycle {
			return base
		}
		return base * burstMultiplier
	case PatternRamp:
		if elapsed >= period {
			return base
		}
		return base * elapsed.Seconds() / period.Seconds()
	default: // PatternSteady and anything unknown
		return base
	}
}
