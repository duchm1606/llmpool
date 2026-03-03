package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/google/uuid"
)

// completionService implements OAuthCompletionService for persisting OAuth tokens
// as encrypted credential profiles. It uses upsert semantics to handle reauth
// scenarios without creating duplicate profiles.
type completionService struct {
	repo      Repository
	encryptor Encryptor
	now       func() time.Time
}

// NewCompletionService creates a new OAuth completion service
func NewCompletionService(repo Repository, encryptor Encryptor) OAuthCompletionService {
	return &completionService{
		repo:      repo,
		encryptor: encryptor,
		now:       time.Now,
	}
}

// CompleteOAuth processes successful OAuth token exchange and persists encrypted credential
// This method implements idempotent upsert - reauth for same account updates existing profile
func (s *completionService) CompleteOAuth(
	ctx context.Context,
	accountID string,
	tokenPayload domainoauth.TokenPayload,
) (domaincredential.Profile, error) {
	// Build the credential payload from OAuth tokens
	profileData := CredentialProfile{
		Type:         "codex",
		AccountID:    accountID,
		AccessToken:  tokenPayload.AccessToken,
		RefreshToken: tokenPayload.RefreshToken,
		Expired:      tokenPayload.ExpiresAt,
		Enabled:      boolPtr(true),
		LastRefresh:  s.now(),
	}

	// Marshal to JSON for encryption
	rawProfile, err := json.Marshal(profileData)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("marshal credential profile: %w", err)
	}

	// Encrypt the profile data
	encProfile, err := s.encryptor.Encrypt(string(rawProfile))
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("encrypt profile: %w", err)
	}

	// Build domain profile for upsert
	profile := domaincredential.Profile{
		ID:               uuid.NewString(),
		Type:             "codex",
		AccountID:        accountID,
		Enabled:          true,
		EncryptedProfile: encProfile,
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    s.now(),
	}

	// Use UpsertByTypeAccount to handle both create and reauth scenarios
	// This ensures we don't create duplicate profiles on reauth
	savedProfile, err := s.repo.UpsertByTypeAccount(ctx, profile)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("upsert credential profile: %w", err)
	}

	return savedProfile, nil
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
