package cache

import (
	"testing"
	"time"
)

func TestCacheOperations(t *testing.T) {
	// Note: These are unit tests that don't require actual Redis connection
	// Integration tests would require a test Redis instance

	t.Run("TTL conversion", func(t *testing.T) {
		ttlSeconds := 60
		ttl := time.Duration(ttlSeconds) * time.Second

		if ttl != 60*time.Second {
			t.Errorf("TTL conversion failed: got %v, want %v", ttl, 60*time.Second)
		}
	})

	t.Run("Zero TTL", func(t *testing.T) {
		ttlSeconds := 0
		ttl := time.Duration(ttlSeconds) * time.Second

		if ttl != 0 {
			t.Errorf("Zero TTL should be 0, got %v", ttl)
		}
	})
}
