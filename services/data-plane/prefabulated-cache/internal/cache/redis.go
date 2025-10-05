package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jlbutler/cloud-native-encabulator/services/data-plane/prefabulated-cache/internal/models"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
		ctx:    ctx,
	}, nil
}

func (r *RedisCache) Set(key string, value interface{}, ttl time.Duration) error {
	// Serialize value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return r.client.Set(r.ctx, key, data, ttl).Err()
}

func (r *RedisCache) Get(key string) (interface{}, bool, error) {
	val, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get value: %w", err)
	}

	// Deserialize JSON
	var result interface{}
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return result, true, nil
}

func (r *RedisCache) Delete(key string) error {
	return r.client.Del(r.ctx, key).Err()
}

func (r *RedisCache) GetStats() (*models.CacheStats, error) {
	// Get total keys
	dbSize, err := r.client.DBSize(r.ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get db size: %w", err)
	}

	// Get memory info
	info, err := r.client.Info(r.ctx, "memory").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory info: %w", err)
	}

	// Get stats info for hit rate
	statsInfo, err := r.client.Info(r.ctx, "stats").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats info: %w", err)
	}

	// Parse hit rate (simplified)
	hitRate := 0.0
	_ = statsInfo // TODO: Parse actual hit rate from stats

	return &models.CacheStats{
		TotalKeys:   dbSize,
		UsedMemory:  info,
		HitRate:     hitRate,
		ConnectedAt: time.Now(),
	}, nil
}

func (r *RedisCache) Ping() error {
	return r.client.Ping(r.ctx).Err()
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}
