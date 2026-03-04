package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

func TestCopilotChecker_Check_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/copilot_internal/user" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "token test-token" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"copilot_plan": "individual",
			"quota_snapshots": {
				"premium_interactions": {
					"entitlement": 100,
					"remaining": 75,
					"percent_remaining": 75.0
				},
				"chat": {
					"percent_remaining": 100.0
				}
			}
		}`))
	}))
	defer server.Close()

	checker := NewCopilotChecker(CopilotCheckerConfig{
		UsageURL: server.URL + "/copilot_internal/user",
	})

	result, err := checker.Check(context.Background(), "copilot", "test-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Healthy {
		t.Error("expected healthy result")
	}
	if result.Quota.Remaining != 750 {
		t.Errorf("quota.remaining = %d, want 750 (75%% of 1000)", result.Quota.Remaining)
	}
	if result.Quota.Limit != 1000 {
		t.Errorf("quota.limit = %d, want 1000", result.Quota.Limit)
	}
	if result.Quota.Ratio < 0.74 || result.Quota.Ratio > 0.76 {
		t.Errorf("quota.ratio = %f, want around 0.75", result.Quota.Ratio)
	}
}

func TestCopilotChecker_Check_WithEntitlement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Response with entitlement/remaining instead of percent_remaining
		_, _ = w.Write([]byte(`{
			"copilot_plan": "business",
			"quota_snapshots": {
				"premium_interactions": {
					"entitlement": 200,
					"remaining": 150
				}
			}
		}`))
	}))
	defer server.Close()

	checker := NewCopilotChecker(CopilotCheckerConfig{
		UsageURL: server.URL + "/copilot_internal/user",
	})

	result, err := checker.Check(context.Background(), "copilot", "test-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Healthy {
		t.Error("expected healthy result")
	}
	if result.Quota.Remaining != 150 {
		t.Errorf("quota.remaining = %d, want 150", result.Quota.Remaining)
	}
	if result.Quota.Limit != 200 {
		t.Errorf("quota.limit = %d, want 200", result.Quota.Limit)
	}
	if result.Quota.Ratio < 0.74 || result.Quota.Ratio > 0.76 {
		t.Errorf("quota.ratio = %f, want around 0.75", result.Quota.Ratio)
	}
}

func TestCopilotChecker_Check_Exhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"copilot_plan": "individual",
			"quota_snapshots": {
				"premium_interactions": {
					"entitlement": 100,
					"remaining": 0,
					"percent_remaining": 0.0
				}
			}
		}`))
	}))
	defer server.Close()

	checker := NewCopilotChecker(CopilotCheckerConfig{
		UsageURL: server.URL + "/copilot_internal/user",
	})

	result, err := checker.Check(context.Background(), "copilot", "test-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Healthy {
		t.Error("expected unhealthy result when quota exhausted")
	}
	if result.ErrorCode != http.StatusTooManyRequests {
		t.Errorf("error_code = %d, want 429", result.ErrorCode)
	}
	if result.Quota.Ratio != 0 {
		t.Errorf("quota.ratio = %f, want 0", result.Quota.Ratio)
	}
}

func TestCopilotChecker_Check_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	checker := NewCopilotChecker(CopilotCheckerConfig{
		UsageURL: server.URL + "/copilot_internal/user",
	})

	result, err := checker.Check(context.Background(), "copilot", "bad-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Healthy {
		t.Error("expected unhealthy result for unauthorized")
	}
	if result.ErrorCode != http.StatusUnauthorized {
		t.Errorf("error_code = %d, want 401", result.ErrorCode)
	}
	if result.ErrorMessage != "unauthorized" {
		t.Errorf("error_message = %s, want unauthorized", result.ErrorMessage)
	}
}

func TestCopilotChecker_Check_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	checker := NewCopilotChecker(CopilotCheckerConfig{
		UsageURL: server.URL + "/copilot_internal/user",
	})

	result, err := checker.Check(context.Background(), "copilot", "test-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Healthy {
		t.Error("expected unhealthy result for rate limited")
	}
	if result.ErrorCode != http.StatusTooManyRequests {
		t.Errorf("error_code = %d, want 429", result.ErrorCode)
	}
}

func TestCopilotChecker_Check_UnlimitedQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Unlimited quota - should not be used for ratio calculation
		_, _ = w.Write([]byte(`{
			"copilot_plan": "enterprise",
			"quota_snapshots": {
				"premium_interactions": {
					"unlimited": true
				},
				"chat": {
					"percent_remaining": 80.0
				}
			}
		}`))
	}))
	defer server.Close()

	checker := NewCopilotChecker(CopilotCheckerConfig{
		UsageURL: server.URL + "/copilot_internal/user",
	})

	result, err := checker.Check(context.Background(), "copilot", "test-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Healthy {
		t.Error("expected healthy result")
	}
	// Should use chat quota since premium_interactions is unlimited
	if result.Quota.Remaining != 800 {
		t.Errorf("quota.remaining = %d, want 800 (80%% of 1000)", result.Quota.Remaining)
	}
}

func TestCopilotChecker_Check_MostRestrictiveQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// premium_interactions is more restrictive than chat
		_, _ = w.Write([]byte(`{
			"copilot_plan": "business",
			"quota_snapshots": {
				"premium_interactions": {
					"percent_remaining": 20.0
				},
				"chat": {
					"percent_remaining": 80.0
				}
			}
		}`))
	}))
	defer server.Close()

	checker := NewCopilotChecker(CopilotCheckerConfig{
		UsageURL: server.URL + "/copilot_internal/user",
	})

	result, err := checker.Check(context.Background(), "copilot", "test-token", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Healthy {
		t.Error("expected healthy result")
	}
	// Should use premium_interactions (20%) since it's more restrictive
	if result.Quota.Remaining != 200 {
		t.Errorf("quota.remaining = %d, want 200 (20%% of 1000)", result.Quota.Remaining)
	}
}

func TestCopilotChecker_DefaultConfig(t *testing.T) {
	checker := NewCopilotChecker(CopilotCheckerConfig{})
	if checker.cfg.UsageURL != defaultCopilotUsageURL {
		t.Errorf("usage url = %q, want %q", checker.cfg.UsageURL, defaultCopilotUsageURL)
	}
}

func TestCopilotChecker_extractBestQuota(t *testing.T) {
	checker := NewCopilotChecker(CopilotCheckerConfig{})

	// Test with nil details
	quota := checker.extractBestQuota(nil, nil, nil)
	if quota != domainquota.NewQuotaInfoUnknown() {
		t.Errorf("expected unknown quota for nil details, got %+v", quota)
	}

	// Test with only unlimited
	unlimited := true
	quota = checker.extractBestQuota(&copilotQuotaDetail{Unlimited: &unlimited})
	if quota != domainquota.NewQuotaInfoUnknown() {
		t.Errorf("expected unknown quota for unlimited, got %+v", quota)
	}

	// Test with percent_remaining
	pct := 50.0
	quota = checker.extractBestQuota(&copilotQuotaDetail{PercentRemaining: &pct})
	if quota.Remaining != 500 || quota.Limit != 1000 {
		t.Errorf("unexpected quota: %+v", quota)
	}
}

func TestCopilotChecker_isExhausted(t *testing.T) {
	checker := NewCopilotChecker(CopilotCheckerConfig{})

	// Not exhausted
	pct := 50.0
	if checker.isExhausted(&copilotQuotaDetail{PercentRemaining: &pct}, nil) {
		t.Error("should not be exhausted at 50%")
	}

	// Exhausted by percent_remaining
	zero := 0.0
	if !checker.isExhausted(&copilotQuotaDetail{PercentRemaining: &zero}, nil) {
		t.Error("should be exhausted at 0%")
	}

	// Exhausted by remaining count
	if !checker.isExhausted(&copilotQuotaDetail{Entitlement: 100, Remaining: 0}, nil) {
		t.Error("should be exhausted when remaining is 0")
	}

	// Not exhausted with unlimited
	unlimited := true
	if checker.isExhausted(&copilotQuotaDetail{Unlimited: &unlimited}, nil) {
		t.Error("should not be exhausted when unlimited")
	}
}
