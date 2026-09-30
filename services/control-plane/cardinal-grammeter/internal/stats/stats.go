// Package stats keeps a rolling window of journey outcomes so the console can
// show what the grammeter is actually achieving, not what it was asked to do.
package stats

import (
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Window is the trailing period over which journey statistics are reported.
const Window = 60 * time.Second

// RecentWindow is the shorter period used for the actual-RPS figure, so the
// number tracks bursts and ramps instead of averaging them away.
const RecentWindow = 10 * time.Second

// defaultCapacity holds a minute of traffic at the maximum allowed rate
// (500 rps) with room to spare for multi-call journeys.
const defaultCapacity = 1 << 16

// JourneyStats is the per-journey summary exposed by GET /v1/traffic.
type JourneyStats struct {
	Count  int     `json:"count"`
	Errors int     `json:"errors"`
	P50Ms  float64 `json:"p50Ms"`
	P99Ms  float64 `json:"p99Ms"`
}

// Snapshot is a point-in-time view of the rolling window.
type Snapshot struct {
	Journeys  map[string]JourneyStats `json:"journeys"`
	Statuses  map[string]int          `json:"statuses"`
	ActualRps float64                 `json:"actualRps"`
}

type journeySample struct {
	name     string
	duration time.Duration
	failed   bool
}

type entry[T any] struct {
	at time.Time
	v  T
}

// ring is a fixed-capacity FIFO of time-stamped values. Once full it overwrites
// the oldest entry, which bounds memory without any background pruning.
type ring[T any] struct {
	entries []entry[T]
	next    int
	size    int
}

func newRing[T any](capacity int) *ring[T] {
	return &ring[T]{entries: make([]entry[T], capacity)}
}

func (r *ring[T]) add(at time.Time, v T) {
	r.entries[r.next] = entry[T]{at: at, v: v}
	r.next = (r.next + 1) % len(r.entries)
	if r.size < len(r.entries) {
		r.size++
	}
}

// each visits entries no older than since, oldest first.
func (r *ring[T]) each(since time.Time, fn func(at time.Time, v T)) {
	start := (r.next - r.size + len(r.entries)) % len(r.entries)
	for i := 0; i < r.size; i++ {
		e := r.entries[(start+i)%len(r.entries)]
		if e.at.Before(since) {
			continue
		}
		fn(e.at, e.v)
	}
}

// Recorder accumulates journey and per-request outcomes.
type Recorder struct {
	mu       sync.Mutex
	journeys *ring[journeySample]
	statuses *ring[string]
	now      func() time.Time
}

// NewRecorder returns a recorder that reads the clock through now.
func NewRecorder(now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	return &Recorder{
		journeys: newRing[journeySample](defaultCapacity),
		statuses: newRing[string](defaultCapacity),
		now:      now,
	}
}

// RecordJourney notes one completed journey and how long it took end to end.
func (r *Recorder) RecordJourney(name string, duration time.Duration, failed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.journeys.add(r.now(), journeySample{name: name, duration: duration, failed: failed})
}

// RecordRequest notes the HTTP status of one call. A status of 0 means the
// request never got an answer and is reported as "error".
func (r *Recorder) RecordRequest(status int) {
	label := StatusLabel(status)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses.add(r.now(), label)
}

// StatusLabel renders an HTTP status for use as a metric label or map key.
func StatusLabel(status int) string {
	if status <= 0 {
		return "error"
	}
	return strconv.Itoa(status)
}

// Snapshot summarises the trailing window.
func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	cutoff := now.Add(-Window)
	recentCutoff := now.Add(-RecentWindow)

	durations := map[string][]float64{}
	out := Snapshot{
		Journeys: map[string]JourneyStats{},
		Statuses: map[string]int{},
	}
	recent := 0

	r.journeys.each(cutoff, func(at time.Time, s journeySample) {
		js := out.Journeys[s.name]
		js.Count++
		if s.failed {
			js.Errors++
		}
		out.Journeys[s.name] = js
		durations[s.name] = append(durations[s.name], float64(s.duration)/float64(time.Millisecond))
		if !at.Before(recentCutoff) {
			recent++
		}
	})

	for name, ds := range durations {
		sort.Float64s(ds)
		js := out.Journeys[name]
		js.P50Ms = percentile(ds, 0.50)
		js.P99Ms = percentile(ds, 0.99)
		out.Journeys[name] = js
	}

	r.statuses.each(cutoff, func(_ time.Time, label string) {
		out.Statuses[label]++
	})

	out.ActualRps = float64(recent) / RecentWindow.Seconds()
	return out
}

// percentile returns the nearest-rank percentile of a sorted slice.
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted)) * q))
	if idx < 1 {
		idx = 1
	}
	if idx > len(sorted) {
		idx = len(sorted)
	}
	return round2(sorted[idx-1])
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
