package main

import (
	"log"
	"os"

	"github.com/encabulator/differential-girdle/internal/api"
	"github.com/encabulator/differential-girdle/internal/config"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize configuration store
	store := config.NewStore()

	// Set up Gin router
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// Register routes
	api.RegisterRoutes(router, store, cfg)

	// Start server
	addr := ":" + cfg.Port
	log.Printf("Starting Differential Girdle on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
