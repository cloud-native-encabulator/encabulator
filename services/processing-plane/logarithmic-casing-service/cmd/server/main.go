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

	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/logarithmic-casing-service/internal/api"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/logarithmic-casing-service/internal/broker"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/logarithmic-casing-service/pkg/health"
)

func main() {
	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Get configuration from environment
	port := getEnv("PORT", "8080")
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")

	// Initialize NATS broker
	logger.Info().Msg("Connecting to NATS...")
	natsBroker, err := broker.NewNATSBroker(natsURL, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to NATS")
	}
	defer natsBroker.Close()

	// Setup health checks
	healthHandler := health.NewHealthHandler()
	healthHandler.AddChecker("nats", health.NewBrokerChecker(natsBroker))

	// Setup API handlers
	apiHandler := api.NewHandler(natsBroker, logger)

	// Setup router
	router := mux.NewRouter()

	// Health endpoints
	router.HandleFunc("/health", healthHandler.LivenessHandler).Methods("GET")
	router.HandleFunc("/ready", healthHandler.ReadinessHandler).Methods("GET")
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// API endpoints
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.HandleFunc("/messages", apiHandler.PublishMessage).Methods("POST")
	apiRouter.HandleFunc("/stats", apiHandler.GetStats).Methods("GET")
	apiRouter.HandleFunc("/topics/{topic}/stats", apiHandler.GetTopicStats).Methods("GET")

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
