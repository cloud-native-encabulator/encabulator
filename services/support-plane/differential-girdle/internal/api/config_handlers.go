package api

import (
	"net/http"

	"github.com/encabulator/differential-girdle/internal/config"
	"github.com/gin-gonic/gin"
)

func listConfigsHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		configs := store.ListConfigs()
		c.JSON(http.StatusOK, gin.H{
			"configs": configs,
			"count":   len(configs),
		})
	}
}

func getConfigHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Param("key")

		cfg, ok := store.GetConfig(key)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "configuration not found",
			})
			return
		}

		c.JSON(http.StatusOK, cfg)
	}
}

func createConfigHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cfg config.ConfigValue
		if err := c.ShouldBindJSON(&cfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Check if config already exists
		if _, ok := store.GetConfig(cfg.Key); ok {
			c.JSON(http.StatusConflict, gin.H{
				"error": "configuration already exists",
			})
			return
		}

		store.SetConfig(&cfg)
		c.JSON(http.StatusCreated, cfg)
	}
}

func updateConfigHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Param("key")

		var cfg config.ConfigValue
		if err := c.ShouldBindJSON(&cfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Ensure key matches
		cfg.Key = key

		store.SetConfig(&cfg)
		c.JSON(http.StatusOK, cfg)
	}
}

func deleteConfigHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Param("key")

		if !store.DeleteConfig(key) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "configuration not found",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "configuration deleted",
		})
	}
}
