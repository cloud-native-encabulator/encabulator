package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/api"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/internal/db"
	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/hydrocoptic-datastore/pkg/health"
)

func main() {
	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Get configuration from environment
	port := getEnv("PORT", "8080")
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "hydrocoptic")

	// Build connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Initialize database
	logger.Info().Msg("Connecting to database...")
	database, err := db.NewPostgresDB(connStr)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer database.Close()

	// Initialize schema
	logger.Info().Msg("Initializing database schema...")
	if err := database.InitSchema(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize schema")
	}

	// Setup health checks
	healthHandler := health.NewHealthHandler()
	healthHandler.AddChecker("database", health.NewDBChecker(database))

	// Setup API handlers
	apiHandler := api.NewHandler(database, logger)

	// Setup router
	router := mux.NewRouter()

	// Health endpoints
	router.HandleFunc("/health", healthHandler.LivenessHandler).Methods("GET")
	router.HandleFunc("/ready", healthHandler.ReadinessHandler).Methods("GET")
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// API endpoints
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.HandleFunc("/encabulations", apiHandler.CreateEncabulation).Methods("POST")
	apiRouter.HandleFunc("/encabulations", apiHandler.ListEncabulations).Methods("GET")
	apiRouter.HandleFunc("/encabulations/{id}", apiHandler.GetEncabulation).Methods("GET")
	apiRouter.HandleFunc("/encabulations/{id}", apiHandler.UpdateEncabulation).Methods("PUT")
	apiRouter.HandleFunc("/encabulations/{id}", apiHandler.DeleteEncabulation).Methods("DELETE")

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
