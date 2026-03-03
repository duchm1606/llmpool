package logger

import (
	"sync"

	configinfra "github.com/duchoang/llmpool/internal/infra/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	module string
}

func (l *Logger) getLogger() *zap.Logger {
	return forModule(l.module)
}

func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.getLogger().Info(msg, fields...)
}

func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.getLogger().Error(msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.getLogger().Warn(msg, fields...)
}

func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.getLogger().Debug(msg, fields...)
}

func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.getLogger().Fatal(msg, fields...)
}

var (
	globalBase  *zap.Logger
	loggers     = make(map[string]*zap.Logger)
	mu          sync.RWMutex
	initialized bool
)

func Initialize(cfg configinfra.LogConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if initialized {
		return nil
	}

	var zapConfig zap.Config
	if cfg.Development {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}
	zapConfig.Level = zap.NewAtomicLevelAt(level)

	if cfg.Format == "json" {
		zapConfig.Encoding = "json"
	} else {
		zapConfig.Encoding = "console"
	}

	zapConfig.OutputPaths = []string{"stdout"}
	zapConfig.ErrorOutputPaths = []string{"stderr"}

	base, err := zapConfig.Build()
	if err != nil {
		return err
	}

	globalBase = base
	initialized = true
	loggers = make(map[string]*zap.Logger)

	return nil
}

func ForModuleLazy(module string) *Logger {
	return &Logger{module: module}
}

func ForModule(module string) *zap.Logger {
	return forModule(module)
}

func forModule(module string) *zap.Logger {
	mu.RLock()
	base := globalBase
	mu.RUnlock()

	if base == nil {
		return zap.NewNop()
	}

	mu.RLock()
	if moduleLogger, exists := loggers[module]; exists {
		mu.RUnlock()
		return moduleLogger
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	if moduleLogger, exists := loggers[module]; exists {
		return moduleLogger
	}

	moduleLogger := globalBase.WithOptions(zap.AddCallerSkip(1)).With(
		zap.String("module", module),
	)
	loggers[module] = moduleLogger
	return moduleLogger
}

func Sync() error {
	mu.RLock()
	base := globalBase
	mu.RUnlock()

	if base != nil {
		return base.Sync()
	}
	return nil
}
