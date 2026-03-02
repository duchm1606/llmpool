package oauth_test

import (
	"testing"

	"github.com/duchoang/llmpool/internal/domain/oauth"
)

func TestOAuthStateConstants(t *testing.T) {
	tests := []struct {
		name     string
		state    oauth.OAuthState
		expected string
	}{
		{"pending state", oauth.StatePending, "pending"},
		{"ok state", oauth.StateOK, "ok"},
		{"error state", oauth.StateError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.state))
			}
		})
	}
}

func TestOAuthSessionStructure(t *testing.T) {
	// Basic compilation and structure test
	session := oauth.OAuthSession{
		SessionID:    "test-session",
		State:        oauth.StatePending,
		PKCEVerifier: "verifier",
		Provider:     "codex",
	}

	if session.SessionID != "test-session" {
		t.Errorf("expected SessionID to be test-session, got %s", session.SessionID)
	}

	if session.State != oauth.StatePending {
		t.Errorf("expected State to be pending, got %s", session.State)
	}
}

func TestTokenPayloadStructure(t *testing.T) {
	// Basic compilation and structure test
	payload := oauth.TokenPayload{
		AccessToken:  "access",
		RefreshToken: "refresh",
		AccountID:    "account-123",
		TokenType:    "Bearer",
	}

	if payload.AccessToken != "access" {
		t.Errorf("expected AccessToken to be access, got %s", payload.AccessToken)
	}

	if payload.AccountID != "account-123" {
		t.Errorf("expected AccountID to be account-123, got %s", payload.AccountID)
	}
}
