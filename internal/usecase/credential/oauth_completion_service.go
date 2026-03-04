package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/google/uuid"
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
	// Validate accountID - must be non-empty
	validatedAccountID := strings.TrimSpace(accountID)
	if validatedAccountID == "" {
		return domaincredential.Profile{}, fmt.Errorf("invalid oauth credential: account_id is required")
	}

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
		ID:               uuid.NewString(),
		Type:             "codex", // Codex provider type
		AccountID:        validatedAccountID,
		Enabled:          true,
		Email:            strings.TrimSpace(tokenPayload.Email),
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    s.now(),
		EncryptedProfile: encProfile,
	}

	return s.repo.UpsertByTypeAccount(ctx, profile)
}
