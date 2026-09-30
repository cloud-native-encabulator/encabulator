// Package flags is the differential-girdlespring client described in design
// section 6: poll GET /v1/flags every five seconds with If-None-Match, keep the
// last good set, and fall back to built-in defaults when the service is
// unreachable at startup.
package flags

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Flag keys from design section 6.
const (
	TurboEncabulation     = "turbo-encabulation"
	QuoteCache            = "quote-cache"
	RetroEncabulatorSales = "retro-encabulator-sales"
	StrictSideFumbling    = "strict-side-fumbling"
	AmuliteFoundryRate    = "amulite-foundry-rate"
	GatewayRateLimitRPS   = "gateway-rate-limit-rps"
	DingleArmDamping      = "dingle-arm-damping"
)

// Flag types.
const (
	TypeBoolean = "boolean"
	TypeNumber  = "number"
)

// PollInterval is the flag poll period fixed by the design.
const PollInterval = 5 * time.Second

// Flag is one feature flag as the girdlespring serves it.
type Flag struct {
	Key            string          `json:"key"`
	Type           string          `json:"type"`
	Value          json.RawMessage `json:"value"`
	RolloutPercent *int            `json:"rolloutPercent,omitempty"`
	Description    string          `json:"description,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	UpdatedBy      string          `json:"updatedBy,omitempty"`
}

// Bool reads the flag as a boolean, ignoring any rollout percentage.
func (f Flag) Bool() (bool, bool) {
	var b bool
	if err := json.Unmarshal(f.Value, &b); err != nil {
		return false, false
	}
	return b, true
}

// Number reads the flag as a float64.
func (f Flag) Number() (float64, bool) {
	var n float64
	if err := json.Unmarshal(f.Value, &n); err != nil {
		return 0, false
	}
	return n, true
}

// ListResponse is the shape of GET /v1/flags.
type ListResponse struct {
	Flags []Flag `json:"flags"`
}

// Defaults are the built-in fallbacks. They match the table in design section 6
// and are the values a service uses when the girdlespring has never answered.
func Defaults() map[string]any {
	return map[string]any{
		TurboEncabulation:     false,
		QuoteCache:            true,
		RetroEncabulatorSales: true,
		StrictSideFumbling:    false,
		AmuliteFoundryRate:    float64(20),
		GatewayRateLimitRPS:   float64(50),
		DingleArmDamping:      float64(50),
	}
}

// Config configures a Client.
type Config struct {
	// BaseURL is DIFFERENTIAL_GIRDLESPRING_URL. Empty means defaults only.
	BaseURL string
	// Interval overrides the 5s poll period (tests).
	Interval time.Duration
	// Defaults overrides the built-in defaults. Missing keys fall back to
	// Defaults().
	Defaults map[string]any
	// Logger is optional.
	Logger *slog.Logger
	// HTTPClient overrides the untraced poll client (tests).
	HTTPClient *http.Client
}

// Client caches a flag set and keeps it fresh.
type Client struct {
	url      string
	interval time.Duration
	client   *http.Client
	log      *slog.Logger
	defaults map[string]any

	mu      sync.RWMutex
	flags   map[string]Flag
	etag    string
	fetched bool
}

// New builds a client. It never fails; with no URL it simply serves defaults.
func New(cfg Config) *Client {
	interval := cfg.Interval
	if interval <= 0 {
		interval = PollInterval
	}
	client := cfg.HTTPClient
	if client == nil {
		// Untraced by design: the flag poll must not litter the trace graph.
		client = &http.Client{Timeout: 2 * time.Second}
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	defaults := Defaults()
	for k, v := range cfg.Defaults {
		defaults[k] = v
	}
	return &Client{
		url:      cfg.BaseURL,
		interval: interval,
		client:   client,
		log:      log,
		defaults: defaults,
		flags:    map[string]Flag{},
	}
}

// Start fetches once, then polls until ctx is cancelled. A failed first fetch is
// logged and the client serves defaults.
func (c *Client) Start(ctx context.Context) {
	if c.url == "" {
		c.log.InfoContext(ctx, "girdlespring URL unset; using built-in flag defaults")
		return
	}
	if err := c.Refresh(ctx); err != nil {
		c.log.WarnContext(ctx, "initial flag fetch failed; using built-in defaults", slog.Any("error", err))
	}
	go func() {
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.Refresh(ctx); err != nil {
					c.log.Debug("flag refresh failed; keeping last good set", slog.Any("error", err))
				}
			}
		}
	}()
}

// Refresh fetches the flag set once, honoring the cached ETag.
func (c *Client) Refresh(ctx context.Context) error {
	if c.url == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/v1/flags", nil)
	if err != nil {
		return err
	}
	c.mu.RLock()
	etag := c.etag
	c.mu.RUnlock()
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil
	case http.StatusOK:
	default:
		return fmt.Errorf("flags: girdlespring returned %d", resp.StatusCode)
	}

	var out ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("flags: decode: %w", err)
	}
	next := make(map[string]Flag, len(out.Flags))
	for _, f := range out.Flags {
		next[f.Key] = f
	}
	c.mu.Lock()
	c.flags = next
	c.etag = resp.Header.Get("ETag")
	c.fetched = true
	c.mu.Unlock()
	return nil
}

// Fetched reports whether the girdlespring has ever answered.
func (c *Client) Fetched() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fetched
}

// Snapshot returns the current flag set.
func (c *Client) Snapshot() map[string]Flag {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]Flag, len(c.flags))
	for k, v := range c.flags {
		out[k] = v
	}
	return out
}

// Bool evaluates a boolean flag with no subject, so a rollout percentage is
// treated as "off for nobody in particular".
func (c *Client) Bool(key string) bool { return c.BoolFor(key, "") }

// BoolFor evaluates a boolean flag for a subject. A flag carrying
// rolloutPercent applies to the subjects for which
// fnv1a32(key + ":" + subject) % 100 < rolloutPercent.
func (c *Client) BoolFor(key, subject string) bool {
	f, ok := c.lookup(key)
	if !ok {
		b, _ := c.defaults[key].(bool)
		return b
	}
	v, ok := f.Bool()
	if !ok {
		b, _ := c.defaults[key].(bool)
		return b
	}
	if f.RolloutPercent == nil {
		return v
	}
	if !InRollout(key, subject, *f.RolloutPercent) {
		return !v
	}
	return v
}

// Number evaluates a numeric flag.
func (c *Client) Number(key string) float64 {
	f, ok := c.lookup(key)
	if !ok {
		return c.defaultNumber(key)
	}
	v, ok := f.Number()
	if !ok {
		return c.defaultNumber(key)
	}
	return v
}

func (c *Client) defaultNumber(key string) float64 {
	switch v := c.defaults[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}

func (c *Client) lookup(key string) (Flag, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	f, ok := c.flags[key]
	return f, ok
}

// InRollout is the rollout predicate from design section 6:
// fnv1a32(key + ":" + subject) % 100 < rolloutPercent.
func InRollout(key, subject string, rolloutPercent int) bool {
	if rolloutPercent <= 0 {
		return false
	}
	if rolloutPercent >= 100 {
		return true
	}
	h := fnv.New32a()
	// fnv.New32a never returns an error from Write.
	_, _ = h.Write([]byte(key + ":" + subject))
	return int(h.Sum32()%100) < rolloutPercent
}
