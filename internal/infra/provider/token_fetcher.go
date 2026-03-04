package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"go.uber.org/zap"
)

// Verify interface compliance
var _ ExtendedTokenFetcher = (*PooledTokenFetcher)(nil)

// CredentialSelection represents the selected credential for a request.
// This is used for logging which credential was selected.
type CredentialSelection struct {
	ProfileID   string
	AccountID   string
	ProfileType string
	Email       string // Safe to log (no secrets)
	SelectedAt  time.Time
}

// CredentialRepository is the interface for accessing credential profiles.
// This is implemented by the postgres repository.
type CredentialRepository interface {
	ListEnabled(ctx context.Context) ([]domaincredential.Profile, error)
}

// CredentialDecryptor decrypts credential profile data.
type CredentialDecryptor interface {
	Decrypt(cipher string) (string, error)
}

// DecryptedCredential holds the decrypted credential data.
type DecryptedCredential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccountID    string    `json:"account_id"`
	Email        string    `json:"email"`
	Expired      time.Time `json:"expired"`
	Type         string    `json:"type"`
}

// PooledTokenFetcher implements TokenFetcher using the credential repository.
// It provides round-robin load balancing across enabled credentials.
type PooledTokenFetcher struct {
	repo      CredentialRepository
	decryptor CredentialDecryptor
	logger    *zap.Logger

	// Round-robin counters per provider type
	mu       sync.RWMutex
	counters map[string]*uint64

	// Selection callback for logging
	onSelection func(selection CredentialSelection)
}

// PooledTokenFetcherConfig configures the pooled token fetcher.
type PooledTokenFetcherConfig struct {
	// OnSelection is called whenever a credential is selected for use.
	// Use this for logging/auditing which credentials are being used.
	OnSelection func(selection CredentialSelection)
}

// NewPooledTokenFetcher creates a new pooled token fetcher.
func NewPooledTokenFetcher(
	repo CredentialRepository,
	decryptor CredentialDecryptor,
	logger *zap.Logger,
	config PooledTokenFetcherConfig,
) *PooledTokenFetcher {
	return &PooledTokenFetcher{
		repo:        repo,
		decryptor:   decryptor,
		logger:      logger,
		counters:    make(map[string]*uint64),
		onSelection: config.OnSelection,
	}
}

// GetNextToken returns the next available token for the provider using round-robin.
// It logs detailed selection information including profile ID, account ID, and email.
func (f *PooledTokenFetcher) GetNextToken(ctx context.Context, providerType string) (string, error) {
	token, _, err := f.GetNextTokenWithInfo(ctx, providerType)
	return token, err
}

// GetNextTokenWithInfo returns the next available token along with credential ID.
// This implements ExtendedTokenFetcher for detailed credential tracking.
func (f *PooledTokenFetcher) GetNextTokenWithInfo(
	ctx context.Context,
	providerType string,
) (string, usecasecompletion.CredentialMetadata, error) {
	meta := usecasecompletion.CredentialMetadata{Type: providerType}
	// Get all enabled credentials
	profiles, err := f.repo.ListEnabled(ctx)
	if err != nil {
		f.logger.Error("failed to list enabled credentials",
			zap.String("provider_type", providerType),
			zap.Error(err),
		)
		return "", meta, fmt.Errorf("list enabled credentials: %w", err)
	}

	// Filter by provider type
	var matching []domaincredential.Profile
	for _, p := range profiles {
		if p.Type == providerType {
			matching = append(matching, p)
		}
	}

	if len(matching) == 0 {
		f.logger.Warn("no enabled credentials found for provider",
			zap.String("provider_type", providerType),
		)
		return "", meta, fmt.Errorf("no enabled credentials for provider: %s", providerType)
	}

	// Get or create counter for this provider type
	f.mu.Lock()
	counter, ok := f.counters[providerType]
	if !ok {
		var c uint64
		counter = &c
		f.counters[providerType] = counter
	}
	f.mu.Unlock()

	// Round-robin selection
	idx := atomic.AddUint64(counter, 1) % uint64(len(matching))
	selected := matching[idx]

	// Decrypt the credential
	decrypted, err := f.decryptProfile(selected)
	if err != nil {
		f.logger.Error("failed to decrypt credential",
			zap.String("profile_id", selected.ID),
			zap.String("account_id", selected.AccountID),
			zap.Error(err),
		)
		return "", meta, fmt.Errorf("decrypt credential: %w", err)
	}

	// Check expiration
	if time.Now().After(decrypted.Expired) {
		f.logger.Warn("selected credential is expired",
			zap.String("profile_id", selected.ID),
			zap.String("account_id", selected.AccountID),
			zap.Time("expired_at", decrypted.Expired),
		)
		// Still return it - the refresh worker should handle this
		// The upstream will reject it and we'll fallback
	}

	// Log the selection
	selection := CredentialSelection{
		ProfileID:   selected.ID,
		AccountID:   selected.AccountID,
		ProfileType: selected.Type,
		Email:       maskEmail(decrypted.Email),
		SelectedAt:  time.Now(),
	}

	f.logger.Info("credential selected for request",
		zap.String("provider_type", providerType),
		zap.String("profile_id", selection.ProfileID),
		zap.String("account_id", selection.AccountID),
		zap.String("email", selection.Email),
		zap.Int("pool_size", len(matching)),
		zap.Uint64("round_robin_index", idx),
	)

	// Call selection callback if configured
	if f.onSelection != nil {
		f.onSelection(selection)
	}

	meta.CredentialID = selected.ID
	meta.AccountID = selected.AccountID

	return decrypted.AccessToken, meta, nil
}

// GetCredentialCount returns the number of enabled credentials for a provider type.
func (f *PooledTokenFetcher) GetCredentialCount(ctx context.Context, providerType string) (int, error) {
	profiles, err := f.repo.ListEnabled(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, p := range profiles {
		if p.Type == providerType {
			count++
		}
	}
	return count, nil
}

// GetAvailableProviderTypes returns all provider types that have enabled credentials.
func (f *PooledTokenFetcher) GetAvailableProviderTypes(ctx context.Context) ([]string, error) {
	profiles, err := f.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}

	typeSet := make(map[string]bool)
	for _, p := range profiles {
		typeSet[p.Type] = true
	}

	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	return types, nil
}

// decryptProfile decrypts a credential profile and extracts the access token.
func (f *PooledTokenFetcher) decryptProfile(profile domaincredential.Profile) (*DecryptedCredential, error) {
	decrypted, err := f.decryptor.Decrypt(profile.EncryptedProfile)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	var cred DecryptedCredential
	if err := json.Unmarshal([]byte(decrypted), &cred); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &cred, nil
}

// maskEmail masks an email address for safe logging.
// e.g., "user@example.com" -> "u***@example.com"
func maskEmail(email string) string {
	if email == "" {
		return ""
	}

	atIdx := -1
	for i, c := range email {
		if c == '@' {
			atIdx = i
			break
		}
	}

	if atIdx <= 0 {
		return "***"
	}

	// Keep first character, mask the rest before @
	masked := string(email[0])
	if atIdx > 1 {
		masked += "***"
	}
	if atIdx < len(email) {
		masked += email[atIdx:]
	}

	return masked
}
