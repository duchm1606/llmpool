package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func Connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	return conn, nil
}
