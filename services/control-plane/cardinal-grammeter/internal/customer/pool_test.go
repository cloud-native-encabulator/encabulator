package customer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/gateway"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/cardinal-grammeter/internal/rng"
)

// detractorRules is the username rule from the design: 3 to 64 characters of
// [a-z0-9._-].
var detractorRules = regexp.MustCompile(`^[a-z0-9._-]{3,64}$`)

func TestUsernamesSatisfyTheDetractorRules(t *testing.T) {
	seen := map[string]bool{}
	for i := 1; i <= MaxCustomers; i++ {
		name := Username(i)
		if !detractorRules.MatchString(name) {
			t.Fatalf("username %q would be rejected by the detractor", name)
		}
		if seen[name] {
			t.Fatalf("username %q is not unique", name)
		}
		seen[name] = true
	}
}

func TestPasswordIsDerivedFromTheSeed(t *testing.T) {
	a := Password("seed-one", "grammeter-0001")
	b := Password("seed-one", "grammeter-0001")
	c := Password("seed-two", "grammeter-0001")
	d := Password("seed-one", "grammeter-0002")

	if a != b {
		t.Error("the same seed and username must give the same password")
	}
	if a == c {
		t.Error("a different seed must give a different password")
	}
	if a == d {
		t.Error("a different username must give a different password")
	}
	if len(a) != 35 {
		t.Errorf("password length = %d, want 35", len(a))
	}
	if a == "" || a == "G1-" {
		t.Error("password is empty")
	}
}

func TestPoolResizeKeepsIdentityAndTokens(t *testing.T) {
	p := NewPool(nil, "seed", 3, nil)
	if p.Size() != 3 {
		t.Fatalf("size = %d, want 3", p.Size())
	}
	first := p.customers[0]
	first.token = "tok"

	p.Resize(1)
	if p.Size() != 1 {
		t.Errorf("size = %d, want 1", p.Size())
	}
	p.Resize(5)
	if p.Size() != 5 {
		t.Errorf("size = %d, want 5", p.Size())
	}
	if p.customers[0] != first || p.customers[0].token != "tok" {
		t.Error("resizing forgot an existing customer")
	}
	if p.customers[4].Name() != Username(5) {
		t.Errorf("new customer named %q, want %q", p.customers[4].Name(), Username(5))
	}
}

func TestPoolResizeClampsToBounds(t *testing.T) {
	p := NewPool(nil, "seed", 0, nil)
	if p.Size() != 1 {
		t.Errorf("size = %d, want the floor of 1", p.Size())
	}
	p.Resize(MaxCustomers + 500)
	if p.Size() != MaxCustomers {
		t.Errorf("size = %d, want the ceiling of %d", p.Size(), MaxCustomers)
	}
}

func TestPoolPickStaysInsideTheActiveSet(t *testing.T) {
	p := NewPool(nil, "seed", 10, nil)
	p.Resize(2)
	source := rng.Seeded(1)
	for range 200 {
		name := p.Pick(source).Name()
		if name != Username(1) && name != Username(2) {
			t.Fatalf("picked retired customer %q", name)
		}
	}
}

// detractor is a stand-in for the identity service behind the gateway.
type detractor struct {
	mu sync.Mutex
	// registered records the usernames that have been created.
	registered map[string]string
	logins     int
	registers  int
	// registerStatus overrides the register response when non-zero.
	registerStatus int
	// loginStatus overrides the login response when non-zero.
	loginStatus int
	// expiresIn is the token lifetime the login reports.
	expiresIn int
}

func newDetractor(t *testing.T) (*detractor, *gateway.Client) {
	t.Helper()
	d := &detractor{registered: map[string]string{}, expiresIn: 3600}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", d.register)
	mux.HandleFunc("POST /api/v1/auth/login", d.login)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return d, gateway.New(srv.URL, &http.Client{Timeout: 5 * time.Second}, nil)
}

func (d *detractor) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	d.mu.Lock()
	defer d.mu.Unlock()
	d.registers++
	if d.registerStatus != 0 {
		w.WriteHeader(d.registerStatus)
		return
	}
	if !detractorRules.MatchString(body.Username) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if body.DisplayName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if _, exists := d.registered[body.Username]; exists {
		w.WriteHeader(http.StatusConflict)
		return
	}
	d.registered[body.Username] = body.Password
	w.WriteHeader(http.StatusCreated)
}

func (d *detractor) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	d.mu.Lock()
	defer d.mu.Unlock()
	d.logins++
	if d.loginStatus != 0 {
		w.WriteHeader(d.loginStatus)
		return
	}
	if pw, ok := d.registered[body.Username]; !ok || pw != body.Password {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accessToken": "tok-" + body.Username + "-" + time.Now().Format("150405.000000000"),
		"tokenType":   "Bearer",
		"expiresIn":   d.expiresIn,
	})
}

func TestTokenRegistersThenLogsIn(t *testing.T) {
	d, client := newDetractor(t)
	p := NewPool(client, "seed", 1, nil)
	cust := p.Pick(rng.Seeded(1))

	token, err := cust.Token(context.Background(), time.Now)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if d.registers != 1 || d.logins != 1 {
		t.Errorf("registers = %d, logins = %d, want 1 and 1", d.registers, d.logins)
	}

	// A cached token costs nothing.
	if _, err := cust.Token(context.Background(), time.Now); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if d.registers != 1 || d.logins != 1 {
		t.Errorf("registers = %d, logins = %d, want the cached token reused", d.registers, d.logins)
	}
}

func TestRegisterToleratesAnExistingAccount(t *testing.T) {
	d, client := newDetractor(t)
	// Pre-create the account, as a previous run of the grammeter would have.
	name := Username(1)
	d.registered[name] = Password("seed", name)

	p := NewPool(client, "seed", 1, nil)
	if _, err := p.Pick(rng.Seeded(1)).Token(context.Background(), time.Now); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if d.registers != 1 {
		t.Errorf("registers = %d, want 1 (the 409 is tolerated)", d.registers)
	}
	if d.logins != 1 {
		t.Errorf("logins = %d, want 1", d.logins)
	}
}

func TestTokenIsRenewedBeforeExpiry(t *testing.T) {
	d, client := newDetractor(t)
	d.expiresIn = 120 // two minutes, so renewBefore of 60s bites at t+60s

	now := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	p := NewPool(client, "seed", 1, nil)
	cust := p.Pick(rng.Seeded(1))
	first, err := cust.Token(context.Background(), clock)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}

	now = now.Add(30 * time.Second)
	if again, _ := cust.Token(context.Background(), clock); again != first {
		t.Error("token was renewed too early")
	}

	now = now.Add(40 * time.Second) // 70s in: inside the renewal margin
	renewed, err := cust.Token(context.Background(), clock)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if renewed == first {
		t.Error("token was not renewed inside the margin")
	}
	if d.logins != 2 {
		t.Errorf("logins = %d, want 2", d.logins)
	}
	if d.registers != 1 {
		t.Errorf("registers = %d, want 1: registration happens once", d.registers)
	}
}

func TestInvalidateForcesOneReLogin(t *testing.T) {
	d, client := newDetractor(t)
	p := NewPool(client, "seed", 1, nil)
	cust := p.Pick(rng.Seeded(1))

	first, err := cust.Token(context.Background(), time.Now)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}

	// Invalidating some other token must not touch the cached one.
	cust.Invalidate("someone-elses-token")
	if again, _ := cust.Token(context.Background(), time.Now); again != first {
		t.Error("Invalidate dropped a token it did not own")
	}

	cust.Invalidate(first)
	second, err := cust.Token(context.Background(), time.Now)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if second == first {
		t.Error("Invalidate did not force a new login")
	}
	if d.logins != 2 {
		t.Errorf("logins = %d, want 2", d.logins)
	}
}

func TestTokenReportsAuthFailures(t *testing.T) {
	d, client := newDetractor(t)
	d.loginStatus = http.StatusUnauthorized

	p := NewPool(client, "seed", 1, nil)
	_, err := p.Pick(rng.Seeded(1)).Token(context.Background(), time.Now)
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
}

func TestTokenReportsRegistrationFailures(t *testing.T) {
	d, client := newDetractor(t)
	d.registerStatus = http.StatusInternalServerError

	p := NewPool(client, "seed", 1, nil)
	_, err := p.Pick(rng.Seeded(1)).Token(context.Background(), time.Now)
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
}

func TestConcurrentTokenCallsShareOneLogin(t *testing.T) {
	d, client := newDetractor(t)
	p := NewPool(client, "seed", 1, nil)
	cust := p.Pick(rng.Seeded(1))

	var wg sync.WaitGroup
	tokens := make([]string, 16)
	for i := range tokens {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok, err := cust.Token(context.Background(), time.Now)
			if err != nil {
				t.Errorf("Token: %v", err)
				return
			}
			tokens[i] = tok
		}(i)
	}
	wg.Wait()

	for i, tok := range tokens {
		if tok != tokens[0] {
			t.Fatalf("token %d differs from the first: %q vs %q", i, tok, tokens[0])
		}
	}
	if d.logins != 1 {
		t.Errorf("logins = %d, want 1: concurrent callers must share one attempt", d.logins)
	}
	if d.registers != 1 {
		t.Errorf("registers = %d, want 1", d.registers)
	}
}

func TestPoolClockDefaultsToWallTime(t *testing.T) {
	p := NewPool(nil, "seed", 1, nil)
	if p.Clock() == nil {
		t.Fatal("Clock() is nil")
	}
	if p.Now().IsZero() {
		t.Fatal("Now() returned the zero time")
	}
}
