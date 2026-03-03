package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodexProvider_BuildAuthURL(t *testing.T) {
	config := CodexConfig{
		ClientID:    "test-client-id",
		AuthURL:     "https://auth.openai.com/authorize",
		TokenURL:    "https://auth.openai.com/token",
		RedirectURI: "http://localhost:8080/oauth/callback",
		Timeout:     30,
	}

	provider := NewCodexProvider(config, nil)

	ctx := context.Background()
	state := "test-state-123"
	verifier := "test-verifier-xyz"

	authURL, err := provider.BuildAuthURL(ctx, state, verifier)
	if err != nil {
		t.Fatalf("BuildAuthURL() error = %v", err)
	}

	// Verify URL contains required PKCE parameters
	if !strings.Contains(authURL.URL, "code_challenge=") {
		t.Error("Auth URL missing code_challenge parameter")
	}
	if !strings.Contains(authURL.URL, "code_challenge_method=S256") {
		t.Error("Auth URL missing or incorrect code_challenge_method")
	}
	if !strings.Contains(authURL.URL, "state="+state) {
		t.Error("Auth URL missing state parameter")
	}
	if !strings.Contains(authURL.URL, "redirect_uri=") {
		t.Error("Auth URL missing redirect_uri parameter")
	}
	if !strings.Contains(authURL.URL, "response_type=code") {
		t.Error("Auth URL missing response_type parameter")
	}
	if !strings.Contains(authURL.URL, "prompt=login") {
		t.Error("auth URL missing prompt=login")
	}
	if !strings.Contains(authURL.URL, "id_token_add_organizations=true") {
		t.Error("auth URL missing id_token_add_organizations")
	}
	if !strings.Contains(authURL.URL, "codex_cli_simplified_flow=true") {
		t.Error("auth URL missing codex_cli_simplified_flow")
	}
	// Verify state is returned
	if authURL.State != state {
		t.Errorf("authURL.State = %v, want %v", authURL.State, state)
	}
}

func TestCodexProvider_BuildAuthURL_ChallengeCorrectness(t *testing.T) {
	config := CodexConfig{
		ClientID:    "test-client-id",
		AuthURL:     "https://auth.openai.com/authorize",
		TokenURL:    "https://auth.openai.com/token",
		RedirectURI: "http://localhost:8080/oauth/callback",
		Timeout:     30,
	}

	provider := NewCodexProvider(config, nil)

	ctx := context.Background()
	state := "test-state"
	verifier := "E9Mrozoa2owUednlCZ_lXvJwV2ChwuCHofo37Zv7h8"

	authURL, err := provider.BuildAuthURL(ctx, state, verifier)
	if err != nil {
		t.Fatalf("BuildAuthURL() error = %v", err)
	}

	// Compute expected challenge
	expectedChallenge := GenerateChallenge(verifier)

	// Verify challenge is in the URL
	if !strings.Contains(authURL.URL, "code_challenge="+expectedChallenge) {
		t.Errorf("Auth URL challenge mismatch. Expected challenge: %s", expectedChallenge)
	}
}

func TestCodexProvider_ExchangeCode_Success(t *testing.T) {
	// Create mock token server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and content type
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Expected form encoding, got %s", r.Header.Get("Content-Type"))
		}

		// Verify required parameters
		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}

		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("Expected grant_type=authorization_code, got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("code") != "auth-code-123" {
			t.Errorf("Expected code=auth-code-123, got %s", r.FormValue("code"))
		}
		if r.FormValue("code_verifier") != "verifier-xyz" {
			t.Errorf("Expected code_verifier, got %s", r.FormValue("code_verifier"))
		}

		// Return mock token response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
            "access_token": "mock-access-token",
            "refresh_token": "mock-refresh-token",
            "expires_in": 3600,
            "token_type": "Bearer"
        }`))
	}))
	defer mockServer.Close()

	config := CodexConfig{
		ClientID:    "test-client-id",
		AuthURL:     "https://auth.openai.com/authorize",
		TokenURL:    mockServer.URL,
		RedirectURI: "http://localhost:8080/oauth/callback",
		Timeout:     30,
	}

	provider := NewCodexProvider(config, nil)

	ctx := context.Background()
	payload, err := provider.ExchangeCode(ctx, "auth-code-123", "verifier-xyz")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}

	if payload.AccessToken != "mock-access-token" {
		t.Errorf("AccessToken = %v, want mock-access-token", payload.AccessToken)
	}
	if payload.RefreshToken != "mock-refresh-token" {
		t.Errorf("RefreshToken = %v, want mock-refresh-token", payload.RefreshToken)
	}
	if payload.TokenType != "Bearer" {
		t.Errorf("TokenType = %v, want Bearer", payload.TokenType)
	}
}

func TestCodexProvider_ExchangeCode_Error(t *testing.T) {
	// Create mock server that returns error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{
            "error": "invalid_grant",
            "error_description": "The provided authorization grant is invalid"
        }`))
	}))
	defer mockServer.Close()

	config := CodexConfig{
		ClientID:    "test-client-id",
		AuthURL:     "https://auth.openai.com/authorize",
		TokenURL:    mockServer.URL,
		RedirectURI: "http://localhost:8080/oauth/callback",
		Timeout:     30,
	}

	provider := NewCodexProvider(config, nil)

	ctx := context.Background()
	_, err := provider.ExchangeCode(ctx, "invalid-code", "verifier-xyz")
	if err == nil {
		t.Error("ExchangeCode() expected error for invalid grant, got nil")
	}
}
