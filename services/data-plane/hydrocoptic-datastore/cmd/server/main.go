// Command server runs hydrocoptic-datastore: six hydrocoptic marzlevanes of pure
// persistence, also known as the order ledger.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/config"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/events"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/consumer"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/hydrocoptic-datastore/internal/store"
)

const serviceName = "hydrocoptic-datastore"

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("the marzlevanes have stopped turning", slog.Any("error", err))
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
	rt.Log.InfoContext(ctx, "ledger schema migrated")

	nc, js, err := events.Connect(config.String("NATS_URL", "nats://nats:4222"), serviceName)
	if err != nil {
		return err
	}
	defer nc.Close()

	stream, err := events.EnsureStream(ctx, js)
	if err != nil {
		return err
	}

	cons := consumer.New(st, rt.Log, rt.Malaise, rt.Registry)
	consumeCtx, err := cons.Start(ctx, stream)
	if err != nil {
		return err
	}
	defer consumeCtx.Stop()
	rt.Log.InfoContext(ctx, "consuming the whole stream",
		slog.String("durable", consumer.Durable), slog.String("filter", events.StreamSubjects))

	// Readiness covers this service's own hard dependencies and nothing else.
	rt.Server.AddReadyCheck("postgres", st.Ping)
	rt.Server.AddReadyCheck("nats", func(context.Context) error {
		if !nc.IsConnected() {
			return errors.New("not connected to NATS")
		}
		return nil
	})

	api.New(st, rt.Log, rt.Registry).Routes(rt.Server)

	return rt.Run(ctx)
}
