package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

func TestCodexChecker_Check_HealthyWithQuotaHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("ChatGPT-Account-Id") != "acct-1" {
			t.Errorf("unexpected account id header: %s", r.Header.Get("ChatGPT-Account-Id"))
		}
		if r.URL.Path != "/backend-api/wham/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("x-ratelimit-remaining-requests", "950")
		w.Header().Set("x-ratelimit-limit-requests", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":5}}}`))
	}))
	defer server.Close()

	checker := NewCodexChecker(CodexCheckerConfig{
		UsageURL: server.URL + "/backend-api/wham/usage",
		Timeout:  5 * time.Second,
	})

	result, err := checker.Check(context.Background(), "codex", "test-token", "acct-1")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !result.Healthy {
		t.Error("expected healthy = true")
	}
	if result.ErrorCode != 0 {
		t.Errorf("expected error_code = 0, got %d", result.ErrorCode)
	}
	if result.Quota.Remaining != 950 {
		t.Errorf("quota.remaining = %d, want 950", result.Quota.Remaining)
	}
	if result.Quota.Limit != 1000 {
		t.Errorf("quota.limit = %d, want 1000", result.Quota.Limit)
	}
}

func TestCodexChecker_Check_QuotaFromUsageBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":25}}}`))
	}))
	defer server.Close()

	checker := NewCodexChecker(CodexCheckerConfig{UsageURL: server.URL, Timeout: 5 * time.Second})
	result, err := checker.Check(context.Background(), "codex", "test-token", "acct-1")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.Healthy {
		t.Fatal("expected healthy result")
	}
	if !result.Quota.IsKnown() {
		t.Fatal("expected known quota from usage body")
	}
	if result.Quota.Ratio < 0.74 || result.Quota.Ratio > 0.76 {
		t.Fatalf("quota ratio = %f, want around 0.75", result.Quota.Ratio)
	}
}

func TestCodexChecker_Check_QuotaExhaustedFromUsageBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"credits":{"has_credits":false,"unlimited":false},"rate_limit":{"primary_window":{"used_percent":100}}}`))
	}))
	defer server.Close()

	checker := NewCodexChecker(CodexCheckerConfig{UsageURL: server.URL, Timeout: 5 * time.Second})
	result, err := checker.Check(context.Background(), "codex", "test-token", "acct-1")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Healthy {
		t.Fatal("expected unhealthy result when usage says exhausted")
	}
	if result.ErrorCode != http.StatusTooManyRequests {
		t.Fatalf("error_code = %d, want 429", result.ErrorCode)
	}
	if result.Quota.Ratio != 0 {
		t.Fatalf("quota ratio = %f, want 0", result.Quota.Ratio)
	}
}

func TestCodexChecker_Check_HealthyWithoutAccountIDHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("ChatGPT-Account-Id"); got != "" {
			t.Errorf("expected no account header, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	checker := NewCodexChecker(CodexCheckerConfig{
		UsageURL: server.URL,
		Timeout:  5 * time.Second,
	})

	result, err := checker.Check(context.Background(), "codex", "test-token", "")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.Healthy {
		t.Error("expected healthy = true")
	}
}

func TestCodexChecker_Check_UnauthorizedAndForbidden(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errCode    int
		errMsg     string
	}{
		{name: "401", statusCode: http.StatusUnauthorized, errCode: 401, errMsg: "unauthorized"},
		{name: "403", statusCode: http.StatusForbidden, errCode: 403, errMsg: "forbidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			checker := NewCodexChecker(CodexCheckerConfig{UsageURL: server.URL, Timeout: 5 * time.Second})
			result, err := checker.Check(context.Background(), "codex", "test-token", "acct-1")
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if result.Healthy {
				t.Fatalf("expected unhealthy for %d", tt.statusCode)
			}
			if result.ErrorCode != tt.errCode {
				t.Fatalf("error_code = %d, want %d", result.ErrorCode, tt.errCode)
			}
			if result.ErrorMessage != tt.errMsg {
				t.Fatalf("error_message = %q, want %q", result.ErrorMessage, tt.errMsg)
			}
		})
	}
}

func TestCodexChecker_Check_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ratelimit-remaining-requests", "0")
		w.Header().Set("x-ratelimit-limit-requests", "100")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	checker := NewCodexChecker(CodexCheckerConfig{UsageURL: server.URL, Timeout: 5 * time.Second})
	result, err := checker.Check(context.Background(), "codex", "test-token", "acct-1")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy for 429")
	}
	if result.ErrorCode != 429 {
		t.Errorf("error_code = %d, want 429", result.ErrorCode)
	}
	if result.Quota.Remaining != 0 || result.Quota.Ratio != 0 {
		t.Errorf("expected zero quota, got remaining=%d ratio=%f", result.Quota.Remaining, result.Quota.Ratio)
	}
}

func TestCodexChecker_Check_NetworkError(t *testing.T) {
	checker := NewCodexChecker(CodexCheckerConfig{
		UsageURL: "http://localhost:1",
		Timeout:  1 * time.Second,
	})

	_, err := checker.Check(context.Background(), "codex", "test-token", "acct-1")
	if err == nil {
		t.Error("expected network error")
	}
}

func TestCodexChecker_Defaults(t *testing.T) {
	checker := NewCodexChecker(CodexCheckerConfig{})
	if checker.cfg.UsageURL != defaultCodexUsageURL {
		t.Fatalf("usage url = %q, want %q", checker.cfg.UsageURL, defaultCodexUsageURL)
	}
	if checker.cfg.Timeout <= 0 {
		t.Fatalf("timeout should be > 0, got %v", checker.cfg.Timeout)
	}
}

func TestCodexChecker_extractQuotaFromHeaders(t *testing.T) {
	checker := NewCodexChecker(DefaultCodexCheckerConfig())

	headers := http.Header{}
	headers.Set("x-ratelimit-remaining-requests", "100")
	headers.Set("x-ratelimit-limit-requests", "1000")
	quota := checker.extractQuotaFromHeaders(headers)
	if quota.Remaining != 100 || quota.Limit != 1000 {
		t.Fatalf("unexpected quota: %+v", quota)
	}

	headers = http.Header{}
	headers.Set("x-ratelimit-remaining-tokens", "50")
	headers.Set("x-ratelimit-limit-tokens", "200")
	quota = checker.extractQuotaFromHeaders(headers)
	if quota.Remaining != 50 || quota.Limit != 200 {
		t.Fatalf("unexpected quota fallback: %+v", quota)
	}

	headers = http.Header{}
	quota = checker.extractQuotaFromHeaders(headers)
	if quota != domainquota.NewQuotaInfoUnknown() {
		t.Fatalf("expected unknown quota, got %+v", quota)
	}
}
