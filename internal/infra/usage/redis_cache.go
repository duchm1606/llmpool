package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	"github.com/redis/go-redis/v9"
)

const (
	statsKeyPrefix   = "usage:stats:"
	statsKeysPattern = "usage:stats:*"
)

// RedisStatsCache implements StatsCache using Redis.
type RedisStatsCache struct {
	client *redis.Client
}

// NewRedisStatsCache creates a new Redis-backed stats cache.
func NewRedisStatsCache(client *redis.Client) *RedisStatsCache {
	return &RedisStatsCache{client: client}
}

// GetDashboardStats returns cached dashboard stats.
func (c *RedisStatsCache) GetDashboardStats(ctx context.Context, period string) (*domainusage.DashboardStats, error) {
	key := statsKeyPrefix + period
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("get dashboard stats: %w", err)
	}

	var stats domainusage.DashboardStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("unmarshal dashboard stats: %w", err)
	}

	return &stats, nil
}

// SetDashboardStats caches dashboard stats.
func (c *RedisStatsCache) SetDashboardStats(ctx context.Context, period string, stats domainusage.DashboardStats, ttl time.Duration) error {
	key := statsKeyPrefix + period
	data, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("marshal dashboard stats: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("set dashboard stats: %w", err)
	}

	return nil
}

// InvalidateStats invalidates all cached stats.
func (c *RedisStatsCache) InvalidateStats(ctx context.Context) error {
	var cursor uint64
	var allKeys []string

	// Scan for all stats keys
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, statsKeysPattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scan stats keys: %w", err)
		}
		allKeys = append(allKeys, keys...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(allKeys) == 0 {
		return nil
	}

	// Delete all stats keys
	if err := c.client.Del(ctx, allKeys...).Err(); err != nil {
		return fmt.Errorf("delete stats keys: %w", err)
	}

	return nil
}
