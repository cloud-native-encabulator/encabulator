// Package customer maintains the pool of synthetic customers. Each one has a
// stable username, a password derived from a secret seed, and a bearer token it
// renews on demand.
package customer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/gateway"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/rng"
)

// MaxCustomers bounds the pool so a stray API call cannot exhaust the detractor.
const MaxCustomers = 1000

// renewBefore is how long before expiry a token is treated as stale.
const renewBefore = 60 * time.Second

// ErrAuth reports that a customer could not obtain a token.
var ErrAuth = errors.New("authentication failed")

// Username returns the stable username for the nth synthetic customer. It fits
// the detractor's rules: 3 to 64 characters of [a-z0-9._-].
func Username(n int) string { return fmt.Sprintf("grammeter-%04d", n) }

// Password derives a customer's password from the pool seed. The seed is a
// secret, so the password never appears in configuration or logs.
func Password(seed, username string) string {
	mac := hmac.New(sha256.New, []byte(seed))
	mac.Write([]byte("cardinal-grammeter/v1\x00"))
	mac.Write([]byte(username))
	// Base64 keeps the alphabet wide without risking characters that some
	// validator might reject; 32 characters is plenty of entropy.
	return "G1-" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:32]
}

// Customer is one synthetic shopper.
type Customer struct {
	name     string
	password string
	gw       *gateway.Client

	mu         sync.Mutex
	token      string
	expiresAt  time.Time
	registered bool
}

// Name returns the customer's username.
func (c *Customer) Name() string { return c.name }

// loginResponse is the detractor's reply, relayed by the gateway.
type loginResponse struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
}

// Token returns a usable bearer token, registering and logging in when needed.
// Concurrent callers for the same customer share one authentication attempt.
func (c *Customer) Token(ctx context.Context, now func() time.Time) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && now().Before(c.expiresAt.Add(-renewBefore)) {
		return c.token, nil
	}

	if !c.registered {
		if err := c.register(ctx); err != nil {
			return "", err
		}
		c.registered = true
	}

	var out loginResponse
	status, err := c.gw.Do(ctx, gateway.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/auth/login",
		Body:   map[string]string{"username": c.name, "password": c.password},
		Out:    &out,
	})
	if err != nil {
		return "", fmt.Errorf("%w: login %s: %w", ErrAuth, c.name, err)
	}
	if status != http.StatusOK || out.AccessToken == "" {
		return "", fmt.Errorf("%w: login %s returned %d", ErrAuth, c.name, status)
	}

	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	c.token = out.AccessToken
	c.expiresAt = now().Add(ttl)
	return c.token, nil
}

// register creates the customer, tolerating the common case that a previous
// run already did. 409 and 422 both mean "the name is taken", which for a
// deterministic username is the same as success.
func (c *Customer) register(ctx context.Context) error {
	status, err := c.gw.Do(ctx, gateway.Request{
		Method: http.MethodPost,
		Path:   "/api/v1/auth/register",
		Body: map[string]string{
			"username":    c.name,
			"password":    c.password,
			"displayName": "Synthetic Customer " + c.name[len("grammeter-"):],
		},
	})
	if err != nil {
		return fmt.Errorf("%w: register %s: %w", ErrAuth, c.name, err)
	}
	switch {
	case status >= 200 && status <= 299:
		return nil
	case status == http.StatusConflict, status == http.StatusUnprocessableEntity:
		return nil
	default:
		return fmt.Errorf("%w: register %s returned %d", ErrAuth, c.name, status)
	}
}

// Invalidate discards the token if it is still the one the caller used. This is
// how a 401 from any journey triggers exactly one re-login.
func (c *Customer) Invalidate(stale string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == stale {
		c.token = ""
		c.expiresAt = time.Time{}
	}
}

// Pool is a resizable set of synthetic customers.
type Pool struct {
	gw   *gateway.Client
	seed string
	now  func() time.Time

	mu        sync.RWMutex
	customers []*Customer
	active    int
}

// NewPool builds a pool of size customers. now may be nil, meaning time.Now.
func NewPool(gw *gateway.Client, seed string, size int, now func() time.Time) *Pool {
	if now == nil {
		now = time.Now
	}
	p := &Pool{gw: gw, seed: seed, now: now}
	p.Resize(size)
	return p
}

// Resize grows or shrinks the active pool. Shrinking retires customers without
// forgetting them, so growing again reuses the same accounts and tokens.
func (p *Pool) Resize(size int) {
	if size < 1 {
		size = 1
	}
	if size > MaxCustomers {
		size = MaxCustomers
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.customers) < size {
		name := Username(len(p.customers) + 1)
		p.customers = append(p.customers, &Customer{
			name:     name,
			password: Password(p.seed, name),
			gw:       p.gw,
		})
	}
	p.active = size
}

// Size returns the current active pool size.
func (p *Pool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// Pick returns an active customer at random.
func (p *Pool) Pick(source rng.Source) *Customer {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.active == 0 {
		return nil
	}
	return p.customers[source.IntN(p.active)]
}

// Now exposes the pool's clock so journeys can share it.
func (p *Pool) Now() time.Time { return p.now() }

// Clock returns the pool's clock function.
func (p *Pool) Clock() func() time.Time { return p.now }
