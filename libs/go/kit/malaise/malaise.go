// Package malaise applies the faults that the malaise injection engine
// prescribes for this service, per design section 7, "Malaise (fault injection)
// middleware".
//
// An Injector polls GET ${MALAISE_ENGINE_URL}/v1/directives?service=<name>
// every two seconds with an untraced client, keeps the last known directives
// until each one's expiresAt, and applies them to inbound HTTP requests
// (Middleware) or to message handling (Apply / WrapMessage).
//
// Services in the control plane -- the console, the grammeter, the engine itself
// and the orchestrator -- do not inject malaise into themselves, and simply do
// not build an Injector.
package malaise

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"
)

// Directive kinds understood by the in-process middleware. The Kubernetes-level
// kinds (pod-kill, partition, scale) are the engine's business, not ours.
const (
	KindLatency    = "latency"
	KindError      = "error"
	KindCongestion = "congestion"
	KindCPUBurn    = "cpu-burn"
)

// PollInterval is the directive poll period fixed by the design.
const PollInterval = 2 * time.Second

// Directive is one active fault.
type Directive struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Params    Params    `json:"params"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Params are the union of every kind's parameters.
type Params struct {
	// latency
	Ms       int `json:"ms,omitempty"`
	JitterMs int `json:"jitterMs,omitempty"`
	// latency, error
	Percent *int `json:"percent,omitempty"`
	// error
	Status int `json:"status,omitempty"`
	// congestion
	MaxConcurrent  int `json:"maxConcurrent,omitempty"`
	QueueTimeoutMs int `json:"queueTimeoutMs,omitempty"`
	// cpu-burn
	Millicores int `json:"millicores,omitempty"`
}

// DirectivesResponse is the engine's reply.
type DirectivesResponse struct {
	Directives []Directive `json:"directives"`
}

// Errors returned by Apply so that message consumers can nak.
var (
	// ErrInjected means an error directive fired.
	ErrInjected = errors.New("malaise injected")
	// ErrCongested means a congestion directive's queue timed out.
	ErrCongested = errors.New("congested")
)

// Config configures an Injector.
type Config struct {
	// EngineURL is MALAISE_ENGINE_URL. Empty disables the injector entirely.
	EngineURL string
	// Service is OTEL_SERVICE_NAME, the directive query's subject.
	Service string
	// Interval overrides the 2s poll period (tests).
	Interval time.Duration
	// Registry receives encabulator_malaise_applied_total.
	Registry prometheus.Registerer
	// Logger is optional.
	Logger *slog.Logger
	// HTTPClient overrides the untraced poll client (tests).
	HTTPClient *http.Client
}

// Injector applies directives. A nil *Injector is a no-op, so callers need not
// branch on whether malaise is configured.
type Injector struct {
	url      string
	service  string
	interval time.Duration
	client   *http.Client
	log      *slog.Logger
	applied  *prometheus.CounterVec

	mu         sync.RWMutex
	directives []Directive
	gate       *gate

	burn *burner
}

// New builds an Injector. It returns nil when EngineURL is empty, which
// disables the middleware.
func New(cfg Config) *Injector {
	if cfg.EngineURL == "" {
		return nil
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = PollInterval
	}
	client := cfg.HTTPClient
	if client == nil {
		// Deliberately not instrumented: the design forbids tracing this loop.
		client = &http.Client{Timeout: 2 * time.Second}
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	applied := httpx.RegisterCounterVec(cfg.Registry, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "encabulator_malaise_applied_total",
		Help: "Injected faults applied, by kind.",
	}, []string{"kind"}))

	return &Injector{
		url:      cfg.EngineURL,
		service:  cfg.Service,
		interval: interval,
		client:   client,
		log:      log,
		applied:  applied,
		gate:     newGate(),
		burn:     newBurner(),
	}
}

// Start launches the poll loop. It returns immediately; the loop stops when ctx
// is cancelled.
func (i *Injector) Start(ctx context.Context) {
	if i == nil {
		return
	}
	go func() {
		i.poll(ctx)
		t := time.NewTicker(i.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				i.burn.stop()
				return
			case <-t.C:
				i.poll(ctx)
			}
		}
	}()
}

// Directives returns the currently active, unexpired directives.
func (i *Injector) Directives() []Directive {
	if i == nil {
		return nil
	}
	now := time.Now()
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]Directive, 0, len(i.directives))
	for _, d := range i.directives {
		if d.ExpiresAt.IsZero() || d.ExpiresAt.After(now) {
			out = append(out, d)
		}
	}
	return out
}

func (i *Injector) poll(ctx context.Context) {
	endpoint := fmt.Sprintf("%s/v1/directives?service=%s", i.url, url.QueryEscape(i.service))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return
	}
	resp, err := i.client.Do(req)
	if err != nil {
		// Keep applying what we have until each directive expires.
		i.log.Debug("malaise engine unreachable; keeping last directives", slog.Any("error", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		i.log.Debug("malaise engine returned unexpected status", slog.Int("status", resp.StatusCode))
		return
	}
	var out DirectivesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		i.log.Debug("malaise directives undecodable", slog.Any("error", err))
		return
	}
	i.set(out.Directives)
}

func (i *Injector) set(ds []Directive) {
	i.mu.Lock()
	i.directives = ds
	i.mu.Unlock()

	// congestion and cpu-burn are stateful; reconcile them on every poll.
	var maxConcurrent, queueTimeoutMs, millicores int
	for _, d := range ds {
		if !d.ExpiresAt.IsZero() && !d.ExpiresAt.After(time.Now()) {
			continue
		}
		switch d.Kind {
		case KindCongestion:
			if d.Params.MaxConcurrent > 0 && (maxConcurrent == 0 || d.Params.MaxConcurrent < maxConcurrent) {
				maxConcurrent = d.Params.MaxConcurrent
				queueTimeoutMs = d.Params.QueueTimeoutMs
			}
		case KindCPUBurn:
			millicores += d.Params.Millicores
		}
	}
	i.gate.resize(maxConcurrent, time.Duration(queueTimeoutMs)*time.Millisecond)
	i.burn.set(millicores)
}

// Middleware applies latency, error and congestion to inbound HTTP requests.
// Health and metrics endpoints never reach it, because the kit registers them
// outside the API middleware chain.
func (i *Injector) Middleware(next http.Handler) http.Handler {
	if i == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, d := range i.Directives() {
			switch d.Kind {
			case KindLatency:
				if i.fires(d) {
					i.count(KindLatency)
					if err := sleepCtx(r.Context(), latencyOf(d.Params)); err != nil {
						return
					}
				}
			case KindError:
				if i.fires(d) {
					i.count(KindError)
					status := d.Params.Status
					if status == 0 {
						status = http.StatusServiceUnavailable
					}
					httpx.WriteError(w, status, httpx.CodeMalaise,
						"Malaise injected: the encabulator is experiencing sinusoidal repleneration")
					return
				}
			}
		}

		release, err := i.gate.enter(r.Context())
		if err != nil {
			i.count(KindCongestion)
			httpx.WriteError(w, http.StatusServiceUnavailable, httpx.CodeCongested,
				"Congested: too many marzlevanes in flight")
			return
		}
		if release != nil {
			i.count(KindCongestion)
			defer release()
		}
		next.ServeHTTP(w, r)
	})
}

// Apply runs fn under the active directives, which is how message consumers
// honor malaise. It returns ErrInjected when an error directive fires (nak the
// message) and ErrCongested when a congestion queue times out.
func (i *Injector) Apply(ctx context.Context, fn func(context.Context) error) error {
	if i == nil {
		return fn(ctx)
	}
	for _, d := range i.Directives() {
		switch d.Kind {
		case KindLatency:
			if i.fires(d) {
				i.count(KindLatency)
				if err := sleepCtx(ctx, latencyOf(d.Params)); err != nil {
					return err
				}
			}
		case KindError:
			if i.fires(d) {
				i.count(KindError)
				return ErrInjected
			}
		}
	}

	release, err := i.gate.enter(ctx)
	if err != nil {
		i.count(KindCongestion)
		return ErrCongested
	}
	if release != nil {
		i.count(KindCongestion)
		defer release()
	}
	return fn(ctx)
}

// WrapMessage wraps a message handler so that it honors malaise.
func WrapMessage[T any](i *Injector, h func(context.Context, T) error) func(context.Context, T) error {
	return func(ctx context.Context, msg T) error {
		return i.Apply(ctx, func(ctx context.Context) error { return h(ctx, msg) })
	}
}

func (i *Injector) count(kind string) {
	if i.applied != nil {
		i.applied.WithLabelValues(kind).Inc()
	}
}

// fires evaluates a directive's percent parameter. An absent percent means 100.
func (i *Injector) fires(d Directive) bool {
	pct := 100
	if d.Params.Percent != nil {
		pct = *d.Params.Percent
	}
	if pct <= 0 {
		return false
	}
	if pct >= 100 {
		return true
	}
	return rand.IntN(100) < pct
}

func latencyOf(p Params) time.Duration {
	d := time.Duration(p.Ms) * time.Millisecond
	if p.JitterMs > 0 {
		jitter := time.Duration(rand.IntN(2*p.JitterMs+1)-p.JitterMs) * time.Millisecond
		d += jitter
	}
	if d < 0 {
		return 0
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// gate is the congestion semaphore. A zero capacity means no congestion.
type gate struct {
	mu      sync.Mutex
	tokens  chan struct{}
	cap     int
	timeout time.Duration
}

func newGate() *gate { return &gate{} }

func (g *gate) resize(capacity int, timeout time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if capacity == g.cap {
		g.timeout = timeout
		return
	}
	g.cap = capacity
	g.timeout = timeout
	if capacity <= 0 {
		g.tokens = nil
		return
	}
	g.tokens = make(chan struct{}, capacity)
}

// enter acquires a slot. It returns a nil release func when congestion is not
// active, and an error when the queue timed out.
func (g *gate) enter(ctx context.Context) (func(), error) {
	g.mu.Lock()
	tokens, timeout := g.tokens, g.timeout
	g.mu.Unlock()
	if tokens == nil {
		return nil, nil
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case tokens <- struct{}{}:
		return func() { <-tokens }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.C:
		return nil, ErrCongested
	}
}

// burner burns roughly the requested millicores of CPU until told to stop.
type burner struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	current int
}

func newBurner() *burner { return &burner{} }

func (b *burner) set(millicores int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if millicores == b.current {
		return
	}
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.current = millicores
	if millicores <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel

	// One goroutine per whole core, plus one duty-cycled goroutine for the
	// remainder. 1000 millicores is one core saturated.
	whole := millicores / 1000
	remainder := millicores % 1000
	if whole > runtime.NumCPU() {
		whole = runtime.NumCPU()
	}
	for range whole {
		go spin(ctx, 1000)
	}
	if remainder > 0 {
		go spin(ctx, remainder)
	}
}

func (b *burner) stop() { b.set(0) }

// spin busy-loops for duty/1000 of every 10ms slice.
func spin(ctx context.Context, duty int) {
	const slice = 10 * time.Millisecond
	busy := slice * time.Duration(duty) / 1000
	idle := slice - busy
	for {
		if ctx.Err() != nil {
			return
		}
		deadline := time.Now().Add(busy)
		for time.Now().Before(deadline) {
			// Burn. The compiler cannot elide a loop with a time call in it.
		}
		if idle > 0 {
			t := time.NewTimer(idle)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
	}
}
