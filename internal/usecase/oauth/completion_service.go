package oauth

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

// CompletionService handles OAuth completion by persisting encrypted credentials
type CompletionService struct {
	credentialRepo CredentialRepository
	encryptor      Encryptor
	now            func() time.Time
}

// CredentialRepository defines the contract for credential storage
type CredentialRepository interface {
	UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
}

// Encryptor defines the contract for encrypting sensitive data
type Encryptor interface {
	Encrypt(plain string) (string, error)
	Decrypt(cipher string) (string, error)
}

// NewCompletionService creates a new CompletionService
func NewCompletionService(repo CredentialRepository, encryptor Encryptor) *CompletionService {
	return &CompletionService{
		credentialRepo: repo,
		encryptor:      encryptor,
		now:            time.Now,
	}
}

// HandleCompletion processes successful OAuth token exchange and stores encrypted credentials
func (s *CompletionService) HandleCompletion(ctx context.Context, sessionID string, tokenPayload domainoauth.TokenPayload) error {
	// 1. Create credential profile from token payload
	_ = sessionID

	accountID := strings.TrimSpace(tokenPayload.AccountID)
	if accountID == "" {
		return fmt.Errorf("missing account identifier")
	}

	// Build profile data structure
	profileData := map[string]interface{}{
		"access_token":  tokenPayload.AccessToken,
		"refresh_token": tokenPayload.RefreshToken,
		"expires_at":    tokenPayload.ExpiresAt.Format(time.RFC3339),
		"token_type":    tokenPayload.TokenType,
		"scope":         tokenPayload.Scope,
	}

	profileJSON, err := json.Marshal(profileData)
	if err != nil {
		return fmt.Errorf("marshal credential profile: %w", err)
	}

	// 2. Encrypt the profile
	encryptedProfile, err := s.encryptor.Encrypt(string(profileJSON))
	if err != nil {
		return fmt.Errorf("encrypt profile json: %w", err)
	}

	// 3. Build credential profile entity
	profile := domaincredential.Profile{
		ID:               uuid.NewString(),
		Type:             "codex",
		AccountID:        accountID,
		Enabled:          true,
		Email:            "", // OAuth may not provide email
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    s.now(),
		EncryptedProfile: encryptedProfile,
	}

	// 4. Upsert to database using credentialRepo.UpsertByTypeAccount
	_, err = s.credentialRepo.UpsertByTypeAccount(ctx, profile)
	if err != nil {
		return fmt.Errorf("upsert credential profile: %w", err)
	}

	return nil
}
