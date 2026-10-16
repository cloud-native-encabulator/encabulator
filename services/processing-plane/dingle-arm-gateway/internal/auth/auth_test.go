package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/auth"
)

// detractor is a stand-in for the unilateral phase detractor's JWKS endpoint.
type detractor struct {
	keys    map[string]*rsa.PrivateKey
	fetches atomic.Int32
	fail    atomic.Bool
	srv     *httptest.Server
}

func newDetractor(t *testing.T, kids ...string) *detractor {
	t.Helper()
	d := &detractor{keys: map[string]*rsa.PrivateKey{}}
	for _, kid := range kids {
		d.keys[kid] = newKey(t)
	}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.fetches.Add(1)
		if d.fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		type jwk struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		}
		out := struct {
			Keys []jwk `json:"keys"`
		}{}
		for kid, key := range d.keys {
			out.Keys = append(out.Keys, jwk{
				Kty: "RSA", Kid: kid, Alg: "RS256", Use: "sig",
				N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func (d *detractor) url() string { return d.srv.URL + "/.well-known/jwks.json" }

func (d *detractor) addKey(t *testing.T, kid string) {
	t.Helper()
	d.keys[kid] = newKey(t)
}

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

type claimOverrides struct {
	subject   string
	roles     any
	issuer    string
	audience  any
	expiresAt *time.Time
	method    jwt.SigningMethod
	kid       string
	signWith  *rsa.PrivateKey
}

func (d *detractor) sign(t *testing.T, kid string, o claimOverrides) string {
	t.Helper()
	key := d.keys[kid]
	if o.signWith != nil {
		key = o.signWith
	}
	if key == nil {
		t.Fatalf("no key for kid %q", kid)
	}

	claims := jwt.MapClaims{}
	claims["sub"] = or(o.subject, "wendy")
	if o.roles != nil {
		claims["roles"] = o.roles
	} else {
		claims["roles"] = []string{"customer"}
	}
	claims["iss"] = or(o.issuer, auth.Issuer)
	if o.audience != nil {
		claims["aud"] = o.audience
	} else {
		claims["aud"] = auth.Audience
	}
	exp := time.Now().Add(time.Hour)
	if o.expiresAt != nil {
		exp = *o.expiresAt
	}
	claims["exp"] = exp.Unix()
	claims["iat"] = time.Now().Add(-time.Minute).Unix()

	method := o.method
	if method == nil {
		method = jwt.SigningMethodRS256
	}
	token := jwt.NewWithClaims(method, claims)
	headerKid := kid
	if o.kid != "" {
		headerKid = o.kid
	}
	if headerKid != "-" {
		token.Header["kid"] = headerKid
	}

	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func newVerifier(t *testing.T, d *detractor) *auth.Verifier {
	t.Helper()
	v, err := auth.NewVerifier(auth.Config{
		JWKSURL:            d.url(),
		MinRefreshInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVerifyAcceptsAValidToken(t *testing.T) {
	d := newDetractor(t, "k1")
	v := newVerifier(t, d)
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	token := d.sign(t, "k1", claimOverrides{subject: "wendy", roles: []string{"customer", "viewer"}})
	id, err := v.Verify(context.Background(), "Bearer "+token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Subject != "wendy" {
		t.Errorf("subject = %q", id.Subject)
	}
	if !id.HasRole("customer") || !id.HasRole("viewer") {
		t.Errorf("roles = %v", id.Roles)
	}
}

func TestVerifyRejects(t *testing.T) {
	d := newDetractor(t, "k1")
	v := newVerifier(t, d)
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)

	cases := []struct {
		name string
		o    claimOverrides
	}{
		{"wrong issuer", claimOverrides{issuer: "some-other-service"}},
		{"wrong audience", claimOverrides{audience: "somebody-else"}},
		{"expired", claimOverrides{expiresAt: &past}},
		{"no subject", claimOverrides{subject: " "}},
		{"signed by a stranger", claimOverrides{signWith: newKey(t)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := d.sign(t, "k1", tc.o)
			if _, err := v.Verify(context.Background(), "Bearer "+token); err == nil {
				t.Fatal("expected verification to fail")
			}
		})
	}
}

func TestVerifyRejectsNoneAlgorithm(t *testing.T) {
	d := newDetractor(t, "k1")
	v := newVerifier(t, d)
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The classic attack: re-sign the claims with alg "none".
	claims := jwt.MapClaims{
		"sub": "attacker", "roles": []string{"admin"},
		"iss": auth.Issuer, "aud": auth.Audience,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	token.Header["kid"] = "k1"
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), "Bearer "+signed); err == nil {
		t.Fatal("a token with alg none must be rejected")
	}
}

func TestVerifyRejectsHS256SignedWithTheModulus(t *testing.T) {
	d := newDetractor(t, "k1")
	v := newVerifier(t, d)
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	claims := jwt.MapClaims{
		"sub": "attacker", "roles": []string{"admin"},
		"iss": auth.Issuer, "aud": auth.Audience,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = "k1"
	signed, err := token.SignedString(d.keys["k1"].N.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), "Bearer "+signed); err == nil {
		t.Fatal("an algorithm-confusion token must be rejected")
	}
}

func TestBearerTokenParsing(t *testing.T) {
	for _, header := range []string{"", "   ", "Basic abc", "Bearer", "Bearer   ", "token abc"} {
		if _, err := auth.BearerToken(header); !errors.Is(err, auth.ErrNoToken) {
			t.Errorf("BearerToken(%q) = %v, want ErrNoToken", header, err)
		}
	}
	got, err := auth.BearerToken("bearer   abc.def.ghi  ")
	if err != nil || got != "abc.def.ghi" {
		t.Errorf("BearerToken = %q, %v", got, err)
	}
}

func TestVerifyWithoutAnyKeys(t *testing.T) {
	d := newDetractor(t, "k1")
	d.fail.Store(true)
	v := newVerifier(t, d)
	// Refresh fails, so nothing is cached.
	if err := v.Refresh(context.Background()); err == nil {
		t.Fatal("expected the refresh to fail")
	}

	d.fail.Store(false)
	token := d.sign(t, "k1", claimOverrides{})
	d.fail.Store(true)

	_, err := v.Verify(context.Background(), "Bearer "+token)
	if !errors.Is(err, auth.ErrKeysUnavailable) {
		t.Fatalf("err = %v, want ErrKeysUnavailable", err)
	}
}

func TestUnknownKidTriggersOneRefresh(t *testing.T) {
	d := newDetractor(t, "k1")
	v := newVerifier(t, d)
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := d.fetches.Load()

	// The detractor rotates its key without telling anybody.
	d.addKey(t, "k2")
	token := d.sign(t, "k2", claimOverrides{subject: "otto"})

	id, err := v.Verify(context.Background(), "Bearer "+token)
	if err != nil {
		t.Fatalf("Verify after rotation: %v", err)
	}
	if id.Subject != "otto" {
		t.Errorf("subject = %q", id.Subject)
	}
	if d.fetches.Load() <= before {
		t.Error("an unknown kid did not trigger a JWKS refresh")
	}
	if v.KeyCount() != 2 {
		t.Errorf("cached keys = %d, want 2", v.KeyCount())
	}
}

func TestUnknownKidRefreshIsThrottled(t *testing.T) {
	d := newDetractor(t, "k1")
	v, err := auth.NewVerifier(auth.Config{
		JWKSURL:            d.url(),
		MinRefreshInterval: time.Hour, // never refresh again during this test
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := d.fetches.Load()

	stranger := newKey(t)
	for range 20 {
		token := d.sign(t, "k1", claimOverrides{kid: "unknown-kid", signWith: stranger})
		if _, err := v.Verify(context.Background(), "Bearer "+token); !errors.Is(err, auth.ErrUnknownKey) {
			t.Fatalf("err = %v, want ErrUnknownKey", err)
		}
	}
	if got := d.fetches.Load(); got != before {
		t.Errorf("a burst of unknown kids caused %d extra JWKS fetches", got-before)
	}
}

func TestRefreshKeepsCachedKeysOnFailure(t *testing.T) {
	d := newDetractor(t, "k1")
	v := newVerifier(t, d)
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	token := d.sign(t, "k1", claimOverrides{})

	d.fail.Store(true)
	if err := v.Refresh(context.Background()); err == nil {
		t.Fatal("expected the refresh to fail")
	}
	if v.KeyCount() != 1 {
		t.Fatalf("cached keys = %d; a failed refresh must not empty the cache", v.KeyCount())
	}
	if _, err := v.Verify(context.Background(), "Bearer "+token); err != nil {
		t.Errorf("verification broke after a failed refresh: %v", err)
	}
}

func TestStartFetchesAndRefreshes(t *testing.T) {
	d := newDetractor(t, "k1")
	v, err := auth.NewVerifier(auth.Config{
		JWKSURL:         d.url(),
		RefreshInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v.Start(ctx)

	if v.KeyCount() != 1 {
		t.Fatalf("Start did not fetch the JWKS synchronously (keys = %d)", v.KeyCount())
	}
	before := d.fetches.Load()
	deadline := time.Now().Add(2 * time.Second)
	for d.fetches.Load() <= before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if d.fetches.Load() <= before {
		t.Error("the background refresh never ran")
	}
}

func TestNewVerifierRequiresAURL(t *testing.T) {
	if _, err := auth.NewVerifier(auth.Config{}); err == nil {
		t.Fatal("expected an error with no JWKS URL")
	}
}

func TestRolesClaimShapes(t *testing.T) {
	d := newDetractor(t, "k1")
	v := newVerifier(t, d)
	if err := v.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		roles any
		want  []string
	}{
		{"array", []string{"customer", "operator"}, []string{"customer", "operator"}},
		{"mixed case and spaces", []string{" Customer ", "ADMIN"}, []string{"customer", "admin"}},
		{"comma string", "customer,viewer", []string{"customer", "viewer"}},
		{"empty array", []string{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := d.sign(t, "k1", claimOverrides{roles: tc.roles})
			id, err := v.Verify(context.Background(), "Bearer "+token)
			if err != nil {
				t.Fatal(err)
			}
			if len(id.Roles) != len(tc.want) {
				t.Fatalf("roles = %v, want %v", id.Roles, tc.want)
			}
			for i := range tc.want {
				if id.Roles[i] != tc.want[i] {
					t.Errorf("roles = %v, want %v", id.Roles, tc.want)
				}
			}
		})
	}
}
