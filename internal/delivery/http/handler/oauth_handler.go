package handler

import (
	"net/http"
	"time"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/duchoang/llmpool/internal/infra/config"
	"github.com/duchoang/llmpool/internal/infra/oauth"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
	"github.com/gin-gonic/gin"
)

// OAuthHandler handles OAuth flow endpoints
type OAuthHandler struct {
	provider     usecaseoauth.OAuthProvider
	sessionStore usecaseoauth.OAuthSessionStore
	config       config.CodexOAuthConfig
	sessionTTL   time.Duration
}

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(
	provider usecaseoauth.OAuthProvider,
	sessionStore usecaseoauth.OAuthSessionStore,
	cfg config.CodexOAuthConfig,
	sessionTTL time.Duration,
) *OAuthHandler {
	return &OAuthHandler{
		provider:     provider,
		sessionStore: sessionStore,
		config:       cfg,
		sessionTTL:   sessionTTL,
	}
}

// GetAuthURL handles native auth URL endpoint: GET /v1/internal/oauth/codex-auth-url
func (h *OAuthHandler) GetAuthURL(c *gin.Context) {
	ctx := c.Request.Context()

	// Generate state and verifier
	state := oauth.GenerateState()

	verifier, err := oauth.GenerateVerifier()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to generate verifier",
		})
		return
	}

	// Build authorization URL
	authURL, err := h.provider.BuildAuthURL(ctx, state, verifier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to build authorization URL",
		})
		return
	}

	// Create pending session in Redis
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StatePending,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(h.sessionTTL),
		CreatedAt:    time.Now(),
	}

	if err := h.sessionStore.CreatePending(ctx, session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to create session",
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"url":    authURL.URL,
		"state":  state,
	})
}

// GetAuthURLCompatibility handles compatibility alias: GET /v0/management/codex-auth-url
// This endpoint maintains backward compatibility with Proxypal and CLIProxyAPI clients
func (h *OAuthHandler) GetAuthURLCompatibility(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse optional is_webui query parameter (for Proxypal compatibility)
	// Currently doesn't change behavior, but preserves the parameter for future use
	_ = c.Query("is_webui")

	// Generate state and verifier
	state := oauth.GenerateState()

	verifier, err := oauth.GenerateVerifier()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to generate verifier",
		})
		return
	}

	// Build authorization URL
	authURL, err := h.provider.BuildAuthURL(ctx, state, verifier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to build authorization URL",
		})
		return
	}

	// Create pending session in Redis
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StatePending,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(h.sessionTTL),
		CreatedAt:    time.Now(),
	}

	if err := h.sessionStore.CreatePending(ctx, session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to create session",
		})
		return
	}

	// Return success response with same shape as native endpoint
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"url":    authURL.URL,
		"state":  state,
	})
}



// HandleCallback handles OAuth callback: GET /v1/internal/oauth/callback
func (h *OAuthHandler) HandleCallback(c *gin.Context) {
    ctx := c.Request.Context()
    
    // Get query params
    code := c.Query("code")
    state := c.Query("state")
    errorParam := c.Query("error")
    errorDescription := c.Query("error_description")
    
    // If error param present from OAuth provider
    if errorParam != "" {
        _ = h.sessionStore.MarkError(ctx, state, errorParam, errorDescription)
        c.JSON(http.StatusBadRequest, gin.H{
            "status": "error",
            "error":  errorParam,
        })
        return
    }
    
    // Validate state exists
    session, err := h.sessionStore.GetStatus(ctx, state)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "status": "error",
            "error":  "invalid or expired state",
        })
        return
    }
    
    // Exchange code for tokens
    tokenPayload, err := h.provider.ExchangeCode(ctx, code, session.PKCEVerifier)
    if err != nil {
        _ = h.sessionStore.MarkError(ctx, state, "exchange_failed", err.Error())
        c.JSON(http.StatusInternalServerError, gin.H{
            "status": "error",
            "error":  "failed to exchange code",
        })
        return
    }
    
    // Mark as complete (use access token as account ID for now)
    if err := h.sessionStore.MarkComplete(ctx, state, tokenPayload.AccessToken); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "status": "error",
            "error":  "failed to complete session",
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetStatus handles status polling: GET /v1/internal/oauth/status
func (h *OAuthHandler) GetStatus(c *gin.Context) {
    ctx := c.Request.Context()
    state := c.Query("state")
    
    if state == "" {
        c.JSON(http.StatusBadRequest, gin.H{
            "status": "error",
            "error":  "state parameter required",
        })
        return
    }
    
    session, err := h.sessionStore.GetStatus(ctx, state)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "status": "error",
            "error":  "session not found",
        })
        return
    }
    
    switch session.State {
    case domainoauth.StatePending:
        c.JSON(http.StatusOK, gin.H{"status": "wait"})
    case domainoauth.StateOK:
        c.JSON(http.StatusOK, gin.H{
            "status":     "ok",
            "account_id": session.AccountID,
        })
    case domainoauth.StateError:
        c.JSON(http.StatusOK, gin.H{
            "status":        "error",
            "error_code":    session.ErrorCode,
            "error_message": session.ErrorMessage,
        })
    default:
        c.JSON(http.StatusOK, gin.H{"status": "wait"})
    }
}

// StartDeviceFlow handles device authorization flow initiation: POST /v1/internal/oauth/codex-device-code
func (h *OAuthHandler) StartDeviceFlow(c *gin.Context) {
	ctx := c.Request.Context()

	// Start device flow with provider
	deviceResp, err := h.provider.StartDeviceFlow(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to start device flow",
		})
		return
	}

	// Create pending session in Redis
	session := domainoauth.OAuthSession{
		SessionID:      deviceResp.DeviceCode,
		State:          domainoauth.StatePending,
		Provider:       "codex",
		Expiry:         time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second),
		CreatedAt:      time.Now(),
		DeviceCode:     deviceResp.DeviceCode,
		UserCode:       deviceResp.UserCode,
		VerificationURI: deviceResp.VerificationURI,
		Interval:       deviceResp.Interval,
	}

	if err := h.sessionStore.CreatePending(ctx, session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to create session",
		})
		return
	}

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

// GetDeviceStatus handles device flow status polling: GET /v1/internal/oauth/codex-device-status
func (h *OAuthHandler) GetDeviceStatus(c *gin.Context) {
	ctx := c.Request.Context()
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

		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to poll device",
		})
		return
	}

	// Mark session as complete
	if err := h.sessionStore.MarkComplete(ctx, deviceCode, tokenPayload.AccessToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to complete session",
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"account_id":  tokenPayload.AccessToken,
	})
}
