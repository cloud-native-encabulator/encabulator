package api

import (
	"net/http"

	"github.com/encabulator/differential-girdle/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RegisterRoutes registers all API routes
func RegisterRoutes(router *gin.Engine, store *config.Store, cfg *config.Config) {
	// Health endpoints
	router.GET("/health", healthHandler)
	router.GET("/ready", readyHandler)

	// Metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Configuration endpoints
		configs := v1.Group("/configs")
		{
			configs.GET("", listConfigsHandler(store))
			configs.GET("/:key", getConfigHandler(store))
			configs.POST("", createConfigHandler(store))
			configs.PUT("/:key", updateConfigHandler(store))
			configs.DELETE("/:key", deleteConfigHandler(store))
		}

		// Feature flag endpoints
		flags := v1.Group("/flags")
		{
			flags.GET("", listFlagsHandler(store))
			flags.GET("/:name", getFlagHandler(store))
			flags.POST("", createFlagHandler(store))
			flags.PUT("/:name", updateFlagHandler(store))
			flags.DELETE("/:name", deleteFlagHandler(store))
			flags.POST("/:name/evaluate", evaluateFlagHandler(store))
		}
	}
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

func readyHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
	})
}
