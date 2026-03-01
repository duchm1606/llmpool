package logger

import (
	"fmt"

	configinfra "github.com/duchoang/llmpool/internal/infra/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg configinfra.LogConfig) (*zap.Logger, error) {
	var zapConfig zap.Config
	if cfg.Development {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	zapConfig.Level = zap.NewAtomicLevelAt(level)

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}

	return logger, nil
}
