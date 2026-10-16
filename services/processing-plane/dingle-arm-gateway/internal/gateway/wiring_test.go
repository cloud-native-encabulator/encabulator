package gateway_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/identity"

	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/auth"
	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/gateway"
	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/ratelimit"
)

// TestRealVerifierAndLimiterTogether wires the gateway the way main.go does: the
// real JWKS verifier against a detractor that actually signs tokens, and the real
// token bucket reading its rate from a function, as the flag client supplies.
func TestRealVerifierAndLimiterTogether(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "detractor-1"

	// A detractor that publishes its JWKS and proxies nothing else.
	detractor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/jwks.json" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}}})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"upstream": "unilateral-phase-detractor"})
	}))
	defer detractor.Close()

	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"path":    r.URL.Path,
			"subject": r.Header.Get(identity.HeaderSubject),
			"roles":   r.Header.Get(identity.HeaderRoles),
		})
	}))
	defer echo.Close()

	urls := map[string]string{}
	for _, name := range gateway.Upstreams() {
		urls[name] = echo.URL
	}
	urls["unilateral-phase-detractor"] = detractor.URL

	verifier, err := auth.NewVerifier(auth.Config{
		JWKSURL: detractor.URL + "/.well-known/jwks.json",
		Logger:  slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	verifier.Start(ctx)

	// Two requests per second, so the burst is four and the fifth is rejected.
	limiter := ratelimit.New(func() float64 { return 2 }, 100)

	registry := prometheus.NewRegistry()
	gw, err := gateway.New(gateway.Config{
		Upstreams: urls,
		Verifier:  verifier,
		Limiter:   limiter,
		Logger:    slog.New(slog.DiscardHandler),
		Registry:  registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httpx.NewServer(httpx.ServerConfig{
		Service: "dingle-arm-gateway", Addr: "127.0.0.1:0",
		Logger: slog.New(slog.DiscardHandler), Registry: registry,
	})
	gw.Routes(srv)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "wendy", "roles": []string{"customer"},
		"iss": auth.Issuer, "aud": auth.Audience,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	get := func(target, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	// A genuine token gets through, and the identity headers reach the upstream.
	rec := get("/api/v1/orders/ord_1", signed)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var echoed map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &echoed); err != nil {
		t.Fatal(err)
	}
	if echoed["path"] != "/v1/orders/ord_1" {
		t.Errorf("upstream path = %q", echoed["path"])
	}
	if echoed["subject"] != "wendy" || echoed["roles"] != "customer" {
		t.Errorf("identity = %v", echoed)
	}

	// A token signed by somebody else does not.
	stranger, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "attacker", "roles": []string{"admin"},
		"iss": auth.Issuer, "aud": auth.Audience,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	forged.Header["kid"] = kid
	forgedSigned, err := forged.SignedString(stranger)
	if err != nil {
		t.Fatal(err)
	}
	if rec := get("/api/v1/orders/ord_1", forgedSigned); rec.Code != http.StatusUnauthorized {
		t.Errorf("forged token -> %d, want 401", rec.Code)
	}

	// The real bucket: three more get through (four total in the burst), then 429.
	allowed := 1
	var limited int
	for range 10 {
		if rec := get("/api/v1/orders", signed); rec.Code == http.StatusOK {
			allowed++
		} else if rec.Code == http.StatusTooManyRequests {
			limited++
			if rec.Header().Get("Retry-After") == "" {
				t.Error("429 carried no Retry-After")
			}
		} else {
			t.Fatalf("unexpected status %d", rec.Code)
		}
	}
	if allowed != 4 {
		t.Errorf("%d requests allowed, want a burst of 4 at 2 rps", allowed)
	}
	if limited == 0 {
		t.Error("the real limiter never rejected anything")
	}
}
