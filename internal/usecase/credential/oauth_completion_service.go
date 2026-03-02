package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

type oAuthCompletionService struct {
	repo      Repository
	encryptor Encryptor
	now       func() time.Time
}

// NewOAuthCompletionService creates a new OAuth completion service
func NewOAuthCompletionService(repo Repository, encryptor Encryptor) OAuthCompletionService {
	return &oAuthCompletionService{
		repo:      repo,
		encryptor: encryptor,
		now:       time.Now,
	}
}

// CompleteOAuth persists OAuth tokens as an encrypted credential profile
func (s *oAuthCompletionService) CompleteOAuth(ctx context.Context, accountID string, tokenPayload domainoauth.TokenPayload) (domaincredential.Profile, error) {
	// Build credential profile from token payload
	profileData := map[string]interface{}{
		"access_token":  tokenPayload.AccessToken,
		"refresh_token": tokenPayload.RefreshToken,
		"token_type":    tokenPayload.TokenType,
		"scope":         tokenPayload.Scope,
	}

	rawProfile, err := json.Marshal(profileData)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("marshal credential profile: %w", err)
	}

	encProfile, err := s.encryptor.Encrypt(string(rawProfile))
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("encrypt profile: %w", err)
	}

	profile := domaincredential.Profile{
		Type:             "codex", // Codex provider type
		AccountID:        accountID,
		Enabled:          true,
		Email:            "", // Could be extracted from token if available
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    s.now(),
		EncryptedProfile: encProfile,
	}

	return s.repo.UpsertByTypeAccount(ctx, profile)
}
