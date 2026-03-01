package redis

import (
	"context"
	"fmt"

	redislib "github.com/redis/go-redis/v9"
)

func Connect(ctx context.Context, addr, password string, db int) (*redislib.Client, error) {
	client := redislib.NewClient(&redislib.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	return client, nil
}
