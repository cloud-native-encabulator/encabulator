package gateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/identity"

	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/dingle-arm-gateway/internal/auth"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/dingle-arm-gateway/internal/gateway"
)

// upstream records what the gateway forwarded.
type upstream struct {
	name string
	srv  *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
	status   int
	delay    time.Duration
	stream   bool
}

type recordedRequest struct {
	Method  string
	Path    string
	Query   string
	Body    string
	Headers http.Header
}

func newUpstream(t *testing.T, name string) *upstream {
	t.Helper()
	u := &upstream{name: name, status: http.StatusOK}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.requests = append(u.requests, recordedRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Body: string(body), Headers: r.Header.Clone(),
		})
		status, delay, stream := u.status, u.delay, u.stream
		u.mu.Unlock()

		if delay > 0 {
			time.Sleep(delay)
		}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(status)
			flusher, _ := w.(http.Flusher)
			for i := range 3 {
				_, _ = fmt.Fprintf(w, "chunk %d\n", i)
				if flusher != nil {
					flusher.Flush()
				}
			}
			return
		}
		httpx.WriteJSON(w, status, map[string]string{"upstream": u.name, "path": r.URL.Path})
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) last(t *testing.T) recordedRequest {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.requests) == 0 {
		t.Fatalf("upstream %s received no requests", u.name)
	}
	return u.requests[len(u.requests)-1]
}

func (u *upstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func (u *upstream) setStatus(status int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status = status
}

// fakeVerifier accepts a fixed token map.
type fakeVerifier struct {
	tokens map[string]identity.Identity
	err    error
}

func (f *fakeVerifier) Verify(_ context.Context, authorization string) (identity.Identity, error) {
	if f.err != nil {
		return identity.Identity{}, f.err
	}
	token, err := auth.BearerToken(authorization)
	if err != nil {
		return identity.Identity{}, err
	}
	id, ok := f.tokens[token]
	if !ok {
		return identity.Identity{}, auth.ErrInvalidToken
	}
	return id, nil
}

// fakeLimiter allows a fixed number of requests, then rejects.
type fakeLimiter struct {
	mu      sync.Mutex
	allowed map[string]int
	budget  int
}

func newFakeLimiter(budget int) *fakeLimiter {
	return &fakeLimiter{allowed: map[string]int{}, budget: budget}
}

func (f *fakeLimiter) Allow(key string) (bool, time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.allowed[key] >= f.budget {
		return false, 2 * time.Second
	}
	f.allowed[key]++
	return true, 0
}

func (f *fakeLimiter) keys() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, v := range f.allowed {
		out[k] = v
	}
	return out
}

type harness struct {
	srv       *httpx.Server
	upstreams map[string]*upstream
	verifier  *fakeVerifier
	limiter   *fakeLimiter
	registry  *prometheus.Registry
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		upstreams: map[string]*upstream{},
		verifier: &fakeVerifier{tokens: map[string]identity.Identity{
			"wendy-token": {Subject: "wendy", Roles: []string{identity.RoleCustomer}},
			"opal-token":  {Subject: "opal", Roles: []string{identity.RoleOperator}},
			"empty-roles": {Subject: "nobody"},
		}},
		limiter:  newFakeLimiter(1000),
		registry: prometheus.NewRegistry(),
	}

	urls := map[string]string{}
	for _, name := range gateway.Upstreams() {
		u := newUpstream(t, name)
		h.upstreams[name] = u
		urls[name] = u.srv.URL
	}

	gw, err := gateway.New(gateway.Config{
		Upstreams: urls,
		Verifier:  h.verifier,
		Limiter:   h.limiter,
		Logger:    slog.New(slog.DiscardHandler),
		Registry:  h.registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	h.srv = httpx.NewServer(httpx.ServerConfig{
		Service: "dingle-arm-gateway", Addr: "127.0.0.1:0",
		Logger: slog.New(slog.DiscardHandler), Registry: h.registry,
	})
	gw.Routes(h.srv)
	return h
}

func (h *harness) do(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestNewRequiresEveryUpstream(t *testing.T) {
	_, err := gateway.New(gateway.Config{
		Upstreams: map[string]string{"unilateral-phase-detractor": "http://detractor:8080"},
		Verifier:  &fakeVerifier{},
		Limiter:   newFakeLimiter(10),
	})
	if err == nil {
		t.Fatal("expected an error with upstreams missing")
	}
	if !strings.Contains(err.Error(), "no base URL") {
		t.Errorf("error = %v", err)
	}

	if _, err := gateway.New(gateway.Config{Verifier: &fakeVerifier{}}); err == nil {
		t.Error("expected an error with no limiter")
	}
	if _, err := gateway.New(gateway.Config{Limiter: newFakeLimiter(1)}); err == nil {
		t.Error("expected an error with no verifier")
	}
}

func TestRouteTableMatchesTheDesign(t *testing.T) {
	want := map[string]string{
		"POST /api/v1/auth/login":         "unilateral-phase-detractor /v1/auth/login",
		"POST /api/v1/auth/register":      "unilateral-phase-detractor /v1/auth/register",
		"GET /api/v1/users/me":            "unilateral-phase-detractor /v1/users/me",
		"GET /api/v1/parts":               "prefabulated-amulite-depot /v1/parts",
		"POST /api/v1/quotes":             "spurving-bearing-quoter /v1/quotes",
		"POST /api/v1/orders":             "panametric-fam-processor /v1/orders",
		"GET /api/v1/orders":              "hydrocoptic-datastore /v1/orders",
		"GET /api/v1/orders/{id}":         "hydrocoptic-datastore /v1/orders/{id}",
		"GET /api/v1/shipments/{orderId}": "nofer-trunnion-dispatch /v1/shipments/{orderId}",
	}
	routes := gateway.Routes()
	if len(routes) != len(want) {
		t.Fatalf("%d routes, want %d", len(routes), len(want))
	}
	for _, r := range routes {
		got := r.Upstream + " " + r.UpstreamPath
		if want[r.Pattern] != got {
			t.Errorf("%s -> %s, want %s", r.Pattern, got, want[r.Pattern])
		}
	}
}

func TestPublicRoutesNeedNoToken(t *testing.T) {
	h := newHarness(t)

	for _, tc := range []struct {
		method, path, upstream, upstreamPath string
	}{
		{http.MethodPost, "/api/v1/auth/login", "unilateral-phase-detractor", "/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/register", "unilateral-phase-detractor", "/v1/auth/register"},
		{http.MethodGet, "/api/v1/parts", "prefabulated-amulite-depot", "/v1/parts"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"username":"wendy"}`))
		rec := h.do(t, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s -> %d body=%s", tc.method, tc.path, rec.Code, rec.Body)
		}
		got := h.upstreams[tc.upstream].last(t)
		if got.Path != tc.upstreamPath {
			t.Errorf("%s forwarded to %s, want %s", tc.path, got.Path, tc.upstreamPath)
		}
		if got.Method != tc.method {
			t.Errorf("method = %s", got.Method)
		}
	}
}

func TestAuthenticatedRoutesRequireAToken(t *testing.T) {
	h := newHarness(t)

	for _, target := range []string{
		"/api/v1/users/me", "/api/v1/orders", "/api/v1/orders/ord_1", "/api/v1/shipments/ord_1",
	} {
		rec := h.do(t, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with no token -> %d, want 401", target, rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("GET %s: no WWW-Authenticate header", target)
		}
	}

	rec := h.do(t, httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/v1/orders with no token -> %d", rec.Code)
	}

	// A token the verifier does not know is also a 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer forged")
	if rec := h.do(t, req); rec.Code != http.StatusUnauthorized {
		t.Errorf("forged token -> %d", rec.Code)
	}
}

func TestTokenWithNoRolesIsForbidden(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer empty-roles")
	rec := h.do(t, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var env httpx.ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != httpx.CodeForbidden {
		t.Errorf("code = %q", env.Error.Code)
	}
}

func TestUnavailableKeysIs503(t *testing.T) {
	h := newHarness(t)
	h.verifier.err = auth.ErrKeysUnavailable

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer wendy-token")
	if rec := h.do(t, req); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestIdentityHeadersAreSetAndForgeriesStripped(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer wendy-token")
	// A client trying to promote itself.
	req.Header.Set(identity.HeaderSubject, "admin")
	req.Header.Set(identity.HeaderRoles, "admin")
	req.Header.Set("X-Encabulator-Anything", "nonsense")

	if rec := h.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}

	got := h.upstreams["hydrocoptic-datastore"].last(t)
	if got.Headers.Get(identity.HeaderSubject) != "wendy" {
		t.Errorf("subject header = %q, want wendy", got.Headers.Get(identity.HeaderSubject))
	}
	if got.Headers.Get(identity.HeaderRoles) != "customer" {
		t.Errorf("roles header = %q, want customer", got.Headers.Get(identity.HeaderRoles))
	}
	if got.Headers.Get("X-Encabulator-Anything") != "" {
		t.Error("a forged X-Encabulator-* header reached the upstream")
	}
	if got.Headers.Get("Authorization") != "" {
		t.Error("the bearer token was forwarded to an internal service")
	}
}

func TestPublicRoutesSendNoIdentity(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"wendy"}`))
	req.Header.Set(identity.HeaderSubject, "admin")
	req.Header.Set(identity.HeaderRoles, "admin")
	if rec := h.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	got := h.upstreams["unilateral-phase-detractor"].last(t)
	if got.Headers.Get(identity.HeaderSubject) != "" || got.Headers.Get(identity.HeaderRoles) != "" {
		t.Errorf("a public route forwarded identity headers: %v", got.Headers)
	}
	if got.Body != `{"username":"wendy"}` {
		t.Errorf("body = %q", got.Body)
	}
}

func TestProxyPreservesPathQueryAndBody(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders?status=fabricating&limit=5", nil)
	req.Header.Set("Authorization", "Bearer opal-token")
	if rec := h.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got := h.upstreams["hydrocoptic-datastore"].last(t)
	if got.Path != "/v1/orders" {
		t.Errorf("path = %q", got.Path)
	}
	if got.Query != "status=fabricating&limit=5" {
		t.Errorf("query = %q", got.Query)
	}

	body := `{"model":"turbo","quantity":3,"options":{"dingleArm":true}}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer wendy-token")
	req.Header.Set("Content-Type", "application/json")
	if rec := h.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got = h.upstreams["spurving-bearing-quoter"].last(t)
	if got.Body != body {
		t.Errorf("body = %q, want %q", got.Body, body)
	}
	if got.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", got.Headers.Get("Content-Type"))
	}
}

func TestWildcardPathsAreSubstituted(t *testing.T) {
	h := newHarness(t)

	for _, tc := range []struct {
		inbound, upstream, wantPath string
	}{
		{"/api/v1/orders/ord_01JABCDEF", "hydrocoptic-datastore", "/v1/orders/ord_01JABCDEF"},
		{"/api/v1/shipments/ord_01JABCDEF", "nofer-trunnion-dispatch", "/v1/shipments/ord_01JABCDEF"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.inbound, nil)
		req.Header.Set("Authorization", "Bearer wendy-token")
		if rec := h.do(t, req); rec.Code != http.StatusOK {
			t.Fatalf("GET %s -> %d", tc.inbound, rec.Code)
		}
		if got := h.upstreams[tc.upstream].last(t); got.Path != tc.wantPath {
			t.Errorf("GET %s forwarded to %q, want %q", tc.inbound, got.Path, tc.wantPath)
		}
	}
}

func TestUnknownRoutesAreNotProxied(t *testing.T) {
	h := newHarness(t)

	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/api/v1/users"},                   // admin route: not exposed
		{http.MethodDelete, "/api/v1/orders/ord_1"},         // wrong method
		{http.MethodGet, "/v1/orders"},                      // missing the /api prefix
		{http.MethodGet, "/api/v1/flags"},                   // girdlespring is internal
		{http.MethodPost, "/api/v1/parts/AMU-BASE/restock"}, // operator route: internal
	} {
		req := httptest.NewRequest(tc.method, tc.target, nil)
		req.Header.Set("Authorization", "Bearer opal-token")
		rec := h.do(t, req)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s -> %d, want 404 or 405", tc.method, tc.target, rec.Code)
		}
	}
	for name, u := range h.upstreams {
		if u.count() != 0 {
			t.Errorf("upstream %s received %d requests for unrouted paths", name, u.count())
		}
	}
}

func TestRateLimitKeysOnSubjectThenClientAddress(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer wendy-token")
	h.do(t, req)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	h.do(t, req)

	keys := h.limiter.keys()
	if keys["wendy"] != 1 {
		t.Errorf("authenticated traffic was not keyed on the subject: %v", keys)
	}
	if keys["203.0.113.7"] != 1 {
		t.Errorf("public traffic was not keyed on the first X-Forwarded-For hop: %v", keys)
	}
}

func TestOverTheLimitIs429WithRetryAfter(t *testing.T) {
	h := newHarness(t)
	h.limiter.budget = 1

	req := httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil)
	req.RemoteAddr = "198.51.100.9:54321"
	if rec := h.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("first request -> %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil)
	req.RemoteAddr = "198.51.100.9:54321"
	rec := h.do(t, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request -> %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "2" {
		t.Errorf("Retry-After = %q, want 2", rec.Header().Get("Retry-After"))
	}
	var env httpx.ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != httpx.CodeRateLimited {
		t.Errorf("code = %q", env.Error.Code)
	}
	if h.upstreams["prefabulated-amulite-depot"].count() != 1 {
		t.Error("a rate-limited request still reached the upstream")
	}
}

func TestUpstreamStatusIsPassedThrough(t *testing.T) {
	h := newHarness(t)
	h.upstreams["prefabulated-amulite-depot"].setStatus(http.StatusConflict)

	rec := h.do(t, httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want the upstream's 409", rec.Code)
	}
}

func TestUnreachableUpstreamIs502(t *testing.T) {
	h := newHarness(t)
	// Close the depot so the connection is refused.
	h.upstreams["prefabulated-amulite-depot"].srv.Close()

	rec := h.do(t, httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var env httpx.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "upstream_unavailable" {
		t.Errorf("code = %q", env.Error.Code)
	}
}

func TestStreamingResponsesArePassedThrough(t *testing.T) {
	h := newHarness(t)
	depot := h.upstreams["prefabulated-amulite-depot"]
	depot.mu.Lock()
	depot.stream = true
	depot.mu.Unlock()

	rec := h.do(t, httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "chunk 0") || !strings.Contains(rec.Body.String(), "chunk 2") {
		t.Errorf("streamed body = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestMetrics(t *testing.T) {
	h := newHarness(t)
	h.limiter.budget = 1

	req := httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil)
	req.RemoteAddr = "198.51.100.9:1"
	h.do(t, req)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil)
	req.RemoteAddr = "198.51.100.9:1"
	h.do(t, req)

	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	for _, want := range []string{
		`encabulator_gateway_requests_total{route="/api/v1/parts",status="200"} 1`,
		`encabulator_gateway_requests_total{route="/api/v1/parts",status="429"} 1`,
		"encabulator_gateway_rate_limited_total 1",
		`server_address="prefabulated-amulite-depot"`,
		`http_route="/api/v1/parts"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics is missing %q", want)
		}
	}
	if strings.Contains(body, "127.0.0.1:") {
		t.Error("an upstream host:port leaked into a metric label")
	}
}

func TestRequestIDIsForwarded(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil)
	req.Header.Set(httpx.RequestIDHeader, "caller-supplied-id")
	if rec := h.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := h.upstreams["prefabulated-amulite-depot"].last(t); got.Headers.Get(httpx.RequestIDHeader) != "caller-supplied-id" {
		t.Errorf("X-Request-Id = %q", got.Headers.Get(httpx.RequestIDHeader))
	}
}

func TestXForwardedForIsAppended(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/parts", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.RemoteAddr = "10.0.0.5:4321"
	if rec := h.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got := h.upstreams["prefabulated-amulite-depot"].last(t)
	xff := got.Headers.Get("X-Forwarded-For")
	if !strings.HasPrefix(xff, "203.0.113.7") {
		t.Errorf("X-Forwarded-For = %q; the original hop should come first", xff)
	}
}

func TestRouteLabelIsAlwaysATemplate(t *testing.T) {
	for _, r := range gateway.Routes() {
		if !strings.HasPrefix(r.Label(), "/api/") {
			t.Errorf("route %q has label %q", r.Pattern, r.Label())
		}
		if strings.Contains(r.Label(), " ") {
			t.Errorf("label %q contains the method", r.Label())
		}
	}
	// The wildcard stays a wildcard, so cardinality stays bounded.
	var found bool
	for _, r := range gateway.Routes() {
		if r.Label() == "/api/v1/orders/{id}" {
			found = true
		}
	}
	if !found {
		t.Error("the parameterized order route lost its template label")
	}
}

func TestVerifierErrorsAreNotLeaked(t *testing.T) {
	h := newHarness(t)
	h.verifier.err = errors.New("the detractor's private key smells of ozone")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer wendy-token")
	rec := h.do(t, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "ozone") {
		t.Error("the verifier's internal error leaked to the client")
	}
}

func TestUpstreamsListIsDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, u := range gateway.Upstreams() {
		if seen[u] {
			t.Errorf("upstream %q listed twice", u)
		}
		seen[u] = true
	}
	if len(seen) != 6 {
		t.Errorf("%d upstreams, want 6", len(seen))
	}
}
