package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duchoang/llmpool/internal/infra/config"
	"time"
)

func TestCodexProvider_BuildAuthURL(t *testing.T) {
	provider := NewCodexProvider(config.CodexOAuthConfig{
		AuthURL:     "https://auth.openai.com/authorize",
		TokenURL:    "https://auth.openai.com/token",
		RedirectURI: "http://localhost:8080/oauth/callback",
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
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			Scope:        "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	provider := NewCodexProvider(config.CodexOAuthConfig{
		AuthURL:     mockServer.URL + "/authorize",
		TokenURL:    mockServer.URL + "/token",
		RedirectURI: "http://localhost:8080/oauth/callback",
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
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			Scope:        "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
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
		json.NewEncoder(w).Encode(resp)
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
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			Scope:        "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
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
}

func TestCodexProvider_PollDevice_AuthorizationPending(t *testing.T) {
	// Create mock token server that returns authorization_pending
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
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
		json.NewEncoder(w).Encode(map[string]string{
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
		json.NewEncoder(w).Encode(map[string]string{
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
