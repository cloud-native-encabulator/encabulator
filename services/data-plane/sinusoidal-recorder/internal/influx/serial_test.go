package influx_test

import (
	"testing"
	"time"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/influx"
)

func TestSerialTimeReadsTheCasingLineTime(t *testing.T) {
	now := time.Date(2026, 10, 14, 3, 30, 0, 0, time.UTC)
	// MV7JP6S9 is 1791948213657 in base 36: 2026-10-14T03:23:33.657Z.
	want := time.UnixMilli(1791948213657).UTC()
	for _, serial := range []string{
		"ENC-retro-MV7JP6S9-EE22",
		"ENC-retro-mv7jp6s9-ee22",     // either case
		"ENC-turbo-x-MV7JP6S9-0000",   // a model with a dash in it
		"ENC-hyper-MV7JP6S9-FFFF",     // the largest suffix
		"ENC-hyper-0000MV7JP6S9-1A2B", // leading zeros
	} {
		got, ok := influx.SerialTime(serial, now)
		if !ok || !got.Equal(want) {
			t.Errorf("SerialTime(%q) = %v, %v; want %v", serial, got, ok, want)
		}
	}
}

func TestSerialTimeRefusesOtherForms(t *testing.T) {
	now := time.Date(2026, 10, 14, 3, 30, 0, 0, time.UTC)
	future := now.Add(2 * time.Hour).UnixMilli()
	for _, serial := range []string{
		"",
		"ENC-turbo-a",                           // no time part
		"ENC-limit",                             // not the format at all
		"unit-1-2-3",                            // no ENC prefix
		"ENC-turbo-MV7JP6S9-XYZ1",               // suffix not hex
		"ENC-turbo-MV7JP6S9-EE2",                // suffix too short
		"ENC-turbo-MV7JP6S9-EE222",              // suffix too long
		"ENC-turbo--EE22",                       // empty time
		"ENC-turbo-MV7J!6S9-EE22",               // not base 36
		"ENC-turbo-1-0000",                      // 1 ms after 1970: not a casing-line time
		"ENC-turbo-ZZZZZZZZZZZZZ-0000",          // too long for a millisecond time
		"ENC-turbo-" + base36(future) + "-0000", // more than an hour ahead of now
		`ENC-safe' OR '1'='1`,
	} {
		if got, ok := influx.SerialTime(serial, now); ok {
			t.Errorf("SerialTime(%q) = %v, want no time", serial, got)
		}
	}
}

func TestReadingsSinceIsTheSerialTimeMinusTheSlack(t *testing.T) {
	now := time.Date(2026, 10, 14, 3, 30, 0, 0, time.UTC)
	minted := time.UnixMilli(1791948213657).UTC()
	if got, want := influx.ReadingsSince("ENC-retro-MV7JP6S9-EE22", now), minted.Add(-influx.SerialClockSlack); !got.Equal(want) {
		t.Errorf("casing-line serial: since = %v, want %v", got, want)
	}
	// A unit minted long ago is still read from its own time, however old.
	old := now.Add(-30 * 24 * time.Hour)
	if got, want := influx.ReadingsSince("ENC-retro-"+base36(old.UnixMilli())+"-0001", now), old.Add(-influx.SerialClockSlack); !got.Equal(want) {
		t.Errorf("old serial: since = %v, want %v", got, want)
	}
	// Anything else reads the fallback window.
	if got, want := influx.ReadingsSince("ENC-turbo-a", now), now.Add(-influx.FallbackLookback); !got.Equal(want) {
		t.Errorf("other serial: since = %v, want %v", got, want)
	}
}

func base36(v int64) string {
	const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if v == 0 {
		return "0"
	}
	out := ""
	for v > 0 {
		out = string(digits[v%36]) + out
		v /= 36
	}
	return out
}
