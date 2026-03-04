package quota

import (
	"context"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

// NoopChecker is a placeholder checker that always returns healthy.
// Replace with real provider-specific implementations.
type NoopChecker struct{}

// NewNoopChecker creates a new noop checker.
func NewNoopChecker() *NoopChecker {
	return &NoopChecker{}
}

// Check performs a noop check that always returns healthy.
// Returns unknown quota since noop checker doesn't make real API calls.
func (c *NoopChecker) Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error) {
	return domainquota.CheckResult{
		CredentialID: "", // Will be set by caller
		Healthy:      true,
		CheckedAt:    time.Now(),
		Quota:        domainquota.NewQuotaInfoUnknown(),
	}, nil
}
