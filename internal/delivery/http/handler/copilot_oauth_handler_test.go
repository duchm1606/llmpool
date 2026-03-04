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
	"github.com/gin-gonic/gin"
)

// Mock CopilotOAuthProvider
type mockCopilotOAuthProvider struct {
	deviceFlowResp domainoauth.DeviceFlowResponse
	pollPayload    domainoauth.TokenPayload
	deviceFlowErr  error
	pollErr        error
}

func (m *mockCopilotOAuthProvider) BuildAuthURL(_ context.Context, _ string, _ string) (domainoauth.AuthorizationURL, error) {
	return domainoauth.AuthorizationURL{}, errors.New("copilot provider uses device flow only")
}

func (m *mockCopilotOAuthProvider) ExchangeCode(_ context.Context, _ string, _ string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, errors.New("copilot provider uses device flow only")
}

func (m *mockCopilotOAuthProvider) RefreshToken(_ context.Context, _ string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

func (m *mockCopilotOAuthProvider) StartDeviceFlow(_ context.Context) (domainoauth.DeviceFlowResponse, error) {
	if m.deviceFlowErr != nil {
		return domainoauth.DeviceFlowResponse{}, m.deviceFlowErr
	}
	return m.deviceFlowResp, nil
}

func (m *mockCopilotOAuthProvider) PollDevice(_ context.Context, _ string) (domainoauth.TokenPayload, error) {
	if m.pollErr != nil {
		return domainoauth.TokenPayload{}, m.pollErr
	}
	return m.pollPayload, nil
}

// Mock CopilotOAuthCompletionService
type mockCopilotOAuthCompletionService struct {
	profile     domaincredential.Profile
	err         error
	callCount   int
	lastAccount string
}

func (m *mockCopilotOAuthCompletionService) CompleteOAuth(_ context.Context, accountID string, _ domainoauth.TokenPayload) (domaincredential.Profile, error) {
	m.callCount++
	m.lastAccount = accountID
	if m.err != nil {
		return domaincredential.Profile{}, m.err
	}

	profile := m.profile
	if profile.AccountID == "" {
		profile.AccountID = accountID
	}

	return profile, nil
}

func TestCopilotOAuthHandler_StartDeviceFlow_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		deviceFlowResp: domainoauth.DeviceFlowResponse{
			DeviceCode:      "github-device-code-123",
			UserCode:        "WXYZ-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		},
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.POST("/v1/internal/oauth/copilot-device-code", handler.StartDeviceFlow)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/oauth/copilot-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
	if body["device_code"] != "github-device-code-123" {
		t.Errorf("expected device_code 'github-device-code-123', got %v", body["device_code"])
	}
	if body["user_code"] != "WXYZ-1234" {
		t.Errorf("expected user_code 'WXYZ-1234', got %v", body["user_code"])
	}
	if body["verification_uri"] != "https://github.com/login/device" {
		t.Errorf("expected verification_uri 'https://github.com/login/device', got %v", body["verification_uri"])
	}
	if body["expires_in"].(float64) != 900 {
		t.Errorf("expected expires_in 900, got %v", body["expires_in"])
	}
	if body["interval"].(float64) != 5 {
		t.Errorf("expected interval 5, got %v", body["interval"])
	}

	// Verify session was stored
	session, err := store.GetStatus(context.Background(), "github-device-code-123")
	if err != nil {
		t.Fatalf("session not found in store: %v", err)
	}
	if session.Provider != "copilot" {
		t.Errorf("expected provider 'copilot', got %s", session.Provider)
	}
	if session.State != domainoauth.StatePending {
		t.Errorf("expected state pending, got %v", session.State)
	}
}

func TestCopilotOAuthHandler_StartDeviceFlow_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		deviceFlowErr: errors.New("device code URL not configured"),
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.POST("/v1/internal/oauth/copilot-device-code", handler.StartDeviceFlow)

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/oauth/copilot-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_MissingDeviceCode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
	if body["error"] != "device_code parameter required" {
		t.Errorf("expected error message about device_code, got %v", body["error"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_AuthorizationPending(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollErr: errors.New("authorization pending"),
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "wait" {
		t.Errorf("expected status 'wait', got %v", body["status"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_SlowDown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollErr: errors.New("slow down"),
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "wait" {
		t.Errorf("expected status 'wait', got %v", body["status"])
	}
	if body["slow_down"] != true {
		t.Errorf("expected slow_down true, got %v", body["slow_down"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_ExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollErr: errors.New("expired token"),
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
	if body["error_code"] != "expired_token" {
		t.Errorf("expected error_code 'expired_token', got %v", body["error_code"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_AccessDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollErr: errors.New("access denied: user canceled"),
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
	if body["error_code"] != "access_denied" {
		t.Errorf("expected error_code 'access_denied', got %v", body["error_code"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_NoSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollErr: errors.New("copilot access forbidden (subscription required?)"),
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
	if body["error_code"] != "no_subscription" {
		t.Errorf("expected error_code 'no_subscription', got %v", body["error_code"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollPayload: domainoauth.TokenPayload{
			AccessToken:  "copilot-session-token",
			RefreshToken: "github-access-token",
			AccountID:    "testuser",
			Email:        "test@example.com",
			ExpiresAt:    time.Now().Add(30 * time.Minute),
		},
	}
	store := newMockSessionStore()
	completionService := &mockCopilotOAuthCompletionService{
		profile: domaincredential.Profile{AccountID: "testuser"},
	}

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, completionService)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
	if body["account_id"] != "testuser" {
		t.Errorf("expected account_id 'testuser', got %v", body["account_id"])
	}

	// Verify completion service was called
	if completionService.callCount != 1 {
		t.Errorf("expected completion service to be called once, got %d", completionService.callCount)
	}
	if completionService.lastAccount != "testuser" {
		t.Errorf("expected last account 'testuser', got %s", completionService.lastAccount)
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_MissingAccountID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollPayload: domainoauth.TokenPayload{
			AccessToken:  "copilot-session-token",
			RefreshToken: "github-access-token",
			AccountID:    "", // Missing account ID
			ExpiresAt:    time.Now().Add(30 * time.Minute),
		},
	}
	store := newMockSessionStore()

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
	if body["error"] != "missing account identifier" {
		t.Errorf("expected error about missing account, got %v", body["error"])
	}
}

func TestCopilotOAuthHandler_GetDeviceStatus_CompletionFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollPayload: domainoauth.TokenPayload{
			AccessToken:  "copilot-session-token",
			RefreshToken: "github-access-token",
			AccountID:    "testuser",
			ExpiresAt:    time.Now().Add(30 * time.Minute),
		},
	}
	store := newMockSessionStore()
	completionService := &mockCopilotOAuthCompletionService{
		err: errors.New("database error"),
	}

	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, completionService)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
	if body["error"] != "failed to persist credentials" {
		t.Errorf("expected error about persisting credentials, got %v", body["error"])
	}
}

func TestCopilotOAuthHandler_NoopCompletionService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCopilotOAuthProvider{
		pollPayload: domainoauth.TokenPayload{
			AccessToken:  "copilot-session-token",
			RefreshToken: "github-access-token",
			AccountID:    "testuser",
			ExpiresAt:    time.Now().Add(30 * time.Minute),
		},
	}
	store := newMockSessionStore()

	// Pass nil for completion service - should use noop
	handler := NewCopilotOAuthHandler(provider, store, 10*time.Minute, nil)

	router := gin.New()
	router.GET("/v1/internal/oauth/copilot-device-status", handler.GetDeviceStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/copilot-device-status?device_code=test-device-code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should succeed even with nil completion service (uses noop)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
	if body["account_id"] != "testuser" {
		t.Errorf("expected account_id 'testuser', got %v", body["account_id"])
	}
}
