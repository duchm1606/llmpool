package http

import (
	"time"

	"github.com/duchoang/llmpool/internal/delivery/http/handler"
	"github.com/duchoang/llmpool/internal/delivery/http/middleware"
	configinfra "github.com/duchoang/llmpool/internal/infra/config"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
	usecasequota "github.com/duchoang/llmpool/internal/usecase/quota"
	"github.com/gin-gonic/gin"
)

// RouterDeps holds all dependencies for the router.
type RouterDeps struct {
	Development            bool
	HealthService          usecasehealth.Service
	ImportService          usecasecredential.ImportService
	RefreshService         usecasecredential.RefreshService
	QuotaService           usecasequota.LivenessService
	OAuthProvider          usecaseoauth.OAuthProvider
	OAuthSessionStore      usecaseoauth.OAuthSessionStore
	OAuthConfig            configinfra.CodexOAuthConfig
	OAuthSessionTTL        time.Duration
	OAuthCompletionService usecasecredential.OAuthCompletionService
	CompletionService      usecasecompletion.CompletionService // Optional: may be nil

	// Copilot OAuth dependencies (optional)
	CopilotOAuthProvider          usecaseoauth.OAuthProvider
	CopilotOAuthSessionTTL        time.Duration
	CopilotOAuthCompletionService usecasecredential.OAuthCompletionService

	// Usage cache for usage endpoint (optional)
	UsageCache handler.UsageCache

	// Dependencies for Anthropic Messages API (optional)
	// When set, enables the /v1/messages endpoint for Anthropic-compatible API
	Router           usecasecompletion.Router
	ProviderRegistry usecasecompletion.ProviderRegistry
}

func NewRouter(
	development bool,
	healthService usecasehealth.Service,
	importService usecasecredential.ImportService,
	refreshService usecasecredential.RefreshService,
	oauthProvider usecaseoauth.OAuthProvider,
	oauthSessionStore usecaseoauth.OAuthSessionStore,
	oauthConfig configinfra.CodexOAuthConfig,
	oauthSessionTTL time.Duration,
	oauthCompletionService usecasecredential.OAuthCompletionService,
	completionService usecasecompletion.CompletionService, // Optional: may be nil
) *gin.Engine {
	return NewRouterWithDeps(RouterDeps{
		Development:            development,
		HealthService:          healthService,
		ImportService:          importService,
		RefreshService:         refreshService,
		OAuthProvider:          oauthProvider,
		OAuthSessionStore:      oauthSessionStore,
		OAuthConfig:            oauthConfig,
		OAuthSessionTTL:        oauthSessionTTL,
		OAuthCompletionService: oauthCompletionService,
		CompletionService:      completionService,
	})
}

func NewRouterWithDeps(deps RouterDeps) *gin.Engine {
	if deps.Development {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	recoveryLogger := loggerinfra.ForModuleLazy("delivery.http.middleware.recovery")
	requestLogger := loggerinfra.ForModuleLazy("delivery.http.middleware.security")

	r.Use(middleware.RequestID())
	r.Use(middleware.SecurityLogger(requestLogger))
	r.Use(middleware.Recovery(recoveryLogger))

	healthHandler := handler.NewHealthHandler(deps.HealthService)
	r.GET("/health", healthHandler.Get)

	credentialHandler := handler.NewCredentialHandler(deps.ImportService)
	r.POST("/v1/internal/auth-profiles/import", credentialHandler.Import)

	refreshHandler := handler.NewRefreshHandler(deps.RefreshService, deps.QuotaService)
	r.POST("/v1/internal/auth-profiles/refresh", refreshHandler.Refresh)

	oauthHandler := handler.NewOAuthHandler(deps.OAuthProvider, deps.OAuthSessionStore, deps.OAuthConfig, deps.OAuthSessionTTL, deps.OAuthCompletionService)
	r.GET("/v1/internal/oauth/codex-auth-url", oauthHandler.GetAuthURL)
	r.GET("/v0/management/codex-auth-url", oauthHandler.GetAuthURLCompatibility)

	// OAuth callback and status endpoints
	r.GET("/v1/internal/oauth/callback", oauthHandler.HandleCallback)
	r.GET("/v1/internal/oauth/status", oauthHandler.GetStatus)

	// Compatibility aliases for callback and status
	r.GET("/v0/management/oauth-callback", oauthHandler.HandleCallback)

	r.GET("/v0/management/get-auth-status", oauthHandler.GetStatus)

	// Device flow endpoints (Codex)
	r.POST("/v1/internal/oauth/codex-device-code", oauthHandler.StartDeviceFlow)
	r.GET("/v1/internal/oauth/codex-device-status", oauthHandler.GetDeviceStatus)

	// New device flow routes (RFC 8628)
	r.POST("/v1/internal/oauth/device/start", oauthHandler.StartDeviceFlow)
	r.GET("/v1/internal/oauth/device/poll", oauthHandler.GetDeviceStatus)

	// Compatibility aliases for device flow
	r.POST("/v0/management/codex-device-code", oauthHandler.StartDeviceFlow)
	r.GET("/v0/management/codex-device-status", oauthHandler.GetDeviceStatus)

	// Copilot OAuth device flow endpoints
	if deps.CopilotOAuthProvider != nil {
		copilotOAuthHandler := handler.NewCopilotOAuthHandler(
			deps.CopilotOAuthProvider,
			deps.OAuthSessionStore, // Reuse session store
			deps.CopilotOAuthSessionTTL,
			deps.CopilotOAuthCompletionService,
		)
		r.POST("/v1/internal/oauth/copilot-device-code", copilotOAuthHandler.StartDeviceFlow)
		r.GET("/v1/internal/oauth/copilot-device-status", copilotOAuthHandler.GetDeviceStatus)
	}

	// Usage endpoint for Copilot quota information
	if deps.UsageCache != nil {
		usageLogger := loggerinfra.ForModule("delivery.http.handler.usage")
		usageHandler := handler.NewUsageHandler(deps.UsageCache, usageLogger)
		r.GET("/v1/internal/usage", usageHandler.ListUsages)
	}

	// OpenAI-compatible completion API routes
	if deps.CompletionService != nil {
		completionLogger := loggerinfra.ForModule("delivery.http.handler.completion")
		chatHandler := handler.NewChatHandler(deps.CompletionService, completionLogger)
		modelsHandler := handler.NewModelsHandler(deps.CompletionService, completionLogger)
		responsesHandler := handler.NewResponsesHandler(deps.CompletionService, completionLogger)

		// Chat completions (primary endpoint)
		r.POST("/v1/chat/completions", chatHandler.ChatCompletion)

		// Legacy text completions (for compatibility)
		r.POST("/v1/completions", chatHandler.Completion)

		// OpenAI Responses API (used by Cursor IDE)
		r.POST("/v1/responses", responsesHandler.CreateResponse)

		// Models endpoints
		r.GET("/v1/models", modelsHandler.ListModels)
		r.GET("/v1/models/:model", modelsHandler.GetModel)
	}

	// Anthropic Messages API endpoint (proxies to Copilot)
	// This endpoint accepts Anthropic Claude format and proxies to Copilot /responses
	if deps.Router != nil && deps.ProviderRegistry != nil {
		messagesLogger := loggerinfra.ForModule("delivery.http.handler.messages")
		messagesHandler := handler.NewMessagesHandler(deps.Router, deps.ProviderRegistry, messagesLogger)

		// Anthropic Messages API
		r.POST("/v1/messages", messagesHandler.CreateMessage)
	}

	return r
}
