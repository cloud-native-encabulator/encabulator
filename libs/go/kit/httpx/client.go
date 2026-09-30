package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/identity"
)

// Outbound defaults from design section 7.
const (
	ClientTimeout    = 3 * time.Second
	ClientRetries    = 2
	RetryBaseBackoff = 100 * time.Millisecond
)

// IdempotencyKeyHeader lets a caller opt a POST into the retry policy.
const IdempotencyKeyHeader = "Idempotency-Key"

// ClientConfig configures an outbound client aimed at one upstream service.
type ClientConfig struct {
	// Service is the upstream service name, for example "hydrocoptic-datastore".
	// It becomes the server_address metric label and the span's peer.service.
	Service string
	// BaseURL is the upstream base URL. Relative request paths are joined to it.
	BaseURL string
	// Timeout is the per-attempt timeout. Default 3s.
	Timeout time.Duration
	// Retries is the number of retries for idempotent requests. Default 2.
	Retries int
	// Registry receives http_client_request_duration_seconds. Optional.
	Registry prometheus.Registerer
	// Logger is optional.
	Logger *slog.Logger
	// Transport overrides the base round tripper (tests).
	Transport http.RoundTripper
}

// Client is an HTTP client with OTel client spans, retries for idempotent
// requests, and the conventional client duration metric.
type Client struct {
	service string
	base    *url.URL
	http    *http.Client
	retries int
	hist    *prometheus.HistogramVec
	log     *slog.Logger
}

// NewClient builds a client. BaseURL may be empty if every call uses an
// absolute URL.
func NewClient(cfg ClientConfig) (*Client, error) {
	var base *url.URL
	if cfg.BaseURL != "" {
		u, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
		if err != nil {
			return nil, fmt.Errorf("httpx: base url %q: %w", cfg.BaseURL, err)
		}
		base = u
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = ClientTimeout
	}
	retries := cfg.Retries
	if retries == 0 {
		retries = ClientRetries
	}
	if retries < 0 {
		retries = 0
	}
	rt := cfg.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	name := cfg.Service
	if name == "" && base != nil {
		name = base.Hostname()
	}

	return &Client{
		service: name,
		base:    base,
		retries: retries,
		hist:    clientDuration(cfg.Registry),
		log:     cfg.Logger,
		http: &http.Client{
			Timeout: timeout,
			Transport: otelhttp.NewTransport(rt,
				otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
					return r.Method + " " + name
				}),
				otelhttp.WithSpanOptions(trace.WithAttributes(
					attribute.String("peer.service", name),
				)),
			),
			// Callers handle redirects explicitly; a proxy must not follow them.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// Service returns the upstream service name.
func (c *Client) Service() string { return c.service }

// URL resolves a path against the client's base URL.
func (c *Client) URL(path string) string {
	if c.base == nil || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if path == "" {
		return c.base.String()
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.base.String() + path
}

// NewRequest builds a request against the client's base URL. body may be nil.
func (c *Client) NewRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL(path), rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	return req, nil
}

// Do sends req, retrying idempotent requests (GET, HEAD, PUT, DELETE, OPTIONS)
// and POSTs that carry an Idempotency-Key. Retries use exponential backoff with
// full jitter from a 100ms base. The caller closes the response body.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	attempts := 1
	if c.retryable(req) {
		attempts += c.retries
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(req.Context(), backoff(attempt)); err != nil {
				return nil, err
			}
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}
		}

		start := time.Now()
		resp, err := c.http.Do(req)
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		c.observe(req.Method, status, time.Since(start))

		if err != nil {
			lastErr = err
			if attempt == attempts-1 {
				return nil, fmt.Errorf("httpx: %s %s: %w", req.Method, c.service, err)
			}
			continue
		}
		if attempt < attempts-1 && retryableStatus(resp.StatusCode) {
			// Drain so the connection can be reused, then try again.
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("upstream %s returned %d", c.service, resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("httpx: %s %s: %w", req.Method, c.service, lastErr)
}

// JSON sends a JSON request and decodes a JSON response. out may be nil. It
// returns the status code. A non-2xx status with a standard error envelope is
// returned as a *StatusError.
func (c *Client) JSON(ctx context.Context, method, path string, in, out any, opts ...RequestOption) (int, error) {
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, fmt.Errorf("httpx: encode request: %w", err)
		}
		body = b
	}
	req, err := c.NewRequest(ctx, method, path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	for _, opt := range opts {
		opt(req)
	}

	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("httpx: read response from %s: %w", c.service, err)
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, upstreamError(c.service, resp.StatusCode, payload)
	}
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return resp.StatusCode, fmt.Errorf("httpx: decode response from %s: %w", c.service, err)
		}
	}
	return resp.StatusCode, nil
}

// RequestOption mutates an outbound request before it is sent.
type RequestOption func(*http.Request)

// WithHeader sets a header.
func WithHeader(name, value string) RequestOption {
	return func(r *http.Request) { r.Header.Set(name, value) }
}

// WithIdentity stamps the identity propagation headers on the outbound request.
func WithIdentity(id identity.Identity) RequestOption {
	return func(r *http.Request) { id.Apply(r.Header) }
}

// WithIdempotencyKey sets Idempotency-Key, which makes a POST retryable.
func WithIdempotencyKey(key string) RequestOption {
	return func(r *http.Request) { r.Header.Set(IdempotencyKeyHeader, key) }
}

// WithQuery sets the raw query string.
func WithQuery(q url.Values) RequestOption {
	return func(r *http.Request) { r.URL.RawQuery = q.Encode() }
}

func (c *Client) retryable(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	case http.MethodPost:
		return req.Header.Get(IdempotencyKeyHeader) != ""
	default:
		return false
	}
}

func (c *Client) observe(method string, status int, d time.Duration) {
	if c.hist == nil {
		return
	}
	code := "error"
	if status > 0 {
		code = strconv.Itoa(status)
	}
	c.hist.WithLabelValues(method, c.service, code).Observe(d.Seconds())
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// backoff returns a full-jitter exponential delay: rand(0, base * 2^(n-1)).
func backoff(attempt int) time.Duration {
	ceiling := RetryBaseBackoff << (attempt - 1)
	if ceiling > 2*time.Second {
		ceiling = 2 * time.Second
	}
	return time.Duration(rand.Int64N(int64(ceiling) + 1))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// upstreamError turns a non-2xx upstream response into a *StatusError, keeping
// the upstream code when it sent the standard envelope.
func upstreamError(service string, status int, payload []byte) error {
	var env ErrorEnvelope
	if err := json.Unmarshal(payload, &env); err == nil && env.Error.Code != "" {
		return &StatusError{Status: status, Code: env.Error.Code, Message: env.Error.Message}
	}
	return &StatusError{
		Status:  status,
		Code:    CodeUpstreamFailed,
		Message: fmt.Sprintf("%s returned %d", service, status),
	}
}

// AsStatusError reports whether err carries an HTTP status, and returns it.
func AsStatusError(err error) (*StatusError, bool) {
	var se *StatusError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}
