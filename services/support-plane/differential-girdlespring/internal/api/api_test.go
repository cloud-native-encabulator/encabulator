package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/flags"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/httpx"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/identity"

	"github.com/cloud-native-encabulator/encabulator/services/support-plane/differential-girdlespring/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/support-plane/differential-girdlespring/internal/store"
)

// fakeStore is an in-memory flag store.
type fakeStore struct {
	flags map[string]flags.Flag
	err   error
}

func newFakeStore(fs ...flags.Flag) *fakeStore {
	m := map[string]flags.Flag{}
	for _, f := range fs {
		m[f.Key] = f
	}
	return &fakeStore{flags: m}
}

func (f *fakeStore) List(context.Context) ([]flags.Flag, error) {
	if f.err != nil {
		return nil, f.err
	}
	// Ordered by key, as the real store is, so the ETag is stable.
	keys := make([]string, 0, len(f.flags))
	for k := range f.flags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]flags.Flag, 0, len(keys))
	for _, k := range keys {
		out = append(out, f.flags[k])
	}
	return out, nil
}

func (f *fakeStore) Get(_ context.Context, key string) (flags.Flag, error) {
	if f.err != nil {
		return flags.Flag{}, f.err
	}
	fl, ok := f.flags[key]
	if !ok {
		return flags.Flag{}, store.ErrNotFound
	}
	return fl, nil
}

func (f *fakeStore) Put(_ context.Context, key string, up store.Update) (flags.Flag, error) {
	if f.err != nil {
		return flags.Flag{}, f.err
	}
	fl, ok := f.flags[key]
	if !ok {
		return flags.Flag{}, store.ErrNotFound
	}
	fl.Value = up.Value
	fl.RolloutPercent = up.RolloutPercent
	if up.Description != nil {
		fl.Description = *up.Description
	}
	fl.UpdatedBy = up.UpdatedBy
	fl.UpdatedAt = time.Now().UTC()
	f.flags[key] = fl
	return fl, nil
}

func flag(key, typ string, value any, rollout *int) flags.Flag {
	raw, _ := json.Marshal(value)
	return flags.Flag{
		Key: key, Type: typ, Value: raw, RolloutPercent: rollout,
		UpdatedAt: time.Unix(1_700_000_000, 0).UTC(), UpdatedBy: "seed",
	}
}

func newServer(t *testing.T, s api.Store) *httpx.Server {
	t.Helper()
	srv := httpx.NewServer(httpx.ServerConfig{
		Service: "differential-girdlespring", Addr: "127.0.0.1:0",
		Logger: slog.New(slog.DiscardHandler), Registry: prometheus.NewRegistry(),
	})
	api.New(s, slog.New(slog.DiscardHandler), srv.Registry()).Routes(srv)
	return srv
}

func request(t *testing.T, srv *httpx.Server, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func operator(req *http.Request) *http.Request {
	req.Header.Set(identity.HeaderSubject, "opal")
	req.Header.Set(identity.HeaderRoles, "operator")
	return req
}

func TestListFlagsWithETag(t *testing.T) {
	srv := newServer(t, newFakeStore(
		flag(flags.TurboEncabulation, flags.TypeBoolean, false, nil),
		flag(flags.GatewayRateLimitRPS, flags.TypeNumber, 50, nil),
	))

	rec := request(t, srv, httptest.NewRequest(http.MethodGet, "/v1/flags", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}

	var body flags.ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Flags) != 2 {
		t.Fatalf("flags = %+v", body.Flags)
	}
	if body.Flags[0].Key != flags.GatewayRateLimitRPS {
		t.Errorf("flags are not ordered by key: %s first", body.Flags[0].Key)
	}

	// If-None-Match with the same tag must be a 304 with an empty body.
	req := httptest.NewRequest(http.MethodGet, "/v1/flags", nil)
	req.Header.Set("If-None-Match", etag)
	rec = request(t, srv, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("304 carried a body: %s", rec.Body)
	}
	if rec.Header().Get("ETag") != etag {
		t.Error("304 did not repeat the ETag")
	}

	// A stale tag must return the set again.
	req = httptest.NewRequest(http.MethodGet, "/v1/flags", nil)
	req.Header.Set("If-None-Match", `"stale"`)
	if rec = request(t, srv, req); rec.Code != http.StatusOK {
		t.Fatalf("stale tag status = %d", rec.Code)
	}
}

func TestETagChangesWithTheFlagSet(t *testing.T) {
	fs := newFakeStore(flag(flags.TurboEncabulation, flags.TypeBoolean, false, nil))
	srv := newServer(t, fs)

	before := request(t, srv, httptest.NewRequest(http.MethodGet, "/v1/flags", nil)).Header().Get("ETag")
	if _, err := fs.Put(context.Background(), flags.TurboEncabulation, store.Update{
		Value: json.RawMessage("true"), UpdatedBy: "opal",
	}); err != nil {
		t.Fatal(err)
	}
	after := request(t, srv, httptest.NewRequest(http.MethodGet, "/v1/flags", nil)).Header().Get("ETag")

	if before == after {
		t.Error("ETag did not change after a flag edit")
	}
}

func TestGetFlag(t *testing.T) {
	srv := newServer(t, newFakeStore(flag(flags.QuoteCache, flags.TypeBoolean, true, nil)))

	rec := request(t, srv, httptest.NewRequest(http.MethodGet, "/v1/flags/quote-cache", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var f flags.Flag
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	if f.Key != flags.QuoteCache || f.Type != flags.TypeBoolean || string(f.Value) != "true" {
		t.Errorf("flag = %+v", f)
	}

	rec = request(t, srv, httptest.NewRequest(http.MethodGet, "/v1/flags/no-such-flag", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown key status = %d", rec.Code)
	}
}

func TestEvaluateWithoutRollout(t *testing.T) {
	srv := newServer(t, newFakeStore(flag(flags.TurboEncabulation, flags.TypeBoolean, true, nil)))

	rec := request(t, srv, httptest.NewRequest(http.MethodGet,
		"/v1/flags/turbo-encabulation/evaluate?subject=wendy", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var ev api.Evaluation
	if err := json.Unmarshal(rec.Body.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if string(ev.Value) != "true" || ev.Reason != api.ReasonEveryone {
		t.Errorf("evaluation = %+v", ev)
	}
	if ev.Subject != "wendy" {
		t.Errorf("subject = %q", ev.Subject)
	}
}

func TestEvaluateWithRollout(t *testing.T) {
	all, none := 100, 0
	srv := newServer(t, newFakeStore(
		flag("rolled-in", flags.TypeBoolean, true, &all),
		flag("rolled-out", flags.TypeBoolean, true, &none),
	))

	cases := []struct {
		key, wantValue, wantReason string
	}{
		{"rolled-in", "true", api.ReasonInRollout},
		{"rolled-out", "false", api.ReasonOutOfRollout},
	}
	for _, tc := range cases {
		rec := request(t, srv, httptest.NewRequest(http.MethodGet,
			"/v1/flags/"+tc.key+"/evaluate?subject=wendy", nil))
		var ev api.Evaluation
		if err := json.Unmarshal(rec.Body.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		if string(ev.Value) != tc.wantValue || ev.Reason != tc.wantReason {
			t.Errorf("%s -> %+v, want value %s reason %s", tc.key, ev, tc.wantValue, tc.wantReason)
		}
	}
}

func TestEvaluateAgreesWithTheKitClient(t *testing.T) {
	// The server and the kit's flag client must reach the same verdict, or a
	// rollout would mean different things on each side of the wire.
	pct := 37
	f := flag(flags.StrictSideFumbling, flags.TypeBoolean, true, &pct)
	for _, subject := range []string{"wendy", "opal", "customer-1", "customer-2", "customer-3"} {
		ev := api.Evaluate(f, subject)
		want := flags.InRollout(f.Key, subject, pct)
		got := string(ev.Value) == "true"
		if got != want {
			t.Errorf("subject %q: server says %v, kit client says %v", subject, got, want)
		}
	}
}

func TestPutFlagRequiresOperatorOrAdmin(t *testing.T) {
	srv := newServer(t, newFakeStore(flag(flags.TurboEncabulation, flags.TypeBoolean, false, nil)))

	body := `{"value":true}`
	for _, roles := range []string{"", "customer", "viewer"} {
		req := httptest.NewRequest(http.MethodPut, "/v1/flags/turbo-encabulation", stringReader(body))
		if roles != "" {
			req.Header.Set(identity.HeaderRoles, roles)
		}
		rec := request(t, srv, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("roles %q: status = %d, want 403", roles, rec.Code)
		}
	}

	for _, roles := range []string{"operator", "admin", "customer,admin"} {
		req := operator(httptest.NewRequest(http.MethodPut, "/v1/flags/turbo-encabulation", stringReader(body)))
		req.Header.Set(identity.HeaderRoles, roles)
		rec := request(t, srv, req)
		if rec.Code != http.StatusOK {
			t.Errorf("roles %q: status = %d body=%s", roles, rec.Code, rec.Body)
		}
	}
}

func TestPutFlagRecordsUpdatedBy(t *testing.T) {
	fs := newFakeStore(flag(flags.TurboEncabulation, flags.TypeBoolean, false, nil))
	srv := newServer(t, fs)

	req := operator(httptest.NewRequest(http.MethodPut, "/v1/flags/turbo-encabulation",
		stringReader(`{"value":true,"description":"on for the demo"}`)))
	rec := request(t, srv, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var f flags.Flag
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	if string(f.Value) != "true" {
		t.Errorf("value = %s", f.Value)
	}
	if f.UpdatedBy != "opal" {
		t.Errorf("updatedBy = %q", f.UpdatedBy)
	}
	if f.Description != "on for the demo" {
		t.Errorf("description = %q", f.Description)
	}
	if time.Since(f.UpdatedAt) > time.Minute {
		t.Errorf("updatedAt = %v", f.UpdatedAt)
	}
}

func TestPutFlagValidatesType(t *testing.T) {
	srv := newServer(t, newFakeStore(
		flag(flags.TurboEncabulation, flags.TypeBoolean, false, nil),
		flag(flags.GatewayRateLimitRPS, flags.TypeNumber, 50, nil),
	))

	cases := []struct {
		name, key, body string
		want            int
	}{
		{"string into boolean", "turbo-encabulation", `{"value":"yes"}`, http.StatusBadRequest},
		{"number into boolean", "turbo-encabulation", `{"value":1}`, http.StatusBadRequest},
		{"boolean into number", "gateway-rate-limit-rps", `{"value":true}`, http.StatusBadRequest},
		{"missing value", "turbo-encabulation", `{}`, http.StatusBadRequest},
		{"rollout on a number", "gateway-rate-limit-rps", `{"value":75,"rolloutPercent":50}`, http.StatusBadRequest},
		{"rollout out of range", "turbo-encabulation", `{"value":true,"rolloutPercent":101}`, http.StatusBadRequest},
		{"negative rollout", "turbo-encabulation", `{"value":true,"rolloutPercent":-1}`, http.StatusBadRequest},
		{"valid boolean", "turbo-encabulation", `{"value":true,"rolloutPercent":25}`, http.StatusOK},
		{"valid number", "gateway-rate-limit-rps", `{"value":125}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := operator(httptest.NewRequest(http.MethodPut, "/v1/flags/"+tc.key, stringReader(tc.body)))
			rec := request(t, srv, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body)
			}
			if tc.want == http.StatusBadRequest {
				var env httpx.ErrorEnvelope
				if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
					t.Fatal(err)
				}
				if env.Error.Code == "" {
					t.Error("error response has no code")
				}
			}
		})
	}
}

func TestPutUnknownFlag(t *testing.T) {
	srv := newServer(t, newFakeStore())
	req := operator(httptest.NewRequest(http.MethodPut, "/v1/flags/invented-flag", stringReader(`{"value":true}`)))
	if rec := request(t, srv, req); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestSeedsCoverEveryDesignFlag(t *testing.T) {
	seeds := api.Seeds()
	defaults := flags.Defaults()
	if len(seeds) != len(defaults) {
		t.Fatalf("%d seeds for %d documented flags", len(seeds), len(defaults))
	}
	for _, s := range seeds {
		want, ok := defaults[s.Key]
		if !ok {
			t.Errorf("seed %q is not a documented flag", s.Key)
			continue
		}
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(s.Value)
		if string(wantJSON) != string(gotJSON) {
			t.Errorf("seed %q = %s, kit default = %s", s.Key, gotJSON, wantJSON)
		}
		if s.Description == "" {
			t.Errorf("seed %q has no description", s.Key)
		}
	}
}

func stringReader(s string) *strings.Reader { return strings.NewReader(s) }
