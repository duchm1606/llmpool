package oauth_test

import (
	"context"
	"testing"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/duchoang/llmpool/internal/usecase/oauth"
)

// Mock implementations for interface compilation testing

type mockOAuthProvider struct{}

func (m *mockOAuthProvider) BuildAuthURL(ctx context.Context, state string, verifier string) (domainoauth.AuthorizationURL, error) {
	return domainoauth.AuthorizationURL{}, nil
}

func (m *mockOAuthProvider) ExchangeCode(ctx context.Context, code string, verifier string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

func (m *mockOAuthProvider) RefreshToken(ctx context.Context, refreshToken string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

func (m *mockOAuthProvider) StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error) {
	return domainoauth.DeviceFlowResponse{}, nil
}

func (m *mockOAuthProvider) PollDevice(ctx context.Context, deviceCode string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

type mockOAuthSessionStore struct{}

func (m *mockOAuthSessionStore) CreatePending(ctx context.Context, session domainoauth.OAuthSession) error {
	return nil
}

func (m *mockOAuthSessionStore) GetStatus(ctx context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	return domainoauth.OAuthSession{}, nil
}

func (m *mockOAuthSessionStore) MarkComplete(ctx context.Context, sessionID string, accountID string) error {
	return nil
}

func (m *mockOAuthSessionStore) MarkError(ctx context.Context, sessionID string, errorCode string, errorMessage string) error {
	return nil
}

func (m *mockOAuthSessionStore) Consume(ctx context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	return domainoauth.OAuthSession{}, nil
}

type mockOAuthCompletionHandler struct{}

func (m *mockOAuthCompletionHandler) HandleCompletion(ctx context.Context, sessionID string, payload domainoauth.TokenPayload) error {
	return nil
}

// Test that mock implementations satisfy interfaces
func TestInterfaceImplementations(t *testing.T) {
	var _ oauth.OAuthProvider = (*mockOAuthProvider)(nil)
	var _ oauth.OAuthSessionStore = (*mockOAuthSessionStore)(nil)
	var _ oauth.OAuthCompletionHandler = (*mockOAuthCompletionHandler)(nil)
}
