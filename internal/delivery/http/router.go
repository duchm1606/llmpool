package http

import (
	"github.com/duchoang/llmpool/internal/delivery/http/handler"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func NewRouter(logger *zap.Logger, healthService usecasehealth.Service, importService usecasecredential.ImportService, refreshService usecasecredential.RefreshService) *gin.Engine {
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

	return r
}
