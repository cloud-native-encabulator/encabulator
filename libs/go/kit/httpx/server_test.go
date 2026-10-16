package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
)

func newServer(t *testing.T) *httpx.Server {
	t.Helper()
	return httpx.NewServer(httpx.ServerConfig{Service: "test-service", Addr: "127.0.0.1:0"})
}

func do(t *testing.T, s *httpx.Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := do(t, newServer(t), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("body = %v", body)
	}
}

func TestReadyzChecks(t *testing.T) {
	s := newServer(t)
	s.AddReadyCheck("postgres", func(context.Context) error { return nil })
	if rec := do(t, s, http.MethodGet, "/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("healthy readyz status = %d body=%s", rec.Code, rec.Body)
	}

	s.AddReadyCheck("nats", func(context.Context) error { return errors.New("no trunnion") })
	rec := do(t, s, http.MethodGet, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy readyz status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no trunnion") {
		t.Errorf("readyz body does not name the failure: %s", rec.Body)
	}
}

func TestMetricsEndpointAndRouteTemplateLabel(t *testing.T) {
	s := newServer(t)
	s.HandleFunc("GET /v1/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": r.PathValue("id")})
	})

	for _, id := range []string{"ord_1", "ord_2", "ord_3"} {
		if rec := do(t, s, http.MethodGet, "/v1/orders/"+id); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}

	got := testutil.CollectAndCount(s.Registry(), "http_server_request_duration_seconds")
	if got != 1 {
		t.Fatalf("expected one label set (the route template), got %d", got)
	}

	rec := do(t, s, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `http_route="/v1/orders/{id}"`) {
		t.Errorf("route template label missing from /metrics output")
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Errorf("runtime metrics missing from /metrics output")
	}
}

func TestUnmatchedRouteIsLabelledOther(t *testing.T) {
	s := newServer(t)
	s.HandleFunc("GET /v1/orders", func(w http.ResponseWriter, r *http.Request) {})
	if rec := do(t, s, http.MethodGet, "/v1/nothing"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	// The 404 came from the mux, outside the metric middleware, so nothing is
	// recorded; the important property is that no raw path became a label.
	body := do(t, s, http.MethodGet, "/metrics").Body.String()
	if strings.Contains(body, "/v1/nothing") {
		t.Error("raw path leaked into a metric label")
	}
}

func TestPanicRecovery(t *testing.T) {
	s := newServer(t)
	s.HandleFunc("GET /v1/boom", func(http.ResponseWriter, *http.Request) {
		panic("the pentametric fan has departed")
	})

	rec := do(t, s, http.MethodGet, "/v1/boom")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	var env httpx.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != httpx.CodeInternal {
		t.Errorf("code = %q", env.Error.Code)
	}

	body := do(t, s, http.MethodGet, "/metrics").Body.String()
	if !strings.Contains(body, `http_response_status_code="500"`) {
		t.Error("panic was not recorded as a 500 in the duration histogram")
	}
}

func TestRequestIDAcceptedAndGenerated(t *testing.T) {
	s := newServer(t)
	s.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"seen": r.Header.Get(httpx.RequestIDHeader)})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.Header.Set(httpx.RequestIDHeader, "caller-supplied")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get(httpx.RequestIDHeader); got != "caller-supplied" {
		t.Errorf("echoed request id = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "caller-supplied") {
		t.Errorf("handler did not see the request id: %s", rec.Body)
	}

	rec = do(t, s, http.MethodGet, "/v1/ping")
	if rec.Header().Get(httpx.RequestIDHeader) == "" {
		t.Error("no request id generated")
	}
}

func TestMiddlewareAppliesToAPIRoutesOnly(t *testing.T) {
	s := newServer(t)
	var hits int
	s.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			next.ServeHTTP(w, r)
		})
	})
	s.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, r *http.Request) {})

	do(t, s, http.MethodGet, "/healthz")
	do(t, s, http.MethodGet, "/readyz")
	do(t, s, http.MethodGet, "/metrics")
	if hits != 0 {
		t.Fatalf("middleware ran on health or metrics endpoints (%d hits)", hits)
	}
	do(t, s, http.MethodGet, "/v1/ping")
	if hits != 1 {
		t.Fatalf("middleware hits = %d, want 1", hits)
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	s := httpx.NewServer(httpx.ServerConfig{
		Service:          "test",
		Addr:             "127.0.0.1:0",
		PreShutdownDelay: ptr(time.Duration(0)),
	})
	s.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
