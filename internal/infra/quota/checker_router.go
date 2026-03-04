package quota

import (
	"context"
	"fmt"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

// checkerRouter routes liveness checks to appropriate provider-specific checker.
type checkerRouter struct {
	checkers     map[string]ProviderChecker
	defaultCheck ProviderChecker
}

// ProviderChecker is the interface for provider-specific checkers.
type ProviderChecker interface {
	Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error)
}

// NewCheckerRouter creates a router that directs checks to provider-specific checkers.
// checkers is a map from credential type to checker.
// defaultChecker is used for unknown credential types; if nil, unknown types return healthy with unknown quota.
func NewCheckerRouter(checkers map[string]ProviderChecker, defaultChecker ProviderChecker) *checkerRouter {
	if checkers == nil {
		checkers = make(map[string]ProviderChecker)
	}
	return &checkerRouter{
		checkers:     checkers,
		defaultCheck: defaultChecker,
	}
}

// Check routes the check to the appropriate provider checker.
func (r *checkerRouter) Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error) {
	checker, ok := r.checkers[credentialType]
	if !ok {
		if r.defaultCheck == nil {
			// No default checker configured - return healthy with unknown quota
			return domainquota.CheckResult{
				Healthy:   true,
				CheckedAt: time.Now(),
				Quota:     domainquota.NewQuotaInfoUnknown(),
			}, nil
		}
		return r.defaultCheck.Check(ctx, credentialType, accessToken, accountID)
	}
	if checker == nil {
		return domainquota.CheckResult{}, fmt.Errorf("nil checker registered for credential type: %s", credentialType)
	}
	return checker.Check(ctx, credentialType, accessToken, accountID)
}
