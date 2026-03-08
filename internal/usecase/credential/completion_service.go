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

// completionService implements OAuthCompletionService for persisting OAuth tokens
// as encrypted credential profiles. It uses upsert semantics to handle reauth
// scenarios without creating duplicate profiles.
type completionService struct {
	repo              Repository
	encryptor         Encryptor
	now               func() time.Time
	registryRefresher RegistryRefresher
}

// NewCompletionService creates a new OAuth completion service
func NewCompletionService(repo Repository, encryptor Encryptor, registryRefresher RegistryRefresher) OAuthCompletionService {
	return &completionService{
		repo:              repo,
		encryptor:         encryptor,
		now:               time.Now,
		registryRefresher: registryRefresher,
	}
}

// CompleteOAuth processes successful OAuth token exchange and persists encrypted credential
// This method implements idempotent upsert - reauth for same account updates existing profile
func (s *completionService) CompleteOAuth(
	ctx context.Context,
	accountID string,
	tokenPayload domainoauth.TokenPayload,
) (domaincredential.Profile, error) {
	now := s.now()

	validatedAccountID, err := validateOAuthCredential(accountID, tokenPayload, now)
	if err != nil {
		return domaincredential.Profile{}, err
	}

	// Build the credential payload from OAuth tokens
	profileData := CredentialProfile{
		Type:         "codex",
		AccountID:    validatedAccountID,
		AccessToken:  tokenPayload.AccessToken,
		RefreshToken: tokenPayload.RefreshToken,
		Expired:      tokenPayload.ExpiresAt,
		Enabled:      boolPtr(true),
		LastRefresh:  now,
	}

	// Marshal to JSON for encryption
	rawProfile, err := json.Marshal(profileData)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("marshal credential profile: %w", err)
	}

	// Encrypt the profile data
	encProfile, iv, tag, err := s.encryptor.Encrypt(string(rawProfile))
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("encrypt profile: %w", err)
	}

	// Build domain profile for upsert
	profile := domaincredential.Profile{
		ID:               uuid.NewString(),
		Type:             "codex",
		AccountID:        validatedAccountID,
		Enabled:          true,
		Email:            strings.TrimSpace(tokenPayload.Email),
		EncryptedProfile: encProfile,
		EncryptedIV:      stringPtrOrNil(iv),
		EncryptedTag:     stringPtrOrNil(tag),
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    now,
	}

	// Use UpsertByTypeAccount to handle both create and reauth scenarios
	// This ensures we don't create duplicate profiles on reauth
	savedProfile, err := s.repo.UpsertByTypeAccount(ctx, profile)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("upsert credential profile: %w", err)
	}

	if s.registryRefresher != nil {
		s.registryRefresher(ctx, savedProfile.Type, savedProfile.AccountID)
	}

	return savedProfile, nil
}

func validateOAuthCredential(accountID string, tokenPayload domainoauth.TokenPayload, now time.Time) (string, error) {
	validatedAccountID := strings.TrimSpace(accountID)
	if validatedAccountID == "" {
		return "", fmt.Errorf("invalid oauth credential: missing account_id")
	}

	if strings.TrimSpace(tokenPayload.AccessToken) == "" {
		return "", fmt.Errorf("invalid oauth credential: missing access_token")
	}

	if strings.TrimSpace(tokenPayload.RefreshToken) == "" {
		return "", fmt.Errorf("invalid oauth credential: missing refresh_token")
	}

	if tokenPayload.ExpiresAt.IsZero() {
		return "", fmt.Errorf("invalid oauth credential: missing expires_at")
	}

	if !tokenPayload.ExpiresAt.After(now) {
		return "", fmt.Errorf("invalid oauth credential: expires_at is not in the future")
	}

	return validatedAccountID, nil
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
