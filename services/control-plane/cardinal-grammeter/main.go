// Command cardinal-grammeter automatically synchronizes cardinal grammeters,
// which is to say it generates synthetic customer traffic through the
// dingle-arm-gateway.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/config"

	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/api"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/customer"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/engine"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/gateway"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/journey"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/metrics"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/rng"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/stats"
	"github.com/jlbutler/cloud-native-encabulator/services/control-plane/cardinal-grammeter/internal/traffic"
)

// serviceName is this service's name in the catalog.
const serviceName = "cardinal-grammeter"

// gatewayService is the one upstream the grammeter is allowed to call.
const gatewayService = "dingle-arm-gateway"

// journeyTimeout bounds a whole journey, which for `track` is three calls.
const journeyTimeout = 20 * time.Second

// defaultPeriodSeconds is the starting period for the shaped patterns. The
// console changes it through PUT /v1/traffic; there is no environment variable
// for it in the design.
const defaultPeriodSeconds = 60

func main() {
	if err := run(context.Background()); err != nil {
		slog.Default().Error("cardinal-grammeter failed to start", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	rt, err := kit.Bootstrap(ctx, kit.Options{
		Service: serviceName,
		// The grammeter does not induce malaise in itself; it would only be
		// measuring its own limp.
		ApplyMalaise: false,
	})
	if err != nil {
		return err
	}
	defer func() {
		if cerr := rt.Close(context.WithoutCancel(ctx)); cerr != nil {
			rt.Log.Warn("telemetry shutdown was not clean", "error", cerr.Error())
		}
	}()

	gatewayURL := config.ServiceURL(gatewayService)
	if gatewayURL == "" {
		return fmt.Errorf("%s is required: the grammeter has nowhere to send traffic",
			config.URLVar(gatewayService))
	}
	seed, err := config.Require("CUSTOMER_PASSWORD_SEED")
	if err != nil {
		return errors.New("CUSTOMER_PASSWORD_SEED is required: synthetic customer passwords are derived from it")
	}

	met := metrics.New(rt.Registry, gatewayService)

	// Outbound calls are traced so that the console's service graph shows the
	// grammeter driving the gateway. No retries: see the gateway package.
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + gatewayService
			}),
		),
	}
	gw := gateway.New(gatewayURL, httpClient, met)

	customers := config.Int("CUSTOMERS", 50)
	pool := customer.NewPool(gw, seed, customers, time.Now)
	recorder := stats.NewRecorder(time.Now)

	runner := journey.NewRunner(journey.RunnerOptions{
		Gateway: gw,
		Pool:    pool,
		Stats:   recorder,
		Metrics: met,
		Source:  rng.Global(),
		Logger:  rt.Log,
		Timeout: journeyTimeout,
	})

	autostart := config.Bool("AUTOSTART", false)
	eng, err := engine.New(engine.Options{
		Runner:      runner,
		Pool:        pool,
		Stats:       recorder,
		Source:      rng.Global(),
		Logger:      rt.Log,
		Target:      gatewayURL,
		MaxInFlight: config.Int("MAX_IN_FLIGHT", 256),
		Initial: engine.Config{
			Running:       autostart,
			RPS:           config.Float("AUTOSTART_RPS", 5),
			Pattern:       traffic.PatternSteady,
			PeriodSeconds: defaultPeriodSeconds,
			Mix:           journey.DefaultMix(),
			Customers:     customers,
		},
		TargetRPSGauge: met.TargetRPS(),
		DroppedCounter: met.Dropped(),
	})
	if err != nil {
		return err
	}
	met.RegisterInFlight(func() float64 { return float64(eng.InFlight()) })

	api.New(eng, rt.Log).Register(rt.Server)

	rt.Log.Info("cardinal grammeters standing by",
		"target", gatewayURL, "customers", customers, "autostart", autostart)

	// The schedule keeps running through the drain so that in-flight journeys
	// finish; it stops when the process context is cancelled.
	loopCtx, stopLoop := context.WithCancel(context.WithoutCancel(ctx))
	defer stopLoop()
	go eng.Run(loopCtx)

	return rt.Run(ctx)
}
