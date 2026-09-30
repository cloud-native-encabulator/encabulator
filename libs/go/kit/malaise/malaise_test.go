package malaise_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/malaise"
)

// engine is a stand-in for the malaise injection engine.
type engine struct {
	mu         sync.Mutex
	directives []malaise.Directive
	queries    atomic.Int32
	srv        *httptest.Server
}

func newEngine(t *testing.T) *engine {
	t.Helper()
	e := &engine{}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.queries.Add(1)
		if r.URL.Query().Get("service") == "" {
			t.Errorf("poller did not send ?service=")
		}
		e.mu.Lock()
		out := malaise.DirectivesResponse{Directives: e.directives}
		e.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *engine) set(ds ...malaise.Directive) {
	e.mu.Lock()
	e.directives = ds
	e.mu.Unlock()
}

func pct(n int) *int { return &n }

func newInjector(t *testing.T, e *engine, reg prometheus.Registerer) *malaise.Injector {
	t.Helper()
	inj := malaise.New(malaise.Config{
		EngineURL: e.srv.URL,
		Service:   "hydrocoptic-datastore",
		Interval:  10 * time.Millisecond,
		Registry:  reg,
	})
	if inj == nil {
		t.Fatal("injector is nil with an engine URL set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	inj.Start(ctx)
	return inj
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within 3s")
}

func TestDisabledWithoutEngineURL(t *testing.T) {
	if inj := malaise.New(malaise.Config{Service: "svc"}); inj != nil {
		t.Fatal("expected a nil injector when MALAISE_ENGINE_URL is unset")
	}
	// A nil injector must still be usable.
	var inj *malaise.Injector
	h := inj.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("nil injector changed behaviour: %d", rec.Code)
	}
	if err := inj.Apply(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("nil injector Apply = %v", err)
	}
}

func TestErrorDirective(t *testing.T) {
	e := newEngine(t)
	e.set(malaise.Directive{
		ID: "mal_1", Kind: malaise.KindError,
		Params:    malaise.Params{Status: 503, Percent: pct(100)},
		ExpiresAt: time.Now().Add(time.Minute),
	})
	reg := prometheus.NewRegistry()
	inj := newInjector(t, e, reg)
	waitFor(t, func() bool { return len(inj.Directives()) == 1 })

	var handlerCalls atomic.Int32
	h := inj.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalls.Add(1)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if handlerCalls.Load() != 0 {
		t.Error("handler ran despite an error directive")
	}
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "malaise_injected" {
		t.Errorf("code = %q", env.Error.Code)
	}
	if got := testutil.ToFloat64(mustCounter(t, reg, "encabulator_malaise_applied_total", "error")); got < 1 {
		t.Errorf("encabulator_malaise_applied_total{kind=error} = %v", got)
	}
}

func TestErrorDirectiveNaksMessages(t *testing.T) {
	e := newEngine(t)
	e.set(malaise.Directive{
		ID: "mal_1", Kind: malaise.KindError,
		Params:    malaise.Params{Status: 503, Percent: pct(100)},
		ExpiresAt: time.Now().Add(time.Minute),
	})
	inj := newInjector(t, e, prometheus.NewRegistry())
	waitFor(t, func() bool { return len(inj.Directives()) == 1 })

	handler := malaise.WrapMessage(inj, func(context.Context, string) error { return nil })
	if err := handler(context.Background(), "evt_1"); !errors.Is(err, malaise.ErrInjected) {
		t.Fatalf("err = %v, want ErrInjected", err)
	}
}

func TestLatencyDirective(t *testing.T) {
	e := newEngine(t)
	e.set(malaise.Directive{
		ID: "mal_2", Kind: malaise.KindLatency,
		Params:    malaise.Params{Ms: 120, JitterMs: 10, Percent: pct(100)},
		ExpiresAt: time.Now().Add(time.Minute),
	})
	inj := newInjector(t, e, prometheus.NewRegistry())
	waitFor(t, func() bool { return len(inj.Directives()) == 1 })

	h := inj.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	start := time.Now()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/orders", nil))
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("latency not applied: %v", elapsed)
	}
}

func TestZeroPercentNeverFires(t *testing.T) {
	e := newEngine(t)
	e.set(malaise.Directive{
		ID: "mal_3", Kind: malaise.KindError,
		Params:    malaise.Params{Status: 503, Percent: pct(0)},
		ExpiresAt: time.Now().Add(time.Minute),
	})
	inj := newInjector(t, e, prometheus.NewRegistry())
	waitFor(t, func() bool { return len(inj.Directives()) == 1 })

	h := inj.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	for range 20 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("percent 0 fired: %d", rec.Code)
		}
	}
}

func TestExpiredDirectiveIsIgnored(t *testing.T) {
	e := newEngine(t)
	e.set(malaise.Directive{
		ID: "mal_4", Kind: malaise.KindError,
		Params:    malaise.Params{Status: 503, Percent: pct(100)},
		ExpiresAt: time.Now().Add(-time.Second),
	})
	inj := newInjector(t, e, prometheus.NewRegistry())
	waitFor(t, func() bool { return e.queries.Load() > 0 })

	if got := inj.Directives(); len(got) != 0 {
		t.Fatalf("expired directive still active: %+v", got)
	}
	h := inj.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("expired directive applied: %d", rec.Code)
	}
}

func TestCongestionDirective(t *testing.T) {
	e := newEngine(t)
	e.set(malaise.Directive{
		ID: "mal_5", Kind: malaise.KindCongestion,
		Params:    malaise.Params{MaxConcurrent: 1, QueueTimeoutMs: 50},
		ExpiresAt: time.Now().Add(time.Minute),
	})
	inj := newInjector(t, e, prometheus.NewRegistry())
	waitFor(t, func() bool { return len(inj.Directives()) == 1 })

	release := make(chan struct{})
	h := inj.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	first := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))
		first <- rec.Code
	}()
	time.Sleep(30 * time.Millisecond)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("second request status = %d, want 503", rec.Code)
	}
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != "congested" {
		t.Errorf("code = %q, want congested", env.Error.Code)
	}

	close(release)
	if code := <-first; code != http.StatusOK {
		t.Errorf("first request status = %d", code)
	}
}

func TestKeepsDirectivesWhenEngineGoesAway(t *testing.T) {
	e := newEngine(t)
	e.set(malaise.Directive{
		ID: "mal_6", Kind: malaise.KindError,
		Params:    malaise.Params{Status: 503, Percent: pct(100)},
		ExpiresAt: time.Now().Add(time.Minute),
	})
	inj := newInjector(t, e, prometheus.NewRegistry())
	waitFor(t, func() bool { return len(inj.Directives()) == 1 })

	e.srv.Close()
	time.Sleep(60 * time.Millisecond)
	if got := inj.Directives(); len(got) != 1 {
		t.Fatalf("directives dropped when the engine became unreachable: %+v", got)
	}
}

func mustCounter(t *testing.T, reg *prometheus.Registry, name string, labels ...string) prometheus.Counter {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetValue() == labels[0] {
					c := prometheus.NewCounter(prometheus.CounterOpts{Name: "shim"})
					c.Add(m.GetCounter().GetValue())
					return c
				}
			}
		}
	}
	t.Fatalf("metric %s{%v} not found", name, labels)
	return nil
}
