package handler

import (
	"net/http"

	"github.com/duchoang/llmpool/internal/delivery/http/middleware"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var refreshLog = loggerinfra.ForModuleLazy("delivery.http.handler.refresh")

type RefreshHandler struct {
	refreshService usecasecredential.RefreshService
}

func NewRefreshHandler(refreshService usecasecredential.RefreshService) *RefreshHandler {
	return &RefreshHandler{refreshService: refreshService}
}

func (h *RefreshHandler) Refresh(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	if err := h.refreshService.RefreshDue(c.Request.Context()); err != nil {
		refreshLog.Warn("manual refresh failed",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	refreshLog.Info("manual refresh triggered", zap.String("request_id", requestID))

	c.JSON(http.StatusOK, gin.H{"message": "refresh triggered"})
}
