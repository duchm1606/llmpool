package handler

import (
	"net/http"
	"strings"

	"github.com/duchoang/llmpool/internal/delivery/http/middleware"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	usecasequota "github.com/duchoang/llmpool/internal/usecase/quota"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var refreshLog = loggerinfra.ForModuleLazy("delivery.http.handler.refresh")

type RefreshHandler struct {
	refreshService usecasecredential.RefreshService
	quotaService   usecasequota.LivenessService
}

func NewRefreshHandler(refreshService usecasecredential.RefreshService, quotaService usecasequota.LivenessService) *RefreshHandler {
	return &RefreshHandler{refreshService: refreshService, quotaService: quotaService}
}

func (h *RefreshHandler) Refresh(c *gin.Context) {
	requestID := middleware.GetRequestID(c)
	credentialID := strings.TrimSpace(c.Query("credential_id"))

	if credentialID != "" {
		if err := h.refreshService.RefreshCredential(c.Request.Context(), credentialID); err != nil {
			refreshLog.Warn("manual credential refresh failed",
				zap.String("request_id", requestID),
				zap.String("credential_id", credentialID),
				zap.Error(err),
			)
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}

		response := gin.H{
			"message":       "refresh triggered",
			"credential_id": credentialID,
		}

		if h.quotaService != nil {
			if err := h.quotaService.CheckCredential(c.Request.Context(), credentialID); err != nil {
				refreshLog.Warn("credential quota refresh failed",
					zap.String("request_id", requestID),
					zap.String("credential_id", credentialID),
					zap.Error(err),
				)
			}

			state, err := h.quotaService.GetCredentialState(c.Request.Context(), credentialID)
			if err != nil {
				refreshLog.Warn("read credential quota state failed",
					zap.String("request_id", requestID),
					zap.String("credential_id", credentialID),
					zap.Error(err),
				)
			} else if state != nil {
				response["status"] = state.Status
				response["quota"] = state.Quota
				response["quota_detail"] = state.QuotaDetail
				response["access_token_hash"] = state.AccessTokenHash
			}
		}
		refreshLog.Info("manual credential refresh triggered",
			zap.String("request_id", requestID),
			zap.String("credential_id", credentialID),
		)

		c.JSON(http.StatusOK, response)
		return
	}

	if err := h.refreshService.RefreshDue(c.Request.Context()); err != nil {
		refreshLog.Warn("manual refresh failed",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if h.quotaService != nil {
		if err := h.quotaService.CheckAll(c.Request.Context()); err != nil {
			refreshLog.Warn("quota refresh after credential refresh failed",
				zap.String("request_id", requestID),
				zap.Error(err),
			)
		}
	}

	refreshLog.Info("manual refresh triggered", zap.String("request_id", requestID))

	c.JSON(http.StatusOK, gin.H{"message": "refresh triggered"})
}
