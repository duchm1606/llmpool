package completion

import (
	"context"
	"testing"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	"go.uber.org/zap"
)

// mockRegistry implements ProviderRegistry for testing.
type mockRegistry struct {
	providers  map[domainprovider.ProviderID]*domainprovider.Provider
	modelIndex map[string][]domainprovider.ProviderID
}

func (m *mockRegistry) GetProvider(id domainprovider.ProviderID) (*domainprovider.Provider, bool) {
	p, ok := m.providers[id]
	return p, ok
}

func (m *mockRegistry) ListProviders() []domainprovider.Provider {
	result := make([]domainprovider.Provider, 0, len(m.providers))
	for _, p := range m.providers {
		result = append(result, *p)
	}
	return result
}

func (m *mockRegistry) GetProvidersForModel(model string) []domainprovider.ProviderID {
	return m.modelIndex[model]
}

func (m *mockRegistry) GetAllModels() []domaincompletion.Model {
	return nil
}

// mockHealthTracker implements ProviderHealthTracker for testing.
type mockHealthTracker struct {
	healthy map[domainprovider.ProviderID]bool
}

func (m *mockHealthTracker) GetHealth(id domainprovider.ProviderID) domainprovider.ProviderHealth {
	healthy := m.healthy[id]
	return domainprovider.ProviderHealth{
		ProviderID: id,
		Healthy:    healthy,
	}
}

func (m *mockHealthTracker) MarkSuccess(id domainprovider.ProviderID) {}

func (m *mockHealthTracker) MarkFailure(id domainprovider.ProviderID, statusCode int, err error) {}

func (m *mockHealthTracker) MarkRateLimited(id domainprovider.ProviderID, resetAt string) {}

// mockCredentialProvider implements CredentialProvider for testing.
type mockCredentialProvider struct {
	tokens map[domainprovider.ProviderID]string
}

func (m *mockCredentialProvider) GetToken(ctx context.Context, providerID domainprovider.ProviderID) (string, error) {
	return m.tokens[providerID], nil
}

func TestRouteWithHint(t *testing.T) {
	// Setup mock registry with codex and copilot both supporting gpt-5
	registry := &mockRegistry{
		providers: map[domainprovider.ProviderID]*domainprovider.Provider{
			domainprovider.ProviderCodex: {
				ID:       domainprovider.ProviderCodex,
				Name:     "Codex",
				Enabled:  true,
				BaseURL:  "https://codex.example.com",
				AuthType: domainprovider.AuthTypeBearerPool,
			},
			domainprovider.ProviderCopilot: {
				ID:       domainprovider.ProviderCopilot,
				Name:     "Copilot",
				Enabled:  true,
				BaseURL:  "https://copilot.example.com",
				AuthType: domainprovider.AuthTypeBearerPool,
			},
		},
		modelIndex: map[string][]domainprovider.ProviderID{
			"gpt-5": {domainprovider.ProviderCodex, domainprovider.ProviderCopilot},
			"gpt-4": {domainprovider.ProviderCodex},
		},
	}

	healthTracker := &mockHealthTracker{
		healthy: map[domainprovider.ProviderID]bool{
			domainprovider.ProviderCodex:   true,
			domainprovider.ProviderCopilot: true,
		},
	}

	credProvider := &mockCredentialProvider{
		tokens: map[domainprovider.ProviderID]string{
			domainprovider.ProviderCodex:   "codex-token",
			domainprovider.ProviderCopilot: "copilot-token",
		},
	}

	logger := zap.NewNop()

	r := NewRouter(registry, healthTracker, credProvider, nil, logger)

	t.Run("no hint routes by priority", func(t *testing.T) {
		decision, err := r.RouteWithHint(context.Background(), "gpt-5", "", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Codex is first in priority, should be selected
		if decision.ProviderID != domainprovider.ProviderCodex {
			t.Errorf("expected codex provider, got %s", decision.ProviderID)
		}
	})

	t.Run("copilot hint forces copilot provider", func(t *testing.T) {
		decision, err := r.RouteWithHint(context.Background(), "gpt-5", "copilot", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision.ProviderID != domainprovider.ProviderCopilot {
			t.Errorf("expected copilot provider, got %s", decision.ProviderID)
		}
	})

	t.Run("codex hint forces codex provider", func(t *testing.T) {
		decision, err := r.RouteWithHint(context.Background(), "gpt-5", "codex", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision.ProviderID != domainprovider.ProviderCodex {
			t.Errorf("expected codex provider, got %s", decision.ProviderID)
		}
	})

	t.Run("hint for unsupported provider returns error", func(t *testing.T) {
		// gpt-4 is only supported by codex
		_, err := r.RouteWithHint(context.Background(), "gpt-4", "copilot", nil)
		if err == nil {
			t.Fatal("expected error for unsupported provider hint")
		}
	})

	t.Run("hint for unavailable provider returns error", func(t *testing.T) {
		// Mark copilot as unhealthy
		healthTracker.healthy[domainprovider.ProviderCopilot] = false

		_, err := r.RouteWithHint(context.Background(), "gpt-5", "copilot", nil)
		if err == nil {
			t.Fatal("expected error for unavailable provider")
		}

		// Restore
		healthTracker.healthy[domainprovider.ProviderCopilot] = true
	})

	t.Run("exclude providers works with hint", func(t *testing.T) {
		// Request copilot but exclude it - should fail
		_, err := r.RouteWithHint(
			context.Background(),
			"gpt-5",
			"copilot",
			[]domainprovider.ProviderID{domainprovider.ProviderCopilot},
		)
		if err == nil {
			t.Fatal("expected error when hinted provider is excluded")
		}
	})
}

func TestRouteWithFallback(t *testing.T) {
	registry := &mockRegistry{
		providers: map[domainprovider.ProviderID]*domainprovider.Provider{
			domainprovider.ProviderCodex: {
				ID:       domainprovider.ProviderCodex,
				Name:     "Codex",
				Enabled:  true,
				BaseURL:  "https://codex.example.com",
				AuthType: domainprovider.AuthTypeBearerPool,
			},
			domainprovider.ProviderCopilot: {
				ID:       domainprovider.ProviderCopilot,
				Name:     "Copilot",
				Enabled:  true,
				BaseURL:  "https://copilot.example.com",
				AuthType: domainprovider.AuthTypeBearerPool,
			},
		},
		modelIndex: map[string][]domainprovider.ProviderID{
			"gpt-5": {domainprovider.ProviderCodex, domainprovider.ProviderCopilot},
		},
	}

	healthTracker := &mockHealthTracker{
		healthy: map[domainprovider.ProviderID]bool{
			domainprovider.ProviderCodex:   true,
			domainprovider.ProviderCopilot: true,
		},
	}

	credProvider := &mockCredentialProvider{
		tokens: map[domainprovider.ProviderID]string{
			domainprovider.ProviderCodex:   "codex-token",
			domainprovider.ProviderCopilot: "copilot-token",
		},
	}

	logger := zap.NewNop()
	r := NewRouter(registry, healthTracker, credProvider, nil, logger)

	t.Run("fallback to next provider when first is excluded", func(t *testing.T) {
		decision, err := r.RouteWithFallback(
			context.Background(),
			"gpt-5",
			[]domainprovider.ProviderID{domainprovider.ProviderCodex},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Codex excluded, should fall back to copilot
		if decision.ProviderID != domainprovider.ProviderCopilot {
			t.Errorf("expected copilot provider after fallback, got %s", decision.ProviderID)
		}
	})

	t.Run("error when all providers excluded", func(t *testing.T) {
		_, err := r.RouteWithFallback(
			context.Background(),
			"gpt-5",
			[]domainprovider.ProviderID{domainprovider.ProviderCodex, domainprovider.ProviderCopilot},
		)
		if err == nil {
			t.Fatal("expected error when all providers excluded")
		}
	})

	t.Run("error for model with prefix slash", func(t *testing.T) {
		// Models with "/" should be rejected (handler should parse them)
		_, err := r.RouteWithFallback(context.Background(), "copilot/gpt-5", nil)
		if err == nil {
			t.Fatal("expected error for model with prefix slash")
		}
	})
}
