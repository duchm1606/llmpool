package redis

import (
	"context"
	"fmt"
	"strings"

	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func Connect(ctx context.Context, addr, password string, db int) (*redislib.Client, error) {
	log := loggerinfra.ForModuleLazy("infra.redis")
	log.Info("connecting redis",
		zap.String("addr", addr),
		zap.Int("db", db),
		zap.Bool("password_set", strings.TrimSpace(password) != ""),
	)

	client := redislib.NewClient(&redislib.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		log.Error("connect redis failed", zap.Error(err))
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	log.Info("redis connected")

	return client, nil
}
