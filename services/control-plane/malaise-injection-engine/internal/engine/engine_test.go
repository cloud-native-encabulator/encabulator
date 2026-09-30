package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/store"
)

var epoch = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type clock struct {
	mu  sync.Mutex
	now time.Time
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

// fakeCluster records what the engine asked the cluster to do.
type fakeCluster struct {
	mu        sync.Mutex
	valid     map[string]bool
	applied   []string
	reverted  []string
	sweeps    int
	applyErr  error
	revertErr error
	sweepErr  error
}

func newFakeCluster(targets ...string) *fakeCluster {
	c := &fakeCluster{valid: map[string]bool{}}
	for _, t := range targets {
		c.valid[t] = true
	}
	return c
}

func (c *fakeCluster) ValidateTarget(_ context.Context, target string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.valid[target] {
		return errors.New("no such workload: " + target)
	}
	return nil
}

func (c *fakeCluster) Apply(_ context.Context, m *malaise.Malaise) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.applyErr != nil {
		return c.applyErr
	}
	c.applied = append(c.applied, m.ID)
	m.Detail = &malaise.Detail{NetworkPolicy: "policy-" + m.Target}
	return nil
}

func (c *fakeCluster) Revert(_ context.Context, m *malaise.Malaise) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revertErr != nil {
		return c.revertErr
	}
	c.reverted = append(c.reverted, m.ID)
	return nil
}

func (c *fakeCluster) Sweep(_ context.Context, _ time.Time) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweeps++
	return 0, c.sweepErr
}

func (c *fakeCluster) snapshot() ([]string, []string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.applied...), append([]string(nil), c.reverted...), c.sweeps
}

type fixture struct {
	engine  *Engine
	store   *store.Store
	cluster *fakeCluster
	clock   *clock
	reg     *prometheus.Registry
}

func newFixture(t *testing.T, cluster Cluster) *fixture {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	c := &clock{now: epoch}
	st, err := store.New(store.Config{Client: client, Now: c.Now})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	reg := prometheus.NewRegistry()
	e, err := New(Options{
		Store:    st,
		Cluster:  cluster,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: reg,
		Now:      c.Now,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fc, _ := cluster.(*fakeCluster)
	return &fixture{engine: e, store: st, cluster: fc, clock: c, reg: reg}
}

func latencyRequest(target string, seconds int) malaise.Request {
	return malaise.Request{
		Target: target, Kind: malaise.KindLatency,
		Params:          malaise.Params{Ms: 400, JitterMs: 100},
		DurationSeconds: &seconds,
	}
}

func TestNewRequiresAStore(t *testing.T) {
	if _, err := New(Options{Registry: prometheus.NewRegistry()}); err == nil {
		t.Fatal("expected an error without a store")
	}
}

func TestNewPublishesZeroForEveryKind(t *testing.T) {
	f := newFixture(t, nil)
	for _, kind := range malaise.Kinds {
		got := testutil.ToFloat64(f.engine.activeGauge.WithLabelValues(string(kind)))
		if got != 0 {
			t.Errorf("gauge for %q = %v, want 0", kind, got)
		}
	}
}

func TestCreateAppLevelMalaise(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()

	m, err := f.engine.Create(ctx, latencyRequest("dingle-arm-gateway", 60))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.State != malaise.StateActive {
		t.Errorf("state = %q, want active", m.State)
	}
	if !strings.HasPrefix(m.ID, malaise.IDPrefix) {
		t.Errorf("id = %q", m.ID)
	}

	stored, err := f.store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("the malaise was not persisted: %v", err)
	}
	if stored.Target != "dingle-arm-gateway" {
		t.Errorf("stored target = %q", stored.Target)
	}

	if got := testutil.ToFloat64(f.engine.activeGauge.WithLabelValues("latency")); got != 1 {
		t.Errorf("active gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(f.engine.totalCount.WithLabelValues("latency", "dingle-arm-gateway")); got != 1 {
		t.Errorf("total counter = %v, want 1", got)
	}
}

func TestCreateRejectsTargetsOutsideTheCatalog(t *testing.T) {
	f := newFixture(t, nil)
	_, err := f.engine.Create(context.Background(), latencyRequest("postgres", 60))
	if !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("err = %v, want ErrUnknownTarget", err)
	}
}

// The control plane does not inject malaise into itself.
func TestCreateRejectsAppLevelMalaiseOnTheControlPlane(t *testing.T) {
	f := newFixture(t, nil)
	for _, target := range []string{
		"malaise-injection-engine", "cardinal-grammeter",
		"encabulation-control-console", "demo-scenario-orchestrator",
	} {
		_, err := f.engine.Create(context.Background(), latencyRequest(target, 60))
		if !errors.Is(err, ErrUnknownTarget) {
			t.Errorf("%s: err = %v, want ErrUnknownTarget", target, err)
		}
	}
}

func TestCreateRejectsInvalidRequests(t *testing.T) {
	f := newFixture(t, nil)
	_, err := f.engine.Create(context.Background(), malaise.Request{
		Target: "dingle-arm-gateway", Kind: malaise.KindLatency,
	})
	if !errors.Is(err, malaise.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestKubernetesKindsNeedACluster(t *testing.T) {
	f := newFixture(t, nil)
	if f.engine.HasCluster() {
		t.Fatal("HasCluster should be false")
	}
	_, err := f.engine.Create(context.Background(), malaise.Request{
		Target: "valkey", Kind: malaise.KindPodKill,
	})
	if !errors.Is(err, ErrClusterUnavailable) {
		t.Fatalf("err = %v, want ErrClusterUnavailable", err)
	}
}

// A Kubernetes-level kind may target a backing store, which is not in the
// service catalog, so long as the cluster says it is labelled for malaise.
func TestKubernetesKindsMayTargetBackingStores(t *testing.T) {
	f := newFixture(t, newFakeCluster("valkey"))
	ctx := context.Background()

	m, err := f.engine.Create(ctx, malaise.Request{
		Target: "valkey", Kind: malaise.KindPartition,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	applied, _, _ := f.cluster.snapshot()
	if len(applied) != 1 || applied[0] != m.ID {
		t.Errorf("applied = %v, want %s", applied, m.ID)
	}
	if m.Detail == nil || m.Detail.NetworkPolicy != "policy-valkey" {
		t.Errorf("detail = %+v, want the policy name the cluster reported", m.Detail)
	}

	_, err = f.engine.Create(ctx, malaise.Request{Target: "nowhere", Kind: malaise.KindPartition})
	if !errors.Is(err, ErrUnknownTarget) {
		t.Errorf("err = %v, want ErrUnknownTarget", err)
	}
}

func TestCreateRecordsAFailedClusterApply(t *testing.T) {
	cluster := newFakeCluster("valkey")
	cluster.applyErr = errors.New("apiserver said no")
	f := newFixture(t, cluster)
	ctx := context.Background()

	if _, err := f.engine.Create(ctx, malaise.Request{
		Target: "valkey", Kind: malaise.KindScale, Params: malaise.Params{Replicas: i32(0)},
	}); err == nil {
		t.Fatal("expected the apply failure to be reported")
	}

	// The failure is on record, and nothing is active.
	recent, err := f.store.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 1 || recent[0].State != malaise.StateFailed {
		t.Fatalf("recent = %+v, want one failed record", recent)
	}
	if recent[0].Error == "" {
		t.Error("the failed record should explain itself")
	}
	active, err := f.store.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("active = %d, want none", len(active))
	}
}

func TestPodKillIsRecordedAsCompleted(t *testing.T) {
	f := newFixture(t, newFakeCluster("logarithmic-casing-line"))
	ctx := context.Background()

	m, err := f.engine.Create(ctx, malaise.Request{
		Target: "logarithmic-casing-line", Kind: malaise.KindPodKill,
		Params: malaise.Params{Count: 2},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.State != malaise.StateCompleted {
		t.Errorf("state = %q, want completed", m.State)
	}
	if list, _ := f.engine.List(ctx, "active", 0); len(list) != 0 {
		t.Errorf("active = %v, want none: a pod kill is instant", list)
	}
	if list, _ := f.engine.List(ctx, "all", 10); len(list) != 1 {
		t.Errorf("history = %v, want the completed record", list)
	}
}

func TestListActiveExcludesExpiredRecords(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()

	short, err := f.engine.Create(ctx, latencyRequest("dingle-arm-gateway", 10))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	long, err := f.engine.Create(ctx, latencyRequest("spurving-bearing-quoter", 600))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if list, _ := f.engine.List(ctx, "active", 0); len(list) != 2 {
		t.Fatalf("active = %d, want 2", len(list))
	}

	// Past the short one's expiry, before the reconcile loop has run.
	f.clock.advance(30 * time.Second)
	list, err := f.engine.List(ctx, "active", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != long.ID {
		t.Errorf("active = %v, want only %s", idsOf(list), long.ID)
	}
	if list, _ = f.engine.List(ctx, "all", 10); len(list) != 2 {
		t.Errorf("history = %v, want both records", idsOf(list))
	}
	_ = short
}

func TestDirectivesAreScopedAndAppLevelOnly(t *testing.T) {
	f := newFixture(t, newFakeCluster("dingle-arm-gateway"))
	ctx := context.Background()

	gatewayLatency, err := f.engine.Create(ctx, latencyRequest("dingle-arm-gateway", 120))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.engine.Create(ctx, malaise.Request{
		Target: "spurving-bearing-quoter", Kind: malaise.KindError,
		Params: malaise.Params{Status: 503},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// A Kubernetes-level malaise on the same target must not become a directive.
	if _, err := f.engine.Create(ctx, malaise.Request{
		Target: "dingle-arm-gateway", Kind: malaise.KindPartition,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	directives, err := f.engine.Directives(ctx, "dingle-arm-gateway")
	if err != nil {
		t.Fatalf("Directives: %v", err)
	}
	if len(directives) != 1 {
		t.Fatalf("directives = %+v, want just the latency one", directives)
	}
	d := directives[0]
	if d.ID != gatewayLatency.ID || d.Kind != malaise.KindLatency {
		t.Errorf("directive = %+v", d)
	}
	if d.Params.Ms != 400 || d.Params.JitterMs != 100 || *d.Params.Percent != 100 {
		t.Errorf("params = %+v", d.Params)
	}
	if !d.ExpiresAt.Equal(gatewayLatency.ExpiresAt) {
		t.Errorf("expiresAt = %v, want %v", d.ExpiresAt, gatewayLatency.ExpiresAt)
	}

	if directives, err = f.engine.Directives(ctx, "hydrocoptic-datastore"); err != nil || len(directives) != 0 {
		t.Errorf("directives for an untargeted service = %+v (err %v)", directives, err)
	}
}

func TestDirectivesStopAtExpiry(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	if _, err := f.engine.Create(ctx, latencyRequest("dingle-arm-gateway", 10)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	f.clock.advance(11 * time.Second)
	directives, err := f.engine.Directives(ctx, "dingle-arm-gateway")
	if err != nil {
		t.Fatalf("Directives: %v", err)
	}
	if len(directives) != 0 {
		t.Errorf("directives = %+v, want none past the expiry", directives)
	}
}

func TestClearEndsAMalaiseEarly(t *testing.T) {
	f := newFixture(t, newFakeCluster("valkey"))
	ctx := context.Background()

	m, err := f.engine.Create(ctx, malaise.Request{Target: "valkey", Kind: malaise.KindPartition})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	f.clock.advance(5 * time.Second)
	cleared, err := f.engine.Clear(ctx, m.ID)
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if cleared.State != malaise.StateCleared {
		t.Errorf("state = %q, want cleared", cleared.State)
	}
	if cleared.EndedAt == nil {
		t.Error("endedAt is not set")
	}

	_, reverted, _ := f.cluster.snapshot()
	if len(reverted) != 1 || reverted[0] != m.ID {
		t.Errorf("reverted = %v, want %s", reverted, m.ID)
	}
	if list, _ := f.engine.List(ctx, "active", 0); len(list) != 0 {
		t.Errorf("active = %v, want none", idsOf(list))
	}
	if got := testutil.ToFloat64(f.engine.activeGauge.WithLabelValues("partition")); got != 0 {
		t.Errorf("active gauge = %v, want 0", got)
	}

	// Clearing twice is idempotent and does not revert twice.
	if _, err := f.engine.Clear(ctx, m.ID); err != nil {
		t.Fatalf("second Clear: %v", err)
	}
	if _, reverted, _ = f.cluster.snapshot(); len(reverted) != 1 {
		t.Errorf("reverted = %v, want one revert only", reverted)
	}
}

func TestClearMissing(t *testing.T) {
	f := newFixture(t, nil)
	if _, err := f.engine.Clear(context.Background(), "mal_nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestClearReportsARevertFailureAndKeepsTheMalaiseActive(t *testing.T) {
	cluster := newFakeCluster("valkey")
	f := newFixture(t, cluster)
	ctx := context.Background()

	m, err := f.engine.Create(ctx, malaise.Request{Target: "valkey", Kind: malaise.KindPartition})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cluster.mu.Lock()
	cluster.revertErr = errors.New("apiserver said no")
	cluster.mu.Unlock()

	if _, err := f.engine.Clear(ctx, m.ID); err == nil {
		t.Fatal("expected the revert failure to be reported")
	}
	// Still active, so the reconcile loop will keep trying.
	if list, _ := f.engine.List(ctx, "active", 0); len(list) != 1 {
		t.Errorf("active = %v, want the malaise still in force", idsOf(list))
	}
}

func TestClearAll(t *testing.T) {
	f := newFixture(t, newFakeCluster("valkey"))
	ctx := context.Background()

	for _, req := range []malaise.Request{
		latencyRequest("dingle-arm-gateway", 120),
		latencyRequest("spurving-bearing-quoter", 120),
		{Target: "valkey", Kind: malaise.KindPartition},
	} {
		if _, err := f.engine.Create(ctx, req); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	cleared, err := f.engine.ClearAll(ctx)
	if err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if cleared != 3 {
		t.Errorf("cleared %d, want 3", cleared)
	}
	if list, _ := f.engine.List(ctx, "active", 0); len(list) != 0 {
		t.Errorf("active = %v, want none", idsOf(list))
	}
	if _, reverted, _ := f.cluster.snapshot(); len(reverted) != 1 {
		t.Errorf("reverted = %v, want only the Kubernetes-level one", reverted)
	}

	// Clearing nothing is fine.
	if cleared, err = f.engine.ClearAll(ctx); err != nil || cleared != 0 {
		t.Errorf("ClearAll on an empty engine = %d, %v", cleared, err)
	}
}

func TestReconcileExpiresAndReverts(t *testing.T) {
	f := newFixture(t, newFakeCluster("valkey"))
	ctx := context.Background()

	short, err := f.engine.Create(ctx, malaise.Request{
		Target: "valkey", Kind: malaise.KindPartition, DurationSeconds: intPtr(10),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	long, err := f.engine.Create(ctx, latencyRequest("dingle-arm-gateway", 600))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Nothing is due yet.
	expired, err := f.engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if expired != 0 {
		t.Errorf("expired %d, want 0", expired)
	}

	f.clock.advance(11 * time.Second)
	if expired, err = f.engine.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if expired != 1 {
		t.Errorf("expired %d, want 1", expired)
	}

	ended, err := f.store.Get(ctx, short.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ended.State != malaise.StateExpired {
		t.Errorf("state = %q, want expired", ended.State)
	}
	if ended.EndedAt == nil {
		t.Error("endedAt is not set")
	}
	_, reverted, sweeps := f.cluster.snapshot()
	if len(reverted) != 1 || reverted[0] != short.ID {
		t.Errorf("reverted = %v, want %s", reverted, short.ID)
	}
	if sweeps != 2 {
		t.Errorf("sweeps = %d, want one per reconcile", sweeps)
	}

	if list, _ := f.engine.List(ctx, "active", 0); len(list) != 1 || list[0].ID != long.ID {
		t.Errorf("active = %v, want only %s", idsOf(list), long.ID)
	}
	if got := testutil.ToFloat64(f.engine.activeGauge.WithLabelValues("partition")); got != 0 {
		t.Errorf("partition gauge = %v, want 0", got)
	}
	if got := testutil.ToFloat64(f.engine.activeGauge.WithLabelValues("latency")); got != 1 {
		t.Errorf("latency gauge = %v, want 1", got)
	}
}

// The reconcile loop must sweep the cluster even when the store is empty, or a
// restart with a wiped Valkey would strand a NetworkPolicy forever.
func TestReconcileSweepsWithAnEmptyStore(t *testing.T) {
	f := newFixture(t, newFakeCluster())
	if _, err := f.engine.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, _, sweeps := f.cluster.snapshot(); sweeps != 1 {
		t.Errorf("sweeps = %d, want 1", sweeps)
	}
}

func TestReconcileReportsASweepFailure(t *testing.T) {
	cluster := newFakeCluster()
	cluster.sweepErr = errors.New("apiserver said no")
	f := newFixture(t, cluster)
	if _, err := f.engine.Reconcile(context.Background()); err == nil {
		t.Fatal("expected the sweep failure to be reported")
	}
}

// An index entry whose record has expired out of Valkey must be dropped rather
// than retried forever.
func TestReconcileDropsOrphanedIndexEntries(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()

	m, err := f.engine.Create(ctx, latencyRequest("dingle-arm-gateway", 10))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Simulate the record's TTL firing while the index entry remains.
	if err := f.store.Deactivate(ctx, m.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	f.clock.advance(11 * time.Second)
	expired, err := f.engine.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if expired != 0 {
		t.Errorf("expired %d, want 0", expired)
	}
}

func TestRunReconcilesUntilCancelled(t *testing.T) {
	f := newFixture(t, newFakeCluster())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.engine.Run(ctx)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, _, sweeps := f.cluster.snapshot(); sweeps > 0 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("the reconcile loop never ran")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestMetricNames(t *testing.T) {
	f := newFixture(t, nil)
	if _, err := f.engine.Create(context.Background(), latencyRequest("dingle-arm-gateway", 60)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	names, err := f.reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := map[string]bool{}
	for _, mf := range names {
		found[mf.GetName()] = true
	}
	for _, want := range []string{"encabulator_malaises_active", "encabulator_malaises_total"} {
		if !found[want] {
			t.Errorf("metric %q is missing", want)
		}
	}
}

func idsOf(list []malaise.Malaise) []string {
	out := make([]string, len(list))
	for i, m := range list {
		out[i] = m.ID
	}
	return out
}

func intPtr(v int) *int  { return &v }
func i32(v int32) *int32 { return &v }
