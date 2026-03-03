package handler

import (
	"net/http"

	"github.com/duchoang/llmpool/internal/delivery/http/middleware"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var credentialLog = loggerinfra.ForModuleLazy("delivery.http.handler.credential")

type CredentialHandler struct {
	importService usecasecredential.ImportService
}

func NewCredentialHandler(importService usecasecredential.ImportService) *CredentialHandler {
	return &CredentialHandler{importService: importService}
}

func (h *CredentialHandler) Import(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	var req usecasecredential.CredentialProfile
	if !bindJSONAndValidate(c, &req) {
		credentialLog.Warn("credential import validation failed",
			zap.String("request_id", requestID),
		)
		return
	}

	profile, err := h.importService.Import(c.Request.Context(), req)
	if err != nil {
		credentialLog.Warn("credential import failed",
			zap.String("request_id", requestID),
			zap.String("type", req.Type),
			zap.String("email", req.Email),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	credentialLog.Info("credential imported",
		zap.String("request_id", requestID),
		zap.String("profile_id", profile.ID),
		zap.String("type", profile.Type),
	)

	c.JSON(http.StatusCreated, gin.H{
		"id":                    profile.ID,
		"type":                  profile.Type,
		"email":                 profile.Email,
		"accountId":             profile.AccountID,
		"enabled":               profile.Enabled,
		"expired":               profile.Expired,
		"last_refresh":          profile.LastRefreshAt,
		"has_encrypted_profile": profile.EncryptedProfile != "",
	})
}
