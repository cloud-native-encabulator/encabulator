package settings

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/engine"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/journey"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/traffic"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// defaults is what main builds from the environment when nothing is saved.
func defaults() engine.Config {
	return engine.Config{
		Running:       false,
		RPS:           5,
		Pattern:       traffic.PatternSteady,
		PeriodSeconds: 60,
		Mix:           journey.DefaultMix(),
		Customers:     50,
	}
}

func applied() engine.Config {
	return engine.Config{
		Running:       true,
		RPS:           25,
		Pattern:       traffic.PatternBurst,
		PeriodSeconds: 30,
		Mix:           map[string]int{"browse": 10, "order": 90},
		Customers:     80,
	}
}

func equal(a, b engine.Config) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// ------------------------------------------------------------------ Encode/Decode

func TestEncodeThenDecodeGivesBackTheSameConfiguration(t *testing.T) {
	data, err := Encode(applied())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data, defaults())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !equal(got, applied()) {
		t.Errorf("round trip = %+v, want %+v", got, applied())
	}
}

func TestDecodeKeepsTheDefaultForAFieldNotSaved(t *testing.T) {
	got, err := Decode([]byte(`{"running":true,"rps":12}`), defaults())
	if err != nil {
		t.Fatal(err)
	}
	want := defaults()
	want.Running, want.RPS = true, 12
	if !equal(got, want) {
		t.Errorf("Decode = %+v, want %+v", got, want)
	}
}

func TestDecodeIgnoresFieldsItDoesNotKnow(t *testing.T) {
	got, err := Decode([]byte(`{"rps":7,"futureKnob":"on"}`), defaults())
	if err != nil {
		t.Fatalf("an unknown field must not make the saved settings unusable: %v", err)
	}
	if got.RPS != 7 {
		t.Errorf("rps = %v, want 7", got.RPS)
	}
}

func TestDecodeRefusesACorruptOrOutOfRangeConfiguration(t *testing.T) {
	for name, data := range map[string]string{
		"not json":          `running=true`,
		"truncated":         `{"running":true,"rps":`,
		"empty":             ``,
		"wrong type":        `{"rps":"fast"}`,
		"rate too high":     `{"rps":9999}`,
		"rate too low":      `{"rps":0}`,
		"unknown pattern":   `{"pattern":"wobble"}`,
		"period too long":   `{"periodSeconds":86400}`,
		"empty mix":         `{"mix":{}}`,
		"unknown journey":   `{"mix":{"teleport":5}}`,
		"no customers":      `{"customers":0}`,
		"an array":          `[1,2,3]`,
		"trailing garbage":  `{"rps":5} and more`,
		"negative mix":      `{"mix":{"browse":-1,"order":2}}`,
		"null rate (typed)": `{"rps":null,"pattern":3}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := Decode([]byte(data), defaults()); err == nil {
				t.Errorf("Decode(%q) = %+v, want an error", data, got)
			}
		})
	}
}

// ------------------------------------------------------------------ Restore

type fakeStore struct {
	mu      sync.Mutex
	data    []byte
	loadErr []error // returned by successive Loads, then nil
	loads   int
	saveErr []error // returned by successive Saves, then nil
	saves   [][]byte
	block   chan struct{} // when set, Save waits for it (or ctx)
}

func (f *fakeStore) Load(context.Context) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	if len(f.loadErr) > 0 {
		err := f.loadErr[0]
		f.loadErr = f.loadErr[1:]
		if err != nil {
			return nil, err
		}
	}
	if f.data == nil {
		return nil, ErrNotSaved
	}
	return f.data, nil
}

func (f *fakeStore) Save(ctx context.Context, data []byte) error {
	f.mu.Lock()
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.saveErr) > 0 {
		err := f.saveErr[0]
		f.saveErr = f.saveErr[1:]
		if err != nil {
			return err
		}
	}
	f.data = append([]byte(nil), data...)
	f.saves = append(f.saves, f.data)
	return nil
}

func (f *fakeStore) saved() (engine.Config, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var c engine.Config
	if f.data != nil {
		_ = json.Unmarshal(f.data, &c)
	}
	return c, len(f.saves)
}

func TestRestoreReturnsTheSavedConfiguration(t *testing.T) {
	data, _ := Encode(applied())
	got := Restore(context.Background(), &fakeStore{data: data}, defaults(), quiet())
	if !equal(got, applied()) {
		t.Errorf("Restore = %+v, want %+v", got, applied())
	}
}

func TestRestoreFallsBackToTheDefaults(t *testing.T) {
	broken := errors.New("connection refused")
	for name, store := range map[string]*fakeStore{
		"nothing saved":      {},
		"corrupt":            {data: []byte(`{"running":tru`)},
		"out of range":       {data: []byte(`{"running":true,"rps":100000}`)},
		"API keeps failing":  {loadErr: []error{broken, broken, broken, broken}},
		"saved but unusable": {data: []byte(`{"pattern":"zigzag","running":true}`)},
	} {
		t.Run(name, func(t *testing.T) {
			got := Restore(context.Background(), store, defaults(), quiet())
			if !equal(got, defaults()) {
				t.Errorf("Restore = %+v, want the defaults", got)
			}
		})
	}
}

func TestRestoreRidesOutABriefAPIOutage(t *testing.T) {
	data, _ := Encode(applied())
	store := &fakeStore{data: data, loadErr: []error{errors.New("i/o timeout")}}
	got := Restore(context.Background(), store, defaults(), quiet())
	if !equal(got, applied()) {
		t.Errorf("Restore = %+v, want the saved configuration after one retry", got)
	}
	if store.loads != 2 {
		t.Errorf("loads = %d, want 2", store.loads)
	}
}

func TestRestoreDoesNotRetryWhenNothingIsSaved(t *testing.T) {
	store := &fakeStore{}
	Restore(context.Background(), store, defaults(), quiet())
	if store.loads != 1 {
		t.Errorf("loads = %d, want 1", store.loads)
	}
}

// ------------------------------------------------------------------ Saver

func fastSaver(store Store) *Saver {
	s := NewSaver(store, quiet())
	s.MinBackoff, s.MaxBackoff, s.Timeout = 5*time.Millisecond, 20*time.Millisecond, time.Second
	return s
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestSaverWritesWhatIsSubmitted(t *testing.T) {
	store := &fakeStore{}
	s := fastSaver(store)
	go s.Run()
	defer s.Close(context.Background())

	s.Submit(applied())
	eventually(t, "the save", func() bool { _, n := store.saved(); return n == 1 })
	if got, _ := store.saved(); !equal(got, applied()) {
		t.Errorf("saved = %+v, want %+v", got, applied())
	}
}

func TestSubmitNeverWaitsForASlowStore(t *testing.T) {
	store := &fakeStore{block: make(chan struct{})}
	s := fastSaver(store)
	go s.Run()

	start := time.Now()
	for i := range 1000 {
		c := applied()
		c.RPS = float64(1 + i%400)
		s.Submit(c)
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Errorf("1000 submits against a stalled store took %v", took)
	}
	close(store.block)
	_ = s.Close(context.Background())
}

func TestSaverKeepsOnlyTheNewestWhileAWriteIsInFlight(t *testing.T) {
	store := &fakeStore{block: make(chan struct{})}
	s := fastSaver(store)
	go s.Run()

	first := applied()
	first.RPS = 1
	s.Submit(first)
	time.Sleep(20 * time.Millisecond) // let the first write start and block
	for _, rps := range []float64{2, 3, 4} {
		c := applied()
		c.RPS = rps
		s.Submit(c)
	}
	close(store.block)
	eventually(t, "the newest settings", func() bool { c, _ := store.saved(); return c.RPS == 4 })
	_ = s.Close(context.Background())
	if _, n := store.saved(); n != 2 {
		t.Errorf("writes = %d, want 2 (the one in flight, then only the newest)", n)
	}
}

func TestSaverRetriesAFailedWrite(t *testing.T) {
	store := &fakeStore{saveErr: []error{errors.New("503"), errors.New("503")}}
	s := fastSaver(store)
	go s.Run()
	defer s.Close(context.Background())

	s.Submit(applied())
	eventually(t, "the retried save", func() bool { _, n := store.saved(); return n == 1 })
}

func TestCloseWritesWhatIsStillPending(t *testing.T) {
	store := &fakeStore{saveErr: []error{errors.New("503")}}
	s := fastSaver(store)
	s.MinBackoff, s.MaxBackoff = time.Hour, time.Hour // the loop will not retry in time
	go s.Run()

	s.Submit(applied())
	eventually(t, "the failed first write", func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.saveErr) == 0
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, n := store.saved(); n != 1 || !equal(got, applied()) {
		t.Errorf("after Close: %d writes, saved %+v", n, got)
	}
}

func TestCloseWithNothingPendingWritesNothing(t *testing.T) {
	store := &fakeStore{}
	s := fastSaver(store)
	go s.Run()
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, n := store.saved(); n != 0 {
		t.Errorf("writes = %d, want 0", n)
	}
}

// ------------------------------------------------------------------ ConfigMap store

// fakeAPI is a Kubernetes API server holding one ConfigMap.
type fakeAPI struct {
	t       *testing.T
	mu      sync.Mutex
	exists  bool
	data    map[string]string
	labels  map[string]string
	status  int // when set, every request answers this
	tokens  []string
	methods []string
}

func (a *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens = append(a.tokens, r.Header.Get("Authorization"))
	a.methods = append(a.methods, r.Method)
	if r.URL.Path != "/api/v1/namespaces/encabulator/configmaps/cardinal-grammeter-settings" {
		http.Error(w, `{"kind":"Status","message":"wrong path `+r.URL.Path+`"}`, http.StatusNotFound)
		return
	}
	if a.status != 0 {
		w.WriteHeader(a.status)
		_, _ = w.Write([]byte(`{"kind":"Status","message":"configmaps \"cardinal-grammeter-settings\" is forbidden: User \"system:serviceaccount:encabulator:cardinal-grammeter\" cannot patch resource"}`))
		return
	}
	if !a.exists {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"kind":"Status","message":"configmaps \"cardinal-grammeter-settings\" not found"}`))
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPatch:
		if ct := r.Header.Get("Content-Type"); ct != "application/merge-patch+json" {
			a.t.Errorf("patch content type = %q", ct)
		}
		var patch struct {
			Data map[string]string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			a.t.Errorf("patch body: %v", err)
		}
		if len(patch.Data) != 1 {
			a.t.Errorf("patch touches %d keys, want only %s", len(patch.Data), Key)
		}
		if a.data == nil {
			a.data = map[string]string{}
		}
		for k, v := range patch.Data {
			a.data[k] = v
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"kind": "ConfigMap", "metadata": map[string]any{"name": "cardinal-grammeter-settings", "labels": a.labels},
		"data": a.data,
	})
}

func newConfigMapStore(t *testing.T, api *fakeAPI) (*ConfigMap, string) {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("token-one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cm, err := NewConfigMap(ConfigMapOptions{
		Server: srv.URL, Namespace: "encabulator", Name: "cardinal-grammeter-settings", TokenFile: token,
	})
	if err != nil {
		t.Fatal(err)
	}
	return cm, token
}

func TestConfigMapSaveThenLoad(t *testing.T) {
	api := &fakeAPI{t: t, exists: true, data: map[string]string{"note": "kept"}, labels: map[string]string{"app.kubernetes.io/name": "cardinal-grammeter"}}
	cm, _ := newConfigMapStore(t, api)
	data, _ := Encode(applied())
	if err := cm.Save(context.Background(), data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := cm.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Load = %s, want %s", got, data)
	}
	if api.data["note"] != "kept" {
		t.Error("Save removed another key from the ConfigMap")
	}
	if strings.Join(api.methods, ",") != "PATCH,GET" {
		t.Errorf("methods = %v, want PATCH then GET (never create)", api.methods)
	}
}

func TestConfigMapMissingOrEmptyIsNothingSaved(t *testing.T) {
	for name, api := range map[string]*fakeAPI{
		"no ConfigMap":      {exists: false},
		"ConfigMap, no key": {exists: true, data: map[string]string{}},
		"empty value":       {exists: true, data: map[string]string{Key: "  \n"}},
	} {
		t.Run(name, func(t *testing.T) {
			api.t = t
			cm, _ := newConfigMapStore(t, api)
			if _, err := cm.Load(context.Background()); !errors.Is(err, ErrNotSaved) {
				t.Errorf("Load error = %v, want ErrNotSaved", err)
			}
		})
	}
}

func TestConfigMapReportsTheAPIServersReason(t *testing.T) {
	api := &fakeAPI{t: t, exists: true, status: http.StatusForbidden}
	cm, _ := newConfigMapStore(t, api)
	err := cm.Save(context.Background(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("Save error = %v, want the 403 and the API server's message", err)
	}
	if _, err := cm.Load(context.Background()); err == nil || errors.Is(err, ErrNotSaved) {
		t.Errorf("Load error = %v, want a real error, not 'nothing saved'", err)
	}
}

func TestConfigMapSaveOfAMissingConfigMapFails(t *testing.T) {
	cm, _ := newConfigMapStore(t, &fakeAPI{t: t, exists: false})
	if err := cm.Save(context.Background(), []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("Save error = %v, want a 404: the grammeter never creates the ConfigMap", err)
	}
}

func TestConfigMapReadsTheTokenOnEveryRequest(t *testing.T) {
	api := &fakeAPI{t: t, exists: true}
	cm, token := newConfigMapStore(t, api)
	_, _ = cm.Load(context.Background())
	if err := os.WriteFile(token, []byte("token-two"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = cm.Load(context.Background())
	if want := []string{"Bearer token-one", "Bearer token-two"}; strings.Join(api.tokens, ",") != strings.Join(want, ",") {
		t.Errorf("tokens = %v, want %v (a projected token rotates)", api.tokens, want)
	}
}

func TestConfigMapUnreachableAPIServerIsAnError(t *testing.T) {
	cm, err := NewConfigMap(ConfigMapOptions{
		Server: "http://127.0.0.1:1", Namespace: "encabulator", Name: "cardinal-grammeter-settings",
		Client: &http.Client{Timeout: time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cm.Load(context.Background()); err == nil || errors.Is(err, ErrNotSaved) {
		t.Errorf("Load error = %v, want a connection error", err)
	}
	got := Restore(context.Background(), cm, defaults(), quiet())
	if !equal(got, defaults()) {
		t.Errorf("Restore against an unreachable API server = %+v, want the defaults", got)
	}
}

func TestInClusterNeedsTheServiceEnvironment(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := InCluster("encabulator", "cardinal-grammeter-settings"); !errors.Is(err, ErrNotInCluster) {
		t.Errorf("InCluster error = %v, want ErrNotInCluster", err)
	}
}
