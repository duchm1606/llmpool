package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/duchoang/llmpool/internal/infra/config"
	"github.com/duchoang/llmpool/internal/infra/oauth"
	"github.com/gin-gonic/gin"
)

// Mock OAuthProvider
type mockOAuthProvider struct {
	authURL domainoauth.AuthorizationURL
	err     error
}

func (m *mockOAuthProvider) BuildAuthURL(_ context.Context, state string, _ string) (domainoauth.AuthorizationURL, error) {
	if m.err != nil {
		return domainoauth.AuthorizationURL{}, m.err
	}
	if m.authURL.URL == "" {
		m.authURL.URL = "https://auth.openai.com/authorize?code_challenge=xyz&state=" + state
		m.authURL.State = state
	}
	return m.authURL, nil
}

func (m *mockOAuthProvider) ExchangeCode(_ context.Context, _ string, _ string) (domainoauth.TokenPayload, error) {
	if m.err != nil {
		return domainoauth.TokenPayload{}, m.err
	}
	return domainoauth.TokenPayload{
		AccessToken: "test-access-token-12345",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
		AccountID:   "test-account-123",
	}, nil
}

func (m *mockOAuthProvider) RefreshToken(_ context.Context, _ string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

func (m *mockOAuthProvider) StartDeviceFlow(_ context.Context) (domainoauth.DeviceFlowResponse, error) {
	return domainoauth.DeviceFlowResponse{}, nil
}

func (m *mockOAuthProvider) PollDevice(_ context.Context, _ string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

// Mock OAuthSessionStore
type mockSessionStore struct {
	sessions map[string]domainoauth.OAuthSession
	err      error
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		sessions: make(map[string]domainoauth.OAuthSession),
	}
}

func (m *mockSessionStore) CreatePending(_ context.Context, session domainoauth.OAuthSession) error {
	if m.err != nil {
		return m.err
	}
	m.sessions[session.SessionID] = session
	return nil
}

func (m *mockSessionStore) GetStatus(_ context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	if m.err != nil {
		return domainoauth.OAuthSession{}, m.err
	}
	session, exists := m.sessions[sessionID]
	if !exists {
		return domainoauth.OAuthSession{}, oauth.ErrSessionNotFound
	}
	return session, nil
}

func (m *mockSessionStore) MarkComplete(_ context.Context, sessionID string, accountID string) error {
	if m.err != nil {
		return m.err
	}
	if session, ok := m.sessions[sessionID]; ok {
		session.State = domainoauth.StateOK
		session.AccountID = accountID
		session.CompletedAt = &time.Time{}
		m.sessions[sessionID] = session
	}
	return nil
}

func (m *mockSessionStore) MarkError(_ context.Context, sessionID string, errorCode string, errorMessage string) error {
	if m.err != nil {
		return m.err
	}
	if session, ok := m.sessions[sessionID]; ok {
		session.State = domainoauth.StateError
		session.ErrorCode = errorCode
		session.ErrorMessage = errorMessage
		m.sessions[sessionID] = session
	}
	return nil
}

func (m *mockSessionStore) Consume(_ context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	if m.err != nil {
		return domainoauth.OAuthSession{}, m.err
	}
	session, exists := m.sessions[sessionID]
	if !exists {
		return domainoauth.OAuthSession{}, oauth.ErrSessionNotFound
	}
	// Simulate consuming session by removing it
	delete(m.sessions, sessionID)
	return session, nil
}

func TestOAuthHandler_GetAuthURL_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	router := gin.New()
	router.GET("/v1/internal/oauth/codex-auth-url", handler.GetAuthURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/codex-auth-url", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Verify response fields
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}

	url, ok := body["url"].(string)
	if !ok || url == "" {
		t.Errorf("expected non-empty url, got %v", body["url"])
	}

	state, ok := body["state"].(string)
	if !ok || state == "" {
		t.Errorf("expected non-empty state, got %v", body["state"])
	}

	// Verify session was stored in Redis
	session, err := store.GetStatus(context.Background(), state)
	if err != nil {
		t.Fatalf("session not found in store: %v", err)
	}

	if session.SessionID != state {
		t.Errorf("expected session ID %s, got %s", state, session.SessionID)
	}

	if session.State != domainoauth.StatePending {
		t.Errorf("expected state pending, got %v", session.State)
	}

	if session.PKCEVerifier == "" {
		t.Error("expected non-empty PKCE verifier")
	}

	if session.Provider != "codex" {
		t.Errorf("expected provider codex, got %s", session.Provider)
	}
}

func TestOAuthHandler_GetAuthURLCompatibility_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	router := gin.New()
	router.GET("/v0/management/codex-auth-url", handler.GetAuthURLCompatibility)

	// Test with is_webui parameter
	req := httptest.NewRequest(http.MethodGet, "/v0/management/codex-auth-url?is_webui=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Verify same response shape as native endpoint
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}

	url, ok := body["url"].(string)
	if !ok || url == "" {
		t.Errorf("expected non-empty url, got %v", body["url"])
	}

	state, ok := body["state"].(string)
	if !ok || state == "" {
		t.Errorf("expected non-empty state, got %v", body["state"])
	}

	// Verify session was stored
	session, err := store.GetStatus(context.Background(), state)
	if err != nil {
		t.Fatalf("session not found in store: %v", err)
	}

	if session.State != domainoauth.StatePending {
		t.Errorf("expected state pending, got %v", session.State)
	}
}

func TestOAuthHandler_GetAuthURL_StateIsUnique(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	router := gin.New()
	router.GET("/v1/internal/oauth/codex-auth-url", handler.GetAuthURL)

	// Make two requests
	states := make([]string, 2)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/codex-auth-url", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i, w.Code)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("request %d: unmarshal response: %v", i, err)
		}

		state, ok := body["state"].(string)
		if !ok || state == "" {
			t.Fatalf("request %d: expected non-empty state", i)
		}

		states[i] = state
	}

	// Verify states are different
	if states[0] == states[1] {
		t.Error("expected unique states for different requests")
	}
}

func TestOAuthHandler_GetAuthURL_VerifierNotExposed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	router := gin.New()
	router.GET("/v1/internal/oauth/codex-auth-url", handler.GetAuthURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/codex-auth-url", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Verify verifier is NOT in response
	if _, exists := body["verifier"]; exists {
		t.Error("verifier must not be exposed in response")
	}

	if _, exists := body["code_verifier"]; exists {
		t.Error("code_verifier must not be exposed in response")
	}

	// Verify verifier IS in session store
	state := body["state"].(string)
	session, err := store.GetStatus(context.Background(), state)
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}

	if session.PKCEVerifier == "" {
		t.Error("verifier must be stored in session")
	}
}

func TestOAuthHandler_HandleCallback_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	// First, create a pending session
	state := oauth.GenerateState()
	verifier, _ := oauth.GenerateVerifier()
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StatePending,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}
	if err := store.CreatePending(context.Background(), session); err != nil {
		t.Fatalf("failed to create pending session: %v", err)
	}

	router := gin.New()
	router.GET("/v1/internal/oauth/callback", handler.HandleCallback)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/callback?code=test-code&state="+state, nil)
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

	// Verify session was marked complete
	updatedSession, err := store.GetStatus(context.Background(), state)
	if err != nil {
		t.Fatalf("session not found after callback: %v", err)
	}

	if updatedSession.State != domainoauth.StateOK {
		t.Errorf("expected session state OK, got %v", updatedSession.State)
	}

	if updatedSession.AccountID == "" {
		t.Error("expected non-empty account ID")
	}
}

func TestOAuthHandler_HandleCallback_MissingCode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	router := gin.New()
	router.GET("/v1/internal/oauth/callback", handler.HandleCallback)

	// Request without code or error should fail at validation
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/callback?state=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return bad request since session not found
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestOAuthHandler_HandleCallback_ProviderError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{
		err: errors.New("invalid code"),
	}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	// Create a pending session
	state := oauth.GenerateState()
	verifier, _ := oauth.GenerateVerifier()
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StatePending,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}
	if err := store.CreatePending(context.Background(), session); err != nil {
		t.Fatalf("failed to create pending session: %v", err)
	}

	router := gin.New()
	router.GET("/v1/internal/oauth/callback", handler.HandleCallback)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/callback?code=invalid-code&state="+state, nil)
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

func TestOAuthHandler_HandleCallback_OAuthError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	// Create a pending session
	state := oauth.GenerateState()
	verifier, _ := oauth.GenerateVerifier()
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StatePending,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}
	if err := store.CreatePending(context.Background(), session); err != nil {
		t.Fatalf("failed to create pending session: %v", err)
	}

	router := gin.New()
	router.GET("/v1/internal/oauth/callback", handler.HandleCallback)

	// OAuth provider returns error parameter
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/callback?error=access_denied&error_description=User+denied&state="+state, nil)
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

	if body["error"] != "access_denied" {
		t.Errorf("expected error code 'access_denied', got %v", body["error"])
	}
}

func TestOAuthHandler_GetStatus_Pending(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	// Create a pending session
	state := oauth.GenerateState()
	verifier, _ := oauth.GenerateVerifier()
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StatePending,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}
	if err := store.CreatePending(context.Background(), session); err != nil {
		t.Fatalf("failed to create pending session: %v", err)
	}

	router := gin.New()
	router.GET("/v1/internal/oauth/status", handler.GetStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/status?state="+state, nil)
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
		t.Errorf("expected status 'wait' for pending session, got %v", body["status"])
	}
}

func TestOAuthHandler_GetStatus_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	// Create a completed session
	state := oauth.GenerateState()
	verifier, _ := oauth.GenerateVerifier()
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StateOK,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
		AccountID:    "account-12345",
	}
	if err := store.CreatePending(context.Background(), session); err != nil {
		t.Fatalf("failed to create pending session: %v", err)
	}

	router := gin.New()
	router.GET("/v1/internal/oauth/status", handler.GetStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/status?state="+state, nil)
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

	if body["account_id"] != "account-12345" {
		t.Errorf("expected account_id 'account-12345', got %v", body["account_id"])
	}
}

func TestOAuthHandler_GetStatus_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	// Create an errored session
	state := oauth.GenerateState()
	verifier, _ := oauth.GenerateVerifier()
	session := domainoauth.OAuthSession{
		SessionID:    state,
		State:        domainoauth.StateError,
		PKCEVerifier: verifier,
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
		ErrorCode:    "access_denied",
		ErrorMessage: "User denied access",
	}
	if err := store.CreatePending(context.Background(), session); err != nil {
		t.Fatalf("failed to create pending session: %v", err)
	}

	router := gin.New()
	router.GET("/v1/internal/oauth/status", handler.GetStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/status?state="+state, nil)
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

	if body["error_message"] != "User denied access" {
		t.Errorf("expected error_message 'User denied access', got %v", body["error_message"])
	}
}

func TestOAuthHandler_GetStatus_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	router := gin.New()
	router.GET("/v1/internal/oauth/status", handler.GetStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/status?state=nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
}

func TestOAuthHandler_GetStatus_MissingState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockOAuthProvider{}
	store := newMockSessionStore()
	cfg := config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
	}

	handler := NewOAuthHandler(provider, store, cfg, 10*time.Minute)

	router := gin.New()
	router.GET("/v1/internal/oauth/status", handler.GetStatus)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/oauth/status", nil)
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
}
