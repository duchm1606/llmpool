package completion

import (
	"context"
	"strings"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	"go.uber.org/zap"
)

// router implements the Router interface.
type router struct {
	registry      ProviderRegistry
	healthTracker ProviderHealthTracker
	credProvider  CredentialProvider
	userMapping   PerUserMappingProvider // May be nil (feature deferred)
	logger        *zap.Logger
}

// CredentialInfo holds information about the credential used for a request.
// This is attached to RoutingDecision for logging purposes.
type CredentialInfo struct {
	ProfileID    string
	AccountID    string
	ProviderType string
}

// RouterConfig holds configuration for the router.
type RouterConfig struct {
	// MaxFallbackAttempts limits how many providers to try before giving up.
	MaxFallbackAttempts int
}

// NewRouter creates a new router instance.
func NewRouter(
	registry ProviderRegistry,
	healthTracker ProviderHealthTracker,
	credProvider CredentialProvider,
	userMapping PerUserMappingProvider, // May be nil
	logger *zap.Logger,
) Router {
	return &router{
		registry:      registry,
		healthTracker: healthTracker,
		credProvider:  credProvider,
		userMapping:   userMapping,
		logger:        logger,
	}
}

// Route selects a provider for the given model.
func (r *router) Route(ctx context.Context, model string) (*domainprovider.RoutingDecision, error) {
	return r.RouteWithFallback(ctx, model, nil)
}

// RouteWithFallback attempts to route, excluding specified providers.
func (r *router) RouteWithFallback(
	ctx context.Context,
	model string,
	excludeProviders []domainprovider.ProviderID,
) (*domainprovider.RoutingDecision, error) {
	// Validate model ID format - reject prefixed models
	if strings.Contains(model, "/") {
		return nil, domaincompletion.ErrInvalidModelID(model)
	}

	// Get providers that support this model, ordered by priority
	providers := r.registry.GetProvidersForModel(model)
	if len(providers) == 0 {
		return nil, domaincompletion.ErrModelNotFound(model)
	}

	// Build exclusion set
	excludeSet := make(map[domainprovider.ProviderID]bool)
	for _, p := range excludeProviders {
		excludeSet[p] = true
	}

	// TODO: Check per-user mapping if available (feature deferred)
	// if r.userMapping != nil {
	//     userID := getUserIDFromContext(ctx)
	//     if userProviders := r.userMapping.GetUserPreferredProviders(ctx, userID, model); userProviders != nil {
	//         providers = userProviders
	//     }
	// }

	// Find first available provider
	for _, providerID := range providers {
		// Skip excluded providers
		if excludeSet[providerID] {
			continue
		}

		// Check provider health
		health := r.healthTracker.GetHealth(providerID)
		if !health.IsAvailable() {
			r.logger.Debug("provider unavailable, skipping",
				zap.String("provider", string(providerID)),
				zap.Bool("healthy", health.Healthy),
				zap.Bool("rate_limited", health.RateLimited),
			)
			continue
		}

		// Get provider configuration
		provider, ok := r.registry.GetProvider(providerID)
		if !ok || !provider.Enabled {
			continue
		}

		// Get authentication token if needed
		var token string
		var credentialMeta CredentialMetadata
		if provider.AuthType == domainprovider.AuthTypeBearerPool || provider.AuthType == domainprovider.AuthTypeOAuth {
			var err error
			// Try extended provider first for credential tracking
			if extProvider, ok := r.credProvider.(ExtendedCredentialProvider); ok {
				token, credentialMeta, err = extProvider.GetTokenWithInfo(ctx, providerID)
			} else {
				token, err = r.credProvider.GetToken(ctx, providerID)
			}
			if err != nil {
				r.logger.Warn("failed to get token for provider, skipping",
					zap.String("provider", string(providerID)),
					zap.Error(err),
				)
				continue
			}
		}

		// Build routing decision
		decision := &domainprovider.RoutingDecision{
			Model:               model,
			ProviderID:          providerID,
			BaseURL:             provider.BaseURL,
			Headers:             copyHeaders(provider.Headers),
			Token:               token,
			CredentialID:        credentialMeta.CredentialID,
			CredentialType:      credentialMeta.Type,
			CredentialAccountID: credentialMeta.AccountID,
		}

		r.logger.Info("routing request to provider",
			zap.String("model", model),
			zap.String("provider", string(providerID)),
			zap.String("credential_id", credentialMeta.CredentialID),
			zap.String("credential_type", credentialMeta.Type),
			zap.String("credential_account_id", credentialMeta.AccountID),
			zap.String("base_url", provider.BaseURL),
		)

		return decision, nil
	}

	// No available provider
	return nil, domaincompletion.ErrNoAvailableProvider(model)
}

// copyHeaders creates a copy of the headers map.
func copyHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	copy := make(map[string]string, len(headers))
	for k, v := range headers {
		copy[k] = v
	}
	return copy
}
