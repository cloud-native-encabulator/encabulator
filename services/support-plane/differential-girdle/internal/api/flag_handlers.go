package api

import (
	"net/http"

	"github.com/encabulator/differential-girdle/internal/config"
	"github.com/gin-gonic/gin"
)

func listFlagsHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		flags := store.ListFlags()
		c.JSON(http.StatusOK, gin.H{
			"flags": flags,
			"count": len(flags),
		})
	}
}

func getFlagHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		flag, ok := store.GetFlag(name)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "feature flag not found",
			})
			return
		}

		c.JSON(http.StatusOK, flag)
	}
}

func createFlagHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var flag config.FeatureFlag
		if err := c.ShouldBindJSON(&flag); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Check if flag already exists
		if _, ok := store.GetFlag(flag.Name); ok {
			c.JSON(http.StatusConflict, gin.H{
				"error": "feature flag already exists",
			})
			return
		}

		// Validate rollout percentage
		if flag.Rollout < 0 || flag.Rollout > 100 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "rollout must be between 0 and 100",
			})
			return
		}

		store.SetFlag(&flag)
		c.JSON(http.StatusCreated, flag)
	}
}

func updateFlagHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		var flag config.FeatureFlag
		if err := c.ShouldBindJSON(&flag); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Ensure name matches
		flag.Name = name

		// Validate rollout percentage
		if flag.Rollout < 0 || flag.Rollout > 100 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "rollout must be between 0 and 100",
			})
			return
		}

		store.SetFlag(&flag)
		c.JSON(http.StatusOK, flag)
	}
}

func deleteFlagHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		if !store.DeleteFlag(name) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "feature flag not found",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "feature flag deleted",
		})
	}
}

func evaluateFlagHandler(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		var req struct {
			UserID string `json:"user_id" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		enabled := store.EvaluateFlag(name, req.UserID)

		c.JSON(http.StatusOK, gin.H{
			"flag":    name,
			"user_id": req.UserID,
			"enabled": enabled,
		})
	}
}
