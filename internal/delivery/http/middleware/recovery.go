package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type panicLogger interface {
	Error(msg string, fields ...zap.Field)
}

func Recovery(logger panicLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered",
					zap.String("request_id", GetRequestID(c)),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.Any("panic", rec),
					zap.ByteString("stack_trace", debug.Stack()),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  "internal server error",
				})
			}
		}()

		c.Next()
	}
}
