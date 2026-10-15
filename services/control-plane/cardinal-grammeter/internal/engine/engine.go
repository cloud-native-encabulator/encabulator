// Package engine drives the traffic schedule: it decides how many journeys are
// due on each tick, launches them, and refuses to queue work it cannot run.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/journey"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/rng"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/stats"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/traffic"
)

// Tick is the scheduler period. The fractional accumulator makes rates far
// below one request per tick come out exactly right over time.
const Tick = 50 * time.Millisecond

// Rate bounds from the design.
const (
	MinRPS = 0.1
	MaxRPS = 500.0
)

// Period bounds for the shaped patterns.
const (
	MinPeriodSeconds = 1
	MaxPeriodSeconds = 3600
)

// Config is the operator-visible traffic configuration.
type Config struct {
	Running       bool            `json:"running"`
	RPS           float64         `json:"rps"`
	Pattern       traffic.Pattern `json:"pattern"`
	PeriodSeconds int             `json:"periodSeconds"`
	Mix           map[string]int  `json:"mix"`
	Customers     int             `json:"customers"`
}

// clone deep-copies the mix so callers cannot mutate engine state.
func (c Config) clone() Config {
	c.Mix = maps.Clone(c.Mix)
	return c
}

// Patch is the body of PUT /v1/traffic. Every field is optional; a nil field
// leaves the current value alone.
type Patch struct {
	Running       *bool            `json:"running"`
	RPS           *float64         `json:"rps"`
	Pattern       *traffic.Pattern `json:"pattern"`
	PeriodSeconds *int             `json:"periodSeconds"`
	Mix           map[string]int   `json:"mix"`
	Customers     *int             `json:"customers"`
}

// With returns c with the patch's fields applied. It does not validate.
func (c Config) With(p Patch) Config {
	next := c.clone()
	if p.Running != nil {
		next.Running = *p.Running
	}
	if p.RPS != nil {
		next.RPS = *p.RPS
	}
	if p.Pattern != nil {
		next.Pattern = *p.Pattern
	}
	if p.PeriodSeconds != nil {
		next.PeriodSeconds = *p.PeriodSeconds
	}
	if p.Mix != nil {
		next.Mix = maps.Clone(p.Mix)
	}
	if p.Customers != nil {
		next.Customers = *p.Customers
	}
	return next
}

// Stats is the live half of GET /v1/traffic.
type Stats struct {
	TargetRPS float64                       `json:"targetRps"`
	ActualRPS float64                       `json:"actualRps"`
	InFlight  int64                         `json:"inFlight"`
	Started   int64                         `json:"started"`
	Dropped   int64                         `json:"dropped"`
	Journeys  map[string]stats.JourneyStats `json:"journeys"`
	Statuses  map[string]int                `json:"statuses"`
}

// Status is the whole of GET /v1/traffic.
type Status struct {
	Config
	Target      string `json:"target"`
	MaxInFlight int    `json:"maxInFlight"`
	TickMs      int64  `json:"tickMs"`
	Stats       Stats  `json:"stats"`
}

// Runner executes one named journey. journey.Runner satisfies it.
type Runner interface {
	Run(ctx context.Context, name string)
}

// Pool is the customer pool, narrowed to what the engine changes.
type Pool interface {
	Resize(int)
	Size() int
}

// Gauge is the target-rate gauge the engine keeps current.
type Gauge interface {
	Set(float64)
}

// Counter counts dropped journeys.
type Counter interface {
	Inc()
}

// Options configures an Engine.
type Options struct {
	Runner      Runner
	Pool        Pool
	Stats       *stats.Recorder
	Source      rng.Source
	Logger      *slog.Logger
	Target      string
	MaxInFlight int
	Initial     Config
	// Now may be nil, meaning time.Now. Tests supply a deterministic clock.
	Now func() time.Time
	// TargetRPSGauge may be nil.
	TargetRPSGauge Gauge
	// DroppedCounter may be nil.
	DroppedCounter Counter
	// OnChange may be nil. It is called with every configuration Update accepts,
	// in the order they were accepted, while the engine still holds its lock: it
	// must return at once and must not call back into the Engine.
	OnChange func(Config)
}

// Engine owns the schedule. Tick is called from exactly one goroutine; the
// configuration may be changed from any.
type Engine struct {
	runner      Runner
	pool        Pool
	rec         *stats.Recorder
	source      rng.Source
	log         *slog.Logger
	target      string
	maxInFlight int
	now         func() time.Time
	gauge       Gauge
	dropCounter Counter
	onChange    func(Config)

	// draining stops new journeys for good once the process is shutting down.
	draining atomic.Bool

	mu         sync.RWMutex
	cfg        Config
	picker     *journey.Picker
	phaseStart time.Time
	lastTick   time.Time

	sched traffic.Scheduler // owned by the ticking goroutine

	inFlight  atomic.Int64
	started   atomic.Int64
	dropped   atomic.Int64
	targetRPS atomic.Uint64 // math.Float64bits
}

// New builds an Engine with the supplied initial configuration, which must be
// valid.
func New(o Options) (*Engine, error) {
	if o.Runner == nil {
		return nil, errors.New("engine: runner is required")
	}
	if o.Source == nil {
		o.Source = rng.Global()
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MaxInFlight < 1 {
		o.MaxInFlight = 256
	}
	if o.Stats == nil {
		o.Stats = stats.NewRecorder(o.Now)
	}

	cfg := o.Initial
	if cfg.Mix == nil {
		cfg.Mix = journey.DefaultMix()
	}
	if cfg.Pattern == "" {
		cfg.Pattern = traffic.PatternSteady
	}
	if cfg.PeriodSeconds == 0 {
		cfg.PeriodSeconds = 60
	}
	if cfg.Customers == 0 && o.Pool != nil {
		cfg.Customers = o.Pool.Size()
	}
	if err := validate(cfg); err != nil {
		return nil, err
	}

	picker, err := journey.NewPicker(cfg.Mix, o.Source)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		runner:      o.Runner,
		pool:        o.Pool,
		rec:         o.Stats,
		source:      o.Source,
		log:         o.Logger,
		target:      o.Target,
		maxInFlight: o.MaxInFlight,
		now:         o.Now,
		gauge:       o.TargetRPSGauge,
		dropCounter: o.DroppedCounter,
		onChange:    o.OnChange,
		cfg:         cfg.clone(),
		picker:      picker,
	}
	now := e.now()
	e.phaseStart, e.lastTick = now, now
	if o.Pool != nil {
		o.Pool.Resize(cfg.Customers)
	}
	return e, nil
}

// Validate reports whether c is a configuration the engine accepts: the same
// rules PUT /v1/traffic enforces.
func Validate(c Config) error { return validate(c) }

func validate(c Config) error {
	if c.RPS < MinRPS || c.RPS > MaxRPS {
		return fmt.Errorf("rps must be between %g and %g", MinRPS, MaxRPS)
	}
	if !c.Pattern.Valid() {
		return fmt.Errorf("unknown pattern %q", c.Pattern)
	}
	if c.PeriodSeconds < MinPeriodSeconds || c.PeriodSeconds > MaxPeriodSeconds {
		return fmt.Errorf("periodSeconds must be between %d and %d", MinPeriodSeconds, MaxPeriodSeconds)
	}
	if err := journey.ValidateMix(c.Mix); err != nil {
		return err
	}
	if c.Customers < 1 {
		return errors.New("customers must be at least 1")
	}
	return nil
}

// Update applies a patch atomically and returns the resulting configuration.
// An invalid patch changes nothing.
func (e *Engine) Update(p Patch) (Config, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	next := e.cfg.With(p)
	if err := validate(next); err != nil {
		return Config{}, err
	}

	picker := e.picker
	if p.Mix != nil {
		var err error
		if picker, err = journey.NewPicker(next.Mix, e.source); err != nil {
			return Config{}, err
		}
	}

	// The shaped patterns are phase-sensitive: restart the phase clock when the
	// run starts or the shape changes, so a ramp really does start at zero.
	restart := (next.Running && !e.cfg.Running) ||
		next.Pattern != e.cfg.Pattern ||
		next.PeriodSeconds != e.cfg.PeriodSeconds
	if restart {
		e.phaseStart = e.now()
	}

	e.cfg = next
	e.picker = picker
	if e.pool != nil {
		e.pool.Resize(next.Customers)
	}
	e.log.Info("traffic configuration changed",
		"running", next.Running, "rps", next.RPS, "pattern", string(next.Pattern),
		"periodSeconds", next.PeriodSeconds, "customers", next.Customers)
	if e.onChange != nil {
		e.onChange(next.clone())
	}
	return next.clone(), nil
}

// Drain stops launching journeys for good, without touching the configuration,
// so a pod that is shutting down never runs alongside its replacement. Journeys
// already in flight finish.
func (e *Engine) Drain() {
	if !e.draining.Swap(true) {
		e.log.Info("shutting down: no new journeys")
	}
}

// Config returns the current configuration.
func (e *Engine) Config() Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg.clone()
}

// Status returns the configuration plus live statistics.
func (e *Engine) Status() Status {
	snap := e.rec.Snapshot()
	return Status{
		Config:      e.Config(),
		Target:      e.target,
		MaxInFlight: e.maxInFlight,
		TickMs:      Tick.Milliseconds(),
		Stats: Stats{
			TargetRPS: math.Float64frombits(e.targetRPS.Load()),
			ActualRPS: snap.ActualRps,
			InFlight:  e.inFlight.Load(),
			Started:   e.started.Load(),
			Dropped:   e.dropped.Load(),
			Journeys:  snap.Journeys,
			Statuses:  snap.Statuses,
		},
	}
}

// Tick advances the schedule and launches whatever is due. It returns the
// number of journeys started and the number dropped for want of capacity.
// Tick must be called from a single goroutine.
func (e *Engine) Tick(ctx context.Context) (started, dropped int) {
	now := e.now()

	e.mu.RLock()
	cfg := e.cfg
	picker := e.picker
	phaseStart := e.phaseStart
	e.mu.RUnlock()

	if !cfg.Running || e.draining.Load() {
		e.sched.Reset()
		e.lastTick = now
		e.setTargetRPS(0)
		return 0, 0
	}

	period := time.Duration(cfg.PeriodSeconds) * time.Second
	rate := traffic.RateAt(cfg.Pattern, cfg.RPS, period, now.Sub(phaseStart))
	e.setTargetRPS(rate)

	dt := now.Sub(e.lastTick)
	e.lastTick = now
	n := e.sched.Advance(rate, dt)

	for range n {
		if e.inFlight.Load() >= int64(e.maxInFlight) {
			e.dropped.Add(1)
			dropped++
			if e.dropCounter != nil {
				e.dropCounter.Inc()
			}
			continue
		}
		e.inFlight.Add(1)
		e.started.Add(1)
		started++
		name := picker.Pick()
		go func() {
			defer e.inFlight.Add(-1)
			e.runner.Run(ctx, name)
		}()
	}
	return started, dropped
}

// Run ticks until ctx is done.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(Tick)
	defer ticker.Stop()
	e.lastTick = e.now()
	for {
		select {
		case <-ctx.Done():
			e.setTargetRPS(0)
			return
		case <-ticker.C:
			e.Tick(ctx)
		}
	}
}

// InFlight reports how many journeys are running right now.
func (e *Engine) InFlight() int64 { return e.inFlight.Load() }

// Dropped reports how many journeys were skipped because the in-flight cap
// was already reached.
func (e *Engine) Dropped() int64 { return e.dropped.Load() }

func (e *Engine) setTargetRPS(v float64) {
	e.targetRPS.Store(math.Float64bits(v))
	if e.gauge != nil {
		e.gauge.Set(v)
	}
}
