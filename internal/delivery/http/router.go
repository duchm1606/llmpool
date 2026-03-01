package http

import (
	"github.com/duchoang/llmpool/internal/delivery/http/handler"
	usecasehealth "github.com/duchoang/llmpool/internal/usecase/health"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func NewRouter(logger *zap.Logger, healthService usecasehealth.Service) *gin.Engine {
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

	return r
}
