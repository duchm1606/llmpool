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
	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	configinfra "github.com/duchoang/llmpool/internal/infra/config"
	credentialrepo "github.com/duchoang/llmpool/internal/infra/credential"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	oauthinfra "github.com/duchoang/llmpool/internal/infra/oauth"
	postgresinfra "github.com/duchoang/llmpool/internal/infra/postgres"
	providerinfra "github.com/duchoang/llmpool/internal/infra/provider"
	quotainfra "github.com/duchoang/llmpool/internal/infra/quota"
	redisinfra "github.com/duchoang/llmpool/internal/infra/redis"
	refreshinfra "github.com/duchoang/llmpool/internal/infra/refresh"
	securityinfra "github.com/duchoang/llmpool/internal/infra/security"
	"github.com/duchoang/llmpool/internal/platform/server"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	usecasequota "github.com/duchoang/llmpool/internal/usecase/quota"
	"go.uber.org/zap"
)

func main() {
	log := loggerinfra.ForModuleLazy("main")

	cfg, err := configinfra.Load()
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	if err := loggerinfra.Initialize(cfg.Log); err != nil {
		panic(fmt.Errorf("init logger: %w", err))
	}
	defer func() {
		_ = loggerinfra.Sync()
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
			log.Error("close postgres connection", zap.Error(closeErr))
		}
	}()

	redisClient, err := redisinfra.Connect(context.Background(), cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		panic(fmt.Errorf("connect redis: %w", err))
	}
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			log.Error("close redis connection", zap.Error(closeErr))
		}
	}()
	log.Info("redis connected", zap.String("addr", cfg.Redis.Addr))

	profileRepo := credentialrepo.NewCredentialRepository(postgresConn)
	importService := usecasecredential.NewImportService(profileRepo, encryptor)
	oauthCompletionService := usecasecredential.NewCompletionService(profileRepo, encryptor)

	oauthProvider := oauthinfra.NewCodexProvider(cfg.OAuth.Codex)
	oauthSessionStore := oauthinfra.NewRedisSessionStore(redisClient, cfg.OAuth.Codex.SessionTTL)

	refreshers := map[string]usecasecredential.Refresher{
		"openai":      refreshinfra.NewNoopRefresher(),
		"anthropic":   refreshinfra.NewNoopRefresher(),
		"gemini":      refreshinfra.NewNoopRefresher(),
		"vertex":      refreshinfra.NewNoopRefresher(),
		"qwen":        refreshinfra.NewNoopRefresher(),
		"iflow":       refreshinfra.NewNoopRefresher(),
		"antigravity": refreshinfra.NewNoopRefresher(),
		"kiro":        refreshinfra.NewNoopRefresher(),
		"codex":       oauthinfra.NewCodexRefresher(oauthProvider),
	}

	// Initialize Copilot OAuth provider and refresher
	copilotProvider := oauthinfra.NewCopilotProvider(cfg.OAuth.Copilot)
	copilotRefresher := oauthinfra.NewCopilotRefresher(copilotProvider)
	refreshers["copilot"] = copilotRefresher

	// Create Copilot OAuth completion service
	copilotOAuthCompletionService := usecasecredential.NewCopilotCompletionService(profileRepo, encryptor)

	refreshService := usecasecredential.NewRefreshService(profileRepo, refreshers, encryptor)

	// Initialize liveness checker with provider-specific routing
	quotaStateCache := quotainfra.NewRedisStateCache(redisClient)

	// Create Codex checker for codex/openai credentials using config
	codexCheckerCfg := quotainfra.CodexCheckerConfig{
		UsageURL: cfg.Liveness.CodexUsageURL,
		Timeout:  cfg.Liveness.CheckTimeout,
	}
	codexChecker := quotainfra.NewCodexChecker(codexCheckerCfg)

	// Create Copilot checker for GitHub Copilot credentials
	copilotCheckerCfg := quotainfra.CopilotCheckerConfig{
		UsageURL: cfg.Liveness.CopilotUsageURL,
		Timeout:  cfg.Liveness.CheckTimeout,
	}
	copilotChecker := quotainfra.NewCopilotChecker(copilotCheckerCfg)

	noopChecker := quotainfra.NewNoopChecker()

	// Route to appropriate checkers by credential type
	quotaCheckers := map[string]quotainfra.ProviderChecker{
		"codex":   codexChecker,
		"openai":  codexChecker,
		"copilot": copilotChecker,
	}
	quotaChecker := quotainfra.NewCheckerRouter(quotaCheckers, noopChecker)
	quotaServiceCfg := usecasequota.ServiceConfig{
		SamplePercent: cfg.Liveness.SamplePercent,
		StateTTL:      cfg.Liveness.StateTTL,
		Cooldown: domainquota.CooldownConfig{
			AuthFailureCooldown:  cfg.Liveness.AuthFailureCooldown,
			RateLimitInitial:     cfg.Liveness.RateLimitInitial,
			RateLimitMaxCooldown: cfg.Liveness.RateLimitMaxCooldown,
			NetworkErrorCooldown: cfg.Liveness.NetworkErrorCooldown,
			NetworkMaxRetries:    cfg.Liveness.NetworkMaxRetries,
		},
	}
	quotaService := usecasequota.NewService(
		profileRepo,
		encryptor,
		quotaChecker,
		quotaStateCache,
		loggerinfra.ForModule("usecase.quota"),
		quotaServiceCfg,
	)

	// Set copilot usage fetcher to enable full usage tracking
	quotaService.SetCopilotUsageFetcher(copilotChecker)

	quotaWorkerCfg := server.QuotaWorkerConfig{
		SampleInterval:    cfg.Liveness.SampleInterval,
		FullSweepInterval: cfg.Liveness.FullSweepInterval,
	}
	quotaWorker := server.NewQuotaWorker(
		quotaService,
		loggerinfra.ForModule("platform.server.quota_worker"),
		quotaWorkerCfg,
	)

	// Initialize completion service if routing is enabled
	var completionService usecasecompletion.CompletionService
	var completionRouter usecasecompletion.Router
	var providerRegistry usecasecompletion.ProviderRegistry
	if cfg.Routing.Enabled {
		completionService, completionRouter, providerRegistry = initCompletionService(cfg, profileRepo, encryptor, log)
		log.Info("completion routing enabled",
			zap.Strings("provider_priority", cfg.Routing.ProviderPriority),
			zap.Int("max_fallback_attempts", cfg.Routing.Fallback.MaxAttempts),
		)
	}

	router := deliveryhttp.NewRouterWithDeps(deliveryhttp.RouterDeps{
		Development:                   cfg.Log.Development,
		HealthService:                 healthService,
		ImportService:                 importService,
		RefreshService:                refreshService,
		OAuthProvider:                 oauthProvider,
		OAuthSessionStore:             oauthSessionStore,
		OAuthConfig:                   cfg.OAuth.Codex,
		OAuthSessionTTL:               cfg.OAuth.Codex.SessionTTL,
		OAuthCompletionService:        oauthCompletionService,
		CompletionService:             completionService, // May be nil if routing disabled
		CopilotOAuthProvider:          copilotProvider,
		CopilotOAuthSessionTTL:        cfg.OAuth.Copilot.SessionTTL,
		CopilotOAuthCompletionService: copilotOAuthCompletionService,
		UsageCache:                    quotaStateCache,  // For usage endpoint
		Router:                        completionRouter, // For Anthropic Messages API
		ProviderRegistry:              providerRegistry, // For Anthropic Messages API
	})

	httpServer := server.NewHTTPServer(cfg.Server, router)
	refreshWorker := server.NewRefreshWorker(refreshService, loggerinfra.ForModuleLazy("platform.server.refresh_worker"), cfg.Credential.RefreshInterval)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go refreshWorker.Start(shutdownCtx)

	// Start quota worker if liveness checking is enabled
	if cfg.Liveness.Enabled {
		go quotaWorker.Start(shutdownCtx)
		log.Info("quota worker started",
			zap.Duration("sample_interval", cfg.Liveness.SampleInterval),
			zap.Duration("full_sweep_interval", cfg.Liveness.FullSweepInterval),
		)
	}

	go func() {
		log.Info("starting API server",
			zap.String("host", cfg.Server.Host),
			zap.Int("port", cfg.Server.Port),
			zap.String("lb_strategy", cfg.Orchestrator.LBStrategy),
			zap.Bool("gin_development", cfg.Log.Development),
		)

		if serveErr := httpServer.Start(); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Fatal("server stopped unexpectedly", zap.Error(serveErr))
		}
	}()

	<-shutdownCtx.Done()
	log.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if shutdownErr := httpServer.Shutdown(ctx); shutdownErr != nil {
		log.Error("graceful shutdown failed", zap.Error(shutdownErr))
		os.Exit(1)
	}

	log.Info("server shutdown complete")
}

// initCompletionService initializes the completion routing service.
// Returns the completion service, router, and provider registry.
func initCompletionService(
	cfg *configinfra.Config,
	credRepo providerinfra.CredentialRepository,
	decryptor providerinfra.CredentialDecryptor,
	log *loggerinfra.Logger,
) (usecasecompletion.CompletionService, usecasecompletion.Router, usecasecompletion.ProviderRegistry) {
	// Create pooled token fetcher for credential selection
	tokenFetcherLogger := loggerinfra.ForModule("infra.provider.token_fetcher")
	tokenFetcherConfig := providerinfra.PooledTokenFetcherConfig{
		OnSelection: func(selection providerinfra.CredentialSelection) {
			// Additional logging callback if needed
			log.Debug("credential selection callback",
				zap.String("profile_id", selection.ProfileID),
				zap.String("account_id", selection.AccountID),
				zap.String("profile_type", selection.ProfileType),
			)
		},
	}
	tokenFetcher := providerinfra.NewPooledTokenFetcher(credRepo, decryptor, tokenFetcherLogger, tokenFetcherConfig)

	// Create credential provider using pooled token fetcher
	credProviderLogger := loggerinfra.ForModule("infra.provider.credential")
	credProvider := providerinfra.NewCredentialProvider(tokenFetcher, credProviderLogger)

	// Convert provider configs for static registry (fallback configuration)
	providerConfigs := make(map[string]providerinfra.ProviderConfig)
	for id, pc := range cfg.Providers {
		baseURL := pc.BaseURL
		if id == "copilot" {
			baseURL = providerinfra.ResolveCopilotBaseURL(
				cfg.OAuth.Copilot.EnterpriseURL,
				cfg.OAuth.Copilot.AccountType,
			)
		}

		providerConfigs[id] = providerinfra.ProviderConfig{
			ID:       id,
			Name:     pc.Name,
			Enabled:  pc.Enabled,
			BaseURL:  baseURL,
			Models:   pc.Models,
			Headers:  pc.Headers,
			AuthType: pc.AuthType,
			Timeout:  pc.Timeout,
		}
	}

	// Create dynamic registry that loads models based on available credentials
	dynamicRegistryConfig := providerinfra.DynamicRegistryConfig{
		ProviderPriority: cfg.Routing.ProviderPriority,
		ProviderConfigs:  providerConfigs,
	}
	registryLogger := loggerinfra.ForModule("infra.provider.registry")
	registry := providerinfra.NewDynamicRegistry(dynamicRegistryConfig, tokenFetcher, registryLogger)

	// Health tracker
	healthTrackerConfig := providerinfra.HealthTrackerConfig{
		FailureThreshold:         cfg.Routing.Health.FailureThreshold,
		CooldownDuration:         cfg.Routing.Health.CooldownDuration,
		RateLimitDefaultCooldown: cfg.Routing.Health.RateLimitDefaultCooldown,
	}
	healthTracker := providerinfra.NewHealthTracker(healthTrackerConfig)

	// Router
	routerLogger := loggerinfra.ForModule("usecase.completion.router")
	router := usecasecompletion.NewRouter(registry, healthTracker, credProvider, nil, routerLogger)

	// Provider client
	clientConfig := providerinfra.ClientConfig{
		Timeout: cfg.Routing.RequestTimeout,
	}
	// Check if copilot provider has responses routing enabled
	if copilotCfg, ok := cfg.Providers["copilot"]; ok {
		clientConfig.EnableCopilotResponsesRouting = copilotCfg.EnableResponsesRouting
	}
	clientLogger := loggerinfra.ForModule("infra.provider.client")
	client := providerinfra.NewClient(clientConfig, clientLogger)

	// Service
	serviceConfig := usecasecompletion.ServiceConfig{
		MaxFallbackAttempts: cfg.Routing.Fallback.MaxAttempts,
		RequestTimeout:      cfg.Routing.RequestTimeout,
	}
	serviceLogger := loggerinfra.ForModule("usecase.completion.service")
	completionSvc := usecasecompletion.NewService(router, registry, healthTracker, client, serviceConfig, serviceLogger)

	log.Info("completion service initialized with credential pool",
		zap.Strings("provider_priority", cfg.Routing.ProviderPriority),
	)

	return completionSvc, router, registry
}
