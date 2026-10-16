// Command server runs sinusoidal-recorder, which records sinusoidal
// repleneration so that it can be reduced.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/cloud-native-encabulator/encabulator/libs/go/kit"
	"github.com/cloud-native-encabulator/encabulator/libs/go/kit/config"

	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/api"
	"github.com/cloud-native-encabulator/encabulator/services/data-plane/sinusoidal-recorder/internal/influx"
)

const serviceName = "sinusoidal-recorder"

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("the recorder has lost the sinusoidal trace", slog.Any("error", err))
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

	url, err := config.Require("INFLUXDB_URL")
	if err != nil {
		return err
	}
	// The token may legitimately be empty: InfluxDB 3 Core can run without auth.
	client, err := influx.New(influx.Config{
		URL:      url,
		Token:    config.String("INFLUXDB_TOKEN", ""),
		Database: config.String("INFLUXDB_DATABASE", "sinusoidal"),
		Logger:   rt.Log,
	})
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	// Creating the database and table up front means a read before the first
	// write answers with an empty result instead of an error.
	if err := client.EnsureDatabase(ctx); err != nil {
		rt.Log.WarnContext(ctx, "could not ensure the telemetry database; writes will create it",
			slog.Any("error", err))
	} else if err := client.EnsureTable(ctx); err != nil {
		rt.Log.WarnContext(ctx, "could not ensure the telemetry table; writes will create it",
			slog.Any("error", err))
	}

	rt.Server.AddReadyCheck("influxdb", client.Health)
	api.New(client, rt.Log, rt.Registry).Routes(rt.Server)

	return rt.Run(ctx)
}
