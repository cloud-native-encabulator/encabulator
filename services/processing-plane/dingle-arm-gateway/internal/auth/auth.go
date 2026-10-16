// Package auth verifies the RS256 tokens the unilateral phase detractor issues,
// against its published JWKS (design 8.1).
package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/identity"
)

// Token expectations fixed by design section 8.1.
const (
	Issuer    = "unilateral-phase-detractor"
	Audience  = "encabulator"
	Algorithm = "RS256"
)

// Refresh cadence.
const (
	// RefreshInterval is the background JWKS refresh period.
	RefreshInterval = 5 * time.Minute
	// MinRefreshInterval throttles the on-demand refresh an unknown kid triggers,
	// so a stream of bogus tokens cannot turn into a stream of JWKS fetches.
	MinRefreshInterval = 10 * time.Second
)

// Errors the gateway turns into responses.
var (
	// ErrNoToken means the request carried no bearer token.
	ErrNoToken = errors.New("no bearer token")
	// ErrInvalidToken means the token failed verification.
	ErrInvalidToken = errors.New("invalid token")
	// ErrUnknownKey means the token's kid is not in the JWKS.
	ErrUnknownKey = errors.New("unknown signing key")
	// ErrKeysUnavailable means the JWKS has never been fetched.
	ErrKeysUnavailable = errors.New("signing keys unavailable")
)

// Config configures a Verifier.
type Config struct {
	// JWKSURL is ${UNILATERAL_PHASE_DETRACTOR_URL}/.well-known/jwks.json.
	JWKSURL string
	// Logger is optional.
	Logger *slog.Logger
	// HTTPClient overrides the JWKS fetch client.
	HTTPClient *http.Client
	// RefreshInterval overrides the 5 minute background refresh (tests).
	RefreshInterval time.Duration
	// MinRefreshInterval overrides the on-demand refresh throttle (tests).
	MinRefreshInterval time.Duration
}

// Verifier caches the detractor's public keys and verifies tokens against them.
type Verifier struct {
	url                string
	client             *http.Client
	log                *slog.Logger
	refreshInterval    time.Duration
	minRefreshInterval time.Duration

	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	lastAttempt time.Time
	fetched     bool

	// refreshMu serializes fetches so a burst of unknown kids causes one request.
	refreshMu sync.Mutex
}

// NewVerifier builds a Verifier. It does not touch the network.
func NewVerifier(cfg Config) (*Verifier, error) {
	if cfg.JWKSURL == "" {
		return nil, errors.New("auth: JWKS URL is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	refresh := cfg.RefreshInterval
	if refresh <= 0 {
		refresh = RefreshInterval
	}
	minRefresh := cfg.MinRefreshInterval
	if minRefresh <= 0 {
		minRefresh = MinRefreshInterval
	}
	return &Verifier{
		url:                cfg.JWKSURL,
		client:             client,
		log:                log,
		refreshInterval:    refresh,
		minRefreshInterval: minRefresh,
		keys:               map[string]*rsa.PublicKey{},
	}, nil
}

// Start fetches the JWKS once, then refreshes it on a timer until ctx is
// cancelled. A failed first fetch is logged, not fatal: the detractor may simply
// not be up yet.
func (v *Verifier) Start(ctx context.Context) {
	if err := v.Refresh(ctx); err != nil {
		v.log.WarnContext(ctx, "initial JWKS fetch failed; tokens will be rejected until it succeeds",
			slog.Any("error", err))
	}
	go func() {
		t := time.NewTicker(v.refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := v.Refresh(ctx); err != nil {
					v.log.Warn("JWKS refresh failed; keeping the cached keys", slog.Any("error", err))
				}
			}
		}
	}()
}

// jwks is the subset of a JWK set the gateway needs.
type jwks struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// Refresh fetches the JWKS and replaces the cache. On failure the previous keys
// stay in place.
func (v *Verifier) Refresh(ctx context.Context) error {
	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()

	v.mu.Lock()
	v.lastAttempt = time.Now()
	v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("auth: fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth: JWKS returned %d", resp.StatusCode)
	}

	var set jwks
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("auth: decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (k.Alg != "" && k.Alg != Algorithm) {
			continue
		}
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			v.log.Warn("skipping unusable JWKS entry", slog.String("kid", k.Kid), slog.Any("error", err))
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return errors.New("auth: JWKS contained no usable RSA signing keys")
	}

	v.mu.Lock()
	v.keys = keys
	v.fetched = true
	v.mu.Unlock()
	v.log.Debug("JWKS refreshed", slog.Int("keys", len(keys)))
	return nil
}

// KeyCount reports how many signing keys are cached.
func (v *Verifier) KeyCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.keys)
}

// Verify checks an Authorization header value and returns the caller's identity.
func (v *Verifier) Verify(ctx context.Context, authorization string) (identity.Identity, error) {
	raw, err := BearerToken(authorization)
	if err != nil {
		return identity.Identity{}, err
	}

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(raw, claims, v.keyFunc(ctx),
		jwt.WithValidMethods([]string{Algorithm}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(Audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, ErrUnknownKey) || errors.Is(err, ErrKeysUnavailable) {
			return identity.Identity{}, err
		}
		return identity.Identity{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	subject, _ := claims["sub"].(string)
	if strings.TrimSpace(subject) == "" {
		return identity.Identity{}, fmt.Errorf("%w: token has no sub", ErrInvalidToken)
	}
	return identity.Identity{Subject: subject, Roles: rolesFromClaims(claims)}, nil
}

// keyFunc resolves the token's kid, refreshing the JWKS once for a kid it has
// not seen, because the detractor rotates keys without telling anyone.
func (v *Verifier) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)

		if key, ok := v.lookup(kid); ok {
			return key, nil
		}
		if !v.mayRefresh() {
			if !v.hasKeys() {
				return nil, ErrKeysUnavailable
			}
			return nil, fmt.Errorf("%w: kid %q", ErrUnknownKey, kid)
		}
		if err := v.Refresh(ctx); err != nil {
			v.log.WarnContext(ctx, "JWKS refresh for an unknown kid failed",
				slog.String("kid", kid), slog.Any("error", err))
		}
		if key, ok := v.lookup(kid); ok {
			return key, nil
		}
		if !v.hasKeys() {
			return nil, ErrKeysUnavailable
		}
		return nil, fmt.Errorf("%w: kid %q", ErrUnknownKey, kid)
	}
}

func (v *Verifier) lookup(kid string) (*rsa.PublicKey, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	// A JWKS with exactly one key is allowed to omit kid on the token.
	if kid == "" && len(v.keys) == 1 {
		for _, key := range v.keys {
			return key, true
		}
	}
	key, ok := v.keys[kid]
	return key, ok
}

func (v *Verifier) hasKeys() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.fetched && len(v.keys) > 0
}

func (v *Verifier) mayRefresh() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return time.Since(v.lastAttempt) >= v.minRefreshInterval
}

// BearerToken extracts the token from an Authorization header value.
func BearerToken(authorization string) (string, error) {
	authorization = strings.TrimSpace(authorization)
	if authorization == "" {
		return "", ErrNoToken
	}
	scheme, token, found := strings.Cut(authorization, " ")
	if !found || !strings.EqualFold(scheme, "bearer") || strings.TrimSpace(token) == "" {
		return "", ErrNoToken
	}
	return strings.TrimSpace(token), nil
}

// rolesFromClaims reads the roles claim, which is always an array of strings.
func rolesFromClaims(claims jwt.MapClaims) []string {
	switch v := claims["roles"].(type) {
	case []any:
		roles := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				roles = append(roles, s)
			}
		}
		return identity.ParseRoles(strings.Join(roles, ","))
	case []string:
		return identity.ParseRoles(strings.Join(v, ","))
	case string:
		// Tolerated, not expected: a comma-separated string.
		return identity.ParseRoles(v)
	default:
		return nil
	}
}

// rsaPublicKey rebuilds an RSA public key from a JWK's base64url modulus and
// exponent.
func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(nB64, "="))
	if err != nil {
		return nil, fmt.Errorf("modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(eB64, "="))
	if err != nil {
		return nil, fmt.Errorf("exponent: %w", err)
	}
	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, errors.New("empty modulus or exponent")
	}
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() <= 0 || e.Int64() > 1<<31 {
		return nil, errors.New("exponent out of range")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(e.Int64())}, nil
}
