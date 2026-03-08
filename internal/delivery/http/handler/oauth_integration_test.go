package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/duchoang/llmpool/internal/infra/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOAuthProviderForIntegration provides a mock that simulates real OAuth provider behavior
type mockOAuthProviderForIntegration struct {
	authURL      domainoauth.AuthorizationURL
	tokenPayload domainoauth.TokenPayload
	deviceResp   domainoauth.DeviceFlowResponse
	err          error
	callCount    map[string]int
}

func newMockOAuthProviderForIntegration() *mockOAuthProviderForIntegration {
	return &mockOAuthProviderForIntegration{
		callCount: make(map[string]int),
	}
}

type mockOAuthCompletionForIntegration struct {
	err       error
	accountID string
	callCount int
}

func (m *mockOAuthCompletionForIntegration) CompleteOAuth(_ context.Context, accountID string, _ domainoauth.TokenPayload) (domaincredential.Profile, error) {
	m.callCount++
	if m.err != nil {
		return domaincredential.Profile{}, m.err
	}

	if m.accountID == "" {
		m.accountID = accountID
	}

	return domaincredential.Profile{AccountID: m.accountID}, nil
}

func (m *mockOAuthProviderForIntegration) BuildAuthURL(ctx context.Context, state string, verifier string) (domainoauth.AuthorizationURL, error) {
	m.callCount["BuildAuthURL"]++
	if m.err != nil {
		return domainoauth.AuthorizationURL{}, m.err
	}
	if m.authURL.URL == "" {
		m.authURL.URL = "https://auth.example.com/authorize?state=" + state
		m.authURL.State = state
	}
	return m.authURL, nil
}

func (m *mockOAuthProviderForIntegration) ExchangeCode(ctx context.Context, code string, verifier string) (domainoauth.TokenPayload, error) {
	m.callCount["ExchangeCode"]++
	if m.err != nil {
		return domainoauth.TokenPayload{}, m.err
	}
	if m.tokenPayload.AccessToken == "" {
		m.tokenPayload = domainoauth.TokenPayload{
			AccessToken:  "test-access-token-" + code,
			RefreshToken: "test-refresh-token-" + code,
			ExpiresAt:    time.Now().Add(time.Hour),
			AccountID:    "integration-account-from-provider",
			TokenType:    "Bearer",
		}
	}
	return m.tokenPayload, nil
}

func (m *mockOAuthProviderForIntegration) RefreshToken(ctx context.Context, refreshToken string) (domainoauth.TokenPayload, error) {
	m.callCount["RefreshToken"]++
	return domainoauth.TokenPayload{
		AccessToken:  "refreshed-access-token",
		RefreshToken: "refreshed-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
	}, nil
}

func (m *mockOAuthProviderForIntegration) StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error) {
	m.callCount["StartDeviceFlow"]++
	if m.err != nil {
		return domainoauth.DeviceFlowResponse{}, m.err
	}
	if m.deviceResp.DeviceCode == "" {
		m.deviceResp = domainoauth.DeviceFlowResponse{
			DeviceCode:      "test-device-code-123",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://auth.example.com/device",
			ExpiresIn:       600,
			Interval:        5,
		}
	}
	return m.deviceResp, nil
}

func (m *mockOAuthProviderForIntegration) PollDevice(ctx context.Context, deviceCode string) (domainoauth.TokenPayload, error) {
	m.callCount["PollDevice"]++
	if m.err != nil {
		return domainoauth.TokenPayload{}, m.err
	}
	return domainoauth.TokenPayload{
		AccessToken:  "device-access-token",
		RefreshToken: "device-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		AccountID:    "integration-device-account-from-provider",
		TokenType:    "Bearer",
	}, nil
}

// TestCodexWebFlowIntegration tests the complete web OAuth flow
func TestCodexWebFlowIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newMockOAuthProviderForIntegration()
	completion := &mockOAuthCompletionForIntegration{accountID: "integration-account-001"}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.example.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute, completion)

	router := gin.New()
	router.GET("/v1/internal/oauth/codex-auth-url", handler.GetAuthURL)
	router.GET("/v1/internal/oauth/callback", handler.HandleCallback)
	router.GET("/v1/internal/oauth/status", handler.GetStatus)

	ctx := context.Background()

	// Step 1: Initiate OAuth flow
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/codex-auth-url", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var authResp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &authResp)
	require.NoError(t, err)

	state := authResp["state"].(string)
	require.NotEmpty(t, state)

	// Verify session was created
	session, err := store.GetStatus(ctx, state)
	require.NoError(t, err)
	assert.Equal(t, domainoauth.StatePending, session.State)

	// Step 2: Simulate OAuth callback
	callbackURL := "/v1/internal/oauth/callback?code=test-auth-code&state=" + state
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var callbackResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &callbackResp)
	require.NoError(t, err)
	assert.Equal(t, "ok", callbackResp["status"])

	// Step 3: Check status
	statusURL := "/v1/internal/oauth/status?state=" + state
	req = httptest.NewRequest(http.MethodGet, statusURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var statusResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &statusResp)
	require.NoError(t, err)
	assert.Equal(t, "ok", statusResp["status"])
	assert.Equal(t, "integration-account-001", statusResp["account_id"])
	assert.Equal(t, 1, completion.callCount)
}

// TestCodexDeviceFlowIntegration tests the complete device flow
func TestCodexDeviceFlowIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newMockOAuthProviderForIntegration()
	completion := &mockOAuthCompletionForIntegration{accountID: "integration-device-account"}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:  "https://auth.example.com/authorize",
		ClientID: "test-client",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute, completion)
	handler.forwarder = &mockCallbackForwarder{}

	router := gin.New()
	router.POST("/v1/internal/oauth/device/start", handler.StartDeviceFlow)
	router.GET("/v1/internal/oauth/device/poll", handler.GetDeviceStatus)

	ctx := context.Background()

	// Step 1: Start device flow
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/oauth/device/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var deviceResp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &deviceResp)
	require.NoError(t, err)

	deviceCode := deviceResp["device_code"].(string)
	require.NotEmpty(t, deviceCode)
	assert.NotEmpty(t, deviceResp["user_code"])
	assert.NotEmpty(t, deviceResp["verification_uri"])

	// Verify session was created
	session, err := store.GetStatus(ctx, deviceCode)
	require.NoError(t, err)
	assert.Equal(t, domainoauth.StatePending, session.State)

	// Step 2: Poll for completion
	pollURL := "/v1/internal/oauth/device/poll?device_code=" + deviceCode
	req = httptest.NewRequest(http.MethodGet, pollURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var pollResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &pollResp)
	require.NoError(t, err)
	assert.Equal(t, "ok", pollResp["status"])
	assert.Equal(t, "integration-device-account", pollResp["account_id"])
	assert.Equal(t, 1, completion.callCount)
}

// TestCompatibilityAliasContracts tests v0/management compatibility routes
func TestCompatibilityAliasContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newMockOAuthProviderForIntegration()
	completion := &mockOAuthCompletionForIntegration{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.example.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute, completion)
	handler.forwarder = &mockCallbackForwarder{}

	router := gin.New()
	// Native routes
	router.GET("/v1/internal/oauth/codex-auth-url", handler.GetAuthURL)
	router.GET("/v1/internal/oauth/status", handler.GetStatus)
	// Compatibility aliases
	router.GET("/v0/management/codex-auth-url", handler.GetAuthURLCompatibility)
	router.GET("/v0/management/get-auth-status", handler.GetStatus)

	// Test that compatibility alias returns same shape as native
	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "native auth url",
			url:            "/v1/internal/oauth/codex-auth-url",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "compatibility auth url",
			url:            "/v0/management/codex-auth-url?is_webui=true",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			// Both should return status, url, state
			assert.Equal(t, "ok", resp["status"])
			assert.NotEmpty(t, resp["url"])
			assert.NotEmpty(t, resp["state"])
		})
	}
}

// TestOAuthFlowWithReplayProtection tests that replay attacks are prevented
func TestOAuthFlowWithReplayProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newMockOAuthProviderForIntegration()
	completion := &mockOAuthCompletionForIntegration{}
	store := newMockSessionStore() // Use mock store to avoid nil panic
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.example.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute, completion)

	router := gin.New()
	router.GET("/v1/internal/oauth/callback", handler.HandleCallback)

	// Try callback with non-existent state
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/callback?code=xyz&state=nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should reject unknown state
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestPKCENotExposedInResponse verifies PKCE verifier is never in API responses
func TestPKCENotExposedInResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newMockOAuthProviderForIntegration()
	completion := &mockOAuthCompletionForIntegration{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.example.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute, completion)

	router := gin.New()
	router.GET("/v1/internal/oauth/codex-auth-url", handler.GetAuthURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/codex-auth-url", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	respBody := w.Body.String()

	// Verify no PKCE-related fields in response
	assert.NotContains(t, respBody, "verifier")
	assert.NotContains(t, respBody, "code_verifier")
	assert.NotContains(t, respBody, "code_challenge")
}

func TestCodexWebFlowIntegration_CompletionFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := newMockOAuthProviderForIntegration()
	completion := &mockOAuthCompletionForIntegration{err: errors.New("persist failed")}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.example.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute, completion)

	router := gin.New()
	router.GET("/v1/internal/oauth/codex-auth-url", handler.GetAuthURL)
	router.GET("/v1/internal/oauth/callback", handler.HandleCallback)
	router.GET("/v1/internal/oauth/status", handler.GetStatus)

	startReq := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/codex-auth-url", nil)
	startRes := httptest.NewRecorder()
	router.ServeHTTP(startRes, startReq)
	require.Equal(t, http.StatusOK, startRes.Code)

	var authResp map[string]interface{}
	err := json.Unmarshal(startRes.Body.Bytes(), &authResp)
	require.NoError(t, err)
	state := authResp["state"].(string)

	callbackReq := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/callback?code=test-auth-code&state="+state, nil)
	callbackRes := httptest.NewRecorder()
	router.ServeHTTP(callbackRes, callbackReq)
	require.Equal(t, http.StatusInternalServerError, callbackRes.Code)

	statusReq := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/status?state="+state, nil)
	statusRes := httptest.NewRecorder()
	router.ServeHTTP(statusRes, statusReq)
	require.Equal(t, http.StatusOK, statusRes.Code)

	var statusResp map[string]interface{}
	err = json.Unmarshal(statusRes.Body.Bytes(), &statusResp)
	require.NoError(t, err)
	assert.Equal(t, "error", statusResp["status"])
	assert.Equal(t, "completion_failed", statusResp["error_code"])
}
