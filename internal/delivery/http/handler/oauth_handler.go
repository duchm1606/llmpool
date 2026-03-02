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
