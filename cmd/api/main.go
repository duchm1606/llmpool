package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	deliveryhttp "github.com/duchoang/llmpool/internal/delivery/http"
	configinfra "github.com/duchoang/llmpool/internal/infra/config"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	"github.com/duchoang/llmpool/internal/platform/server"
	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	"go.uber.org/zap"
)

func main() {
	cfg, err := configinfra.Load()
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	logger, err := loggerinfra.New(cfg.Log)
	if err != nil {
		panic(fmt.Errorf("init logger: %w", err))
	}
	defer func() {
		_ = logger.Sync()
	}()

	healthService := usecasehealth.NewService()
	router := deliveryhttp.NewRouter(logger, healthService)

	httpServer := server.NewHTTPServer(cfg.Server, router)

	go func() {
		logger.Info("starting API server",
			zap.String("host", cfg.Server.Host),
			zap.Int("port", cfg.Server.Port),
			zap.String("lb_strategy", cfg.Orchestrator.LBStrategy),
		)

		if serveErr := httpServer.Start(); serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Fatal("server stopped unexpectedly", zap.Error(serveErr))
		}
	}()

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-shutdownCtx.Done()
	logger.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if shutdownErr := httpServer.Shutdown(ctx); shutdownErr != nil {
		logger.Error("graceful shutdown failed", zap.Error(shutdownErr))
		os.Exit(1)
	}

	logger.Info("server shutdown complete")
}
