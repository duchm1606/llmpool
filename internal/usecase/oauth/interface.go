package oauth

import (
	"context"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

type OAuthProvider interface {
	// BuildAuthURL generates the OAuth authorization URL with PKCE
	BuildAuthURL(ctx context.Context, state string, verifier string) (domainoauth.AuthorizationURL, error)

	// ExchangeCode exchanges authorization code for tokens
	ExchangeCode(ctx context.Context, code string, verifier string) (domainoauth.TokenPayload, error)

	// RefreshToken refreshes an access token using refresh token
	RefreshToken(ctx context.Context, refreshToken string) (domainoauth.TokenPayload, error)

	// StartDeviceFlow initiates device authorization flow
	StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error)

	// PollDevice polls for device authorization completion
	PollDevice(ctx context.Context, deviceCode string) (domainoauth.TokenPayload, error)
}

// OAuthSessionStore defines the contract for OAuth session storage (Redis)
// Infrastructure layer implements this interface
type OAuthSessionStore interface {
	// CreatePending creates a new pending OAuth session
	CreatePending(ctx context.Context, session domainoauth.OAuthSession) error

	// GetStatus retrieves current session status
	GetStatus(ctx context.Context, sessionID string) (domainoauth.OAuthSession, error)

	// MarkComplete marks session as successfully completed with account ID
	MarkComplete(ctx context.Context, sessionID string, accountID string) error

	// MarkError marks session as failed with error details
	MarkError(ctx context.Context, sessionID string, errorCode string, errorMessage string) error

	// Consume retrieves and deletes a session (one-time use)
	Consume(ctx context.Context, sessionID string) (domainoauth.OAuthSession, error)
}

// OAuthCompletionHandler defines the contract for handling OAuth completion
// Usecase layer implements this to update credentials
type OAuthCompletionHandler interface {
	// HandleCompletion processes successful OAuth token exchange
	HandleCompletion(ctx context.Context, sessionID string, payload domainoauth.TokenPayload) error
}
