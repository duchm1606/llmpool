package quota

import (
	"context"
	"testing"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

type mockChecker struct {
	name   string
	result domainquota.CheckResult
	err    error
}

func (m *mockChecker) Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error) {
	return m.result, m.err
}

func TestCheckerRouter_RoutesToCorrectChecker(t *testing.T) {
	codexChecker := &mockChecker{
		name: "codex",
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
			Quota:     domainquota.NewQuotaInfo(100, 200),
		},
	}

	noopChecker := &mockChecker{
		name: "noop",
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
			Quota:     domainquota.NewQuotaInfoUnknown(),
		},
	}

	checkers := map[string]ProviderChecker{
		"codex":  codexChecker,
		"openai": codexChecker,
	}

	router := NewCheckerRouter(checkers, noopChecker)

	tests := []struct {
		credType      string
		expectedQuota int64 // remaining
		expectedKnown bool
	}{
		{"codex", 100, true},
		{"openai", 100, true},
		{"anthropic", -1, false},
		{"gemini", -1, false},
		{"unknown", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.credType, func(t *testing.T) {
			result, err := router.Check(context.Background(), tt.credType, "test-token", "acct-1")
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}

			if result.Quota.Remaining != tt.expectedQuota {
				t.Errorf("quota.remaining = %d, want %d", result.Quota.Remaining, tt.expectedQuota)
			}
			if result.Quota.IsKnown() != tt.expectedKnown {
				t.Errorf("quota.IsKnown() = %v, want %v", result.Quota.IsKnown(), tt.expectedKnown)
			}
		})
	}
}

func TestCheckerRouter_DefaultChecker(t *testing.T) {
	defaultChecker := &mockChecker{
		name: "default",
		result: domainquota.CheckResult{
			Healthy:   false,
			CheckedAt: time.Now(),
			ErrorCode: 500,
		},
	}

	// Empty checkers map - should always use default
	router := NewCheckerRouter(map[string]ProviderChecker{}, defaultChecker)

	result, err := router.Check(context.Background(), "any-type", "token", "acct-1")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result.Healthy {
		t.Error("expected healthy = false from default checker")
	}
	if result.ErrorCode != 500 {
		t.Errorf("error_code = %d, want 500", result.ErrorCode)
	}
}

func TestCheckerRouter_NilDefaultChecker(t *testing.T) {
	// Router with nil default checker - should return healthy for unknown types
	router := NewCheckerRouter(map[string]ProviderChecker{}, nil)

	result, err := router.Check(context.Background(), "unknown-type", "token", "acct-1")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !result.Healthy {
		t.Error("expected healthy = true when no default checker")
	}
	if result.Quota.IsKnown() {
		t.Error("expected unknown quota when no default checker")
	}
}

func TestCheckerRouter_NilCheckersMap(t *testing.T) {
	// Router with nil checkers map - should handle gracefully
	defaultChecker := &mockChecker{
		name: "default",
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
		},
	}

	router := NewCheckerRouter(nil, defaultChecker)

	result, err := router.Check(context.Background(), "any-type", "token", "acct-1")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !result.Healthy {
		t.Error("expected healthy = true from default checker")
	}
}
