package provider

import (
	"context"
	"fmt"
	"sync"

	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"go.uber.org/zap"
)

// Verify interface compliance
var _ usecasecompletion.ExtendedCredentialProvider = (*credentialProvider)(nil)

// CredentialProviderAdapter adapts the existing credential system for completion routing.
// This is a placeholder that will be integrated with the existing credential pool.
type CredentialProviderAdapter interface {
	usecasecompletion.CredentialProvider
}

// TokenFetcher is the interface for fetching tokens from the credential pool.
// This should be implemented by the existing credential system.
type TokenFetcher interface {
	// GetNextToken returns the next available token for the provider using load balancing.
	GetNextToken(ctx context.Context, providerType string) (string, error)
}

// ExtendedTokenFetcher extends TokenFetcher with credential tracking.
type ExtendedTokenFetcher interface {
	TokenFetcher
	// GetNextTokenWithInfo returns token and metadata for tracking.
	GetNextTokenWithInfo(ctx context.Context, providerType string) (token string, meta usecasecompletion.CredentialMetadata, err error)
}

// credentialProvider implements CredentialProvider using the existing credential system.
type credentialProvider struct {
	fetcher TokenFetcher
	logger  *zap.Logger
	mu      sync.RWMutex
	// Cache for static API keys (providers that don't use pool)
	staticKeys map[domainprovider.ProviderID]string
}

// NewCredentialProvider creates a new credential provider.
func NewCredentialProvider(fetcher TokenFetcher, logger *zap.Logger) usecasecompletion.CredentialProvider {
	return &credentialProvider{
		fetcher:    fetcher,
		logger:     logger,
		staticKeys: make(map[domainprovider.ProviderID]string),
	}
}

// SetStaticKey sets a static API key for a provider.
// Use this for providers that don't use the credential pool.
func (cp *credentialProvider) SetStaticKey(providerID domainprovider.ProviderID, key string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.staticKeys[providerID] = key
}

// GetToken returns a valid token for the given provider.
func (cp *credentialProvider) GetToken(
	ctx context.Context,
	providerID domainprovider.ProviderID,
) (string, error) {
	token, _, err := cp.GetTokenWithInfo(ctx, providerID)
	return token, err
}

// GetTokenWithInfo returns a token along with credential ID for logging.
func (cp *credentialProvider) GetTokenWithInfo(
	ctx context.Context,
	providerID domainprovider.ProviderID,
) (token string, meta usecasecompletion.CredentialMetadata, err error) {
	meta.Type = string(providerID)
	// Check for static key first
	cp.mu.RLock()
	if key, ok := cp.staticKeys[providerID]; ok {
		cp.mu.RUnlock()
		meta.CredentialID = "static-key"
		return key, meta, nil
	}
	cp.mu.RUnlock()

	// Map provider ID to credential type
	providerType := mapProviderToCredentialType(providerID)
	if providerType == "" {
		return "", meta, fmt.Errorf("unknown provider: %s", providerID)
	}

	// Fetch token from pool
	if cp.fetcher == nil {
		return "", meta, fmt.Errorf("no token fetcher configured for provider: %s", providerID)
	}

	// Try extended fetcher first
	if extFetcher, ok := cp.fetcher.(ExtendedTokenFetcher); ok {
		token, meta, err = extFetcher.GetNextTokenWithInfo(ctx, providerType)
		if meta.Type == "" {
			meta.Type = providerType
		}
		if err != nil {
			cp.logger.Warn("failed to get token from pool",
				zap.String("provider", string(providerID)),
				zap.Error(err),
			)
			return "", meta, err
		}
		return token, meta, nil
	}

	// Fallback to basic fetcher
	token, err = cp.fetcher.GetNextToken(ctx, providerType)
	if err != nil {
		cp.logger.Warn("failed to get token from pool",
			zap.String("provider", string(providerID)),
			zap.Error(err),
		)
		return "", meta, err
	}

	return token, meta, nil
}

// mapProviderToCredentialType maps provider IDs to credential types used by the pool.
func mapProviderToCredentialType(providerID domainprovider.ProviderID) string {
	switch providerID {
	case domainprovider.ProviderCodex:
		return "codex"
	case domainprovider.ProviderCopilot:
		return "copilot"
	case domainprovider.ProviderOpenAI:
		return "openai"
	case domainprovider.ProviderAnthropic:
		return "anthropic"
	default:
		return string(providerID)
	}
}

// NoopCredentialProvider is a credential provider that always returns empty tokens.
// Use this for testing or when no authentication is needed.
type NoopCredentialProvider struct{}

// NewNoopCredentialProvider creates a new noop credential provider.
func NewNoopCredentialProvider() usecasecompletion.CredentialProvider {
	return &NoopCredentialProvider{}
}

// GetToken always returns an empty token.
func (n *NoopCredentialProvider) GetToken(ctx context.Context, providerID domainprovider.ProviderID) (string, error) {
	return "", nil
}
