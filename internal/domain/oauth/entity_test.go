package oauth

import (
	"testing"
	"time"
)

func TestOAuthState_Constants(t *testing.T) {
	// Verify state constants exist and have expected values
	if StatePending != "pending" {
		t.Errorf("StatePending = %v, want pending", StatePending)
	}
	if StateOK != "ok" {
		t.Errorf("StateOK = %v, want ok", StateOK)
	}
	if StateError != "error" {
		t.Errorf("StateError = %v, want error", StateError)
	}
}

func TestOAuthSession_Structure(t *testing.T) {
	now := time.Now()
	session := OAuthSession{
		SessionID:    "test-session-123",
		State:        StatePending,
		PKCEVerifier: "verifier-xyz",
		Provider:     "codex",
		Expiry:       now.Add(10 * time.Minute),
		ErrorMessage: "",
		ErrorCode:    "",
		CreatedAt:    now,
		AccountID:    "",
	}

	if session.SessionID != "test-session-123" {
		t.Error("SessionID not set correctly")
	}
	if session.State != StatePending {
		t.Error("State not set correctly")
	}
	if session.PKCEVerifier != "verifier-xyz" {
		t.Error("PKCEVerifier not set correctly")
	}
}

func TestTokenPayload_Structure(t *testing.T) {
	expiry := time.Now().Add(1 * time.Hour)
	payload := TokenPayload{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		IDToken:      "id-token-789",
		Email:        "user@example.com",
		ExpiresAt:    expiry,
		AccountID:    "acct-123",
		TokenType:    "Bearer",
		Scope:        "codex",
	}

	if payload.AccessToken != "access-token-123" {
		t.Error("AccessToken not set correctly")
	}
	if payload.RefreshToken != "refresh-token-456" {
		t.Error("RefreshToken not set correctly")
	}
	if payload.IDToken != "id-token-789" {
		t.Error("IDToken not set correctly")
	}
	if payload.Email != "user@example.com" {
		t.Error("Email not set correctly")
	}
	if payload.AccountID != "acct-123" {
		t.Error("AccountID not set correctly")
	}
	if !payload.ExpiresAt.Equal(expiry) {
		t.Error("ExpiresAt not set correctly")
	}
}

func TestAuthorizationURL_Structure(t *testing.T) {
	authURL := AuthorizationURL{
		URL:   "https://auth.openai.com/authorize?...",
		State: "state-abc-123",
	}

	if authURL.URL == "" {
		t.Error("URL should not be empty")
	}
	if authURL.State == "" {
		t.Error("State should not be empty")
	}
}
