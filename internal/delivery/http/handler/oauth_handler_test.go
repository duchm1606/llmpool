package handler

import (
	"context"
	"encoding/json"
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
	return domainoauth.TokenPayload{}, nil
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
	return nil
}

func (m *mockSessionStore) MarkError(_ context.Context, sessionID string, errorCode string, errorMessage string) error {
	return nil
}

func (m *mockSessionStore) Consume(_ context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	return domainoauth.OAuthSession{}, nil
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
