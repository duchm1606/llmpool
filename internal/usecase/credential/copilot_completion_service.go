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

type copilotCompletionService struct {
	repo              Repository
	encryptor         Encryptor
	now               func() time.Time
	registryRefresher RegistryRefresher
}

// NewCopilotCompletionService creates a new Copilot OAuth completion service.
func NewCopilotCompletionService(repo Repository, encryptor Encryptor, registryRefresher RegistryRefresher) OAuthCompletionService {
	return &copilotCompletionService{
		repo:              repo,
		encryptor:         encryptor,
		now:               time.Now,
		registryRefresher: registryRefresher,
	}
}

// CompleteOAuth persists Copilot OAuth tokens as an encrypted credential profile.
// For Copilot:
// - AccessToken is the Copilot session token (short-lived, ~30 min)
// - RefreshToken is the GitHub access token (long-lived, used to get new session tokens)
func (s *copilotCompletionService) CompleteOAuth(ctx context.Context, accountID string, tokenPayload domainoauth.TokenPayload) (domaincredential.Profile, error) {
	// Validate accountID - must be non-empty for Copilot credentials
	validatedAccountID := strings.TrimSpace(accountID)
	if validatedAccountID == "" {
		return domaincredential.Profile{}, fmt.Errorf("invalid copilot credential: account_id is required")
	}

	// Validate required token fields
	if strings.TrimSpace(tokenPayload.AccessToken) == "" {
		return domaincredential.Profile{}, fmt.Errorf("invalid copilot credential: access_token is required")
	}
	if strings.TrimSpace(tokenPayload.RefreshToken) == "" {
		return domaincredential.Profile{}, fmt.Errorf("invalid copilot credential: refresh_token (github token) is required")
	}

	// Build credential profile from token payload
	// Store both the Copilot session token and GitHub token for refresh
	profileData := map[string]interface{}{
		"access_token":  tokenPayload.AccessToken,  // Copilot session token
		"refresh_token": tokenPayload.RefreshToken, // GitHub access token
		"token_type":    tokenPayload.TokenType,
		"scope":         tokenPayload.Scope,
		"expired":       tokenPayload.ExpiresAt,
	}

	rawProfile, err := json.Marshal(profileData)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("marshal credential profile: %w", err)
	}

	encProfile, iv, tag, err := s.encryptor.Encrypt(string(rawProfile))
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("encrypt profile: %w", err)
	}

	profile := domaincredential.Profile{
		ID:               uuid.NewString(),
		Type:             "copilot", // Copilot provider type
		AccountID:        validatedAccountID,
		Enabled:          true,
		Email:            strings.TrimSpace(tokenPayload.Email),
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    s.now(),
		EncryptedProfile: encProfile,
		EncryptedIV:      stringPtrOrNil(iv),
		EncryptedTag:     stringPtrOrNil(tag),
	}

	savedProfile, err := s.repo.UpsertByTypeAccount(ctx, profile)
	if err != nil {
		return domaincredential.Profile{}, err
	}

	if s.registryRefresher != nil {
		s.registryRefresher(ctx, savedProfile.Type, savedProfile.AccountID)
	}

	return savedProfile, nil
}
