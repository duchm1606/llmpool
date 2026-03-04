// Package provider provides infrastructure implementations for provider management.
package provider

import (
	"sync"
	"time"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
)

// RegistryConfig holds the configuration for the provider registry.
type RegistryConfig struct {
	Providers        []ProviderConfig
	ProviderPriority []string // Provider IDs in priority order
}

// ProviderConfig holds configuration for a single provider.
type ProviderConfig struct {
	ID       string
	Name     string
	Enabled  bool
	BaseURL  string
	Models   []string
	Headers  map[string]string
	AuthType string
	Timeout  time.Duration
}

// registry implements the ProviderRegistry interface.
type registry struct {
	mu         sync.RWMutex
	providers  map[domainprovider.ProviderID]*domainprovider.Provider
	priority   []domainprovider.ProviderID
	modelIndex map[string][]domainprovider.ProviderID // model -> providers (ordered)
}

// NewRegistry creates a new provider registry from configuration.
func NewRegistry(config RegistryConfig) usecasecompletion.ProviderRegistry {
	r := &registry{
		providers:  make(map[domainprovider.ProviderID]*domainprovider.Provider),
		priority:   make([]domainprovider.ProviderID, 0, len(config.ProviderPriority)),
		modelIndex: make(map[string][]domainprovider.ProviderID),
	}

	// Set up priority order
	for _, pid := range config.ProviderPriority {
		r.priority = append(r.priority, domainprovider.ProviderID(pid))
	}

	// If no priority specified, use default
	if len(r.priority) == 0 {
		r.priority = domainprovider.DefaultPriority
	}

	// Register providers
	for _, pc := range config.Providers {
		provider := &domainprovider.Provider{
			ID:       domainprovider.ProviderID(pc.ID),
			Name:     pc.Name,
			Enabled:  pc.Enabled,
			BaseURL:  pc.BaseURL,
			Models:   pc.Models,
			Headers:  pc.Headers,
			AuthType: domainprovider.AuthType(pc.AuthType),
			Timeout:  pc.Timeout,
		}
		r.providers[provider.ID] = provider
	}

	// Build model index
	r.rebuildModelIndex()

	return r
}

// rebuildModelIndex builds the model -> providers index.
func (r *registry) rebuildModelIndex() {
	r.modelIndex = make(map[string][]domainprovider.ProviderID)

	// Create a priority map for ordering
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
				pi, pj := priorityMap[providers[j]], priorityMap[providers[j-1]]
				// Lower priority number = higher priority
				if pi < pj {
					providers[j], providers[j-1] = providers[j-1], providers[j]
				}
			}
		}
	}
}

// GetProvider returns the provider configuration by ID.
func (r *registry) GetProvider(id domainprovider.ProviderID) (*domainprovider.Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[id]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent mutation
	copy := *p
	return &copy, true
}

// ListProviders returns all registered providers.
func (r *registry) ListProviders() []domainprovider.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]domainprovider.Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, *p)
	}
	return result
}

// GetProvidersForModel returns providers that support the given model, ordered by priority.
func (r *registry) GetProvidersForModel(model string) []domainprovider.ProviderID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers, ok := r.modelIndex[model]
	if !ok {
		return nil
	}

	// Return a copy
	result := make([]domainprovider.ProviderID, len(providers))
	copy(result, providers)
	return result
}

// GetAllModels returns a deduplicated list of all available models.
func (r *registry) GetAllModels() []domaincompletion.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().Unix()
	models := make([]domaincompletion.Model, 0, len(r.modelIndex))

	for modelID := range r.modelIndex {
		models = append(models, domaincompletion.Model{
			ID:      modelID,
			Object:  "model",
			Created: now,
			OwnedBy: "llmpool",
		})
	}

	return models
}
