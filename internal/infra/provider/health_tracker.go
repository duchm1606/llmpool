package provider

import (
	"net/http"
	"sync"
	"time"

	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
)

// HealthTrackerConfig configures the health tracker behavior.
type HealthTrackerConfig struct {
	// FailureThreshold is the number of consecutive failures before marking unhealthy.
	FailureThreshold int
	// CooldownDuration is how long to wait before retrying an unhealthy provider.
	CooldownDuration time.Duration
	// RateLimitDefaultCooldown is the default cooldown when no Retry-After is provided.
	RateLimitDefaultCooldown time.Duration
}

// DefaultHealthTrackerConfig returns sensible defaults.
func DefaultHealthTrackerConfig() HealthTrackerConfig {
	return HealthTrackerConfig{
		FailureThreshold:         3,
		CooldownDuration:         30 * time.Second,
		RateLimitDefaultCooldown: 60 * time.Second,
	}
}

// healthTracker implements ProviderHealthTracker.
type healthTracker struct {
	mu     sync.RWMutex
	states map[domainprovider.ProviderID]*domainprovider.ProviderHealth
	config HealthTrackerConfig
}

// NewHealthTracker creates a new health tracker.
func NewHealthTracker(config HealthTrackerConfig) usecasecompletion.ProviderHealthTracker {
	return &healthTracker{
		states: make(map[domainprovider.ProviderID]*domainprovider.ProviderHealth),
		config: config,
	}
}

// GetHealth returns the current health state for a provider.
func (h *healthTracker) GetHealth(id domainprovider.ProviderID) domainprovider.ProviderHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()

	state, ok := h.states[id]
	if !ok {
		// Unknown provider is assumed healthy
		return domainprovider.ProviderHealth{
			ProviderID: id,
			Healthy:    true,
		}
	}

	// Check if cooldown has expired
	now := time.Now()
	result := *state

	// Reset cooldown if expired
	if !result.CooldownUntil.IsZero() && now.After(result.CooldownUntil) {
		result.CooldownUntil = time.Time{}
		result.Healthy = true
		result.ConsecutiveFails = 0
	}

	// Reset rate limit if expired
	if result.RateLimited && now.After(result.RateLimitReset) {
		result.RateLimited = false
		result.RateLimitReset = time.Time{}
	}

	return result
}

// MarkSuccess records a successful request to a provider.
func (h *healthTracker) MarkSuccess(id domainprovider.ProviderID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	state := h.getOrCreateState(id)
	state.Healthy = true
	state.ConsecutiveFails = 0
	state.LastChecked = time.Now()
	state.LastError = ""
	state.CooldownUntil = time.Time{}
	state.RateLimited = false
	state.RateLimitReset = time.Time{}
}

// MarkFailure records a failed request to a provider.
func (h *healthTracker) MarkFailure(id domainprovider.ProviderID, statusCode int, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	state := h.getOrCreateState(id)
	state.ConsecutiveFails++
	state.LastChecked = time.Now()
	if err != nil {
		state.LastError = err.Error()
	}

	// Mark unhealthy after threshold
	if state.ConsecutiveFails >= h.config.FailureThreshold {
		state.Healthy = false
		state.CooldownUntil = time.Now().Add(h.config.CooldownDuration)
	}

	// Handle specific status codes
	if statusCode == http.StatusTooManyRequests {
		state.RateLimited = true
		state.RateLimitReset = time.Now().Add(h.config.RateLimitDefaultCooldown)
	}
}

// MarkRateLimited marks a provider as rate-limited with a reset time.
func (h *healthTracker) MarkRateLimited(id domainprovider.ProviderID, resetAt string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	state := h.getOrCreateState(id)
	state.RateLimited = true
	state.LastChecked = time.Now()

	// Parse reset time if provided
	if resetAt != "" {
		// Try parsing as RFC1123
		if t, err := time.Parse(time.RFC1123, resetAt); err == nil {
			state.RateLimitReset = t
			return
		}
		// Try parsing as Unix timestamp
		if t, err := time.Parse("2006-01-02T15:04:05Z", resetAt); err == nil {
			state.RateLimitReset = t
			return
		}
	}

	// Default cooldown
	state.RateLimitReset = time.Now().Add(h.config.RateLimitDefaultCooldown)
}

// getOrCreateState returns the state for a provider, creating it if needed.
// Must be called with lock held.
func (h *healthTracker) getOrCreateState(id domainprovider.ProviderID) *domainprovider.ProviderHealth {
	state, ok := h.states[id]
	if !ok {
		state = &domainprovider.ProviderHealth{
			ProviderID: id,
			Healthy:    true,
		}
		h.states[id] = state
	}
	return state
}
