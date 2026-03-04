package quota

import (
	"testing"
	"time"
)

func TestCredentialState_IsAvailable(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		state    CredentialState
		expected bool
	}{
		{
			name: "healthy is available",
			state: CredentialState{
				Status: StatusHealthy,
			},
			expected: true,
		},
		{
			name: "unknown is available",
			state: CredentialState{
				Status: StatusUnknown,
			},
			expected: true,
		},
		{
			name: "unhealthy is not available",
			state: CredentialState{
				Status: StatusUnhealthy,
			},
			expected: false,
		},
		{
			name: "cooldown in future is not available",
			state: CredentialState{
				Status:        StatusCooldown,
				CooldownUntil: now.Add(10 * time.Minute),
			},
			expected: false,
		},
		{
			name: "cooldown in past is available",
			state: CredentialState{
				Status:        StatusCooldown,
				CooldownUntil: now.Add(-10 * time.Minute),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.IsAvailable(now)
			if got != tt.expected {
				t.Errorf("IsAvailable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestModelState_IsAvailable(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		state    ModelState
		expected bool
	}{
		{
			name: "healthy is available",
			state: ModelState{
				Status: StatusHealthy,
			},
			expected: true,
		},
		{
			name: "unhealthy is not available",
			state: ModelState{
				Status: StatusUnhealthy,
			},
			expected: false,
		},
		{
			name: "cooldown in future is not available",
			state: ModelState{
				Status:        StatusCooldown,
				CooldownUntil: now.Add(5 * time.Minute),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.IsAvailable(now)
			if got != tt.expected {
				t.Errorf("IsAvailable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCalculateRateLimitCooldown(t *testing.T) {
	cfg := DefaultCooldownConfig()

	tests := []struct {
		name       string
		retryCount int
		expected   time.Duration
	}{
		{
			name:       "first retry: 2m",
			retryCount: 0,
			expected:   2 * time.Minute,
		},
		{
			name:       "second retry: 4m",
			retryCount: 1,
			expected:   4 * time.Minute,
		},
		{
			name:       "third retry: 8m",
			retryCount: 2,
			expected:   8 * time.Minute,
		},
		{
			name:       "fourth retry: 16m",
			retryCount: 3,
			expected:   16 * time.Minute,
		},
		{
			name:       "high retry capped at max",
			retryCount: 10,
			expected:   30 * time.Minute, // capped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateRateLimitCooldown(tt.retryCount, cfg)
			if got != tt.expected {
				t.Errorf("CalculateRateLimitCooldown(%d) = %v, want %v", tt.retryCount, got, tt.expected)
			}
		})
	}
}

func TestDefaultCooldownConfig(t *testing.T) {
	cfg := DefaultCooldownConfig()

	if cfg.AuthFailureCooldown != 30*time.Minute {
		t.Errorf("AuthFailureCooldown = %v, want 30m", cfg.AuthFailureCooldown)
	}
	if cfg.RateLimitInitial != 2*time.Minute {
		t.Errorf("RateLimitInitial = %v, want 2m", cfg.RateLimitInitial)
	}
	if cfg.RateLimitMaxCooldown != 30*time.Minute {
		t.Errorf("RateLimitMaxCooldown = %v, want 30m", cfg.RateLimitMaxCooldown)
	}
	if cfg.NetworkErrorCooldown != 5*time.Minute {
		t.Errorf("NetworkErrorCooldown = %v, want 5m", cfg.NetworkErrorCooldown)
	}
	if cfg.NetworkMaxRetries != 3 {
		t.Errorf("NetworkMaxRetries = %d, want 3", cfg.NetworkMaxRetries)
	}
}

func TestQuotaInfo_NewQuotaInfoUnknown(t *testing.T) {
	q := NewQuotaInfoUnknown()

	if q.Remaining != -1 {
		t.Errorf("remaining = %d, want -1", q.Remaining)
	}
	if q.Limit != -1 {
		t.Errorf("limit = %d, want -1", q.Limit)
	}
	if q.Ratio != QuotaUnknown {
		t.Errorf("ratio = %f, want %f", q.Ratio, QuotaUnknown)
	}
	if q.IsKnown() {
		t.Error("expected IsKnown() = false for unknown quota")
	}
}

func TestQuotaInfo_NewQuotaInfo(t *testing.T) {
	tests := []struct {
		name          string
		remaining     int64
		limit         int64
		expectedRatio float64
		expectedKnown bool
	}{
		{
			name:          "normal quota",
			remaining:     50,
			limit:         100,
			expectedRatio: 0.5,
			expectedKnown: true,
		},
		{
			name:          "full quota",
			remaining:     100,
			limit:         100,
			expectedRatio: 1.0,
			expectedKnown: true,
		},
		{
			name:          "zero remaining",
			remaining:     0,
			limit:         100,
			expectedRatio: 0.0,
			expectedKnown: true,
		},
		{
			name:          "zero limit",
			remaining:     50,
			limit:         0,
			expectedRatio: QuotaUnknown,
			expectedKnown: false,
		},
		{
			name:          "negative limit",
			remaining:     50,
			limit:         -1,
			expectedRatio: QuotaUnknown,
			expectedKnown: false,
		},
		{
			name:          "over limit capped at 1",
			remaining:     150,
			limit:         100,
			expectedRatio: 1.0,
			expectedKnown: true,
		},
		{
			name:          "negative remaining capped at 0",
			remaining:     -10,
			limit:         100,
			expectedRatio: 0.0,
			expectedKnown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQuotaInfo(tt.remaining, tt.limit)

			if q.Remaining != tt.remaining {
				t.Errorf("remaining = %d, want %d", q.Remaining, tt.remaining)
			}
			if q.Limit != tt.limit {
				t.Errorf("limit = %d, want %d", q.Limit, tt.limit)
			}
			if q.Ratio != tt.expectedRatio {
				t.Errorf("ratio = %f, want %f", q.Ratio, tt.expectedRatio)
			}
			if q.IsKnown() != tt.expectedKnown {
				t.Errorf("IsKnown() = %v, want %v", q.IsKnown(), tt.expectedKnown)
			}
		})
	}
}

func TestQuotaInfo_IsKnown(t *testing.T) {
	tests := []struct {
		name     string
		quota    QuotaInfo
		expected bool
	}{
		{
			name:     "ratio 0 is known",
			quota:    QuotaInfo{Remaining: 0, Limit: 100, Ratio: 0},
			expected: true,
		},
		{
			name:     "ratio 0.5 is known",
			quota:    QuotaInfo{Remaining: 50, Limit: 100, Ratio: 0.5},
			expected: true,
		},
		{
			name:     "ratio 1 is known",
			quota:    QuotaInfo{Remaining: 100, Limit: 100, Ratio: 1},
			expected: true,
		},
		{
			name:     "ratio -1 is unknown",
			quota:    QuotaInfo{Remaining: -1, Limit: -1, Ratio: -1},
			expected: false,
		},
		{
			name:     "ratio -0.5 is unknown",
			quota:    QuotaInfo{Remaining: -1, Limit: -1, Ratio: -0.5},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.quota.IsKnown(); got != tt.expected {
				t.Errorf("IsKnown() = %v, want %v", got, tt.expected)
			}
		})
	}
}
