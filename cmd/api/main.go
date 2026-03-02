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
	credentialrepo "github.com/duchoang/llmpool/internal/infra/credential"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	oauthinfra "github.com/duchoang/llmpool/internal/infra/oauth"
	postgresinfra "github.com/duchoang/llmpool/internal/infra/postgres"
	refreshinfra "github.com/duchoang/llmpool/internal/infra/refresh"
	redisinfra "github.com/duchoang/llmpool/internal/infra/redis"
	securityinfra "github.com/duchoang/llmpool/internal/infra/security"
	"github.com/duchoang/llmpool/internal/platform/server"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
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

	encryptionKey := os.Getenv("LLMPOOL_SECURITY_ENCRYPTION_KEY")
	if encryptionKey == "" {
		panic(fmt.Errorf("LLMPOOL_SECURITY_ENCRYPTION_KEY is required"))
	}

	encryptor, err := securityinfra.NewAesGCMEncryptor(encryptionKey)
	if err != nil {
		panic(fmt.Errorf("initialize encryptor: %w", err))
	}

	healthService := usecasehealth.NewService()
	postgresConn, err := postgresinfra.Connect(context.Background(), cfg.Postgres.DSN)

	if err != nil {
		panic(fmt.Errorf("connect postgres: %w", err))
	}
	defer func() {
		if closeErr := postgresConn.Close(context.Background()); closeErr != nil {
			logger.Error("close postgres connection", zap.Error(closeErr))
		}
	}()

	redisClient, err := redisinfra.Connect(context.Background(), cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		panic(fmt.Errorf("connect redis: %w", err))
	}
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			logger.Error("close redis connection", zap.Error(closeErr))
		}
	}()
	logger.Info("redis connected", zap.String("addr", cfg.Redis.Addr))
	_ = redisClient

	profileRepo := credentialrepo.NewCredentialRepository(postgresConn)
	importService := usecasecredential.NewImportService(profileRepo, encryptor)

	refreshers := map[string]usecasecredential.Refresher{
		"openai":      refreshinfra.NewNoopRefresher(),
		"anthropic":   refreshinfra.NewNoopRefresher(),
		"gemini":      refreshinfra.NewNoopRefresher(),
		"vertex":      refreshinfra.NewNoopRefresher(),
		"qwen":        refreshinfra.NewNoopRefresher(),
		"iflow":       refreshinfra.NewNoopRefresher(),
		"antigravity": refreshinfra.NewNoopRefresher(),
		"kiro":        refreshinfra.NewNoopRefresher(),
		"copilot":     refreshinfra.NewNoopRefresher(),
	}

	refreshService := usecasecredential.NewRefreshService(profileRepo, refreshers, encryptor)
	oauthProvider := oauthinfra.NewCodexProvider(cfg.OAuth.Codex)
	oauthSessionStore := oauthinfra.NewRedisSessionStore(redisClient, cfg.OAuth.Codex.SessionTTL)
	router := deliveryhttp.NewRouter(logger, healthService, importService, refreshService, oauthProvider, oauthSessionStore, cfg.OAuth.Codex, cfg.OAuth.Codex.SessionTTL)

	httpServer := server.NewHTTPServer(cfg.Server, router)
	refreshWorker := server.NewRefreshWorker(refreshService, logger, cfg.Credential.RefreshInterval)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go refreshWorker.Start(shutdownCtx)

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
