package quota

import (
	"context"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

// CredentialRepository provides access to credential data.
type CredentialRepository interface {
	ListEnabled(ctx context.Context) ([]domaincredential.Profile, error)
	CountEnabled(ctx context.Context) (int64, error)
	RandomSample(ctx context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error)
}

// Encryptor handles credential decryption.
type Encryptor interface {
	Decrypt(cipher, iv, tag string) (string, error)
}

// ProviderChecker performs liveness checks against a provider.
type ProviderChecker interface {
	// Check performs a liveness check for the given credential.
	// accessToken and accountID are decrypted from the credential profile.
	// Returns CheckResult with health status and any error details.
	Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error)
}

// StateCache stores credential states in Redis (cache-only).
type StateCache interface {
	// GetCredentialState retrieves cached credential state.
	// Returns nil if not found (cache miss is not an error).
	GetCredentialState(ctx context.Context, credentialID string) (*domainquota.CredentialState, error)

	// SetCredentialState stores credential state with TTL.
	SetCredentialState(ctx context.Context, state domainquota.CredentialState, ttl time.Duration) error

	// GetModelState retrieves cached per-model state.
	GetModelState(ctx context.Context, credentialID, modelID string) (*domainquota.ModelState, error)

	// SetModelState stores per-model state with TTL.
	SetModelState(ctx context.Context, state domainquota.ModelState, ttl time.Duration) error

	// ListCredentialStates retrieves all cached credential states.
	// Returns empty slice if cache is empty/unavailable.
	ListCredentialStates(ctx context.Context) ([]domainquota.CredentialState, error)

	// Ping checks if Redis is available.
	Ping(ctx context.Context) error

	// CountCredentialStates returns count of cached states.
	CountCredentialStates(ctx context.Context) (int64, error)

	// GetCopilotUsage retrieves cached Copilot usage for a credential.
	GetCopilotUsage(ctx context.Context, credentialID string) (*domainquota.CopilotUsage, error)

	// SetCopilotUsage stores Copilot usage with TTL.
	SetCopilotUsage(ctx context.Context, usage domainquota.CopilotUsage, ttl time.Duration) error

	// ListCopilotUsages retrieves all cached Copilot usages.
	ListCopilotUsages(ctx context.Context) ([]domainquota.CopilotUsage, error)
}

// CopilotUsageFetcher fetches Copilot usage from GitHub API.
type CopilotUsageFetcher interface {
	// FetchUsage fetches the full Copilot usage information for a credential.
	FetchUsage(ctx context.Context, credentialID, accessToken string) (*domainquota.CopilotUsage, error)
}

// LivenessService defines the liveness checking operations.
type LivenessService interface {
	// CheckSample performs a sample-based liveness check (20% of credentials).
	CheckSample(ctx context.Context) error

	// CheckAll performs a full sweep of all credentials.
	CheckAll(ctx context.Context) error

	// CheckCredential performs a liveness check for a single credential.
	CheckCredential(ctx context.Context, credentialID string) error

	// GetCredentialState retrieves current state for a credential.
	GetCredentialState(ctx context.Context, credentialID string) (*domainquota.CredentialState, error)

	// NeedsRehydration returns true if cache is empty/unavailable and needs full sweep.
	NeedsRehydration(ctx context.Context) (bool, error)
}
