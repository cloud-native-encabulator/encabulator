package httpx_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/identity"
)

func TestClientJSONRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(identity.HeaderSubject) != "wendy" {
			t.Errorf("identity header missing: %v", r.Header)
		}
		var in map[string]any
		_ = httpx.DecodeJSON(r, &in, 0)
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{"echo": in["model"]})
	}))
	defer srv.Close()

	c, err := httpx.NewClient(httpx.ClientConfig{Service: "spurving-bearing-quoter", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	status, err := c.JSON(context.Background(), http.MethodPost, "/v1/quotes",
		map[string]any{"model": "turbo"}, &out,
		httpx.WithIdentity(identity.Identity{Subject: "wendy", Roles: []string{"customer"}}))
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusCreated || out["echo"] != "turbo" {
		t.Fatalf("status=%d out=%v", status, out)
	}
}

func TestClientRetriesIdempotentMethods(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "later")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	c, err := httpx.NewClient(httpx.ClientConfig{Service: "datastore", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.JSON(context.Background(), http.MethodGet, "/v1/orders", nil, nil); err != nil {
		t.Fatalf("GET should have succeeded on the third attempt: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3 (1 + 2 retries)", got)
	}
}

func TestClientDoesNotRetryPlainPost(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "later")
	}))
	defer srv.Close()

	c, _ := httpx.NewClient(httpx.ClientConfig{Service: "datastore", BaseURL: srv.URL})
	_, err := c.JSON(context.Background(), http.MethodPost, "/v1/orders", map[string]string{"a": "b"}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1 (POST is not idempotent)", got)
	}
}

func TestClientRetriesPostWithIdempotencyKey(t *testing.T) {
	var calls atomic.Int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		bodies = append(bodies, string(buf[:n]))
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	c, _ := httpx.NewClient(httpx.ClientConfig{Service: "datastore", BaseURL: srv.URL})
	_, err := c.JSON(context.Background(), http.MethodPost, "/v1/orders",
		map[string]string{"id": "ord_1"}, nil, httpx.WithIdempotencyKey("ord_1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatalf("retry did not replay the body: %q", bodies)
	}
}

func TestClientMapsUpstreamErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusConflict, "insufficient_parts", "Out of marzlevanes")
	}))
	defer srv.Close()

	c, _ := httpx.NewClient(httpx.ClientConfig{Service: "prefabulated-amulite-depot", BaseURL: srv.URL})
	status, err := c.JSON(context.Background(), http.MethodPost, "/v1/reservations", map[string]int{"q": 1}, nil)
	if status != http.StatusConflict {
		t.Fatalf("status = %d", status)
	}
	se, ok := httpx.AsStatusError(err)
	if !ok {
		t.Fatalf("err is not a StatusError: %v", err)
	}
	if se.Code != "insufficient_parts" || se.Status != http.StatusConflict {
		t.Errorf("mapped error = %+v", se)
	}
}

func TestClientMetricUsesServiceNameAsServerAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{})
	}))
	defer srv.Close()

	reg := prometheus.NewRegistry()
	c, _ := httpx.NewClient(httpx.ClientConfig{
		Service: "hydrocoptic-datastore", BaseURL: srv.URL, Registry: reg,
	})
	if _, err := c.JSON(context.Background(), http.MethodGet, "/v1/orders", nil, nil); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	metricsHandler(reg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `server_address="hydrocoptic-datastore"`) {
		t.Errorf("server_address label is not the service name:\n%s", body)
	}
	if strings.Contains(body, "127.0.0.1") {
		t.Errorf("host:port leaked into the metric labels:\n%s", body)
	}
}

func TestTwoClientsShareOneHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	if _, err := httpx.NewClient(httpx.ClientConfig{Service: "a", BaseURL: "http://a", Registry: reg}); err != nil {
		t.Fatal(err)
	}
	if _, err := httpx.NewClient(httpx.ClientConfig{Service: "b", BaseURL: "http://b", Registry: reg}); err != nil {
		t.Fatalf("second client must reuse the registered histogram: %v", err)
	}
}

func metricsHandler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
