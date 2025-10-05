package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/dingle-arm-api-gateway/internal/middleware"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/dingle-arm-api-gateway/internal/proxy"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/dingle-arm-api-gateway/pkg/health"
)

func main() {
	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Get configuration from environment
	port := getEnv("PORT", "8080")
	backendURL := getEnv("BACKEND_URL", "http://panametric-fam-processor:8080")
	rpsLimit := getEnvFloat("RATE_LIMIT_RPS", 100.0)
	burstLimit := getEnvInt("RATE_LIMIT_BURST", 200)

	logger.Info().
		Str("port", port).
		Str("backend", backendURL).
		Float64("rps_limit", rpsLimit).
		Int("burst_limit", burstLimit).
		Msg("Starting API Gateway")

	// Initialize reverse proxy
	reverseProxy, err := proxy.NewReverseProxy(backendURL, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create reverse proxy")
	}

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter(rpsLimit, burstLimit)

	// Setup router
	router := mux.NewRouter()

	// Health endpoints (no rate limiting)
	router.HandleFunc("/health", health.LivenessHandler).Methods("GET")
	router.HandleFunc("/ready", health.ReadinessHandler).Methods("GET")
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// API routes (with rate limiting and logging)
	apiRouter := router.PathPrefix("/api").Subrouter()
	apiRouter.Use(middleware.LoggingMiddleware(logger))
	apiRouter.Use(rateLimiter.Middleware)
	apiRouter.PathPrefix("/").Handler(reverseProxy)

	// Setup server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("port", port).Msg("Server started")
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

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
