package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/config"
)

// Timeouts and limits fixed by design section 7.
const (
	ReadHeaderTimeout = 5 * time.Second
	IdleTimeout       = 60 * time.Second
	DrainTimeout      = 20 * time.Second
)

// ReadyCheck reports whether one of a service's own hard dependencies is
// reachable. It must never call a downstream service.
type ReadyCheck func(context.Context) error

// ServerConfig configures a Server. Only Service is required.
type ServerConfig struct {
	// Service is the service name, used for span and metric provenance.
	Service string
	// Addr is the listen address. Empty means ":" + PORT (default 8080).
	Addr string
	// Logger receives lifecycle and panic messages.
	Logger *slog.Logger
	// Registry holds the server's metrics. Empty means a fresh registry with the
	// Go runtime and process collectors.
	Registry *prometheus.Registry
	// PreShutdownDelay is how long readiness reports failure before the listener
	// closes, so that load balancers can take the pod out of rotation. Default 1s.
	PreShutdownDelay *time.Duration
}

// Server is the one HTTP port every service exposes: its API, /healthz,
// /readyz and /metrics.
type Server struct {
	service string
	addr    string
	log     *slog.Logger
	reg     *prometheus.Registry

	mux         *http.ServeMux
	middlewares []Middleware

	mu     sync.RWMutex
	checks map[string]ReadyCheck

	shuttingDown atomic.Bool
	preShutdown  time.Duration
}

// NewServer builds a server with the health, readiness and metrics endpoints
// already mounted.
func NewServer(cfg ServerConfig) *Server {
	addr := cfg.Addr
	if addr == "" {
		addr = ":" + defaultPort()
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	reg := cfg.Registry
	if reg == nil {
		reg = NewRegistry()
	}
	delay := time.Second
	if cfg.PreShutdownDelay != nil {
		delay = *cfg.PreShutdownDelay
	}

	s := &Server{
		service:     cfg.Service,
		addr:        addr,
		log:         log,
		reg:         reg,
		mux:         http.NewServeMux(),
		checks:      map[string]ReadyCheck{},
		preShutdown: delay,
	}

	// Health, readiness and metrics are deliberately untraced and unmetered.
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	s.mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	}))

	return s
}

// Registry exposes the metric registry so services can add their encabulator_*
// domain metrics.
func (s *Server) Registry() *prometheus.Registry { return s.reg }

// Addr returns the listen address.
func (s *Server) Addr() string { return s.addr }

// Use adds middleware applied to API routes only (not to health or metrics),
// inside the router so that the route template is known. The first middleware
// added runs outermost.
func (s *Server) Use(mw ...Middleware) {
	s.middlewares = append(s.middlewares, mw...)
}

// Handle registers an API route. The pattern is a Go 1.22 ServeMux pattern such
// as "POST /v1/orders" or "GET /v1/orders/{id}"; its path becomes the http_route
// label and the server span name.
func (s *Server) Handle(pattern string, h http.Handler) {
	route := routeFromPattern(pattern)
	wrapped := Chain(h, append([]Middleware{Metrics(s.reg), Recover(s.log)}, s.middlewares...)...)
	s.mux.Handle(pattern, otelhttp.NewHandler(wrapped, route,
		otelhttp.WithSpanNameFormatter(func(string, *http.Request) string { return pattern }),
	))
}

// HandleFunc is Handle for a function.
func (s *Server) HandleFunc(pattern string, h http.HandlerFunc) { s.Handle(pattern, h) }

// AddReadyCheck registers a readiness probe for one of the service's own hard
// dependencies.
func (s *Server) AddReadyCheck(name string, check ReadyCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks[name] = check
}

// Handler returns the fully wrapped handler, for httptest and for callers that
// embed the server in something else.
func (s *Server) Handler() http.Handler {
	return Chain(s.mux, RequestID(), Recover(s.log))
}

// Run serves until ctx is cancelled, then fails readiness, stops accepting, and
// drains for up to DrainTimeout before returning.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: ReadHeaderTimeout,
		IdleTimeout:       IdleTimeout,
		BaseContext:       func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("httpx: listen %s: %w", s.addr, err)
	}
	s.log.InfoContext(ctx, "listening", slog.String("addr", ln.Addr().String()))

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.shuttingDown.Store(true)
	s.log.Info("shutting down; failing readiness before draining",
		slog.Duration("pre_shutdown", s.preShutdown), slog.Duration("drain", DrainTimeout))
	if s.preShutdown > 0 {
		time.Sleep(s.preShutdown)
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DrainTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("httpx: shutdown: %w", err)
	}
	return <-errCh
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.shuttingDown.Load() {
		WriteJSON(w, http.StatusServiceUnavailable, readyResponse{Status: "shutting_down"})
		return
	}

	s.mu.RLock()
	checks := make(map[string]ReadyCheck, len(s.checks))
	for name, fn := range s.checks {
		checks[name] = fn
	}
	s.mu.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	results := make(map[string]string, len(checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	ok := true
	for name, fn := range checks {
		wg.Add(1)
		go func(name string, fn ReadyCheck) {
			defer wg.Done()
			msg := "ok"
			if err := fn(ctx); err != nil {
				msg = err.Error()
				mu.Lock()
				ok = false
				mu.Unlock()
			}
			mu.Lock()
			results[name] = msg
			mu.Unlock()
		}(name, fn)
	}
	wg.Wait()

	if !ok {
		WriteJSON(w, http.StatusServiceUnavailable, readyResponse{Status: "unavailable", Checks: results})
		return
	}
	WriteJSON(w, http.StatusOK, readyResponse{Status: "ok", Checks: results})
}

func defaultPort() string {
	return strconv.Itoa(config.Int("PORT", 8080))
}

// routeFromPattern extracts the path part of a ServeMux pattern.
func routeFromPattern(pattern string) string {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '/' {
			return pattern[i:]
		}
	}
	return pattern
}
