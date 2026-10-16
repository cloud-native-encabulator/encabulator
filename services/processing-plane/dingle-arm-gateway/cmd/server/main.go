// Command server runs dingle-arm-gateway, the drawn reciprocation dingle arm,
// employed whenever a barescent skor motion is required.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/config"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/flags"

	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/auth"
	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/gateway"
	"github.com/cloud-native-encabulator/encabulator/services/processing-plane/dingle-arm-gateway/internal/ratelimit"
)

const serviceName = "dingle-arm-gateway"

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("the dingle arm has stopped reciprocating", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	rt, err := kit.Bootstrap(ctx, kit.Options{
		Service:      serviceName,
		Version:      version,
		ApplyMalaise: true,
	})
	if err != nil {
		return err
	}
	defer func() { _ = rt.Close(context.Background()) }()

	// Every upstream in the routing table must be configured, so a missing
	// variable is a startup failure rather than a surprise at the first request.
	upstreams := make(map[string]string, 6)
	for _, service := range gateway.Upstreams() {
		url := config.ServiceURL(service)
		if url == "" {
			return fmt.Errorf("%s is not set", config.URLVar(service))
		}
		upstreams[service] = url
	}

	verifier, err := auth.NewVerifier(auth.Config{
		JWKSURL: upstreams["unilateral-phase-detractor"] + "/.well-known/jwks.json",
		Logger:  rt.Log,
	})
	if err != nil {
		return err
	}

	// The rate is read from the girdlespring on every request, so an operator can
	// move it mid-demo and see the effect immediately.
	limiter := ratelimit.New(
		func() float64 { return rt.Flags.Number(flags.GatewayRateLimitRPS) },
		config.Int("RATE_LIMIT_BUCKETS", ratelimit.DefaultCapacity),
	)

	gw, err := gateway.New(gateway.Config{
		Upstreams:       upstreams,
		Verifier:        verifier,
		Limiter:         limiter,
		UpstreamTimeout: config.Duration("UPSTREAM_TIMEOUT", gateway.DefaultUpstreamTimeout),
		Logger:          rt.Log,
		Registry:        rt.Registry,
	})
	if err != nil {
		return err
	}
	gw.Routes(rt.Server)

	// Readiness deliberately has no checks: the gateway's dependencies are all
	// downstream services, and design section 7 forbids probing those.
	rt.Log.InfoContext(ctx, "dingle arm ready to reciprocate",
		slog.Int("routes", len(gateway.Routes())),
		slog.Int("upstreams", len(upstreams)))

	// Fetch the JWKS before serving, and keep it fresh.
	verifier.Start(ctx)
	// The flag poller has to be running before the first rate limit decision; Run
	// starts it, and until it does the built-in default of 50 rps applies.
	return rt.Run(ctx)
}
