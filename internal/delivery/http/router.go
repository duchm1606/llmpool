package http

import (
	"time"

	"github.com/duchoang/llmpool/internal/delivery/http/handler"
	configinfra "github.com/duchoang/llmpool/internal/infra/config"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func NewRouter(
	logger *zap.Logger,
	healthService usecasehealth.Service,
	importService usecasecredential.ImportService,
	refreshService usecasecredential.RefreshService,
	oauthProvider usecaseoauth.OAuthProvider,
	oauthSessionStore usecaseoauth.OAuthSessionStore,
	oauthConfig configinfra.CodexOAuthConfig,
	oauthSessionTTL time.Duration,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	r.Use(func(c *gin.Context) {
		c.Next()
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	})

	healthHandler := handler.NewHealthHandler(healthService)
	r.GET("/health", healthHandler.Get)

	credentialHandler := handler.NewCredentialHandler(importService)
	r.POST("/v1/internal/auth-profiles/import", credentialHandler.Import)

	refreshHandler := handler.NewRefreshHandler(refreshService)
	r.POST("/v1/internal/auth-profiles/refresh", refreshHandler.Refresh)

	oauthHandler := handler.NewOAuthHandler(oauthProvider, oauthSessionStore, oauthConfig, oauthSessionTTL)
	r.GET("/v1/internal/oauth/codex-auth-url", oauthHandler.GetAuthURL)
	r.GET("/v0/management/codex-auth-url", oauthHandler.GetAuthURLCompatibility)

	// OAuth callback and status endpoints
	r.GET("/v1/internal/oauth/callback", oauthHandler.HandleCallback)
	r.GET("/v1/internal/oauth/status", oauthHandler.GetStatus)

	// Compatibility aliases for callback and status
	r.GET("/v0/management/oauth-callback", oauthHandler.HandleCallback)
	r.GET("/v0/management/get-auth-status", oauthHandler.GetStatus)

	return r
}
