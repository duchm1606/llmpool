// Package provider defines domain entities for LLM providers.
package provider

import "time"

// ProviderID uniquely identifies a provider.
type ProviderID string

// Well-known provider IDs (priority order).
const (
	ProviderCodex     ProviderID = "codex"
	ProviderCopilot   ProviderID = "copilot"
	ProviderOpenAI    ProviderID = "openai"
	ProviderAnthropic ProviderID = "anthropic"
)

// ActiveRuntimeProviders defines the providers supported by the personal-use runtime.
var ActiveRuntimeProviders = []ProviderID{
	ProviderCopilot,
}

// DefaultPriority defines the default provider priority order.
// Higher priority providers are tried first.
var DefaultPriority = []ProviderID{
	ProviderCopilot,
}

// Provider represents an LLM provider configuration.
type Provider struct {
	ID         ProviderID
	Name       string
	Enabled    bool
	BaseURL    string
	Models     []string          // List of supported model IDs
	Headers    map[string]string // Additional headers for this provider
	AuthType   AuthType          // How to authenticate
	Timeout    time.Duration     // Request timeout
	MaxRetries int               // Max retries for transient errors
}

// AuthType defines how to authenticate with a provider.
type AuthType string

const (
	AuthTypeNone       AuthType = "none"
	AuthTypeBearerPool AuthType = "bearer_pool" // Use credential pool
	AuthTypeAPIKey     AuthType = "api_key"     // Use static API key
	AuthTypeOAuth      AuthType = "oauth"       // Use OAuth tokens
)

// ProviderHealth represents the current health state of a provider.
type ProviderHealth struct {
	ProviderID       ProviderID
	Healthy          bool
	LastChecked      time.Time
	LastError        string
	ConsecutiveFails int
	CooldownUntil    time.Time // If set, provider is in cooldown

	// Rate limit tracking
	RateLimited    bool
	RateLimitReset time.Time
}

// IsAvailable returns true if the provider is healthy and not in cooldown.
func (h *ProviderHealth) IsAvailable() bool {
	if !h.Healthy {
		return false
	}
	if h.RateLimited && time.Now().Before(h.RateLimitReset) {
		return false
	}
	if !h.CooldownUntil.IsZero() && time.Now().Before(h.CooldownUntil) {
		return false
	}
	return true
}

// ModelAvailability maps a model ID to the providers that support it.
type ModelAvailability struct {
	ModelID   string
	Providers []ProviderID // Ordered by priority
}

// RoutingDecision represents the result of provider selection.
type RoutingDecision struct {
	Model      string
	ProviderID ProviderID
	BaseURL    string
	Headers    map[string]string
	// Token is populated if AuthType requires it
	Token string

	// Credential info for logging (no secrets)
	CredentialID        string // Profile ID used
	CredentialType      string // Provider type (e.g., "codex", "copilot")
	CredentialAccountID string // Provider account id for upstream headers/logs
	Initiator           string // Upstream initiator hint (e.g., "agent", "user")
}
