package oauth

import (
	"context"

	"github.com/duchoang/llmpool/internal/usecase/credential"
)

// CodexRefresher implements credential.Refresher for Codex OAuth
type CodexRefresher struct {
	provider *CodexProvider
}

// NewCodexRefresher creates a new Codex refresher
func NewCodexRefresher(provider *CodexProvider) *CodexRefresher {
	return &CodexRefresher{provider: provider}
}

// Refresh implements credential.Refresher.Refresh
func (r *CodexRefresher) Refresh(ctx context.Context, refreshToken string) (credential.RefreshResult, error) {
	tokenPayload, err := r.provider.RefreshToken(ctx, refreshToken)
	if err != nil {
		return credential.RefreshResult{}, err
	}

	return credential.RefreshResult{
		AccessToken:  tokenPayload.AccessToken,
		RefreshToken: tokenPayload.RefreshToken,
		ExpiresAt:    tokenPayload.ExpiresAt,
	}, nil
}
