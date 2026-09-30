// Command server runs differential-girdlespring, the feature flag service on the
// "up" end of the grammeters.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit"
	"github.com/jlbutler/cloud-native-encabulator/libs/go/kit/config"

	"github.com/jlbutler/cloud-native-encabulator/services/support-plane/differential-girdlespring/internal/api"
	"github.com/jlbutler/cloud-native-encabulator/services/support-plane/differential-girdlespring/internal/store"
)

const serviceName = "differential-girdlespring"

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("the girdlespring has seized", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	rt, err := kit.Bootstrap(ctx, kit.Options{Service: serviceName, Version: version, ApplyMalaise: true})
	if err != nil {
		return err
	}
	defer func() { _ = rt.Close(context.Background()) }()

	dsn, err := config.Require("DATABASE_URL")
	if err != nil {
		return err
	}
	st, err := store.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := store.Migrate(ctx, st.Pool()); err != nil {
		return err
	}
	if err := st.Seed(ctx, api.Seeds()); err != nil {
		return err
	}
	rt.Log.InfoContext(ctx, "flag table migrated and seeded")

	rt.Server.AddReadyCheck("postgres", st.Ping)
	api.New(st, rt.Log, rt.Registry).Routes(rt.Server)

	return rt.Run(ctx)
}
