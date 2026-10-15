package engine

import (
	"context"
	"io"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/journey"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/rng"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/stats"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/traffic"
)

// clock is a manually advanced clock, so the scheduler can be tested without
// waiting for wall time.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)}
}

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

// fakeRunner counts journeys and can be made to block, which pins the
// in-flight count for the capacity tests.
type fakeRunner struct {
	release chan struct{}

	mu     sync.Mutex
	counts map[string]int
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{counts: map[string]int{}}
}

func (f *fakeRunner) Run(_ context.Context, name string) {
	f.mu.Lock()
	f.counts[name]++
	f.mu.Unlock()
	if f.release != nil {
		<-f.release
	}
}

func (f *fakeRunner) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.counts {
		n += c
	}
	return n
}

func (f *fakeRunner) snapshot() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, v := range f.counts {
		out[k] = v
	}
	return out
}

// fakePool records resize requests.
type fakePool struct {
	mu   sync.Mutex
	size int
}

func (p *fakePool) Resize(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.size = n
}

func (p *fakePool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.size
}

type gauge struct {
	mu sync.Mutex
	v  float64
}

func (g *gauge) Set(v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.v = v
}

func (g *gauge) value() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.v
}

type counter struct{ n int }

func (c *counter) Inc() { c.n++ }

type fixture struct {
	engine *Engine
	clock  *clock
	runner *fakeRunner
	pool   *fakePool
	gauge  *gauge
	drops  *counter
}

func newFixture(t *testing.T, initial Config, maxInFlight int) *fixture {
	t.Helper()
	c := newClock()
	runner := newFakeRunner()
	pool := &fakePool{}
	g := &gauge{}
	drops := &counter{}
	e, err := New(Options{
		Runner:         runner,
		Pool:           pool,
		Stats:          stats.NewRecorder(c.Now),
		Source:         rng.Seeded(1234),
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Target:         "http://dingle-arm-gateway:8080",
		MaxInFlight:    maxInFlight,
		Initial:        initial,
		Now:            c.Now,
		TargetRPSGauge: g,
		DroppedCounter: drops,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &fixture{engine: e, clock: c, runner: runner, pool: pool, gauge: g, drops: drops}
}

// tick advances the clock by one scheduler period and steps the engine.
func (f *fixture) tick(ctx context.Context) (int, int) {
	f.clock.advance(Tick)
	return f.engine.Tick(ctx)
}

// ticks steps n periods and returns the totals.
func (f *fixture) ticks(ctx context.Context, n int) (started, dropped int) {
	for range n {
		s, d := f.tick(ctx)
		started += s
		dropped += d
	}
	return started, dropped
}

func (f *fixture) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.engine.InFlight() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("journeys still in flight: %d", f.engine.InFlight())
		}
		time.Sleep(time.Millisecond)
	}
}

func running(rps float64, pattern traffic.Pattern, period int) Config {
	return Config{
		Running:       true,
		RPS:           rps,
		Pattern:       pattern,
		PeriodSeconds: period,
		Mix:           journey.DefaultMix(),
		Customers:     4,
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	f := newFixture(t, Config{RPS: 5, Customers: 7}, 0)
	cfg := f.engine.Config()
	if cfg.Pattern != traffic.PatternSteady {
		t.Errorf("pattern = %q, want steady", cfg.Pattern)
	}
	if cfg.PeriodSeconds != 60 {
		t.Errorf("periodSeconds = %d, want 60", cfg.PeriodSeconds)
	}
	if len(cfg.Mix) != 4 {
		t.Errorf("mix = %v, want the default four journeys", cfg.Mix)
	}
	if f.engine.Status().MaxInFlight != 256 {
		t.Errorf("maxInFlight = %d, want the default 256", f.engine.Status().MaxInFlight)
	}
	if f.pool.Size() != 7 {
		t.Errorf("pool sized to %d, want 7", f.pool.Size())
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	cases := map[string]Config{
		"rps too low":     {RPS: 0.05, Customers: 1},
		"rps too high":    {RPS: 501, Customers: 1},
		"bad pattern":     {RPS: 5, Pattern: "chaos", PeriodSeconds: 60, Customers: 1},
		"period too low":  {RPS: 5, Pattern: traffic.PatternSine, PeriodSeconds: -1, Customers: 1},
		"period too high": {RPS: 5, Pattern: traffic.PatternSine, PeriodSeconds: 99999, Customers: 1},
		"bad mix":         {RPS: 5, Customers: 1, Mix: map[string]int{"sabotage": 1}},
		"no customers":    {RPS: 5, Customers: -1},
	}
	for name, cfg := range cases {
		if _, err := New(Options{Runner: newFakeRunner(), Initial: cfg}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestNewRequiresARunner(t *testing.T) {
	if _, err := New(Options{Initial: Config{RPS: 1, Customers: 1}}); err == nil {
		t.Fatal("expected an error without a runner")
	}
}

func TestStoppedEngineStartsNothing(t *testing.T) {
	f := newFixture(t, Config{RPS: 50, Customers: 4}, 256)
	started, dropped := f.ticks(context.Background(), 40)
	if started != 0 || dropped != 0 {
		t.Fatalf("stopped engine started %d and dropped %d", started, dropped)
	}
	if f.engine.Status().Stats.TargetRPS != 0 {
		t.Errorf("target rps = %v, want 0 while stopped", f.engine.Status().Stats.TargetRPS)
	}
}

func TestSteadyRateOverTenSeconds(t *testing.T) {
	f := newFixture(t, running(10, traffic.PatternSteady, 60), 100000)
	ctx := context.Background()

	// 200 ticks of 50 ms is ten seconds; ten per second is a hundred journeys.
	started, dropped := f.ticks(ctx, 200)
	if dropped != 0 {
		t.Errorf("dropped %d journeys with a generous cap", dropped)
	}
	if started != 100 {
		t.Errorf("started %d journeys, want 100", started)
	}
	f.waitIdle(t)
	if f.runner.total() != 100 {
		t.Errorf("runner saw %d journeys, want 100", f.runner.total())
	}
	if f.engine.Status().Stats.Started != 100 {
		t.Errorf("status started = %d, want 100", f.engine.Status().Stats.Started)
	}
}

func TestSubTickRateIsHonoured(t *testing.T) {
	// A tenth of a request per second is one request every ten seconds: far
	// below one per tick, and exactly the case the accumulator exists for.
	f := newFixture(t, running(0.1, traffic.PatternSteady, 60), 256)
	started, _ := f.ticks(context.Background(), 20*int(time.Second/Tick))
	if started != 2 {
		t.Fatalf("started %d journeys in 20s at 0.1 rps, want 2", started)
	}
}

func TestRampRisesThenHolds(t *testing.T) {
	f := newFixture(t, running(20, traffic.PatternRamp, 10), 100000)
	ctx := context.Background()

	// First half of the ramp: average rate is 5 rps over 5 seconds.
	firstHalf, _ := f.ticks(ctx, 100)
	// Second half: average rate is 15 rps over 5 seconds.
	secondHalf, _ := f.ticks(ctx, 100)
	// After the ramp the rate holds at 20 rps.
	held, _ := f.ticks(ctx, 100)

	if firstHalf < 20 || firstHalf > 30 {
		t.Errorf("first half started %d, want ~25", firstHalf)
	}
	if secondHalf < 70 || secondHalf > 80 {
		t.Errorf("second half started %d, want ~75", secondHalf)
	}
	if held < 98 || held > 102 {
		t.Errorf("hold phase started %d, want ~100", held)
	}
	if got := f.engine.Status().Stats.TargetRPS; math.Abs(got-20) > 1e-9 {
		t.Errorf("target rps after the ramp = %v, want 20", got)
	}
}

func TestBurstDeliversFiveTimesTheRate(t *testing.T) {
	f := newFixture(t, running(10, traffic.PatternBurst, 10), 100000)
	ctx := context.Background()

	// 80% of a 10s period at 10 rps: 80 journeys.
	quiet, _ := f.ticks(ctx, 160)
	// 20% at 50 rps: 100 journeys.
	loud, _ := f.ticks(ctx, 40)

	if quiet < 76 || quiet > 84 {
		t.Errorf("quiet phase started %d, want ~80", quiet)
	}
	if loud < 95 || loud > 103 {
		t.Errorf("burst phase started %d, want ~100", loud)
	}
}

func TestSineAveragesToTheBaseRate(t *testing.T) {
	f := newFixture(t, running(20, traffic.PatternSine, 20), 100000)
	ctx := context.Background()
	// One full 20 second period at an average of 20 rps is 400 journeys.
	started, _ := f.ticks(ctx, 400)
	if started < 390 || started > 410 {
		t.Errorf("one sine period started %d, want ~400", started)
	}
}

func TestInFlightCapDropsRatherThanQueues(t *testing.T) {
	f := newFixture(t, running(500, traffic.PatternSteady, 60), 4)
	f.runner.release = make(chan struct{})
	ctx := context.Background()

	// 500 rps on a 50 ms tick asks for 25 journeys; only four can run.
	started, dropped := f.tick(ctx)
	if started != 4 {
		t.Errorf("started %d, want 4 (the cap)", started)
	}
	if dropped != 21 {
		t.Errorf("dropped %d, want 21", dropped)
	}

	// The next tick can start nothing at all, and queues none of it.
	started, dropped = f.tick(ctx)
	if started != 0 || dropped != 25 {
		t.Errorf("second tick started %d dropped %d, want 0 and 25", started, dropped)
	}

	if got := f.engine.Dropped(); got != 46 {
		t.Errorf("cumulative dropped = %d, want 46", got)
	}
	if f.drops.n != 46 {
		t.Errorf("dropped counter = %d, want 46", f.drops.n)
	}
	if got := f.engine.InFlight(); got != 4 {
		t.Errorf("in flight = %d, want 4", got)
	}

	close(f.runner.release)
	f.waitIdle(t)
	if f.runner.total() != 4 {
		t.Errorf("runner saw %d journeys, want 4", f.runner.total())
	}
}

func TestUpdateStartsAndStopsTheRun(t *testing.T) {
	f := newFixture(t, Config{RPS: 20, Customers: 4}, 100000)
	ctx := context.Background()

	if started, _ := f.ticks(ctx, 20); started != 0 {
		t.Fatalf("started %d while stopped", started)
	}

	yes := true
	if _, err := f.engine.Update(Patch{Running: &yes}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	started, _ := f.ticks(ctx, 20) // one second at 20 rps
	if started != 20 {
		t.Errorf("started %d after starting, want 20", started)
	}

	no := false
	if _, err := f.engine.Update(Patch{Running: &no}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if started, _ := f.ticks(ctx, 20); started != 0 {
		t.Errorf("started %d after stopping, want 0", started)
	}
}

func TestUpdateRestartsTheRampPhaseOnStart(t *testing.T) {
	f := newFixture(t, Config{RPS: 20, Pattern: traffic.PatternRamp, PeriodSeconds: 10, Customers: 4}, 100000)
	ctx := context.Background()

	// Twenty seconds pass while stopped; a ramp that measured elapsed time from
	// process start would already be at full rate.
	f.ticks(ctx, 400)

	yes := true
	if _, err := f.engine.Update(Patch{Running: &yes}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// The first second of a 10s ramp to 20 rps averages 1 rps.
	started, _ := f.ticks(ctx, 20)
	if started > 3 {
		t.Errorf("first second of the ramp started %d, want at most 3", started)
	}
}

func TestUpdateRejectsInvalidPatchesWithoutChangingAnything(t *testing.T) {
	f := newFixture(t, running(10, traffic.PatternSteady, 60), 256)
	before := f.engine.Config()

	bad := []Patch{
		{RPS: ptr(0.0)},
		{RPS: ptr(1000.0)},
		{Pattern: ptr(traffic.Pattern("chaos"))},
		{PeriodSeconds: ptr(0)},
		{Mix: map[string]int{"sabotage": 1}},
		{Mix: map[string]int{}},
		{Customers: ptr(0)},
	}
	for i, p := range bad {
		if _, err := f.engine.Update(p); err == nil {
			t.Errorf("patch %d should have been rejected", i)
		}
	}
	after := f.engine.Config()
	if after.RPS != before.RPS || after.Pattern != before.Pattern ||
		after.PeriodSeconds != before.PeriodSeconds || after.Customers != before.Customers {
		t.Errorf("a rejected patch changed the configuration: %+v -> %+v", before, after)
	}
}

func TestUpdateChangesTheMix(t *testing.T) {
	f := newFixture(t, running(50, traffic.PatternSteady, 60), 100000)
	ctx := context.Background()

	if _, err := f.engine.Update(Patch{Mix: map[string]int{journey.Order: 1}}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	f.ticks(ctx, 40) // two seconds at 50 rps
	f.waitIdle(t)

	counts := f.runner.snapshot()
	if counts[journey.Order] == 0 {
		t.Fatalf("no orders ran: %v", counts)
	}
	for name, n := range counts {
		if name != journey.Order && n != 0 {
			t.Errorf("journey %q ran %d times with a mix of order only: %v", name, n, counts)
		}
	}
}

func TestUpdateResizesTheCustomerPool(t *testing.T) {
	f := newFixture(t, running(10, traffic.PatternSteady, 60), 256)
	if _, err := f.engine.Update(Patch{Customers: ptr(120)}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.pool.Size() != 120 {
		t.Errorf("pool sized to %d, want 120", f.pool.Size())
	}
}

func TestUpdateIsolatesTheMixFromTheCaller(t *testing.T) {
	f := newFixture(t, running(10, traffic.PatternSteady, 60), 256)
	mix := map[string]int{journey.Browse: 1, journey.Order: 1}
	if _, err := f.engine.Update(Patch{Mix: mix}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	mix[journey.Browse] = 999
	if got := f.engine.Config().Mix[journey.Browse]; got != 1 {
		t.Errorf("engine mix followed the caller's map: %d", got)
	}
	cfg := f.engine.Config()
	cfg.Mix[journey.Order] = 42
	if got := f.engine.Config().Mix[journey.Order]; got != 1 {
		t.Errorf("Config() handed out the live mix: %d", got)
	}
}

func TestStatusReportsTargetAndCap(t *testing.T) {
	f := newFixture(t, running(12, traffic.PatternSteady, 60), 64)
	f.tick(context.Background())
	st := f.engine.Status()
	if st.Target != "http://dingle-arm-gateway:8080" {
		t.Errorf("target = %q", st.Target)
	}
	if st.MaxInFlight != 64 {
		t.Errorf("maxInFlight = %d, want 64", st.MaxInFlight)
	}
	if st.TickMs != 50 {
		t.Errorf("tickMs = %d, want 50", st.TickMs)
	}
	if st.Stats.TargetRPS != 12 {
		t.Errorf("target rps = %v, want 12", st.Stats.TargetRPS)
	}
	if f.gauge.value() != 12 {
		t.Errorf("gauge = %v, want 12", f.gauge.value())
	}
}

// Run is the only part of the engine that needs real time: it exists to turn a
// wall-clock ticker into Tick calls.
func TestRunTicksOnRealTimeAndStopsWithTheContext(t *testing.T) {
	runner := newFakeRunner()
	e, err := New(Options{
		Runner:      runner,
		Pool:        &fakePool{},
		Source:      rng.Seeded(5),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		MaxInFlight: 1000,
		Initial:     running(200, traffic.PatternSteady, 60),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Run(ctx)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	deadline := time.Now().Add(5 * time.Second)
	for e.InFlight() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runner.total() == 0 {
		t.Error("Run started no journeys at 200 rps")
	}
	if e.Status().Stats.TargetRPS != 0 {
		t.Errorf("target rps = %v, want 0 after the run stopped", e.Status().Stats.TargetRPS)
	}
}

func ptr[T any](v T) *T { return &v }

func TestOnChangeSeesEveryAcceptedUpdateInOrder(t *testing.T) {
	var seen []Config
	c := newClock()
	e, err := New(Options{
		Runner:   newFakeRunner(),
		Pool:     &fakePool{},
		Stats:    stats.NewRecorder(c.Now),
		Source:   rng.Seeded(1),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Initial:  Config{RPS: 5, Customers: 50},
		Now:      c.Now,
		OnChange: func(cfg Config) { seen = append(seen, cfg) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(seen) != 0 {
		t.Fatalf("New must not report the initial configuration as a change: %+v", seen)
	}
	if _, err := e.Update(Patch{RPS: ptr(25.0), Pattern: ptr(traffic.PatternBurst)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Update(Patch{RPS: ptr(9999.0)}); err == nil {
		t.Fatal("an out-of-range rate was accepted")
	}
	if _, err := e.Update(Patch{Running: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("OnChange calls = %d, want 2 (the rejected patch must not count)", len(seen))
	}
	if seen[0].Running || seen[0].RPS != 25 || seen[0].Pattern != traffic.PatternBurst {
		t.Errorf("first change = %+v", seen[0])
	}
	if !seen[1].Running || seen[1].RPS != 25 {
		t.Errorf("second change = %+v", seen[1])
	}
	seen[1].Mix["browse"] = 0
	if e.Config().Mix["browse"] == 0 {
		t.Error("OnChange received the engine's own mix map instead of a copy")
	}
}

func TestDrainStopsNewJourneysAndKeepsTheConfiguration(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, running(20, traffic.PatternSteady, 60), 256)
	if started, _ := f.ticks(ctx, 20); started == 0 {
		t.Fatal("a running engine started nothing before the drain")
	}
	f.engine.Drain()
	if started, _ := f.ticks(ctx, 100); started != 0 {
		t.Errorf("started %d journeys after Drain, want 0", started)
	}
	if f.gauge.value() != 0 {
		t.Errorf("target rate after Drain = %v, want 0", f.gauge.value())
	}
	if cfg := f.engine.Config(); !cfg.Running || cfg.RPS != 20 {
		t.Errorf("Drain changed the configuration: %+v", cfg)
	}
	f.waitIdle(t)
}

func TestWithAppliesOnlyTheNamedFields(t *testing.T) {
	base := running(5, traffic.PatternSteady, 60)
	next := base.With(Patch{RPS: ptr(12.0), Mix: map[string]int{"order": 1}})
	if next.RPS != 12 || next.Pattern != traffic.PatternSteady || next.PeriodSeconds != 60 || !next.Running {
		t.Errorf("With = %+v", next)
	}
	if len(next.Mix) != 1 || next.Mix["order"] != 1 {
		t.Errorf("mix = %v, want only order", next.Mix)
	}
	if len(base.Mix) != 4 {
		t.Errorf("With changed the receiver's mix: %v", base.Mix)
	}
}
