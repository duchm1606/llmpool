package provider

import (
	"context"
	"sync"
	"time"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"go.uber.org/zap"
)

// DynamicRegistryConfig holds configuration for the dynamic registry.
type DynamicRegistryConfig struct {
	// ProviderPriority defines the priority order for providers
	ProviderPriority []string
	// ProviderConfigs holds static configuration per provider
	ProviderConfigs map[string]ProviderConfig
	// RefreshInterval is how often to refresh the model list from credentials
	RefreshInterval time.Duration
}

// dynamicRegistry implements ProviderRegistry with dynamic model loading.
// Instead of hardcoding models in config, it loads models based on which
// credential types have enabled credentials.
type dynamicRegistry struct {
	mu sync.RWMutex

	// Static configuration
	priority        []domainprovider.ProviderID
	providerConfigs map[domainprovider.ProviderID]ProviderConfig

	// Dynamic state - updated based on credentials
	providers  map[domainprovider.ProviderID]*domainprovider.Provider
	modelIndex map[string][]domainprovider.ProviderID

	// Dependencies
	tokenFetcher *PooledTokenFetcher
	logger       *zap.Logger
}

// NewDynamicRegistry creates a registry that dynamically loads models based on credentials.
func NewDynamicRegistry(
	config DynamicRegistryConfig,
	tokenFetcher *PooledTokenFetcher,
	logger *zap.Logger,
) usecasecompletion.ProviderRegistry {
	r := &dynamicRegistry{
		priority:        make([]domainprovider.ProviderID, 0, len(config.ProviderPriority)),
		providerConfigs: make(map[domainprovider.ProviderID]ProviderConfig),
		providers:       make(map[domainprovider.ProviderID]*domainprovider.Provider),
		modelIndex:      make(map[string][]domainprovider.ProviderID),
		tokenFetcher:    tokenFetcher,
		logger:          logger,
	}

	// Set priority order
	for _, pid := range config.ProviderPriority {
		r.priority = append(r.priority, domainprovider.ProviderID(pid))
	}

	if len(r.priority) == 0 {
		r.priority = domainprovider.DefaultPriority
	}

	// Store provider configs
	for id, pc := range config.ProviderConfigs {
		r.providerConfigs[domainprovider.ProviderID(id)] = pc
	}

	// Initial load
	r.refreshProviders(context.Background())

	return r
}

// refreshProviders updates the available providers and models based on credentials.
func (r *dynamicRegistry) refreshProviders(ctx context.Context) {
	// Get available provider types from credentials
	availableTypes, err := r.tokenFetcher.GetAvailableProviderTypes(ctx)
	if err != nil {
		r.logger.Error("failed to get available provider types", zap.Error(err))
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing
	r.providers = make(map[domainprovider.ProviderID]*domainprovider.Provider)
	r.modelIndex = make(map[string][]domainprovider.ProviderID)

	// Build providers from available credential types
	for _, providerType := range availableTypes {
		providerID := domainprovider.ProviderID(providerType)

		if cfg, ok := r.providerConfigs[providerID]; ok && !cfg.Enabled {
			r.logger.Info("provider disabled by config, skipping",
				zap.String("provider_id", string(providerID)),
			)
			continue
		}

		// Get models for this provider type from our models.go definitions
		models := GetModelsForProvider(providerType)
		if len(models) == 0 {
			r.logger.Warn("no models defined for provider type",
				zap.String("provider_type", providerType),
			)
			continue
		}

		// Get base config from providerConfigs if available, otherwise use defaults
		var baseURL string
		var authType domainprovider.AuthType
		var timeout time.Duration
		var headers map[string]string
		var name string

		if cfg, ok := r.providerConfigs[providerID]; ok {
			baseURL = cfg.BaseURL
			authType = domainprovider.AuthType(cfg.AuthType)
			timeout = cfg.Timeout
			headers = cfg.Headers
			name = cfg.Name
		} else {
			// Use defaults based on provider type
			baseURL, authType, timeout, name = getProviderDefaults(providerType)
		}

		// Create provider
		provider := &domainprovider.Provider{
			ID:       providerID,
			Name:     name,
			Enabled:  true,
			BaseURL:  baseURL,
			Models:   models,
			Headers:  headers,
			AuthType: authType,
			Timeout:  timeout,
		}

		r.providers[providerID] = provider

		r.logger.Info("provider registered with dynamic models",
			zap.String("provider_id", string(providerID)),
			zap.Int("model_count", len(models)),
			zap.Strings("models", models),
		)
	}

	// Build model index
	r.rebuildModelIndexLocked()
}

// getProviderDefaults returns default configuration for known provider types.
func getProviderDefaults(providerType string) (baseURL string, authType domainprovider.AuthType, timeout time.Duration, name string) {
	switch providerType {
	case "codex":
		return "https://chatgpt.com/backend-api/codex", domainprovider.AuthTypeBearerPool, 120 * time.Second, "OpenAI Codex"
	case "copilot":
		return "https://api.githubcopilot.com", domainprovider.AuthTypeBearerPool, 120 * time.Second, "GitHub Copilot"
	case "openai":
		return "https://api.openai.com", domainprovider.AuthTypeAPIKey, 120 * time.Second, "OpenAI Direct"
	case "anthropic":
		return "https://api.anthropic.com", domainprovider.AuthTypeAPIKey, 120 * time.Second, "Anthropic"
	default:
		return "", domainprovider.AuthTypeNone, 60 * time.Second, providerType
	}
}

// rebuildModelIndexLocked rebuilds the model index. Must be called with lock held.
func (r *dynamicRegistry) rebuildModelIndexLocked() {
	r.modelIndex = make(map[string][]domainprovider.ProviderID)

	// Create priority map for ordering
	priorityMap := make(map[domainprovider.ProviderID]int)
	for i, pid := range r.priority {
		priorityMap[pid] = i
	}

	// Index all models from all providers
	for _, provider := range r.providers {
		if !provider.Enabled {
			continue
		}
		for _, model := range provider.Models {
			r.modelIndex[model] = append(r.modelIndex[model], provider.ID)
		}
	}

	// Sort each model's providers by priority
	for model := range r.modelIndex {
		providers := r.modelIndex[model]
		// Simple insertion sort (small lists)
		for i := 1; i < len(providers); i++ {
			for j := i; j > 0; j-- {
				pi := priorityMap[providers[j]]
				pj := priorityMap[providers[j-1]]
				// Lower priority number = higher priority
				// Providers not in priority list get max int
				if pi == 0 && providers[j] != r.priority[0] {
					pi = 999
				}
				if pj == 0 && providers[j-1] != r.priority[0] {
					pj = 999
				}
				if pi < pj {
					providers[j], providers[j-1] = providers[j-1], providers[j]
				}
			}
		}
	}

	r.logger.Debug("model index rebuilt",
		zap.Int("total_models", len(r.modelIndex)),
	)
}

// GetProvider returns the provider configuration by ID.
func (r *dynamicRegistry) GetProvider(id domainprovider.ProviderID) (*domainprovider.Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[id]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent mutation
	copyP := *p
	return &copyP, true
}

// ListProviders returns all registered providers.
func (r *dynamicRegistry) ListProviders() []domainprovider.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]domainprovider.Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, *p)
	}
	return result
}

// GetProvidersForModel returns providers that support the given model, ordered by priority.
func (r *dynamicRegistry) GetProvidersForModel(model string) []domainprovider.ProviderID {
	// Resolve alias first
	canonicalModel := ResolveModelAlias(model)

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try canonical model first
	if providers, ok := r.modelIndex[canonicalModel]; ok {
		result := make([]domainprovider.ProviderID, len(providers))
		copy(result, providers)
		return result
	}

	// Try original model name
	if providers, ok := r.modelIndex[model]; ok {
		result := make([]domainprovider.ProviderID, len(providers))
		copy(result, providers)
		return result
	}

	return nil
}

// GetAllModels returns a deduplicated list of all available models.
// For models supported by multiple providers, it also returns provider-prefixed versions
// to allow explicit provider selection (e.g., "copilot/gpt-5").
func (r *dynamicRegistry) GetAllModels() []domaincompletion.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().Unix()
	// Estimate capacity: base models + prefixed versions for multi-provider models
	models := make([]domaincompletion.Model, 0, len(r.modelIndex)*2)

	for modelID, providers := range r.modelIndex {
		// Determine owner based on first provider (priority order)
		ownedBy := "llmpool"
		if len(providers) > 0 {
			ownedBy = string(providers[0])
		}

		// Add base model (routes by priority)
		models = append(models, domaincompletion.Model{
			ID:      modelID,
			Object:  "model",
			Created: now,
			OwnedBy: ownedBy,
		})

		// If model is supported by multiple providers, add provider-prefixed versions
		// This allows users to force routing: "copilot/gpt-5" -> force copilot
		if len(providers) > 1 {
			for _, providerID := range providers {
				prefixedID := string(providerID) + "/" + modelID
				models = append(models, domaincompletion.Model{
					ID:      prefixedID,
					Object:  "model",
					Created: now,
					OwnedBy: string(providerID),
				})
			}
		}
	}

	return models
}

// Refresh forces a refresh of the provider/model list.
func (r *dynamicRegistry) Refresh(ctx context.Context) {
	r.refreshProviders(ctx)
}
