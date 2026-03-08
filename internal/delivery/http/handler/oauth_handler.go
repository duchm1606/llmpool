package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/duchoang/llmpool/internal/delivery/http/middleware"
	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/duchoang/llmpool/internal/infra/config"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	"github.com/duchoang/llmpool/internal/infra/oauth"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var oauthLog = loggerinfra.ForModuleLazy("delivery.http.handler.oauth")

// OAuthHandler handles OAuth flow endpoints
type OAuthHandler struct {
	provider          usecaseoauth.OAuthProvider
	completionService usecasecredential.OAuthCompletionService
	sessionStore      usecaseoauth.OAuthSessionStore
	sessionTTL        time.Duration
	forwarder         oauthCallbackForwarder
}

const (
	codexCallbackForwardPort = 1455
	codexCallbackForwardPath = "/auth/callback"
	isWebUIQueryParam        = "is_webui"
)

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(
	provider usecaseoauth.OAuthProvider,
	sessionStore usecaseoauth.OAuthSessionStore,
	_ config.CodexOAuthConfig,
	sessionTTL time.Duration,
	completionServices ...usecasecredential.OAuthCompletionService,
) *OAuthHandler {
	completionService := usecasecredential.OAuthCompletionService(noopOAuthCompletionService{})
	if len(completionServices) > 0 && completionServices[0] != nil {
		completionService = completionServices[0]
	}

	return &OAuthHandler{
		provider:          provider,
		completionService: completionService,
		sessionStore:      sessionStore,
		sessionTTL:        sessionTTL,
		forwarder:         newLocalCallbackForwarder(sessionTTL),
	}
}

type noopOAuthCompletionService struct{}

func (noopOAuthCompletionService) CompleteOAuth(_ context.Context, accountID string, _ domainoauth.TokenPayload) (domaincredential.Profile, error) {
	return domaincredential.Profile{AccountID: accountID}, nil
}

// GetAuthURL handles native auth URL endpoint: GET /v1/internal/oauth/codex-auth-url
func (h *OAuthHandler) GetAuthURL(c *gin.Context) {
	h.startAuthFlow(c, false)
}

// GetAuthURLCompatibility handles compatibility alias: GET /v0/management/codex-auth-url
// This endpoint maintains backward compatibility with Proxypal and CLIProxyAPI clients
func (h *OAuthHandler) GetAuthURLCompatibility(c *gin.Context) {
	h.startAuthFlow(c, isWebUIRequest(c))
}

// HandleCallback handles OAuth callback: GET /v1/internal/oauth/callback
func (h *OAuthHandler) HandleCallback(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := middleware.GetRequestID(c)

	// Get query params
	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")
	errorDescription := c.Query("error_description")
	if state != "" {
		defer h.forwarder.StopByState(state)
	}

	// If error param present from OAuth provider
	if errorParam != "" {
		oauthLog.Warn("oauth callback returned provider error",
			zap.String("request_id", requestID),
			zap.String("state", state),
			zap.String("error", errorParam),
		)
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
		oauthLog.Warn("oauth callback session not found",
			zap.String("request_id", requestID),
			zap.String("state", state),
			zap.Error(err),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "invalid or expired state",
		})
		return
	}

	// Exchange code for tokens
	tokenPayload, err := h.provider.ExchangeCode(ctx, code, session.PKCEVerifier)

	if err != nil {
		oauthLog.Error("oauth code exchange failed",
			zap.String("request_id", requestID),
			zap.String("state", state),
			zap.Error(err),
		)
		_ = h.sessionStore.MarkError(ctx, state, "exchange_failed", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to exchange code",
		})
		return
	}

	accountID := strings.TrimSpace(tokenPayload.AccountID)
	if accountID == "" {
		oauthLog.Error("oauth callback missing account identity",
			zap.String("request_id", requestID),
			zap.String("state", state),
		)
		_ = h.sessionStore.MarkError(ctx, state, "missing_account_id", "missing account identifier in token payload")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "missing account identifier",
		})
		return
	}

	newProfile, err := h.completionService.CompleteOAuth(ctx, accountID, tokenPayload)
	if err != nil {
		oauthLog.Error("oauth completion failed",
			zap.String("request_id", requestID),
			zap.String("state", state),
			zap.String("account_id", accountID),
			zap.Error(err),
		)
		_ = h.sessionStore.MarkError(ctx, state, "completion_failed", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to persist credentials",
		})
		return
	}

	if err := h.sessionStore.MarkComplete(ctx, state, newProfile.AccountID); err != nil {
		oauthLog.Error("oauth session mark complete failed",
			zap.String("request_id", requestID),
			zap.String("state", state),
			zap.String("account_id", newProfile.AccountID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to complete session",
		})
		return
	}

	oauthLog.Info("oauth callback completed",
		zap.String("request_id", requestID),
		zap.String("state", state),
		zap.String("account_id", newProfile.AccountID),
	)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *OAuthHandler) startAuthFlow(c *gin.Context, useForwarder bool) {
	ctx := c.Request.Context()
	state := oauth.GenerateState()

	verifier, err := oauth.GenerateVerifier()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to generate verifier",
		})
		return
	}

	authURL, err := h.provider.BuildAuthURL(ctx, state, verifier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to build authorization URL",
		})
		return
	}

	if useForwarder {
		targetBase := internalCallbackURL(c)
		if targetBase == "" {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "failed to resolve callback target",
			})
			return
		}

		if err := h.forwarder.Start(state, codexCallbackForwardPort, codexCallbackForwardPath, targetBase); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "failed to start callback server",
			})
			return
		}
		authURL.URL = rewriteAuthRedirectURI(authURL.URL, forwarderCallbackURL(codexCallbackForwardPort, codexCallbackForwardPath))
	}

	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StatePending,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(h.sessionTTL),
		CreatedAt:    time.Now(),
	}

	if err := h.sessionStore.CreatePending(ctx, session); err != nil {
		if useForwarder {
			h.forwarder.StopByState(state)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to create session",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"url":    authURL.URL,
		"state":  state,
	})
}

func isWebUIRequest(c *gin.Context) bool {
	raw := strings.TrimSpace(c.Query(isWebUIQueryParam))
	if raw == "" {
		return false
	}

	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func internalCallbackURL(c *gin.Context) string {
	path := "/v1/internal/oauth/callback"

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	host := c.Request.Host
	if strings.TrimSpace(host) == "" {
		host = "localhost:8080"
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

func forwarderCallbackURL(port int, path string) string {
	return fmt.Sprintf("http://localhost:%d%s", port, path)
}

func rewriteAuthRedirectURI(authURL string, redirectURI string) string {
	if strings.TrimSpace(authURL) == "" || strings.TrimSpace(redirectURI) == "" {
		return authURL
	}

	before, query, hasQuery := strings.Cut(authURL, "?")
	if !hasQuery {
		return authURL
	}

	pairs := strings.Split(query, "&")
	updated := false
	for i, pair := range pairs {
		if strings.HasPrefix(pair, "redirect_uri=") {
			pairs[i] = "redirect_uri=" + url.QueryEscape(redirectURI)
			updated = true
			break
		}
	}

	if !updated {
		return authURL
	}

	return before + "?" + strings.Join(pairs, "&")
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
		SessionID:       deviceResp.DeviceCode,
		State:           domainoauth.StatePending,
		Provider:        "codex",
		Expiry:          time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second),
		CreatedAt:       time.Now(),
		DeviceCode:      deviceResp.DeviceCode,
		UserCode:        deviceResp.UserCode,
		VerificationURI: deviceResp.VerificationURI,
		Interval:        deviceResp.Interval,
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

		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "failed to poll device",
		})
		return
	}

	// Mark session as complete
	accountID := strings.TrimSpace(tokenPayload.AccountID)
	if accountID == "" {
		oauthLog.Error("codex device poll missing account identity",
			zap.String("request_id", requestID),
			zap.String("device_code", deviceCode),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "missing account identifier",
		})
		return
	}

	newProfile, err := h.completionService.CompleteOAuth(ctx, accountID, tokenPayload)
	if err != nil {
		oauthLog.Error("codex oauth completion failed",
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

	if err := h.sessionStore.MarkComplete(ctx, deviceCode, newProfile.AccountID); err != nil {
		oauthLog.Error("codex session mark complete failed",
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

	oauthLog.Info("codex device flow completed",
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
