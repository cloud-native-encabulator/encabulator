// Package gateway is a thin JSON client for the dingle-arm-gateway, the only
// public entry point a synthetic customer is allowed to touch.
//
// It deliberately does not retry. The grammeter's job is to offer a known number
// of requests per second and report honestly what came back; a retry would both
// distort the offered rate and hide the failure the demo is trying to show.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxBody caps how much of a response we will read. Gateway responses are
// small; anything larger is a bug and we do not want it in memory.
const maxBody = 1 << 20

// Observer records the conventional outbound HTTP metric. It may be nil.
type Observer interface {
	ObserveClientRequest(method string, status int, d time.Duration)
}

// Client talks to one fixed gateway base URL. It is safe for concurrent use.
type Client struct {
	base string
	http *http.Client
	obs  Observer
	now  func() time.Time
}

// New returns a client for base, which must be an absolute URL such as
// http://dingle-arm-gateway:8080. The supplied http.Client carries the timeout
// and the traced transport; when nil, a plain client is used. obs may be nil.
func New(base string, hc *http.Client, obs Observer) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{base: strings.TrimRight(base, "/"), http: hc, obs: obs, now: time.Now}
}

// Base returns the configured gateway URL.
func (c *Client) Base() string { return c.base }

// Request describes one call to the gateway.
type Request struct {
	Method string
	// Path is an absolute path such as /api/v1/orders.
	Path  string
	Query url.Values
	// Body, when non-nil, is marshalled as JSON.
	Body any
	// Token, when non-empty, is sent as a bearer token.
	Token string
	// Out, when non-nil, receives the decoded JSON response body on 2xx.
	Out any
}

// Do performs the request. The returned status is 0 when the request never
// reached a server, in which case err is non-nil. A non-2xx status is *not*
// an error: callers decide what a 409 or a 401 means for them.
func (c *Client) Do(ctx context.Context, r Request) (int, error) {
	var body io.Reader
	if r.Body != nil {
		encoded, err := json.Marshal(r.Body)
		if err != nil {
			return 0, fmt.Errorf("encode request body: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	target := c.base + r.Path
	if len(r.Query) > 0 {
		target += "?" + r.Query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, r.Method, target, body)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}

	start := c.now()
	resp, err := c.http.Do(req)
	elapsed := c.now().Sub(start)
	if err != nil {
		c.observe(r.Method, 0, elapsed)
		return 0, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		_ = resp.Body.Close()
	}()
	c.observe(r.Method, resp.StatusCode, elapsed)

	if r.Out == nil || resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(r.Out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}
	return resp.StatusCode, nil
}

func (c *Client) observe(method string, status int, d time.Duration) {
	if c.obs != nil {
		c.obs.ObserveClientRequest(method, status, d)
	}
}
