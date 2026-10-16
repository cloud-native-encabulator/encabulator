// Command malaise-injection-engine induces malaise for the purpose of
// demonstrating that the encabulator recovers from it.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/config"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/malaise-injection-engine/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/malaise-injection-engine/internal/engine"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/malaise-injection-engine/internal/kube"
	"github.com/cloud-native-encabulator/encabulator/services/control-plane/malaise-injection-engine/internal/store"
)

const serviceName = "malaise-injection-engine"

// startupReconcileTimeout bounds the boot-time sweep that undoes anything a
// previous incarnation left behind.
const startupReconcileTimeout = 15 * time.Second

func main() {
	if err := run(context.Background()); err != nil {
		slog.Default().Error("malaise-injection-engine failed to start", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	rt, err := kit.Bootstrap(ctx, kit.Options{
		Service: serviceName,
		// The engine does not inject malaise into itself; somebody has to stay
		// well enough to stop the others suffering.
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

	valkeyURL, err := config.Require("VALKEY_URL")
	if err != nil {
		return err
	}
	st, err := store.New(store.Config{URL: valkeyURL})
	if err != nil {
		return err
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			rt.Log.Warn("valkey did not close cleanly", "error", cerr.Error())
		}
	}()
	rt.Server.AddReadyCheck("valkey", st.Ping)

	// Kubernetes-level malaise needs the API server. Without it the engine still
	// serves the app-level kinds, so a local run outside a cluster is useful.
	var cluster engine.Cluster
	actuator, kerr := kube.NewInCluster(kube.Config{
		Namespace:              config.String("POD_NAMESPACE", "encabulator"),
		ObservabilityNamespace: config.String("OBSERVABILITY_NAMESPACE", "encabulator-observability"),
		Logger:                 rt.Log,
	})
	if kerr != nil {
		rt.Log.Warn("kubernetes-level malaise is unavailable; app-level kinds still work",
			"error", kerr.Error())
	} else {
		cluster = actuator
		rt.Server.AddReadyCheck("kubernetes", actuator.Ping)
		rt.Log.Info("kubernetes-level malaise armed", "namespace", actuator.Namespace())
	}

	eng, err := engine.New(engine.Options{
		Store:    st,
		Cluster:  cluster,
		Logger:   rt.Log,
		Registry: rt.Registry,
	})
	if err != nil {
		return err
	}

	api.New(eng, rt.Log, time.Now).Register(rt.Server)

	// A restart must never strand a fault: reconcile once before serving.
	bootCtx, cancelBoot := context.WithTimeout(ctx, startupReconcileTimeout)
	if expired, rerr := eng.Reconcile(bootCtx); rerr != nil {
		rt.Log.Warn("startup reconcile was not clean", "error", rerr.Error())
	} else if expired > 0 {
		rt.Log.Info("cleared malaise left over from a previous life", "expired", expired)
	}
	cancelBoot()

	// The reconcile loop keeps running through the drain so that expiries are
	// honoured right up to the moment the process exits.
	loopCtx, stopLoop := context.WithCancel(context.WithoutCancel(ctx))
	defer stopLoop()
	go eng.Run(loopCtx)

	return rt.Run(ctx)
}
