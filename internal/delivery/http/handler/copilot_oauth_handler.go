package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/duchoang/llmpool/internal/delivery/http/middleware"
	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var copilotOAuthLog = loggerinfra.ForModuleLazy("delivery.http.handler.copilot_oauth")

// CopilotOAuthHandler handles Copilot OAuth flow endpoints.
type CopilotOAuthHandler struct {
	provider          usecaseoauth.OAuthProvider
	completionService usecasecredential.OAuthCompletionService
	sessionStore      usecaseoauth.OAuthSessionStore
	sessionTTL        time.Duration
}

// NewCopilotOAuthHandler creates a new Copilot OAuth handler.
func NewCopilotOAuthHandler(
	provider usecaseoauth.OAuthProvider,
	sessionStore usecaseoauth.OAuthSessionStore,
	sessionTTL time.Duration,
	completionService usecasecredential.OAuthCompletionService,
) *CopilotOAuthHandler {
	if completionService == nil {
		completionService = noopCopilotOAuthCompletionService{}
	}

	return &CopilotOAuthHandler{
		provider:          provider,
		completionService: completionService,
		sessionStore:      sessionStore,
		sessionTTL:        sessionTTL,
	}
}

type noopCopilotOAuthCompletionService struct{}

func (noopCopilotOAuthCompletionService) CompleteOAuth(_ context.Context, accountID string, _ domainoauth.TokenPayload) (domaincredential.Profile, error) {
	return domaincredential.Profile{AccountID: accountID}, nil
}

// StartDeviceFlow handles device authorization flow initiation: POST /v1/internal/oauth/copilot-device-code
func (h *CopilotOAuthHandler) StartDeviceFlow(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := middleware.GetRequestID(c)

	// Start device flow with provider
	deviceResp, err := h.provider.StartDeviceFlow(ctx)
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

	// Create pending session in Redis
	session := domainoauth.OAuthSession{
		SessionID:       deviceResp.DeviceCode,
		State:           domainoauth.StatePending,
		Provider:        "copilot",
		Expiry:          time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second),
		CreatedAt:       time.Now(),
		DeviceCode:      deviceResp.DeviceCode,
		UserCode:        deviceResp.UserCode,
		VerificationURI: deviceResp.VerificationURI,
		Interval:        deviceResp.Interval,
	}

	if err := h.sessionStore.CreatePending(ctx, session); err != nil {
		copilotOAuthLog.Error("copilot device flow session creation failed",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to create session",
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
	deviceCode := c.Query("device_code")

	if deviceCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "device_code parameter required",
		})
		return
	}

	// Try to get tokens from device flow
	tokenPayload, err := h.provider.PollDevice(ctx, deviceCode)
	if err != nil {
		// Check if it's a polling error (authorization_pending, slow_down, expired_token)
		errMsg := err.Error()
		if errMsg == "authorization pending" {
			c.JSON(http.StatusOK, gin.H{"status": "wait"})
			return
		}
		if errMsg == "slow down" {
			c.JSON(http.StatusOK, gin.H{"status": "wait", "slow_down": true})
			return
		}
		if errMsg == "expired token" {
			c.JSON(http.StatusOK, gin.H{
				"status":     "error",
				"error_code": "expired_token",
			})
			return
		}
		if strings.Contains(errMsg, "access denied") {
			c.JSON(http.StatusOK, gin.H{
				"status":     "error",
				"error_code": "access_denied",
			})
			return
		}
		if strings.Contains(errMsg, "forbidden") || strings.Contains(errMsg, "subscription") {
			copilotOAuthLog.Warn("copilot access forbidden - likely no subscription",
				zap.String("request_id", requestID),
				zap.Error(err),
			)
			c.JSON(http.StatusOK, gin.H{
				"status":        "error",
				"error_code":    "no_subscription",
				"error_message": "GitHub Copilot subscription required",
			})
			return
		}

		copilotOAuthLog.Error("copilot device poll failed",
			zap.String("request_id", requestID),
			zap.String("device_code", deviceCode),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to poll device",
		})
		return
	}

	// Validate account ID
	accountID := strings.TrimSpace(tokenPayload.AccountID)
	if accountID == "" {
		copilotOAuthLog.Error("copilot device poll missing account identity",
			zap.String("request_id", requestID),
			zap.String("device_code", deviceCode),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "missing account identifier",
		})
		return
	}

	// Complete OAuth flow - persist credentials
	newProfile, err := h.completionService.CompleteOAuth(ctx, accountID, tokenPayload)
	if err != nil {
		copilotOAuthLog.Error("copilot oauth completion failed",
			zap.String("request_id", requestID),
			zap.String("device_code", deviceCode),
			zap.String("account_id", accountID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to persist credentials",
		})
		return
	}

	// Mark session as complete
	if err := h.sessionStore.MarkComplete(ctx, deviceCode, newProfile.AccountID); err != nil {
		copilotOAuthLog.Error("copilot session mark complete failed",
			zap.String("request_id", requestID),
			zap.String("device_code", deviceCode),
			zap.String("account_id", newProfile.AccountID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to complete session",
		})
		return
	}

	copilotOAuthLog.Info("copilot device flow completed",
		zap.String("request_id", requestID),
		zap.String("device_code", deviceCode),
		zap.String("account_id", newProfile.AccountID),
	)

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"account_id": newProfile.AccountID,
	})
}
