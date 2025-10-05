package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-database/internal/api"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-database/internal/tsdb"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/sinusoidal-database/pkg/health"
)

func main() {
	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Get configuration from environment
	port := getEnv("PORT", "8080")
	influxURL := getEnv("INFLUX_URL", "http://localhost:8086")
	influxToken := getEnv("INFLUX_TOKEN", "")
	influxOrg := getEnv("INFLUX_ORG", "encabulator")
	influxBucket := getEnv("INFLUX_BUCKET", "sinusoidal")

	// Initialize InfluxDB
	logger.Info().Msg("Connecting to InfluxDB...")
	influxDB, err := tsdb.NewInfluxDB(influxURL, influxToken, influxOrg, influxBucket)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to InfluxDB")
	}
	defer influxDB.Close()

	// Setup health checks
	healthHandler := health.NewHealthHandler()
	healthHandler.AddChecker("influxdb", health.NewTSDBChecker(influxDB))

	// Setup API handlers
	apiHandler := api.NewHandler(influxDB, logger)

	// Setup router
	router := mux.NewRouter()

	// Health endpoints
	router.HandleFunc("/health", healthHandler.LivenessHandler).Methods("GET")
	router.HandleFunc("/ready", healthHandler.ReadinessHandler).Methods("GET")
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// API endpoints
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.HandleFunc("/metrics", apiHandler.WriteMetrics).Methods("POST")
	apiRouter.HandleFunc("/metrics/query", apiHandler.QueryMetrics).Methods("POST")
	apiRouter.HandleFunc("/metrics/range", apiHandler.QueryMetricsRange).Methods("GET")

	// Setup server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("port", port).Msg("Starting server...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	logger.Info().Msg("Server exited")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
