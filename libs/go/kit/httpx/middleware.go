package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/logging"
)

// RequestIDHeader carries the correlation id accepted from, or generated for,
// every inbound request.
const RequestIDHeader = "X-Request-Id"

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so that the first listed runs outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		if mws[i] != nil {
			h = mws[i](h)
		}
	}
	return h
}

// RequestID accepts an inbound X-Request-Id or generates one, echoes it on the
// response, and puts it in the request context for logging.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := strings.TrimSpace(r.Header.Get(RequestIDHeader))
			if id == "" || len(id) > 128 {
				id = uuid.NewString()
			}
			w.Header().Set(RequestIDHeader, id)
			r.Header.Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
		})
	}
}

// Recover turns a panicking handler into a 500 with the standard error envelope
// and an error log line including the stack.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &recorder{ResponseWriter: w}
			defer func() {
				if p := recover(); p != nil {
					if log != nil {
						log.ErrorContext(r.Context(), "handler panicked; the encabulator has lost its ambifacient alignment",
							slog.Any("panic", p),
							slog.String("http_route", RouteOf(r)),
							slog.String("stack", string(debug.Stack())),
						)
					}
					if !rec.wrote {
						WriteError(rec, http.StatusInternalServerError, CodeInternal, "Internal error")
					}
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// Metrics observes http_server_request_duration_seconds. It must run *inside*
// the router so that http.Request.Pattern holds the matched route template.
func Metrics(reg prometheus.Registerer) Middleware {
	hist := serverDuration(reg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w}
			defer func() {
				hist.WithLabelValues(
					r.Method,
					RouteOf(r),
					strconv.Itoa(rec.status()),
				).Observe(time.Since(start).Seconds())
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// RouteOf returns the matched route template, never the raw path. Requests that
// did not match a pattern report "other" so that the label stays bounded.
func RouteOf(r *http.Request) string {
	p := r.Pattern
	if p == "" {
		return "other"
	}
	// Patterns are registered as "METHOD /path"; the label wants just the path.
	if i := strings.IndexByte(p, '/'); i > 0 {
		return p[i:]
	}
	return p
}

// recorder captures the status code and whether anything was written.
type recorder struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (rw *recorder) WriteHeader(code int) {
	if !rw.wrote {
		rw.code = code
		rw.wrote = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *recorder) Write(b []byte) (int, error) {
	if !rw.wrote {
		rw.code = http.StatusOK
		rw.wrote = true
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *recorder) status() int {
	if rw.code == 0 {
		return http.StatusOK
	}
	return rw.code
}

// Flush and Unwrap keep streaming proxies and http.ResponseController working
// through the wrapper.
func (rw *recorder) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *recorder) Unwrap() http.ResponseWriter { return rw.ResponseWriter }
