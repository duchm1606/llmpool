package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"github.com/redis/go-redis/v9"
)

const (
	credStateKeyPrefix  = "quota:cred:"  //nolint:gosec // Not credentials
	modelStateKeyPrefix = "quota:model:" //nolint:gosec // Not credentials
	credStatesPattern   = "quota:cred:*" //nolint:gosec // Not credentials
)

// RedisStateCache implements StateCache using Redis.
type RedisStateCache struct {
	client *redis.Client
}

// NewRedisStateCache creates a new Redis-backed state cache.
func NewRedisStateCache(client *redis.Client) *RedisStateCache {
	return &RedisStateCache{client: client}
}

// GetCredentialState retrieves cached credential state.
func (c *RedisStateCache) GetCredentialState(ctx context.Context, credentialID string) (*domainquota.CredentialState, error) {
	key := credStateKeyPrefix + credentialID
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss is not an error
		}
		return nil, fmt.Errorf("get credential state: %w", err)
	}

	var state domainquota.CredentialState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal credential state: %w", err)
	}

	return &state, nil
}

// SetCredentialState stores credential state with TTL.
func (c *RedisStateCache) SetCredentialState(ctx context.Context, state domainquota.CredentialState, ttl time.Duration) error {
	key := credStateKeyPrefix + state.CredentialID
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal credential state: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("set credential state: %w", err)
	}

	return nil
}

// GetModelState retrieves cached per-model state.
func (c *RedisStateCache) GetModelState(ctx context.Context, credentialID, modelID string) (*domainquota.ModelState, error) {
	key := modelStateKeyPrefix + credentialID + ":" + modelID
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get model state: %w", err)
	}

	var state domainquota.ModelState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal model state: %w", err)
	}

	return &state, nil
}

// SetModelState stores per-model state with TTL.
func (c *RedisStateCache) SetModelState(ctx context.Context, state domainquota.ModelState, ttl time.Duration) error {
	key := modelStateKeyPrefix + state.CredentialID + ":" + state.ModelID
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal model state: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("set model state: %w", err)
	}

	return nil
}

// ListCredentialStates retrieves all cached credential states.
// Uses SCAN instead of KEYS to avoid blocking Redis in production.
func (c *RedisStateCache) ListCredentialStates(ctx context.Context) ([]domainquota.CredentialState, error) {
	var allKeys []string
	var cursor uint64

	// Use SCAN to iterate through keys without blocking Redis
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, credStatesPattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("scan credential state keys: %w", err)
		}
		allKeys = append(allKeys, keys...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(allKeys) == 0 {
		return []domainquota.CredentialState{}, nil
	}

	// Use pipeline for efficiency
	pipe := c.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(allKeys))
	for i, key := range allKeys {
		cmds[i] = pipe.Get(ctx, key)
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("exec pipeline: %w", err)
	}

	states := make([]domainquota.CredentialState, 0, len(allKeys))
	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue // Skip errors (key may have expired)
		}

		var state domainquota.CredentialState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		states = append(states, state)
	}

	return states, nil
}

// Ping checks if Redis is available.
func (c *RedisStateCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// CountCredentialStates returns count of cached states.
// Uses SCAN instead of KEYS to avoid blocking Redis in production.
func (c *RedisStateCache) CountCredentialStates(ctx context.Context) (int64, error) {
	var count int64
	var cursor uint64

	// Use SCAN to count keys without blocking Redis
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, credStatesPattern, 100).Result()
		if err != nil {
			return 0, fmt.Errorf("scan credential state keys: %w", err)
		}
		count += int64(len(keys))
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return count, nil
}
