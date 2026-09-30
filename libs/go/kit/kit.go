// Package kit is the shared Go service kit for the Cloud Native Encabulator. It
// implements the cross-cutting conventions of design section 7 so that every Go
// service is observable, configurable and fault-injectable the same way.
//
// The subpackages are usable on their own; Bootstrap simply wires the usual
// combination in one call:
//
//	rt, err := kit.Bootstrap(ctx, kit.Options{Service: "hydrocoptic-datastore", ApplyMalaise: true})
//	if err != nil { ... }
//	defer rt.Close(context.Background())
//	rt.Server.Handle("GET /v1/orders", h)
//	rt.Server.AddReadyCheck("postgres", pool.Ping)
//	return rt.Run(ctx)
package kit

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/config"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/flags"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/httpx"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/logging"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/malaise"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/telemetry"
)

// Options configures Bootstrap.
type Options struct {
	// Service is the service name. Empty falls back to OTEL_SERVICE_NAME.
	Service string
	// Version is reported as service.version. Empty falls back to
	// SERVICE_VERSION, then "dev".
	Version string
	// ApplyMalaise installs the malaise middleware on the API routes. Processing,
	// data and support plane services set it; control plane services do not.
	ApplyMalaise bool
	// FlagDefaults overrides individual built-in flag defaults.
	FlagDefaults map[string]any
	// Addr overrides the listen address (tests).
	Addr string
}

// Runtime is everything a service main needs after Bootstrap.
type Runtime struct {
	Service  string
	Version  string
	Log      *slog.Logger
	Registry *prometheus.Registry
	Server   *httpx.Server
	Flags    *flags.Client
	// Malaise is nil when MALAISE_ENGINE_URL is unset or ApplyMalaise is false.
	Malaise *malaise.Injector

	shutdownTelemetry telemetry.Shutdown
}

// Bootstrap builds the logger, tracing, metric registry, flag client, malaise
// injector and HTTP server for a service.
func Bootstrap(ctx context.Context, opts Options) (*Runtime, error) {
	service := opts.Service
	if service == "" {
		service = config.String("OTEL_SERVICE_NAME", "")
	}
	if service == "" {
		return nil, fmt.Errorf("kit: service name is required (set Options.Service or OTEL_SERVICE_NAME)")
	}
	// Honor the convention that OTEL_SERVICE_NAME may rename a service.
	service = config.String("OTEL_SERVICE_NAME", service)

	version := opts.Version
	if version == "" {
		version = config.String("SERVICE_VERSION", "dev")
	}

	log := logging.New(service, config.String("LOG_LEVEL", "info"))
	slog.SetDefault(log)

	shutdown, err := telemetry.Setup(ctx, telemetry.Config{
		ServiceName:    service,
		ServiceVersion: version,
		Endpoint:       config.String("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
	})
	if err != nil {
		return nil, err
	}

	reg := httpx.NewRegistry()
	srv := httpx.NewServer(httpx.ServerConfig{
		Service:  service,
		Addr:     opts.Addr,
		Logger:   log,
		Registry: reg,
	})

	flagClient := flags.New(flags.Config{
		BaseURL:  config.ServiceURL("differential-girdlespring"),
		Defaults: opts.FlagDefaults,
		Logger:   log,
	})

	var inj *malaise.Injector
	if opts.ApplyMalaise {
		inj = malaise.New(malaise.Config{
			EngineURL: config.String("MALAISE_ENGINE_URL", ""),
			Service:   service,
			Registry:  reg,
			Logger:    log,
		})
		if inj != nil {
			srv.Use(inj.Middleware)
		}
	}

	return &Runtime{
		Service:           service,
		Version:           version,
		Log:               log,
		Registry:          reg,
		Server:            srv,
		Flags:             flagClient,
		Malaise:           inj,
		shutdownTelemetry: shutdown,
	}, nil
}

// Client builds an outbound client for an upstream service, reading its base URL
// from the conventional <SERVICE_NAME>_URL environment variable.
func (r *Runtime) Client(service string) (*httpx.Client, error) {
	baseURL := config.ServiceURL(service)
	if baseURL == "" {
		return nil, fmt.Errorf("kit: %s is not set", config.URLVar(service))
	}
	return httpx.NewClient(httpx.ClientConfig{
		Service:  service,
		BaseURL:  baseURL,
		Registry: r.Registry,
		Logger:   r.Log,
	})
}

// Run starts the flag and malaise pollers, then serves until SIGTERM or SIGINT,
// then drains. It always shuts tracing down before returning.
func (r *Runtime) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	r.Flags.Start(ctx)
	r.Malaise.Start(ctx)

	r.Log.InfoContext(ctx, "encabulator service starting",
		slog.String("version", r.Version), slog.String("addr", r.Server.Addr()))

	err := r.Server.Run(ctx)
	if cerr := r.Close(context.WithoutCancel(ctx)); cerr != nil && err == nil {
		err = cerr
	}
	r.Log.Info("encabulator service stopped")
	return err
}

// Close flushes tracing. Run calls it; callers that do not use Run should defer it.
func (r *Runtime) Close(ctx context.Context) error {
	if r.shutdownTelemetry == nil {
		return nil
	}
	return r.shutdownTelemetry(ctx)
}
