package oauth

import (
	"context"

	"github.com/duchoang/llmpool/internal/usecase/credential"
)

// CopilotRefresher implements credential.Refresher for Copilot OAuth.
// It refreshes the Copilot session token using the stored GitHub access token.
type CopilotRefresher struct {
	provider *CopilotProvider
}

// NewCopilotRefresher creates a new Copilot refresher.
func NewCopilotRefresher(provider *CopilotProvider) *CopilotRefresher {
	return &CopilotRefresher{provider: provider}
}

// Refresh implements credential.Refresher.Refresh.
// For Copilot, the refreshToken parameter is actually the GitHub access token,
// which is used to obtain a new Copilot session token.
func (r *CopilotRefresher) Refresh(ctx context.Context, githubToken string) (credential.RefreshResult, error) {
	tokenPayload, err := r.provider.RefreshToken(ctx, githubToken)
	if err != nil {
		return credential.RefreshResult{}, err
	}

	return credential.RefreshResult{
		AccessToken:  tokenPayload.AccessToken,  // Copilot session token
		RefreshToken: tokenPayload.RefreshToken, // GitHub token (unchanged)
		ExpiresAt:    tokenPayload.ExpiresAt,
	}, nil
}
