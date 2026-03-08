// Package completion provides usecase interfaces and services for completion routing.
package completion

import (
	"context"
	"io"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
)

// ProviderRegistry manages provider configurations and availability.
type ProviderRegistry interface {
	// GetProvider returns the provider configuration by ID.
	GetProvider(id domainprovider.ProviderID) (*domainprovider.Provider, bool)

	// ListProviders returns all registered providers.
	ListProviders() []domainprovider.Provider

	// GetProvidersForModel returns providers that support the given model, ordered by priority.
	GetProvidersForModel(model string) []domainprovider.ProviderID

	// GetAllModels returns a deduplicated list of all available models.
	GetAllModels() []domaincompletion.Model
}

// ProviderHealthTracker tracks provider health states.
type ProviderHealthTracker interface {
	// GetHealth returns the current health state for a provider.
	GetHealth(id domainprovider.ProviderID) domainprovider.ProviderHealth

	// MarkSuccess records a successful request to a provider.
	MarkSuccess(id domainprovider.ProviderID)

	// MarkFailure records a failed request to a provider.
	MarkFailure(id domainprovider.ProviderID, statusCode int, err error)

	// MarkRateLimited marks a provider as rate-limited with a reset time.
	MarkRateLimited(id domainprovider.ProviderID, resetAt string)
}

// CredentialProvider provides authentication tokens for providers.
type CredentialProvider interface {
	// GetToken returns a valid token for the given provider.
	// It may use credential pooling and load balancing internally.
	GetToken(ctx context.Context, providerID domainprovider.ProviderID) (string, error)
}

// CredentialMetadata contains safe, non-secret credential selection details.
type CredentialMetadata struct {
	CredentialID string
	AccountID    string
	Type         string
}

// ExtendedCredentialProvider extends CredentialProvider with selection info.
// Implementations that support tracking which credential was selected should implement this.
type ExtendedCredentialProvider interface {
	CredentialProvider

	// GetTokenWithInfo returns a token along with credential metadata for logging.
	GetTokenWithInfo(ctx context.Context, providerID domainprovider.ProviderID) (token string, meta CredentialMetadata, err error)
}

// CredentialRefresher refreshes a selected credential on demand.
// Used for inline recovery flows (for example, expired Copilot session token).
type CredentialRefresher interface {
	RefreshCredential(ctx context.Context, credentialID string) error
}

// ProviderClient executes requests to upstream providers.
type ProviderClient interface {
	// ChatCompletion executes a chat completion request.
	ChatCompletion(
		ctx context.Context,
		decision domainprovider.RoutingDecision,
		req domaincompletion.ChatCompletionRequest,
	) (*domaincompletion.ChatCompletionResponse, error)

	// ChatCompletionStream executes a streaming chat completion request.
	// Returns a channel that receives streaming chunks.
	ChatCompletionStream(
		ctx context.Context,
		decision domainprovider.RoutingDecision,
		req domaincompletion.ChatCompletionRequest,
	) (<-chan StreamChunk, error)
}

// StreamChunk represents a chunk in streaming response.
type StreamChunk struct {
	Data  []byte
	Error error
	Done  bool
}

// Router selects the appropriate provider for a request.
type Router interface {
	// Route selects a provider for the given model.
	// It considers provider priority, health, and availability.
	Route(ctx context.Context, model string) (*domainprovider.RoutingDecision, error)

	// RouteWithFallback attempts to route, falling back to next provider on failure.
	RouteWithFallback(
		ctx context.Context,
		model string,
		excludeProviders []domainprovider.ProviderID,
	) (*domainprovider.RoutingDecision, error)

	// RouteWithHint routes with an explicit provider hint.
	// If providerHint is non-empty, it forces routing to that provider (if available).
	// If the hinted provider doesn't support the model or is unavailable, returns an error.
	RouteWithHint(
		ctx context.Context,
		model string,
		providerHint string,
		excludeProviders []domainprovider.ProviderID,
	) (*domainprovider.RoutingDecision, error)
}

// CompletionService orchestrates completion requests.
type CompletionService interface {
	// ValidateRequest validates request fields and preflights routing feasibility.
	// Handlers can use this before committing streaming headers.
	ValidateRequest(ctx context.Context, req domaincompletion.ChatCompletionRequest) error

	// ChatCompletion handles a chat completion request with routing and fallback.
	ChatCompletion(
		ctx context.Context,
		req domaincompletion.ChatCompletionRequest,
	) (*domaincompletion.ChatCompletionResponse, error)

	// ChatCompletionStream handles a streaming chat completion request.
	ChatCompletionStream(
		ctx context.Context,
		req domaincompletion.ChatCompletionRequest,
		writer io.Writer,
	) error

	// ListModels returns all available models.
	ListModels(ctx context.Context) (*domaincompletion.ModelsResponse, error)

	// SetUsagePublisher sets the usage publisher for tracking.
	// This is optional - if not set, usage tracking is disabled.
	SetUsagePublisher(publisher UsagePublisher)
}

// PerUserMappingProvider provides per-user provider preferences.
// This is an extension point for future per-user mapping feature.
type PerUserMappingProvider interface {
	// GetUserPreferredProviders returns the user's preferred provider order for a model.
	// Returns nil if no custom mapping exists for the user.
	GetUserPreferredProviders(ctx context.Context, userID string, model string) []domainprovider.ProviderID
}
