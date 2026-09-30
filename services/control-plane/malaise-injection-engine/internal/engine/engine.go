// Package engine is the malaise lifecycle: validate a request, apply it, record
// it, and take it back off again when its time is up.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/catalog"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/store"
)

// ReconcileInterval is the loop period fixed by the design.
const ReconcileInterval = 2 * time.Second

// Errors the API turns into status codes.
var (
	// ErrNotFound means no malaise exists under that id.
	ErrNotFound = errors.New("malaise not found")
	// ErrUnknownTarget means the target is neither a catalogued service nor a
	// workload labelled for malaise.
	ErrUnknownTarget = errors.New("unknown malaise target")
	// ErrClusterUnavailable means a Kubernetes-level kind was asked for without a
	// Kubernetes client.
	ErrClusterUnavailable = errors.New("kubernetes api is not available")
)

// Store is the persistence the engine needs.
type Store interface {
	Put(ctx context.Context, m malaise.Malaise) error
	Get(ctx context.Context, id string) (malaise.Malaise, error)
	Active(ctx context.Context) ([]malaise.Malaise, error)
	Recent(ctx context.Context, limit int) ([]malaise.Malaise, error)
	DueBefore(ctx context.Context, t time.Time) ([]string, error)
	Deactivate(ctx context.Context, id string) error
}

// Cluster is the Kubernetes actuator. It is optional: without it the engine
// still serves app-level malaise.
type Cluster interface {
	ValidateTarget(ctx context.Context, target string) error
	Apply(ctx context.Context, m *malaise.Malaise) error
	Revert(ctx context.Context, m *malaise.Malaise) error
	Sweep(ctx context.Context, now time.Time) (int, error)
}

// Options configures an Engine.
type Options struct {
	Store   Store
	Cluster Cluster
	Logger  *slog.Logger
	// Registry receives the domain metrics; nil means the default registry.
	Registry prometheus.Registerer
	// Now may be nil, meaning time.Now.
	Now func() time.Time
}

// Engine creates, lists, clears and expires malaises.
type Engine struct {
	store   Store
	cluster Cluster
	log     *slog.Logger
	now     func() time.Time

	activeGauge *prometheus.GaugeVec
	totalCount  *prometheus.CounterVec
}

// New builds an Engine.
func New(o Options) (*Engine, error) {
	if o.Store == nil {
		return nil, errors.New("engine: store is required")
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Registry == nil {
		o.Registry = prometheus.DefaultRegisterer
	}

	e := &Engine{
		store:   o.Store,
		cluster: o.Cluster,
		log:     o.Logger,
		now:     o.Now,
		activeGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "encabulator_malaises_active",
			Help: "Malaises currently in force, by kind.",
		}, []string{"kind"}),
		totalCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "encabulator_malaises_total",
			Help: "Malaises induced, by kind and target.",
		}, []string{"kind", "target"}),
	}
	if err := o.Registry.Register(e.activeGauge); err != nil {
		return nil, fmt.Errorf("engine: register active gauge: %w", err)
	}
	if err := o.Registry.Register(e.totalCount); err != nil {
		return nil, fmt.Errorf("engine: register total counter: %w", err)
	}
	// Publish a zero for every kind so the console's panels are never empty.
	for _, kind := range malaise.Kinds {
		e.activeGauge.WithLabelValues(string(kind)).Set(0)
	}
	return e, nil
}

// HasCluster reports whether Kubernetes-level malaise is available.
func (e *Engine) HasCluster() bool { return e.cluster != nil }

// Create validates, applies and records a malaise.
func (e *Engine) Create(ctx context.Context, req malaise.Request) (malaise.Malaise, error) {
	m, err := malaise.Build(req, e.now())
	if err != nil {
		return malaise.Malaise{}, err
	}
	if err := e.validateTarget(ctx, m); err != nil {
		return malaise.Malaise{}, err
	}

	if m.Kind.Kubernetes() {
		if err := e.cluster.Apply(ctx, &m); err != nil {
			// Record the failure so the operator can see what went wrong, then
			// report it. A failed malaise is never active.
			m.State = malaise.StateFailed
			ended := e.now().UTC()
			m.EndedAt, m.Error = &ended, err.Error()
			if perr := e.store.Put(ctx, m); perr != nil {
				e.log.Error("could not record a failed malaise", "id", m.ID, "error", perr.Error())
			}
			return malaise.Malaise{}, err
		}
	}

	if err := e.store.Put(ctx, m); err != nil {
		// The cluster change is already made; the sweeper will find it by its
		// expiry annotation even though the record did not land.
		return malaise.Malaise{}, err
	}

	e.totalCount.WithLabelValues(string(m.Kind), m.Target).Inc()
	e.log.Info("malaise induced",
		"id", m.ID, "kind", string(m.Kind), "target", m.Target,
		"durationSeconds", m.DurationSeconds, "state", string(m.State))
	e.refreshActiveGauge(ctx)
	return m, nil
}

// validateTarget enforces the design's rule: app-level kinds must name a service
// in the catalog; Kubernetes-level kinds may name any workload labelled
// encabulator.io/malaise-target=true, which includes the backing stores.
func (e *Engine) validateTarget(ctx context.Context, m malaise.Malaise) error {
	if m.Kind.AppLevel() {
		if !catalog.Known(m.Target) {
			return fmt.Errorf("%w: %s is not in the service catalog", ErrUnknownTarget, m.Target)
		}
		if !catalog.AppLevel(m.Target) {
			return fmt.Errorf("%w: %s does not apply malaise to itself", ErrUnknownTarget, m.Target)
		}
		return nil
	}

	if e.cluster == nil {
		return ErrClusterUnavailable
	}
	if err := e.cluster.ValidateTarget(ctx, m.Target); err != nil {
		return fmt.Errorf("%w: %w", ErrUnknownTarget, err)
	}
	return nil
}

// List returns malaises. state "active" returns only those in force; anything
// else returns recent history, newest first.
func (e *Engine) List(ctx context.Context, state string, limit int) ([]malaise.Malaise, error) {
	if state == "active" || state == "" {
		active, err := e.store.Active(ctx)
		if err != nil {
			return nil, err
		}
		now := e.now()
		out := make([]malaise.Malaise, 0, len(active))
		for _, m := range active {
			if m.State == malaise.StateActive && !m.Expired(now) {
				out = append(out, m)
			}
		}
		return out, nil
	}
	return e.store.Recent(ctx, limit)
}

// Get returns one malaise, active or not.
func (e *Engine) Get(ctx context.Context, id string) (malaise.Malaise, error) {
	m, err := e.store.Get(ctx, id)
	if err != nil {
		return malaise.Malaise{}, notFound(id, err)
	}
	return m, nil
}

// notFound maps a missing record to ErrNotFound and leaves real store failures
// alone, so a Valkey outage is never reported to the operator as a 404.
func notFound(id string, err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return err
}

// Directives returns the app-level directives a service should apply.
func (e *Engine) Directives(ctx context.Context, service string) ([]malaise.Directive, error) {
	active, err := e.store.Active(ctx)
	if err != nil {
		return nil, err
	}
	now := e.now()
	out := make([]malaise.Directive, 0, len(active))
	for _, m := range active {
		if m.Target != service || !m.Kind.AppLevel() {
			continue
		}
		if m.State != malaise.StateActive || m.Expired(now) {
			continue
		}
		out = append(out, m.Directive())
	}
	return out, nil
}

// Clear ends one malaise early, reverting any cluster change.
func (e *Engine) Clear(ctx context.Context, id string) (malaise.Malaise, error) {
	m, err := e.store.Get(ctx, id)
	if err != nil {
		return malaise.Malaise{}, notFound(id, err)
	}
	if m.State.Terminal() {
		return m, nil
	}
	if err := e.end(ctx, &m, malaise.StateCleared); err != nil {
		return malaise.Malaise{}, err
	}
	e.refreshActiveGauge(ctx)
	return m, nil
}

// ClearAll ends every active malaise and returns how many it ended.
func (e *Engine) ClearAll(ctx context.Context) (int, error) {
	active, err := e.store.Active(ctx)
	if err != nil {
		return 0, err
	}
	cleared := 0
	var errs []error
	for i := range active {
		m := active[i]
		if m.State.Terminal() {
			if derr := e.store.Deactivate(ctx, m.ID); derr != nil {
				errs = append(errs, derr)
			}
			continue
		}
		if eerr := e.end(ctx, &m, malaise.StateCleared); eerr != nil {
			errs = append(errs, eerr)
			continue
		}
		cleared++
	}
	e.refreshActiveGauge(ctx)
	return cleared, errors.Join(errs...)
}

// end reverts a malaise's effect and writes its final state.
func (e *Engine) end(ctx context.Context, m *malaise.Malaise, state malaise.State) error {
	if m.Kind.Kubernetes() && e.cluster != nil {
		if err := e.cluster.Revert(ctx, m); err != nil {
			return fmt.Errorf("revert %s: %w", m.ID, err)
		}
	}
	ended := e.now().UTC().Truncate(time.Millisecond)
	m.State = state
	m.EndedAt = &ended
	if err := e.store.Put(ctx, *m); err != nil {
		return err
	}
	e.log.Info("malaise ended",
		"id", m.ID, "kind", string(m.Kind), "target", m.Target, "state", string(state))
	return nil
}

// Reconcile expires everything that is due and sweeps the cluster for stranded
// changes. It returns the number of malaises it expired.
func (e *Engine) Reconcile(ctx context.Context) (int, error) {
	now := e.now()
	var errs []error

	due, err := e.store.DueBefore(ctx, now)
	if err != nil {
		errs = append(errs, err)
	}
	expired := 0
	for _, id := range due {
		m, gerr := e.store.Get(ctx, id)
		if gerr != nil {
			// The record is gone; drop the index entry so we stop retrying.
			if derr := e.store.Deactivate(ctx, id); derr != nil {
				errs = append(errs, derr)
			}
			continue
		}
		if m.State.Terminal() {
			if derr := e.store.Deactivate(ctx, id); derr != nil {
				errs = append(errs, derr)
			}
			continue
		}
		if eerr := e.end(ctx, &m, malaise.StateExpired); eerr != nil {
			errs = append(errs, eerr)
			continue
		}
		expired++
	}

	if e.cluster != nil {
		if _, serr := e.cluster.Sweep(ctx, now); serr != nil {
			errs = append(errs, serr)
		}
	}

	e.refreshActiveGauge(ctx)
	return expired, errors.Join(errs...)
}

// Run reconciles every ReconcileInterval until ctx is done.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(ReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := e.Reconcile(ctx); err != nil && ctx.Err() == nil {
				e.log.Warn("reconcile did not finish cleanly", "error", err.Error())
			}
		}
	}
}

// refreshActiveGauge republishes encabulator_malaises_active from the store.
func (e *Engine) refreshActiveGauge(ctx context.Context) {
	active, err := e.store.Active(ctx)
	if err != nil {
		e.log.Warn("could not refresh the active malaise gauge", "error", err.Error())
		return
	}
	counts := map[malaise.Kind]int{}
	now := e.now()
	for _, m := range active {
		if m.State == malaise.StateActive && !m.Expired(now) {
			counts[m.Kind]++
		}
	}
	for _, kind := range malaise.Kinds {
		e.activeGauge.WithLabelValues(string(kind)).Set(float64(counts[kind]))
	}
}
