// Package gateway is the drawn reciprocation dingle arm: the only public entry
// point for customer traffic (design 8.1). It authenticates, rate limits, and
// proxies to exactly the nine upstream operations in the routing table.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/identity"

	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/auth"
)

// DefaultUpstreamTimeout bounds how long the gateway waits for an upstream's
// response headers. It is longer than the kit's 3s RPC default because the
// processor's order saga legitimately takes several hops.
const DefaultUpstreamTimeout = 10 * time.Second

// Verifier is the token verification the gateway needs.
type Verifier interface {
	Verify(ctx context.Context, authorization string) (identity.Identity, error)
}

// Limiter is the rate limiting the gateway needs.
type Limiter interface {
	Allow(key string) (bool, time.Duration)
}

// Config configures a Gateway.
type Config struct {
	// Upstreams maps a service name to its base URL.
	Upstreams map[string]string
	// Verifier checks bearer tokens.
	Verifier Verifier
	// Limiter applies the token bucket.
	Limiter Limiter
	// UpstreamTimeout bounds the wait for upstream response headers.
	UpstreamTimeout time.Duration
	// Logger is optional.
	Logger *slog.Logger
	// Registry receives the gateway's metrics.
	Registry prometheus.Registerer
	// Transport overrides the base round tripper (tests).
	Transport http.RoundTripper
}

// Gateway holds one reverse proxy per route.
type Gateway struct {
	log      *slog.Logger
	verifier Verifier
	limiter  Limiter

	routes  []Route
	proxies map[string]*httputil.ReverseProxy

	requests    *prometheus.CounterVec
	rateLimited prometheus.Counter
}

// New builds the gateway. Every upstream named in the routing table must have a
// base URL, so a missing environment variable fails at startup rather than at the
// first request.
func New(cfg Config) (*Gateway, error) {
	if cfg.Verifier == nil {
		return nil, errors.New("gateway: a verifier is required")
	}
	if cfg.Limiter == nil {
		return nil, errors.New("gateway: a limiter is required")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	timeout := cfg.UpstreamTimeout
	if timeout <= 0 {
		timeout = DefaultUpstreamTimeout
	}

	g := &Gateway{
		log:      log,
		verifier: cfg.Verifier,
		limiter:  cfg.Limiter,
		routes:   Routes(),
		proxies:  map[string]*httputil.ReverseProxy{},
		requests: httpx.RegisterCounterVec(cfg.Registry, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "encabulator_gateway_requests_total",
			Help: "Requests through the dingle arm, by route and response status.",
		}, []string{"route", "status"})),
	}
	g.rateLimited = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "encabulator_gateway_rate_limited_total",
		Help: "Requests rejected by the token bucket.",
	})
	if cfg.Registry != nil {
		if err := cfg.Registry.Register(g.rateLimited); err != nil {
			var already prometheus.AlreadyRegisteredError
			if !errors.As(err, &already) {
				return nil, err
			}
			if existing, ok := already.ExistingCollector.(prometheus.Counter); ok {
				g.rateLimited = existing
			}
		}
	}

	// One transport per upstream keeps connection pools and the client duration
	// metric separated by target service.
	clientDuration := registerClientDuration(cfg.Registry)
	transports := map[string]http.RoundTripper{}
	for _, service := range Upstreams() {
		base := strings.TrimRight(cfg.Upstreams[service], "/")
		if base == "" {
			return nil, fmt.Errorf("gateway: no base URL for upstream %s", service)
		}
		target, err := url.Parse(base)
		if err != nil {
			return nil, fmt.Errorf("gateway: upstream %s URL %q: %w", service, base, err)
		}
		if target.Scheme == "" || target.Host == "" {
			return nil, fmt.Errorf("gateway: upstream %s URL %q needs a scheme and host", service, base)
		}
		transports[service] = newTransport(service, timeout, cfg.Transport, clientDuration)
	}

	for _, route := range g.routes {
		target, err := url.Parse(strings.TrimRight(cfg.Upstreams[route.Upstream], "/"))
		if err != nil {
			return nil, err
		}
		g.proxies[route.Pattern] = g.newProxy(route, target, transports[route.Upstream])
	}
	return g, nil
}

// Routes registers every proxied route on the kit's HTTP server.
func (g *Gateway) Routes(s *httpx.Server) {
	for _, route := range g.routes {
		s.Handle(route.Pattern, g.handler(route))
	}
}

// handler authenticates, rate limits, then proxies.
func (g *Gateway) handler(route Route) http.Handler {
	proxy := g.proxies[route.Pattern]
	label := route.Label()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		defer func() { g.requests.WithLabelValues(label, strconv.Itoa(rec.status())).Inc() }()

		// Whatever the client sent, it does not get to claim an identity.
		identity.Strip(r.Header)

		var caller identity.Identity
		if route.Access == Authenticated {
			var err error
			caller, err = g.verifier.Verify(r.Context(), r.Header.Get("Authorization"))
			if err != nil {
				g.writeAuthError(rec, r, err)
				return
			}
			if len(caller.Roles) == 0 {
				httpx.WriteError(rec, http.StatusForbidden, httpx.CodeForbidden,
					"This token carries no roles")
				return
			}
		}

		// Authenticated traffic is limited per subject; public traffic per client.
		key := caller.Subject
		if key == "" {
			key = clientAddress(r)
		}
		if ok, retryAfter := g.limiter.Allow(key); !ok {
			g.rateLimited.Inc()
			rec.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			httpx.WriteError(rec, http.StatusTooManyRequests, httpx.CodeRateLimited,
				"The dingle arm is reciprocating as fast as it can; slow down")
			return
		}

		if route.Access == Authenticated {
			r = r.WithContext(identity.NewContext(r.Context(), caller))
		}
		proxy.ServeHTTP(rec, r)
	})
}

// newProxy builds the reverse proxy for one route. It preserves the method,
// query and body, rewrites the path to the upstream's, and stamps the identity.
func (g *Gateway) newProxy(route Route, target *url.URL, transport http.RoundTripper) *httputil.ReverseProxy {
	label := route.Label()
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.URL.Path = resolvePath(route.UpstreamPath, pr.In)
			pr.Out.URL.RawPath = ""
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			pr.Out.Host = target.Host

			// SetXForwarded replaces the inbound chain with the peer address;
			// keep the chain and append, so an upstream sees the whole path and
			// the hop the gateway rate-limited on stays first.
			inboundForwardedFor := pr.In.Header.Get("X-Forwarded-For")
			pr.SetXForwarded()
			if inboundForwardedFor != "" {
				pr.Out.Header.Set("X-Forwarded-For",
					inboundForwardedFor+", "+pr.Out.Header.Get("X-Forwarded-For"))
			}

			// Internal services trust the identity headers, so the bearer token
			// itself never needs to leave the edge.
			pr.Out.Header.Del("Authorization")
			identity.Strip(pr.Out.Header)
			if caller := identity.FromContext(pr.In.Context()); caller.Subject != "" {
				caller.Apply(pr.Out.Header)
			}
			if id := pr.In.Header.Get(httpx.RequestIDHeader); id != "" {
				pr.Out.Header.Set(httpx.RequestIDHeader, id)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status, code := http.StatusBadGateway, "upstream_unavailable"
			if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
				status, code = http.StatusGatewayTimeout, "upstream_timeout"
			}
			g.log.ErrorContext(r.Context(), "upstream call failed",
				slog.String("route", label),
				slog.String("upstream", route.Upstream),
				slog.Any("error", err))
			httpx.WriteError(w, status, code,
				fmt.Sprintf("%s did not answer", route.Upstream))
		},
		// Stream responses rather than buffering them.
		FlushInterval: 100 * time.Millisecond,
	}
}

func (g *Gateway) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="encabulator"`)
	switch {
	case errors.Is(err, auth.ErrNoToken):
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized,
			"A bearer token is required")
	case errors.Is(err, auth.ErrKeysUnavailable):
		g.log.ErrorContext(r.Context(), "cannot verify tokens: no signing keys cached")
		httpx.WriteError(w, http.StatusServiceUnavailable, httpx.CodeUnavailable,
			"Token verification is unavailable")
	default:
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized,
			"The token was not accepted")
	}
}

// clientAddress is the first X-Forwarded-For hop, or the peer address.
func clientAddress(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// newTransport wraps the base round tripper with the client duration metric and
// OTel client spans naming the target service.
func newTransport(service string, timeout time.Duration, base http.RoundTripper, hist *prometheus.HistogramVec) http.RoundTripper {
	if base == nil {
		base = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   16,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: time.Second,
			ResponseHeaderTimeout: timeout,
			ForceAttemptHTTP2:     true,
		}
	}
	instrumented := otelhttp.NewTransport(base,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + service
		}),
		otelhttp.WithSpanOptions(trace.WithAttributes(attribute.String("peer.service", service))),
	)
	return &measuringTransport{service: service, next: instrumented, hist: hist}
}

// measuringTransport records http_client_request_duration_seconds with
// server_address set to the upstream service name rather than host:port.
type measuringTransport struct {
	service string
	next    http.RoundTripper
	hist    *prometheus.HistogramVec
}

func (t *measuringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	if t.hist != nil {
		code := "error"
		if resp != nil {
			code = strconv.Itoa(resp.StatusCode)
		}
		t.hist.WithLabelValues(req.Method, t.service, code).Observe(time.Since(start).Seconds())
	}
	return resp, err
}

func registerClientDuration(reg prometheus.Registerer) *prometheus.HistogramVec {
	hist := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_client_request_duration_seconds",
		Help:    "Duration of outbound HTTP requests.",
		Buckets: httpx.DurationBuckets,
	}, []string{"http_request_method", "server_address", "http_response_status_code"})
	if reg == nil {
		return hist
	}
	if err := reg.Register(hist); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			if existing, ok := already.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing
			}
		}
		return hist
	}
	return hist
}

// statusRecorder captures the status code and keeps streaming working.
type statusRecorder struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (rw *statusRecorder) WriteHeader(code int) {
	if !rw.wrote {
		rw.code = code
		rw.wrote = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *statusRecorder) Write(b []byte) (int, error) {
	if !rw.wrote {
		rw.code = http.StatusOK
		rw.wrote = true
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *statusRecorder) status() int {
	if rw.code == 0 {
		return http.StatusOK
	}
	return rw.code
}

func (rw *statusRecorder) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *statusRecorder) Unwrap() http.ResponseWriter { return rw.ResponseWriter }
