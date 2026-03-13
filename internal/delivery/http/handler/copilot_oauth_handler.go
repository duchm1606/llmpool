package handler

import (
	"net/http"
	"strings"

	"github.com/duchoang/llmpool/internal/delivery/http/middleware"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var copilotOAuthLog = loggerinfra.ForModuleLazy("delivery.http.handler.copilot_oauth")

// CopilotOAuthHandler handles Copilot OAuth flow endpoints.
type CopilotOAuthHandler struct {
	deviceFlow usecaseoauth.DeviceFlowCoordinator
}

// NewCopilotOAuthHandler creates a new Copilot OAuth handler.
func NewCopilotOAuthHandler(
	deviceFlow usecaseoauth.DeviceFlowCoordinator,
) *CopilotOAuthHandler {
	return &CopilotOAuthHandler{
		deviceFlow: deviceFlow,
	}
}

// StartDeviceFlow handles device authorization flow initiation: POST /v1/internal/oauth/copilot-device-code
func (h *CopilotOAuthHandler) StartDeviceFlow(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := middleware.GetRequestID(c)

	deviceResp, err := h.deviceFlow.StartDeviceFlow(ctx)
	if err != nil {
		copilotOAuthLog.Error("copilot device flow start failed",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to start device flow",
		})
		return
	}

	copilotOAuthLog.Info("copilot device flow started",
		zap.String("request_id", requestID),
		zap.String("user_code", deviceResp.UserCode),
		zap.String("verification_uri", deviceResp.VerificationURI),
	)

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status":           "ok",
		"device_code":      deviceResp.DeviceCode,
		"user_code":        deviceResp.UserCode,
		"verification_uri": deviceResp.VerificationURI,
		"expires_in":       deviceResp.ExpiresIn,
		"interval":         deviceResp.Interval,
	})
}

// GetDeviceStatus handles device flow status polling: GET /v1/internal/oauth/copilot-device-status
func (h *CopilotOAuthHandler) GetDeviceStatus(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := middleware.GetRequestID(c)
	deviceCode := strings.TrimSpace(c.Query("device_code"))

	if deviceCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "device_code parameter required",
		})
		return
	}

	status, err := h.deviceFlow.GetDeviceStatus(ctx, deviceCode)
	if err != nil {
		copilotOAuthLog.Error("copilot device status lookup failed",
			zap.String("request_id", requestID),
			zap.String("device_code", deviceCode),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to get device flow status",
		})
		return
	}

	if status == nil {
		c.JSON(http.StatusOK, gin.H{"status": "wait"})
		return
	}

	switch status.State {
	case "pending":
		c.JSON(http.StatusOK, gin.H{"status": "wait"})
	case "ok":
		response := gin.H{"status": "ok", "account_id": status.AccountID}
		if status.Connection != nil {
			response["connection"] = gin.H{
				"id":              status.Connection.ID,
				"account_id":      status.Connection.AccountID,
				"email":           status.Connection.Email,
				"provider":        status.Connection.Provider,
				"expires_at":      status.Connection.ExpiresAt,
				"last_refresh_at": status.Connection.LastRefreshAt,
				"enabled":         status.Connection.Enabled,
			}
		}
		c.JSON(http.StatusOK, response)
	case "error":
		response := gin.H{
			"status":     "error",
			"error_code": status.ErrorCode,
		}
		if status.ErrorMessage != "" {
			response["error_message"] = status.ErrorMessage
		}
		c.JSON(http.StatusOK, response)
	default:
		copilotOAuthLog.Error("copilot device flow returned unknown state",
			zap.String("request_id", requestID),
			zap.String("device_code", deviceCode),
			zap.String("state", string(status.State)),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "unknown device flow status",
		})
	}
}
