package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duchoang/llmpool/internal/infra/config"
	"time"
)

func makeIDToken(t *testing.T, payload map[string]any) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal id token payload: %v", err)
	}
	body := base64.RawURLEncoding.EncodeToString(bodyBytes)

	return header + "." + body + ".sig"
}

func TestCodexProvider_BuildAuthURL(t *testing.T) {
	provider := NewCodexProvider(config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/oauth/authorize",
		TokenURL:    "https://auth.openai.com/oauth/token",
		RedirectURI: "http://localhost:1455/auth/callback",
		ClientID:    "test-client-id",
		Timeout:     30 * time.Second,
	})

	ctx := context.Background()
	state := "test-state-123"
	verifier, err := GenerateVerifier()
	if err != nil {
		t.Fatalf("generate verifier: %v", err)
	}

	authURL, err := provider.BuildAuthURL(ctx, state, verifier)
	if err != nil {
		t.Fatalf("BuildAuthURL error: %v", err)
	}

	// Verify URL contains required components
	if authURL.URL == "" {
		t.Error("auth URL is empty")
	}
	if authURL.State != state {
		t.Errorf("state mismatch: got %q, want %q", authURL.State, state)
	}

	// Verify PKCE params in URL
	if !strings.Contains(authURL.URL, "code_challenge=") {
		t.Error("auth URL missing code_challenge")
	}
	if !strings.Contains(authURL.URL, "code_challenge_method=S256") {
		t.Error("auth URL missing code_challenge_method=S256")
	}
	if !strings.Contains(authURL.URL, "client_id=test-client-id") {
		t.Error("auth URL missing client_id")
	}
	if !strings.Contains(authURL.URL, "state=test-state-123") {
		t.Error("auth URL missing state")
	}
	if !strings.Contains(authURL.URL, "response_type=code") {
		t.Error("auth URL missing response_type=code")
	}
	if !strings.Contains(authURL.URL, "scope=openid") {
		t.Error("auth URL missing scope")
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
}

func TestCodexProvider_BuildAuthURL_MissingConfig(t *testing.T) {
	provider := NewCodexProvider(config.CodexOAuthConfig{
		// Empty config
	})

	ctx := context.Background()
	_, err := provider.BuildAuthURL(ctx, "state", "verifier")
	if err == nil {
		t.Error("expected error for missing auth URL")
	}
}

func TestCodexProvider_ExchangeCode(t *testing.T) {
	// Create mock token server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("grant_type: got %q, want %q", r.FormValue("grant_type"), "authorization_code")
		}
		if r.FormValue("code") != "test-auth-code" {
			t.Errorf("code: got %q, want %q", r.FormValue("code"), "test-auth-code")
		}
		if r.FormValue("code_verifier") != "test-verifier" {
			t.Errorf("code_verifier: got %q, want %q", r.FormValue("code_verifier"), "test-verifier")
		}
		if r.FormValue("client_id") != "test-client" {
			t.Errorf("client_id: got %q, want %q", r.FormValue("client_id"), "test-client")
		}

		// Return mock token response
		resp := tokenResponse{
			AccessToken:  "mock-access-token",
			RefreshToken: "mock-refresh-token",
			IDToken: makeIDToken(t, map[string]any{
				"email": "user@example.com",
				"sub":   "sub-123",
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_account_id": "acct-123",
				},
			}),
			ExpiresIn: 3600,
			TokenType: "Bearer",
			Scope:     "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck,gosec
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		AuthURL:     mockServer.URL + "/authorize",
		TokenURL:    mockServer.URL + "/token",
		RedirectURI: "http://localhost:1455/auth/callback",
		ClientID:    "test-client",
		Timeout:     30 * time.Second,
	})

	ctx := context.Background()
	payload, err := provider.ExchangeCode(ctx, "test-auth-code", "test-verifier")
	if err != nil {
		t.Fatalf("ExchangeCode error: %v", err)
	}

	if payload.AccessToken != "mock-access-token" {
		t.Errorf("access token: got %q, want %q", payload.AccessToken, "mock-access-token")
	}
	if payload.RefreshToken != "mock-refresh-token" {
		t.Errorf("refresh token: got %q, want %q", payload.RefreshToken, "mock-refresh-token")
	}
	if payload.TokenType != "Bearer" {
		t.Errorf("token type: got %q, want %q", payload.TokenType, "Bearer")
	}
	if payload.Scope != "read write" {
		t.Errorf("scope: got %q, want %q", payload.Scope, "read write")
	}
	if payload.AccountID != "acct-123" {
		t.Errorf("account id: got %q, want %q", payload.AccountID, "acct-123")
	}
	if payload.Email != "user@example.com" {
		t.Errorf("email: got %q, want %q", payload.Email, "user@example.com")
	}
}

func TestCodexProvider_ExchangeCode_MissingIDToken(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tokenResponse{
			AccessToken:  "mock-access-token",
			RefreshToken: "mock-refresh-token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			Scope:        "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck,gosec
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		TokenURL:    mockServer.URL + "/token",
		RedirectURI: "http://localhost:1455/auth/callback",
		ClientID:    "test-client",
		Timeout:     30 * time.Second,
	})

	_, err := provider.ExchangeCode(context.Background(), "test-auth-code", "test-verifier")
	if err == nil {
		t.Fatal("expected error when id_token is missing")
	}
	if !strings.Contains(err.Error(), "extract account identity") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractAccountIdentity_FallbackOrder(t *testing.T) {
	t.Run("uses chatgpt_account_id when present", func(t *testing.T) {
		idToken := makeIDToken(t, map[string]any{
			"email": "user@example.com",
			"sub":   "sub-123",
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": "acct-123",
			},
		})

		accountID, email, err := extractAccountIdentity(idToken)
		if err != nil {
			t.Fatalf("extract account identity: %v", err)
		}
		if accountID != "acct-123" {
			t.Fatalf("expected account id acct-123, got %q", accountID)
		}
		if email != "user@example.com" {
			t.Fatalf("expected email user@example.com, got %q", email)
		}
	})

	t.Run("falls back to sub", func(t *testing.T) {
		idToken := makeIDToken(t, map[string]any{
			"email": "user@example.com",
			"sub":   "sub-456",
		})

		accountID, _, err := extractAccountIdentity(idToken)
		if err != nil {
			t.Fatalf("extract account identity: %v", err)
		}
		if accountID != "sub-456" {
			t.Fatalf("expected account id sub-456, got %q", accountID)
		}
	})

	t.Run("falls back to email", func(t *testing.T) {
		idToken := makeIDToken(t, map[string]any{
			"email": "user@example.com",
		})

		accountID, email, err := extractAccountIdentity(idToken)
		if err != nil {
			t.Fatalf("extract account identity: %v", err)
		}
		if accountID != "user@example.com" {
			t.Fatalf("expected account id user@example.com, got %q", accountID)
		}
		if email != "user@example.com" {
			t.Fatalf("expected email user@example.com, got %q", email)
		}
	})

	t.Run("errors when no identifier", func(t *testing.T) {
		idToken := makeIDToken(t, map[string]any{
			"name": "test",
		})

		_, _, err := extractAccountIdentity(idToken)
		if err == nil {
			t.Fatal("expected error when no identity claims are present")
		}
	})
}

func TestCodexProvider_ExchangeCode_ErrorResponse(t *testing.T) {
	// Create mock server that returns error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		TokenURL: mockServer.URL + "/token",
		ClientID: "test-client",
		Timeout:  30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.ExchangeCode(ctx, "code", "verifier")
	if err == nil {
		t.Error("expected error for bad status code")
	}
}

func TestCodexProvider_ExchangeCode_MissingConfig(t *testing.T) {
	provider := NewCodexProvider(config.CodexOAuthConfig{
		// Empty config - no token URL
	})

	ctx := context.Background()
	_, err := provider.ExchangeCode(ctx, "code", "verifier")
	if err == nil {
		t.Error("expected error for missing token URL")
	}
}

func TestCodexProvider_RefreshToken_MissingConfig(t *testing.T) {
	provider := NewCodexProvider(config.CodexOAuthConfig{
		// Empty config - no token URL
	})

	ctx := context.Background()
	_, err := provider.RefreshToken(ctx, "refresh-token")
	if err == nil {
		t.Error("expected error for missing token URL")
	}
	if !strings.Contains(err.Error(), "token URL not configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCodexProvider_RefreshToken_Success(t *testing.T) {
	// Create mock token server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type: got %q, want %q", r.FormValue("grant_type"), "refresh_token")
		}
		if r.FormValue("refresh_token") != "test-refresh-token" {
			t.Errorf("refresh_token: got %q, want %q", r.FormValue("refresh_token"), "test-refresh-token")
		}
		if r.FormValue("client_id") != "test-client" {
			t.Errorf("client_id: got %q, want %q", r.FormValue("client_id"), "test-client")
		}

		// Return mock token response
		resp := tokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			IDToken: makeIDToken(t, map[string]any{
				"sub": "sub-refresh-1",
			}),
			ExpiresIn: 3600,
			TokenType: "Bearer",
			Scope:     "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck,gosec
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		TokenURL: mockServer.URL + "/token",
		ClientID: "test-client",
		Timeout:  30 * time.Second,
	})

	ctx := context.Background()
	payload, err := provider.RefreshToken(ctx, "test-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken error: %v", err)
	}

	if payload.AccessToken != "new-access-token" {
		t.Errorf("access token: got %q, want %q", payload.AccessToken, "new-access-token")
	}
	if payload.RefreshToken != "new-refresh-token" {
		t.Errorf("refresh token: got %q, want %q", payload.RefreshToken, "new-refresh-token")
	}
	if payload.TokenType != "Bearer" {
		t.Errorf("token type: got %q, want %q", payload.TokenType, "Bearer")
	}
	if payload.AccountID != "sub-refresh-1" {
		t.Errorf("account id: got %q, want %q", payload.AccountID, "sub-refresh-1")
	}
}

func TestCodexProvider_StartDeviceFlow_MissingConfig(t *testing.T) {
	provider := NewCodexProvider(config.CodexOAuthConfig{
		// Empty config - no device URL
	})

	ctx := context.Background()
	_, err := provider.StartDeviceFlow(ctx)
	if err == nil {
		t.Error("expected error for missing device URL")
	}
	if !strings.Contains(err.Error(), "device URL not configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCodexProvider_StartDeviceFlow_Success(t *testing.T) {
	// Create mock device auth server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		if r.FormValue("client_id") != "test-client" {
			t.Errorf("client_id: got %q, want %q", r.FormValue("client_id"), "test-client")
		}

		// Return mock device auth response
		resp := deviceAuthResponse{
			DeviceCode:      "test-device-code-123",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://auth.openai.com/device",
			ExpiresIn:       600,
			Interval:        5,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck,gosec
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		DeviceURL: mockServer.URL + "/device",
		ClientID:  "test-client",
		Timeout:   30 * time.Second,
	})

	ctx := context.Background()
	resp, err := provider.StartDeviceFlow(ctx)
	if err != nil {
		t.Fatalf("StartDeviceFlow error: %v", err)
	}

	if resp.DeviceCode != "test-device-code-123" {
		t.Errorf("device_code: got %q, want %q", resp.DeviceCode, "test-device-code-123")
	}
	if resp.UserCode != "ABCD-EFGH" {
		t.Errorf("user_code: got %q, want %q", resp.UserCode, "ABCD-EFGH")
	}
	if resp.VerificationURI != "https://auth.openai.com/device" {
		t.Errorf("verification_uri: got %q, want %q", resp.VerificationURI, "https://auth.openai.com/device")
	}
	if resp.ExpiresIn != 600 {
		t.Errorf("expires_in: got %d, want %d", resp.ExpiresIn, 600)
	}
	if resp.Interval != 5 {
		t.Errorf("interval: got %d, want %d", resp.Interval, 5)
	}
}

func TestCodexProvider_StartDeviceFlow_ErrorResponse(t *testing.T) {
	// Create mock server that returns error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		DeviceURL: mockServer.URL + "/device",
		ClientID:  "test-client",
		Timeout:   30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.StartDeviceFlow(ctx)
	if err == nil {
		t.Error("expected error for bad status code")
	}
}

func TestCodexProvider_PollDevice_MissingConfig(t *testing.T) {
	provider := NewCodexProvider(config.CodexOAuthConfig{
		// Empty config - no token URL
	})

	ctx := context.Background()
	_, err := provider.PollDevice(ctx, "device-code")
	if err == nil {
		t.Error("expected error for missing token URL")
	}
	if !strings.Contains(err.Error(), "token URL not configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCodexProvider_PollDevice_Success(t *testing.T) {
	// Create mock token server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("grant_type: got %q, want %q", r.FormValue("grant_type"), "urn:ietf:params:oauth:grant-type:device_code")
		}
		if r.FormValue("device_code") != "test-device-code" {
			t.Errorf("device_code: got %q, want %q", r.FormValue("device_code"), "test-device-code")
		}
		if r.FormValue("client_id") != "test-client" {
			t.Errorf("client_id: got %q, want %q", r.FormValue("client_id"), "test-client")
		}

		// Return mock token response
		resp := tokenResponse{
			AccessToken:  "device-access-token",
			RefreshToken: "device-refresh-token",
			IDToken: makeIDToken(t, map[string]any{
				"sub": "sub-device-1",
			}),
			ExpiresIn: 3600,
			TokenType: "Bearer",
			Scope:     "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck,gosec
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		TokenURL: mockServer.URL + "/token",
		ClientID: "test-client",
		Timeout:  30 * time.Second,
	})

	ctx := context.Background()
	payload, err := provider.PollDevice(ctx, "test-device-code")
	if err != nil {
		t.Fatalf("PollDevice error: %v", err)
	}

	if payload.AccessToken != "device-access-token" {
		t.Errorf("access token: got %q, want %q", payload.AccessToken, "device-access-token")
	}
	if payload.RefreshToken != "device-refresh-token" {
		t.Errorf("refresh token: got %q, want %q", payload.RefreshToken, "device-refresh-token")
	}
	if payload.TokenType != "Bearer" {
		t.Errorf("token type: got %q, want %q", payload.TokenType, "Bearer")
	}
	if payload.AccountID != "sub-device-1" {
		t.Errorf("account id: got %q, want %q", payload.AccountID, "sub-device-1")
	}
}

func TestCodexProvider_PollDevice_AuthorizationPending(t *testing.T) {
	// Create mock token server that returns authorization_pending
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
			"error": "authorization_pending",
		})
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		TokenURL: mockServer.URL + "/token",
		ClientID: "test-client",
		Timeout:  30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.PollDevice(ctx, "test-device-code")
	if err == nil {
		t.Fatal("expected error for authorization_pending")
	}
	if !strings.Contains(err.Error(), "authorization pending") {
		t.Errorf("expected 'authorization pending' error, got: %v", err)
	}
}

func TestCodexProvider_PollDevice_SlowDown(t *testing.T) {
	// Create mock token server that returns slow_down
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
			"error": "slow_down",
		})
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		TokenURL: mockServer.URL + "/token",
		ClientID: "test-client",
		Timeout:  30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.PollDevice(ctx, "test-device-code")
	if err == nil {
		t.Fatal("expected error for slow_down")
	}
	if !strings.Contains(err.Error(), "slow down") {
		t.Errorf("expected 'slow down' error, got: %v", err)
	}
}

func TestCodexProvider_PollDevice_ExpiredToken(t *testing.T) {
	// Create mock token server that returns expired_token
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
			"error": "expired_token",
		})
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		TokenURL: mockServer.URL + "/token",
		ClientID: "test-client",
		Timeout:  30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.PollDevice(ctx, "test-device-code")
	if err == nil {
		t.Fatal("expected error for expired_token")
	}
	if !strings.Contains(err.Error(), "expired token") {
		t.Errorf("expected 'expired token' error, got: %v", err)
	}
}
