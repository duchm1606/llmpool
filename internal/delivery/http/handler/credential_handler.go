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
	listService   usecasecredential.ListService
	statusService usecasecredential.StatusService
}

func NewCredentialHandler(
	importService usecasecredential.ImportService,
	listService usecasecredential.ListService,
	statusService usecasecredential.StatusService,
) *CredentialHandler {
	return &CredentialHandler{
		importService: importService,
		listService:   listService,
		statusService: statusService,
	}
}

func (h *CredentialHandler) List(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	if h.listService == nil {
		credentialLog.Error("credential list service not configured",
			zap.String("request_id", requestID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "credential list service unavailable"})
		return
	}

	profiles, err := h.listService.List(c.Request.Context())
	if err != nil {
		credentialLog.Error("credential list failed",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to list credential profiles"})
		return
	}

	data := make([]gin.H, 0, len(profiles))
	for _, profile := range profiles {
		data = append(data, gin.H{
			"id":              profile.ID,
			"type":            profile.Type,
			"account_id":      profile.AccountID,
			"email":           profile.Email,
			"enabled":         profile.Enabled,
			"expired":         profile.Expired,
			"last_refresh_at": profile.LastRefreshAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  data,
		"count": len(data),
	})
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

func (h *CredentialHandler) SetStatus(c *gin.Context) {
	requestID := middleware.GetRequestID(c)

	if h.statusService == nil {
		credentialLog.Error("credential status service not configured",
			zap.String("request_id", requestID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "credential status service unavailable"})
		return
	}

	credentialID := c.Param("id")
	if credentialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "credential id is required"})
		return
	}

	var req struct {
		Enabled *bool `json:"enabled" binding:"required"`
	}
	if !bindJSONAndValidate(c, &req) {
		credentialLog.Warn("credential status validation failed",
			zap.String("request_id", requestID),
			zap.String("credential_id", credentialID),
		)
		return
	}

	updated, err := h.statusService.SetEnabled(c.Request.Context(), credentialID, *req.Enabled)
	if err != nil {
		if err == usecasecredential.ErrCredentialNotFound {
			c.JSON(http.StatusNotFound, gin.H{"message": "credential not found"})
			return
		}

		credentialLog.Error("credential status update failed",
			zap.String("request_id", requestID),
			zap.String("credential_id", credentialID),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":              updated.ID,
		"enabled":         updated.Enabled,
		"last_refresh_at": updated.LastRefreshAt,
		"expired":         updated.Expired,
	})
}
