package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duchoang/llmpool/internal/infra/config"
)

func TestCopilotProvider_BuildAuthURL_NotSupported(t *testing.T) {
	provider := NewCopilotProvider(config.CopilotOAuthConfig{})

	ctx := context.Background()
	_, err := provider.BuildAuthURL(ctx, "state", "verifier")
	if err == nil {
		t.Error("expected error for device flow only provider")
	}
	if !strings.Contains(err.Error(), "device flow only") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCopilotProvider_ExchangeCode_NotSupported(t *testing.T) {
	provider := NewCopilotProvider(config.CopilotOAuthConfig{})

	ctx := context.Background()
	_, err := provider.ExchangeCode(ctx, "code", "verifier")
	if err == nil {
		t.Error("expected error for device flow only provider")
	}
	if !strings.Contains(err.Error(), "device flow only") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCopilotProvider_StartDeviceFlow_MissingConfig(t *testing.T) {
	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		// Empty config - no device code URL
	})

	ctx := context.Background()
	_, err := provider.StartDeviceFlow(ctx)
	if err == nil {
		t.Error("expected error for missing device code URL")
	}
	if !strings.Contains(err.Error(), "device code URL not configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCopilotProvider_StartDeviceFlow_Success(t *testing.T) {
	// Create mock GitHub device auth server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify Content-Type is JSON (aligned with reference implementation)
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type header: got %q, want %q", ct, "application/json")
		}

		// Verify Accept header
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept header: got %q, want %q", r.Header.Get("Accept"), "application/json")
		}

		// Parse JSON body
		var body struct {
			ClientID string `json:"client_id"`
			Scope    string `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode json body: %v", err)
		}

		if body.ClientID != "test-client-id" {
			t.Errorf("client_id: got %q, want %q", body.ClientID, "test-client-id")
		}
		if body.Scope != "read:user" {
			t.Errorf("scope: got %q, want %q", body.Scope, "read:user")
		}

		// Return mock device auth response
		resp := githubDeviceResponse{
			DeviceCode:      "github-device-code-123",
			UserCode:        "WXYZ-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck,gosec
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID:      "test-client-id",
		DeviceCodeURL: mockServer.URL + "/device/code",
		Scope:         "read:user",
		Timeout:       30 * time.Second,
	})

	ctx := context.Background()
	resp, err := provider.StartDeviceFlow(ctx)
	if err != nil {
		t.Fatalf("StartDeviceFlow error: %v", err)
	}

	if resp.DeviceCode != "github-device-code-123" {
		t.Errorf("device_code: got %q, want %q", resp.DeviceCode, "github-device-code-123")
	}
	if resp.UserCode != "WXYZ-1234" {
		t.Errorf("user_code: got %q, want %q", resp.UserCode, "WXYZ-1234")
	}
	if resp.VerificationURI != "https://github.com/login/device" {
		t.Errorf("verification_uri: got %q, want %q", resp.VerificationURI, "https://github.com/login/device")
	}
	if resp.ExpiresIn != 900 {
		t.Errorf("expires_in: got %d, want %d", resp.ExpiresIn, 900)
	}
	if resp.Interval != 5 {
		t.Errorf("interval: got %d, want %d", resp.Interval, 5)
	}
}

func TestCopilotProvider_StartDeviceFlow_ErrorResponse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid_client"}`)) //nolint:errcheck,gosec
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID:      "test-client-id",
		DeviceCodeURL: mockServer.URL + "/device/code",
		Timeout:       30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.StartDeviceFlow(ctx)
	if err == nil {
		t.Error("expected error for bad status code")
	}
}

func TestCopilotProvider_PollDevice_MissingConfig(t *testing.T) {
	provider := NewCopilotProvider(config.CopilotOAuthConfig{
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

func TestCopilotProvider_PollDevice_AuthorizationPending(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
			"error":             "authorization_pending",
			"error_description": "The authorization request is still pending",
		})
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL + "/oauth/access_token",
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

func TestCopilotProvider_PollDevice_SlowDown(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
			"error": "slow_down",
		})
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL + "/oauth/access_token",
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

func TestCopilotProvider_PollDevice_ExpiredToken(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
			"error": "expired_token",
		})
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL + "/oauth/access_token",
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

func TestCopilotProvider_PollDevice_AccessDenied(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
			"error":             "access_denied",
			"error_description": "User denied access",
		})
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID: "test-client-id",
		TokenURL: mockServer.URL + "/oauth/access_token",
		Timeout:  30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.PollDevice(ctx, "test-device-code")
	if err == nil {
		t.Fatal("expected error for access_denied")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("expected 'access denied' error, got: %v", err)
	}
}

func TestCopilotProvider_PollDevice_Success(t *testing.T) {
	callCount := 0

	// Create mock servers for all 3 endpoints
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/oauth/access_token"):
			// GitHub token endpoint - expects JSON body (aligned with reference implementation)
			var body struct {
				ClientID   string `json:"client_id"`
				DeviceCode string `json:"device_code"`
				GrantType  string `json:"grant_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode json body: %v", err)
			}

			if body.GrantType != "urn:ietf:params:oauth:grant-type:device_code" {
				t.Errorf("grant_type: got %q, want device_code grant", body.GrantType)
			}
			if body.DeviceCode != "test-device-code" {
				t.Errorf("device_code: got %q, want %q", body.DeviceCode, "test-device-code")
			}
			if body.ClientID != "test-client-id" {
				t.Errorf("client_id: got %q, want %q", body.ClientID, "test-client-id")
			}

			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
				"access_token": "github-access-token-xyz",
				"token_type":   "bearer",
				"scope":        "read:user",
			})

		case strings.Contains(r.URL.Path, "/copilot_internal"):
			// Copilot token endpoint
			authHeader := r.Header.Get("Authorization")
			if authHeader != "token github-access-token-xyz" {
				t.Errorf("Authorization header: got %q, want %q", authHeader, "token github-access-token-xyz")
			}

			json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck,gosec
				"token":      "copilot-session-token-abc",
				"expires_at": time.Now().Add(30 * time.Minute).Unix(),
				"endpoints": map[string]string{
					"api": "https://api.githubcopilot.com",
				},
			})

		case strings.Contains(r.URL.Path, "/user"):
			// GitHub user info endpoint
			authHeader := r.Header.Get("Authorization")
			if authHeader != "token github-access-token-xyz" {
				t.Errorf("Authorization header: got %q, want %q", authHeader, "token github-access-token-xyz")
			}

			json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck,gosec
				"id":    12345,
				"login": "testuser",
				"email": "test@example.com",
				"name":  "Test User",
			})

		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID:        "test-client-id",
		TokenURL:        mockServer.URL + "/oauth/access_token",
		CopilotTokenURL: mockServer.URL + "/copilot_internal/v2/token",
		UserInfoURL:     mockServer.URL + "/user",
		Scope:           "read:user",
		Timeout:         30 * time.Second,
	})

	ctx := context.Background()
	payload, err := provider.PollDevice(ctx, "test-device-code")
	if err != nil {
		t.Fatalf("PollDevice error: %v", err)
	}

	// Verify we made all 3 calls
	if callCount != 3 {
		t.Errorf("expected 3 API calls, got %d", callCount)
	}

	// Verify payload
	if payload.AccessToken != "copilot-session-token-abc" {
		t.Errorf("access token: got %q, want %q", payload.AccessToken, "copilot-session-token-abc")
	}
	if payload.RefreshToken != "github-access-token-xyz" {
		t.Errorf("refresh token (github token): got %q, want %q", payload.RefreshToken, "github-access-token-xyz")
	}
	if payload.AccountID != "testuser" {
		t.Errorf("account id: got %q, want %q", payload.AccountID, "testuser")
	}
	if payload.Email != "test@example.com" {
		t.Errorf("email: got %q, want %q", payload.Email, "test@example.com")
	}
	if payload.TokenType != "Bearer" {
		t.Errorf("token type: got %q, want %q", payload.TokenType, "Bearer")
	}
	if payload.Scope != "read:user" {
		t.Errorf("scope: got %q, want %q", payload.Scope, "read:user")
	}
}

func TestCopilotProvider_PollDevice_CopilotForbidden(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/oauth/access_token"):
			// GitHub token success
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck,gosec
				"access_token": "github-token",
				"token_type":   "bearer",
			})

		case strings.Contains(r.URL.Path, "/copilot_internal"):
			// Copilot token forbidden (no subscription)
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message": "Resource not accessible by integration"}`)) //nolint:errcheck,gosec

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		ClientID:        "test-client-id",
		TokenURL:        mockServer.URL + "/oauth/access_token",
		CopilotTokenURL: mockServer.URL + "/copilot_internal/v2/token",
		Timeout:         30 * time.Second,
	})

	ctx := context.Background()
	_, err := provider.PollDevice(ctx, "test-device-code")
	if err == nil {
		t.Fatal("expected error for forbidden")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected 'forbidden' error, got: %v", err)
	}
}

func TestCopilotProvider_RefreshToken_MissingConfig(t *testing.T) {
	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		// Empty config - no copilot token URL
	})

	ctx := context.Background()
	_, err := provider.RefreshToken(ctx, "github-token")
	if err == nil {
		t.Error("expected error for missing copilot token URL")
	}
	if !strings.Contains(err.Error(), "copilot token URL not configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCopilotProvider_RefreshToken_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/copilot_internal"):
			// Verify authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "token github-access-token" {
				t.Errorf("Authorization header: got %q, want %q", authHeader, "token github-access-token")
			}

			json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck,gosec
				"token":      "new-copilot-session-token",
				"expires_at": time.Now().Add(30 * time.Minute).Unix(),
				"endpoints": map[string]string{
					"api": "https://api.githubcopilot.com",
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	provider := NewCopilotProvider(config.CopilotOAuthConfig{
		CopilotTokenURL: mockServer.URL + "/copilot_internal/v2/token",
		Scope:           "read:user",
		Timeout:         30 * time.Second,
	})

	ctx := context.Background()
	payload, err := provider.RefreshToken(ctx, "github-access-token")
	if err != nil {
		t.Fatalf("RefreshToken error: %v", err)
	}

	if payload.AccessToken != "new-copilot-session-token" {
		t.Errorf("access token: got %q, want %q", payload.AccessToken, "new-copilot-session-token")
	}
	if payload.RefreshToken != "github-access-token" {
		t.Errorf("refresh token: got %q, want %q", payload.RefreshToken, "github-access-token")
	}
	// RefreshToken no longer fetches user info - AccountID should be empty
	// The refresh service preserves existing account identity from the credential profile
	if payload.AccountID != "" {
		t.Errorf("account id: got %q, want empty (refresh preserves existing identity)", payload.AccountID)
	}
}

func TestGithubUserResponse_AccountID(t *testing.T) {
	tests := []struct {
		name     string
		user     *githubUserResponse
		expected string
	}{
		{
			name:     "nil user",
			user:     nil,
			expected: "",
		},
		{
			name:     "login present",
			user:     &githubUserResponse{Login: "testuser", ID: 123, Email: "test@example.com"},
			expected: "testuser",
		},
		{
			name:     "no login, uses ID",
			user:     &githubUserResponse{ID: 456, Email: "test@example.com"},
			expected: "456",
		},
		{
			name:     "no login or ID, uses email",
			user:     &githubUserResponse{Email: "test@example.com"},
			expected: "test@example.com",
		},
		{
			name:     "empty user",
			user:     &githubUserResponse{},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.user.accountID()
			if result != tc.expected {
				t.Errorf("accountID(): got %q, want %q", result, tc.expected)
			}
		})
	}
}
