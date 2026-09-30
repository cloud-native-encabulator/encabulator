package journey

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/customer"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/gateway"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/rng"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/stats"
)

// countingMetrics records what the runner reports to Prometheus.
type countingMetrics struct {
	seen map[string]int
}

func (m *countingMetrics) ObserveRequest(journey, status string) {
	if m.seen == nil {
		m.seen = map[string]int{}
	}
	m.seen[journey+"/"+status]++
}

type harness struct {
	gw      *fakeGateway
	runner  *Runner
	rec     *stats.Recorder
	metrics *countingMetrics
	pool    *customer.Pool
}

func newHarness(t *testing.T, customers int, seed uint64) *harness {
	t.Helper()
	gw := newFakeGateway(t)
	client := gateway.New(gw.URL(), &http.Client{Timeout: 5 * time.Second}, nil)
	pool := customer.NewPool(client, "test-seed", customers, time.Now)
	rec := stats.NewRecorder(time.Now)
	metrics := &countingMetrics{}
	runner := NewRunner(RunnerOptions{
		Gateway: client,
		Pool:    pool,
		Stats:   rec,
		Metrics: metrics,
		Source:  rng.Seeded(seed),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Timeout: 5 * time.Second,
	})
	return &harness{gw: gw, runner: runner, rec: rec, metrics: metrics, pool: pool}
}

func TestFirstJourneyRegistersAndLogsIn(t *testing.T) {
	h := newHarness(t, 1, 1)
	h.runner.Run(context.Background(), Browse)

	got := h.gw.routeCounts()
	for _, route := range []string{
		"POST /api/v1/auth/register",
		"POST /api/v1/auth/login",
		"GET /api/v1/parts",
	} {
		if got[route] != 1 {
			t.Errorf("route %q called %d times, want 1 (all calls: %v)", route, got[route], got)
		}
	}
}

func TestRegistrationIsIdempotentAcrossJourneys(t *testing.T) {
	h := newHarness(t, 1, 2)
	for range 5 {
		h.runner.Run(context.Background(), Browse)
	}
	got := h.gw.routeCounts()
	if got["POST /api/v1/auth/register"] != 1 {
		t.Errorf("registered %d times, want once", got["POST /api/v1/auth/register"])
	}
	if got["POST /api/v1/auth/login"] != 1 {
		t.Errorf("logged in %d times, want once", got["POST /api/v1/auth/login"])
	}
	if got["GET /api/v1/parts"] != 5 {
		t.Errorf("browsed %d times, want 5", got["GET /api/v1/parts"])
	}
}

func TestEachJourneyCallsItsDesignedRoutes(t *testing.T) {
	cases := []struct {
		journey string
		want    map[string]int
	}{
		{Browse, map[string]int{"GET /api/v1/parts": 1}},
		{Quote, map[string]int{"POST /api/v1/quotes": 1}},
		{Order, map[string]int{"POST /api/v1/orders": 1}},
		{Track, map[string]int{
			"GET /api/v1/orders":              1,
			"GET /api/v1/orders/{id}":         1,
			"GET /api/v1/shipments/{orderId}": 1,
		}},
	}
	for _, c := range cases {
		t.Run(c.journey, func(t *testing.T) {
			h := newHarness(t, 1, 5)
			h.runner.Run(context.Background(), c.journey)
			got := h.gw.routeCounts()
			for route, want := range c.want {
				if got[route] != want {
					t.Errorf("route %q called %d times, want %d (all: %v)", route, got[route], want, got)
				}
			}
			if h.rec.Snapshot().Journeys[c.journey].Errors != 0 {
				t.Errorf("journey %q reported an error it should not have", c.journey)
			}
		})
	}
}

func TestUnknownJourneyIsAFailure(t *testing.T) {
	h := newHarness(t, 1, 5)
	h.runner.Run(context.Background(), "sabotage")
	if snap := h.rec.Snapshot(); snap.Journeys["sabotage"].Errors != 1 {
		t.Fatalf("unknown journey was not counted as an error: %+v", snap.Journeys)
	}
}

func TestTokenIsRefreshedOnUnauthorized(t *testing.T) {
	h := newHarness(t, 1, 3)
	h.runner.Run(context.Background(), Browse)
	if h.gw.issuedTokens() != 1 {
		t.Fatalf("issued %d tokens, want 1", h.gw.issuedTokens())
	}

	// The gateway forgets every token. The next journey must notice the 401,
	// log in again, and still succeed.
	h.gw.revokeAll()
	h.runner.Run(context.Background(), Browse)

	if h.gw.issuedTokens() != 2 {
		t.Fatalf("issued %d tokens, want 2 after the refresh", h.gw.issuedTokens())
	}
	got := h.gw.routeCounts()
	if got["GET /api/v1/parts"] != 3 {
		t.Errorf("/parts called %d times, want 3 (one ok, one 401, one retry)", got["GET /api/v1/parts"])
	}
	snap := h.rec.Snapshot()
	if snap.Statuses["401"] != 1 {
		t.Errorf("status counts %v, want one 401", snap.Statuses)
	}
	if snap.Statuses["200"] != 2 {
		t.Errorf("status counts %v, want two 200s", snap.Statuses)
	}
	// A refreshed token is not a journey failure.
	if snap.Journeys[Browse].Errors != 0 {
		t.Errorf("browse errors = %d, want 0", snap.Journeys[Browse].Errors)
	}
}

func TestTrackToleratesAnEmptyLedger(t *testing.T) {
	h := newHarness(t, 1, 4)
	h.gw.noOrders = true
	h.runner.Run(context.Background(), Track)

	got := h.gw.routeCounts()
	if got["GET /api/v1/orders"] != 1 {
		t.Errorf("order list called %d times, want 1", got["GET /api/v1/orders"])
	}
	if got["GET /api/v1/orders/{id}"] != 0 {
		t.Errorf("fetched an order detail with no orders to fetch")
	}
	if h.rec.Snapshot().Journeys[Track].Errors != 0 {
		t.Error("an empty ledger should not count as a failure")
	}
}

func TestTrackToleratesAMissingShipment(t *testing.T) {
	h := newHarness(t, 1, 4)
	h.gw.failShipments = true
	h.runner.Run(context.Background(), Track)

	snap := h.rec.Snapshot()
	if snap.Statuses["404"] != 1 {
		t.Errorf("status counts %v, want one 404", snap.Statuses)
	}
	if snap.Journeys[Track].Errors != 0 {
		t.Error("an undispatched order should not count as a failure")
	}
}

func TestServerErrorsCountAsJourneyFailures(t *testing.T) {
	h := newHarness(t, 1, 6)
	h.gw.quoteStatus = http.StatusServiceUnavailable
	h.runner.Run(context.Background(), Quote)

	snap := h.rec.Snapshot()
	if snap.Journeys[Quote].Errors != 1 {
		t.Errorf("quote errors = %d, want 1", snap.Journeys[Quote].Errors)
	}
	if snap.Statuses["503"] != 1 {
		t.Errorf("status counts %v, want one 503", snap.Statuses)
	}
	if h.metrics.seen["quote/503"] != 1 {
		t.Errorf("metrics %v, want quote/503", h.metrics.seen)
	}
}

func TestRateLimitingCountsAsAJourneyFailure(t *testing.T) {
	h := newHarness(t, 1, 6)
	h.gw.quoteStatus = http.StatusTooManyRequests
	h.runner.Run(context.Background(), Quote)
	if got := h.rec.Snapshot().Journeys[Quote].Errors; got != 1 {
		t.Errorf("quote errors = %d, want 1 for a 429", got)
	}
}

func TestBusinessRejectionsAreNotFailures(t *testing.T) {
	h := newHarness(t, 1, 6)
	h.gw.quoteStatus = http.StatusConflict
	h.runner.Run(context.Background(), Quote)
	snap := h.rec.Snapshot()
	if snap.Journeys[Quote].Errors != 0 {
		t.Errorf("quote errors = %d, want 0 for a 409", snap.Journeys[Quote].Errors)
	}
	if snap.Statuses["409"] != 1 {
		t.Errorf("status counts %v, want one 409", snap.Statuses)
	}
}

// The journey mix is the headline traffic control, so check it end to end:
// a weighted picker feeding the real runner against the fake gateway.
func TestJourneyMixMatchesWeightsThroughTheGateway(t *testing.T) {
	h := newHarness(t, 8, 99)
	picker, err := NewPicker(DefaultMix(), rng.Seeded(2024))
	if err != nil {
		t.Fatalf("NewPicker: %v", err)
	}

	const n = 4000
	for range n {
		h.runner.Run(context.Background(), picker.Pick())
	}

	// Each journey makes exactly one call to a route no other journey touches.
	marker := map[string]string{
		Browse: "GET /api/v1/parts",
		Quote:  "POST /api/v1/quotes",
		Order:  "POST /api/v1/orders",
		Track:  "GET /api/v1/orders",
	}
	routes := h.gw.routeCounts()
	for name, weight := range DefaultMix() {
		want := float64(weight) / 100
		got := float64(routes[marker[name]]) / n
		if math.Abs(got-want) > 0.02 {
			t.Errorf("journey %q ran %.3f of the time, want %.3f (routes: %v)", name, got, want, routes)
		}
	}

	snap := h.rec.Snapshot()
	total := 0
	for _, name := range Names {
		total += snap.Journeys[name].Count
		if snap.Journeys[name].Errors != 0 {
			t.Errorf("journey %q reported %d errors against a healthy gateway", name, snap.Journeys[name].Errors)
		}
	}
	if total != n {
		t.Errorf("recorded %d journeys, want %d", total, n)
	}

	// Eight customers, each registered and logged in exactly once.
	if routes["POST /api/v1/auth/register"] != 8 {
		t.Errorf("registered %d customers, want 8", routes["POST /api/v1/auth/register"])
	}
	if routes["POST /api/v1/auth/login"] != 8 {
		t.Errorf("logged in %d times, want 8", routes["POST /api/v1/auth/login"])
	}
}

func TestOrderListAcceptsBothShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"bare array", `[{"id":"ord_1"},{"id":"ord_2"}]`, 2},
		{"orders envelope", `{"orders":[{"id":"ord_1"}]}`, 1},
		{"items envelope", `{"items":[{"id":"ord_1"},{"id":"ord_2"},{"id":"ord_3"}]}`, 3},
		{"empty envelope", `{"orders":[]}`, 0},
		{"unrelated object", `{"total":0}`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var list orderList
			if err := json.Unmarshal([]byte(c.body), &list); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(list.orders) != c.want {
				t.Errorf("parsed %d orders, want %d", len(list.orders), c.want)
			}
		})
	}
}

func TestRunnerReportsTransportFailures(t *testing.T) {
	client := gateway.New("http://127.0.0.1:1", &http.Client{Timeout: 200 * time.Millisecond}, nil)
	pool := customer.NewPool(client, "seed", 1, time.Now)
	rec := stats.NewRecorder(time.Now)
	runner := NewRunner(RunnerOptions{
		Gateway: client,
		Pool:    pool,
		Stats:   rec,
		Source:  rng.Seeded(1),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Timeout: time.Second,
	})
	runner.Run(context.Background(), Browse)

	snap := rec.Snapshot()
	if snap.Journeys[Browse].Errors != 1 {
		t.Errorf("browse errors = %d, want 1", snap.Journeys[Browse].Errors)
	}
	if snap.Statuses["error"] == 0 {
		t.Errorf("status counts %v, want an 'error' entry", snap.Statuses)
	}
}
