package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/malaise-injection-engine/internal/engine"
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

// stubCluster accepts anything it is told about, so the API tests can exercise
// the Kubernetes-level kinds without a cluster.
type stubCluster struct {
	valid map[string]bool
}

func (c stubCluster) ValidateTarget(_ context.Context, target string) error {
	if c.valid[target] {
		return nil
	}
	return errors.New("no such workload: " + target)
}

func (c stubCluster) Apply(_ context.Context, m *malaise.Malaise) error {
	m.Detail = &malaise.Detail{NetworkPolicy: "policy-" + m.Target}
	return nil
}

func (c stubCluster) Revert(context.Context, *malaise.Malaise) error { return nil }

func (c stubCluster) Sweep(context.Context, time.Time) (int, error) { return 0, nil }

type fixture struct {
	handler http.Handler
	clock   *clock
	engine  *engine.Engine
}

func newFixture(t *testing.T, cluster engine.Cluster) *fixture {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	c := &clock{now: epoch}
	st, err := store.New(store.Config{Client: client, Now: c.Now})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e, err := engine.New(engine.Options{
		Store: st, Cluster: cluster, Logger: log,
		Registry: prometheus.NewRegistry(), Now: c.Now,
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	mux := http.NewServeMux()
	New(e, log, c.Now).Register(mux)
	return &fixture{handler: mux, clock: c, engine: e}
}

func (f *fixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func TestCreateMalaise(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"dingle-arm-gateway","kind":"latency","params":{"ms":400,"jitterMs":100},"durationSeconds":60}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q", ct)
	}

	got := decode[view](t, rec)
	if got.Target != "dingle-arm-gateway" || got.Kind != malaise.KindLatency {
		t.Errorf("malaise = %+v", got.Malaise)
	}
	if got.State != malaise.StateActive {
		t.Errorf("state = %q, want active", got.State)
	}
	if got.RemainingSeconds != 60 {
		t.Errorf("remainingSeconds = %d, want 60", got.RemainingSeconds)
	}
	if want := "/v1/malaises/" + got.ID; rec.Header().Get("Location") != want {
		t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), want)
	}
	if got.Params.Percent == nil || *got.Params.Percent != 100 {
		t.Errorf("percent = %v, want the default 100", got.Params.Percent)
	}
}

func TestCreateDefaultsTheDuration(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"spurving-bearing-quoter","kind":"cpu-burn","params":{"millicores":500}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[view](t, rec)
	if got.DurationSeconds != malaise.DefaultDurationSeconds {
		t.Errorf("duration = %d, want %d", got.DurationSeconds, malaise.DefaultDurationSeconds)
	}
}

func TestCreateBadRequests(t *testing.T) {
	cases := map[string]struct {
		body   string
		status int
		code   string
	}{
		"malformed json": {`{`, http.StatusBadRequest, httpx.CodeInvalidJSON},
		"unknown field":  {`{"target":"dingle-arm-gateway","kind":"latency","params":{"ms":1},"extra":1}`, http.StatusBadRequest, httpx.CodeInvalidJSON},
		"no target":      {`{"kind":"latency","params":{"ms":400}}`, http.StatusBadRequest, "invalid_malaise"},
		"unknown kind":   {`{"target":"dingle-arm-gateway","kind":"ennui"}`, http.StatusBadRequest, "invalid_malaise"},
		"missing params": {`{"target":"dingle-arm-gateway","kind":"latency"}`, http.StatusBadRequest, "invalid_malaise"},
		"bad duration":   {`{"target":"dingle-arm-gateway","kind":"latency","params":{"ms":400},"durationSeconds":9000}`, http.StatusBadRequest, "invalid_malaise"},
		"unknown target": {`{"target":"postgres","kind":"latency","params":{"ms":400}}`, http.StatusBadRequest, "unknown_target"},
		"control plane":  {`{"target":"cardinal-grammeter","kind":"latency","params":{"ms":400}}`, http.StatusBadRequest, "unknown_target"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, nil)
			rec := f.do(t, http.MethodPost, "/v1/malaises", c.body)
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, c.status, rec.Body.String())
			}
			env := decode[httpx.ErrorEnvelope](t, rec)
			if env.Error.Code != c.code {
				t.Errorf("code = %q, want %q", env.Error.Code, c.code)
			}
			if env.Error.Message == "" {
				t.Error("message is empty")
			}
		})
	}
}

func TestKubernetesKindWithoutACluster(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"valkey","kind":"pod-kill","params":{"count":1}}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if code := decode[httpx.ErrorEnvelope](t, rec).Error.Code; code != "kubernetes_unavailable" {
		t.Errorf("code = %q, want kubernetes_unavailable", code)
	}
}

func TestPartitionAcceptsAStringOrAnArray(t *testing.T) {
	cluster := stubCluster{valid: map[string]bool{"hydrocoptic-datastore": true}}
	for _, body := range []string{
		`{"target":"hydrocoptic-datastore","kind":"partition","params":{"from":"*"}}`,
		`{"target":"hydrocoptic-datastore","kind":"partition","params":{"from":["panametric-fam-processor"]}}`,
		`{"target":"hydrocoptic-datastore","kind":"partition"}`,
	} {
		f := newFixture(t, cluster)
		rec := f.do(t, http.MethodPost, "/v1/malaises", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s: status = %d: %s", body, rec.Code, rec.Body.String())
		}
		got := decode[view](t, rec)
		if len(got.Params.From) == 0 {
			t.Errorf("%s: from = %v, want at least one entry", body, got.Params.From)
		}
		if got.Detail == nil || got.Detail.NetworkPolicy == "" {
			t.Errorf("%s: detail = %+v, want the policy name", body, got.Detail)
		}
	}
}

func TestListMalaises(t *testing.T) {
	f := newFixture(t, nil)
	first := decode[view](t, f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"dingle-arm-gateway","kind":"latency","params":{"ms":400},"durationSeconds":10}`))
	second := decode[view](t, f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"spurving-bearing-quoter","kind":"error","params":{"status":503},"durationSeconds":600}`))

	got := decode[listResponse](t, f.do(t, http.MethodGet, "/v1/malaises", ""))
	if got.Count != 2 || len(got.Malaises) != 2 {
		t.Fatalf("active = %+v, want 2", got)
	}
	// Soonest expiry first, with a countdown.
	if got.Malaises[0].ID != first.ID {
		t.Errorf("first = %s, want %s", got.Malaises[0].ID, first.ID)
	}
	if got.Malaises[0].RemainingSeconds != 10 {
		t.Errorf("remaining = %d, want 10", got.Malaises[0].RemainingSeconds)
	}

	// Past the first one's expiry it drops out of the active list but stays in
	// the history.
	f.clock.advance(30 * time.Second)
	got = decode[listResponse](t, f.do(t, http.MethodGet, "/v1/malaises?state=active", ""))
	if got.Count != 1 || got.Malaises[0].ID != second.ID {
		t.Errorf("active = %+v, want only %s", got, second.ID)
	}
	got = decode[listResponse](t, f.do(t, http.MethodGet, "/v1/malaises?state=all", ""))
	if got.Count != 2 {
		t.Errorf("history = %+v, want 2", got)
	}
}

func TestListRejectsBadQueries(t *testing.T) {
	f := newFixture(t, nil)
	for _, path := range []string{"/v1/malaises?state=sideways", "/v1/malaises?limit=0", "/v1/malaises?limit=lots"} {
		rec := f.do(t, http.MethodGet, path, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", path, rec.Code)
		}
	}
}

func TestListLimitApplies(t *testing.T) {
	f := newFixture(t, nil)
	for range 4 {
		f.do(t, http.MethodPost, "/v1/malaises",
			`{"target":"dingle-arm-gateway","kind":"latency","params":{"ms":400}}`)
		f.clock.advance(time.Millisecond)
	}
	got := decode[listResponse](t, f.do(t, http.MethodGet, "/v1/malaises?state=all&limit=2", ""))
	if got.Count != 2 {
		t.Errorf("count = %d, want 2", got.Count)
	}
}

func TestGetOneMalaise(t *testing.T) {
	f := newFixture(t, nil)
	created := decode[view](t, f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"dingle-arm-gateway","kind":"latency","params":{"ms":400}}`))

	rec := f.do(t, http.MethodGet, "/v1/malaises/"+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[view](t, rec); got.ID != created.ID {
		t.Errorf("id = %q, want %q", got.ID, created.ID)
	}

	rec = f.do(t, http.MethodGet, "/v1/malaises/mal_nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDirectivesEndpoint(t *testing.T) {
	f := newFixture(t, nil)
	created := decode[view](t, f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"dingle-arm-gateway","kind":"error","params":{"status":503,"percent":25},"durationSeconds":120}`))

	rec := f.do(t, http.MethodGet, "/v1/directives?service=dingle-arm-gateway", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	// Check the wire shape the kit's middleware parses, not just the Go types.
	var raw struct {
		Directives []struct {
			ID        string         `json:"id"`
			Kind      string         `json:"kind"`
			Params    map[string]any `json:"params"`
			ExpiresAt string         `json:"expiresAt"`
		} `json:"directives"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw.Directives) != 1 {
		t.Fatalf("directives = %+v, want 1", raw.Directives)
	}
	d := raw.Directives[0]
	if d.ID != created.ID || d.Kind != "error" {
		t.Errorf("directive = %+v", d)
	}
	if d.Params["status"] != float64(503) || d.Params["percent"] != float64(25) {
		t.Errorf("params = %v", d.Params)
	}
	if _, err := time.Parse(time.RFC3339, d.ExpiresAt); err != nil {
		t.Errorf("expiresAt %q is not RFC 3339: %v", d.ExpiresAt, err)
	}

	// An untargeted service gets an empty list, never a null.
	rec = f.do(t, http.MethodGet, "/v1/directives?service=hydrocoptic-datastore", "")
	if !strings.Contains(rec.Body.String(), `"directives":[]`) {
		t.Errorf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestDirectivesRequiresAService(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(t, http.MethodGet, "/v1/directives", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decode[httpx.ErrorEnvelope](t, rec).Error.Code; code != "invalid_request" {
		t.Errorf("code = %q", code)
	}
}

func TestClearOne(t *testing.T) {
	f := newFixture(t, nil)
	created := decode[view](t, f.do(t, http.MethodPost, "/v1/malaises",
		`{"target":"dingle-arm-gateway","kind":"latency","params":{"ms":400}}`))

	rec := f.do(t, http.MethodDelete, "/v1/malaises/"+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[view](t, rec)
	if got.State != malaise.StateCleared {
		t.Errorf("state = %q, want cleared", got.State)
	}
	if got.RemainingSeconds != 0 {
		t.Errorf("remaining = %d, want 0", got.RemainingSeconds)
	}

	if list := decode[listResponse](t, f.do(t, http.MethodGet, "/v1/malaises", "")); list.Count != 0 {
		t.Errorf("active = %+v, want none", list)
	}
}

func TestClearMissing(t *testing.T) {
	f := newFixture(t, nil)
	rec := f.do(t, http.MethodDelete, "/v1/malaises/mal_nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if code := decode[httpx.ErrorEnvelope](t, rec).Error.Code; code != "not_found" {
		t.Errorf("code = %q", code)
	}
}

func TestClearAll(t *testing.T) {
	f := newFixture(t, nil)
	for _, target := range []string{"dingle-arm-gateway", "spurving-bearing-quoter", "hydrocoptic-datastore"} {
		f.do(t, http.MethodPost, "/v1/malaises",
			`{"target":"`+target+`","kind":"latency","params":{"ms":400}}`)
	}

	rec := f.do(t, http.MethodDelete, "/v1/malaises", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[clearAllResponse](t, rec); got.Cleared != 3 {
		t.Errorf("cleared = %d, want 3", got.Cleared)
	}
	if list := decode[listResponse](t, f.do(t, http.MethodGet, "/v1/malaises", "")); list.Count != 0 {
		t.Errorf("active = %+v, want none", list)
	}
}

func TestTargets(t *testing.T) {
	f := newFixture(t, stubCluster{})
	got := decode[targetsResponse](t, f.do(t, http.MethodGet, "/v1/targets", ""))
	if len(got.Services) != 15 {
		t.Errorf("services = %d, want the 15 in the catalog", len(got.Services))
	}
	if len(got.AppLevelServices) != 11 {
		t.Errorf("app-level services = %d, want 11", len(got.AppLevelServices))
	}
	if len(got.Kinds) != 7 {
		t.Errorf("kinds = %v, want 7", got.Kinds)
	}
	if !got.KubernetesReady {
		t.Error("kubernetesReady should be true with a cluster")
	}

	f = newFixture(t, nil)
	if decode[targetsResponse](t, f.do(t, http.MethodGet, "/v1/targets", "")).KubernetesReady {
		t.Error("kubernetesReady should be false without a cluster")
	}
}

func TestUnsupportedMethods(t *testing.T) {
	f := newFixture(t, nil)
	for _, c := range []struct{ method, path string }{
		{http.MethodPut, "/v1/malaises"},
		{http.MethodPost, "/v1/directives"},
	} {
		rec := f.do(t, c.method, c.path, "")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want 405", c.method, c.path, rec.Code)
		}
	}
}
