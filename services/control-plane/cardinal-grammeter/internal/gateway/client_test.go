package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingObserver struct {
	mu      sync.Mutex
	method  string
	status  int
	calls   int
	elapsed time.Duration
}

func (o *recordingObserver) ObserveClientRequest(method string, status int, d time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.method, o.status, o.elapsed = method, status, d
	o.calls++
}

func TestDoSendsMethodPathQueryBodyAndToken(t *testing.T) {
	var (
		gotMethod, gotPath, gotQuery, gotAuth, gotType, gotAccept, gotBody string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"ord_1"}`))
	}))
	defer srv.Close()

	obs := &recordingObserver{}
	// A trailing slash on the base must not double up.
	c := New(srv.URL+"/", &http.Client{Timeout: 5 * time.Second}, obs)
	if c.Base() != srv.URL {
		t.Errorf("Base() = %q, want %q", c.Base(), srv.URL)
	}

	var out struct {
		ID string `json:"id"`
	}
	status, err := c.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   "/api/v1/orders",
		Query:  url.Values{"limit": []string{"5"}},
		Body:   map[string]any{"model": "turbo", "quantity": 2},
		Token:  "tok-123",
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if status != http.StatusAccepted {
		t.Errorf("status = %d, want 202", status)
	}
	if out.ID != "ord_1" {
		t.Errorf("decoded id = %q", out.ID)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/orders" || gotQuery != "limit=5" {
		t.Errorf("request = %s %s?%s", gotMethod, gotPath, gotQuery)
	}
	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotType != "application/json" || gotAccept != "application/json" {
		t.Errorf("headers: content-type %q accept %q", gotType, gotAccept)
	}
	if !strings.Contains(gotBody, `"model":"turbo"`) {
		t.Errorf("body = %q", gotBody)
	}

	obs.mu.Lock()
	defer obs.mu.Unlock()
	if obs.calls != 1 || obs.method != http.MethodPost || obs.status != http.StatusAccepted {
		t.Errorf("observer saw %d calls, %s %d", obs.calls, obs.method, obs.status)
	}
}

func TestDoOmitsTheTokenAndBodyWhenAbsent(t *testing.T) {
	var gotAuth, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, nil, nil)
	if _, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/api/v1/parts"}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want none", gotAuth)
	}
	if gotType != "" {
		t.Errorf("Content-Type = %q, want none", gotType)
	}
}

// A non-2xx status is data, not an error: the journeys decide what it means.
func TestDoReportsNonSuccessWithoutAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"insufficient_parts"}}`))
	}))
	defer srv.Close()

	var out map[string]any
	status, err := New(srv.URL, nil, nil).Do(context.Background(), Request{
		Method: http.MethodPost, Path: "/api/v1/orders", Out: &out,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if status != http.StatusConflict {
		t.Errorf("status = %d, want 409", status)
	}
	if out != nil {
		t.Errorf("out = %v, want the body left undecoded on a non-2xx", out)
	}
}

func TestDoReportsTransportFailures(t *testing.T) {
	obs := &recordingObserver{}
	c := New("http://127.0.0.1:1", &http.Client{Timeout: 200 * time.Millisecond}, obs)
	status, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/api/v1/parts"})
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
	obs.mu.Lock()
	defer obs.mu.Unlock()
	if obs.calls != 1 || obs.status != 0 {
		t.Errorf("observer saw %d calls with status %d", obs.calls, obs.status)
	}
}

func TestDoReportsAnUndecodableBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	var out map[string]any
	status, err := New(srv.URL, nil, nil).Do(context.Background(), Request{
		Method: http.MethodGet, Path: "/api/v1/parts", Out: &out,
	})
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want the status to be reported alongside the error", status)
	}
}

func TestDoRejectsAnUnencodableBody(t *testing.T) {
	c := New("http://example.invalid", nil, nil)
	_, err := c.Do(context.Background(), Request{
		Method: http.MethodPost, Path: "/x", Body: func() {},
	})
	if err == nil {
		t.Fatal("expected an encode error")
	}
}

func TestDoHonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := New(srv.URL, nil, nil).Do(ctx, Request{
		Method: http.MethodGet, Path: "/slow",
	}); err == nil {
		t.Fatal("expected the cancelled context to be reported")
	}
}
