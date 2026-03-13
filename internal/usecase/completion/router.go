package completion

import (
	"context"
	"fmt"
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
	return r.RouteWithFallback(ctx, model, SessionQuotaConsume, nil)
}

// RouteWithFallback attempts to route, excluding specified providers.
func (r *router) RouteWithFallback(
	ctx context.Context,
	model string,
	quotaMode SessionQuotaMode,
	excludeProviders []domainprovider.ProviderID,
) (*domainprovider.RoutingDecision, error) {
	return r.RouteWithHint(ctx, model, "", quotaMode, excludeProviders)
}

// RouteWithHint routes with an explicit provider hint.
// If providerHint is non-empty, it forces routing to that provider.
func (r *router) RouteWithHint(
	ctx context.Context,
	model string,
	providerHint string,
	quotaMode SessionQuotaMode,
	excludeProviders []domainprovider.ProviderID,
) (*domainprovider.RoutingDecision, error) {
	// Validate model ID format - reject prefixed models (should be parsed by handler)
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

	// If provider hint is specified, filter to only that provider
	if providerHint != "" {
		hintedProviderID := domainprovider.ProviderID(providerHint)

		// Check if the hinted provider supports this model
		found := false
		for _, pid := range providers {
			if pid == hintedProviderID {
				found = true
				break
			}
		}

		if !found {
			r.logger.Warn("hinted provider does not support model",
				zap.String("provider_hint", providerHint),
				zap.String("model", model),
			)
			return nil, domaincompletion.NewAPIError(
				400,
				domaincompletion.ErrorTypeInvalidRequest,
				fmt.Sprintf("provider '%s' does not support model '%s'", providerHint, model),
			)
		}

		// Only try the hinted provider
		providers = []domainprovider.ProviderID{hintedProviderID}

		r.logger.Info("routing with provider hint",
			zap.String("provider_hint", providerHint),
			zap.String("model", model),
		)
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
			if providerHint != "" {
				r.logger.Warn("hinted provider unavailable",
					zap.String("provider_hint", providerHint),
					zap.String("model", model),
					zap.Bool("healthy", health.Healthy),
					zap.Bool("rate_limited", health.RateLimited),
					zap.Time("rate_limit_reset", health.RateLimitReset),
					zap.Time("cooldown_until", health.CooldownUntil),
					zap.Int("consecutive_fails", health.ConsecutiveFails),
					zap.String("last_error", health.LastError),
				)
			}
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
				token, credentialMeta, err = extProvider.GetTokenWithInfoForQuotaMode(ctx, providerID, quotaMode)
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
			Initiator:           credentialMeta.Initiator,
		}

		r.logger.Info("routing request to provider",
			zap.String("model", model),
			zap.String("provider", string(providerID)),
			zap.String("credential_id", credentialMeta.CredentialID),
			zap.String("credential_type", credentialMeta.Type),
			zap.String("credential_account_id", credentialMeta.AccountID),
			zap.String("initiator", credentialMeta.Initiator),
			zap.String("base_url", provider.BaseURL),
			zap.String("provider_hint", providerHint),
		)

		return decision, nil
	}

	// No available provider
	if providerHint != "" {
		return nil, domaincompletion.NewAPIError(
			503,
			domaincompletion.ErrorTypeServiceUnavailable,
			fmt.Sprintf("hinted provider '%s' is not available for model '%s'", providerHint, model),
		)
	}
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
