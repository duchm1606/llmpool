package postgres

import (
	"context"
	"fmt"
	"strings"

	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	log := loggerinfra.ForModuleLazy("infra.postgres")
	log.Info("connecting postgres", zap.Bool("dsn_set", strings.TrimSpace(dsn) != ""))

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Error("connect postgres failed", zap.Error(err))
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	// Verify connection is working
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		log.Error("ping postgres failed", zap.Error(err))
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	log.Info("postgres pool connected", zap.Int32("max_conns", pool.Config().MaxConns))

	return pool, nil
}
