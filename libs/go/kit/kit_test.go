package kit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/flags"
)

func TestBootstrapRequiresAServiceName(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "")
	if _, err := kit.Bootstrap(context.Background(), kit.Options{}); err == nil {
		t.Fatal("expected an error with no service name")
	}
}

func TestBootstrapWiresTheStandardEndpoints(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("MALAISE_ENGINE_URL", "")
	t.Setenv("DIFFERENTIAL_GIRDLESPRING_URL", "")

	rt, err := kit.Bootstrap(context.Background(), kit.Options{
		Service:      "hydrocoptic-datastore",
		Version:      "1.2.3",
		ApplyMalaise: true,
		Addr:         "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rt.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()

	if rt.Malaise != nil {
		t.Error("malaise injector should be nil with MALAISE_ENGINE_URL unset")
	}
	if !rt.Flags.Bool(flags.QuoteCache) {
		t.Error("flag client should serve built-in defaults")
	}

	rt.Server.HandleFunc("GET /v1/orders", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for path, want := range map[string]int{
		"/healthz":    http.StatusOK,
		"/readyz":     http.StatusOK,
		"/metrics":    http.StatusOK,
		"/v1/orders":  http.StatusOK,
		"/v1/nothing": http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		rt.Server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestClientRequiresTheURLVariable(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	rt, err := kit.Bootstrap(context.Background(), kit.Options{Service: "svc", Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	if _, err := rt.Client("spurving-bearing-quoter"); err == nil {
		t.Fatal("expected an error when SPURVING_BEARING_QUOTER_URL is unset")
	} else if !strings.Contains(err.Error(), "SPURVING_BEARING_QUOTER_URL") {
		t.Errorf("error should name the variable: %v", err)
	}

	t.Setenv("SPURVING_BEARING_QUOTER_URL", "http://quoter:8080")
	c, err := rt.Client("spurving-bearing-quoter")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.URL("/v1/quotes"); got != "http://quoter:8080/v1/quotes" {
		t.Errorf("URL = %q", got)
	}
}

func TestMalaiseMiddlewareInstalledWhenEngineConfigured(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"directives":[]}`))
	}))
	defer engine.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("MALAISE_ENGINE_URL", engine.URL)
	rt, err := kit.Bootstrap(context.Background(), kit.Options{
		Service: "svc", ApplyMalaise: true, Addr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	if rt.Malaise == nil {
		t.Fatal("expected a malaise injector")
	}
}
