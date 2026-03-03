package postgres

import (
	"context"
	"fmt"
	"strings"

	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

func Connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	log := loggerinfra.ForModuleLazy("infra.postgres")
	log.Info("connecting postgres", zap.Bool("dsn_set", strings.TrimSpace(dsn) != ""))

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Error("connect postgres failed", zap.Error(err))
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	log.Info("postgres connected")

	return conn, nil
}
