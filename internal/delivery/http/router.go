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
	"github.com/gin-gonic/gin"
)

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
	if development {
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

	healthHandler := handler.NewHealthHandler(healthService)
	r.GET("/health", healthHandler.Get)

	credentialHandler := handler.NewCredentialHandler(importService)
	r.POST("/v1/internal/auth-profiles/import", credentialHandler.Import)

	refreshHandler := handler.NewRefreshHandler(refreshService)
	r.POST("/v1/internal/auth-profiles/refresh", refreshHandler.Refresh)

	oauthHandler := handler.NewOAuthHandler(oauthProvider, oauthSessionStore, oauthConfig, oauthSessionTTL, oauthCompletionService)
	r.GET("/v1/internal/oauth/codex-auth-url", oauthHandler.GetAuthURL)
	r.GET("/v0/management/codex-auth-url", oauthHandler.GetAuthURLCompatibility)

	// OAuth callback and status endpoints
	r.GET("/v1/internal/oauth/callback", oauthHandler.HandleCallback)
	r.GET("/v1/internal/oauth/status", oauthHandler.GetStatus)

	// Compatibility aliases for callback and status
	r.GET("/v0/management/oauth-callback", oauthHandler.HandleCallback)

	r.GET("/v0/management/get-auth-status", oauthHandler.GetStatus)

	// Device flow endpoints
	r.POST("/v1/internal/oauth/codex-device-code", oauthHandler.StartDeviceFlow)
	r.GET("/v1/internal/oauth/codex-device-status", oauthHandler.GetDeviceStatus)

	// New device flow routes (RFC 8628)
	r.POST("/v1/internal/oauth/device/start", oauthHandler.StartDeviceFlow)
	r.GET("/v1/internal/oauth/device/poll", oauthHandler.GetDeviceStatus)

	// Compatibility aliases for device flow
	r.POST("/v0/management/codex-device-code", oauthHandler.StartDeviceFlow)
	r.GET("/v0/management/codex-device-status", oauthHandler.GetDeviceStatus)

	// OpenAI-compatible completion API routes
	if completionService != nil {
		completionLogger := loggerinfra.ForModule("delivery.http.handler.completion")
		chatHandler := handler.NewChatHandler(completionService, completionLogger)
		modelsHandler := handler.NewModelsHandler(completionService, completionLogger)
		responsesHandler := handler.NewResponsesHandler(completionService, completionLogger)

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

	return r
}
