package quota

import "time"

// CredentialStatus represents the liveness status of a credential.
type CredentialStatus string

const (
	StatusUnknown   CredentialStatus = "unknown"
	StatusHealthy   CredentialStatus = "healthy"
	StatusUnhealthy CredentialStatus = "unhealthy"
	StatusCooldown  CredentialStatus = "cooldown"
)

// QuotaUnknown indicates that quota information is not available.
const QuotaUnknown float64 = -1

// QuotaInfo holds quota data suitable for load balancing decisions.
// All fields are cache-only; extracted from provider response headers.
type QuotaInfo struct {
	Remaining int64   // Remaining requests/tokens; -1 if unknown
	Limit     int64   // Total limit; -1 if unknown
	Ratio     float64 // Normalized ratio [0,1]; -1 if unknown
}

// NewQuotaInfoUnknown returns a QuotaInfo with all fields set to unknown.
func NewQuotaInfoUnknown() QuotaInfo {
	return QuotaInfo{
		Remaining: -1,
		Limit:     -1,
		Ratio:     QuotaUnknown,
	}
}

// NewQuotaInfo creates a QuotaInfo and computes the normalized ratio.
// If limit <= 0, ratio is set to -1 (unknown).
func NewQuotaInfo(remaining, limit int64) QuotaInfo {
	q := QuotaInfo{
		Remaining: remaining,
		Limit:     limit,
		Ratio:     QuotaUnknown,
	}
	if limit > 0 {
		q.Ratio = float64(remaining) / float64(limit)
		if q.Ratio < 0 {
			q.Ratio = 0
		}
		if q.Ratio > 1 {
			q.Ratio = 1
		}
	}
	return q
}

// IsKnown returns true if quota information is available.
func (q QuotaInfo) IsKnown() bool {
	return q.Ratio >= 0
}

// CredentialState represents the full liveness/quota state of a credential in cache.
type CredentialState struct {
	CredentialID    string
	Status          CredentialStatus
	LastCheckedAt   time.Time
	CooldownUntil   time.Time // Zero if not in cooldown
	RetryCount      int       // For exponential backoff
	ErrorCode       int       // Last error code (401, 403, 429, etc.)
	ErrorMessage    string
	AvailableQuota  int64 // -1 if unknown (deprecated: use Quota)
	Quota           QuotaInfo
	QuotaDetail     *CopilotQuotaSnapshots
	AccessTokenHash string
}

// IsAvailable returns true if credential can be used for requests.
func (s CredentialState) IsAvailable(now time.Time) bool {
	if s.Status == StatusUnhealthy {
		return false
	}
	if s.Status == StatusCooldown && now.Before(s.CooldownUntil) {
		return false
	}
	return true
}

// ModelState represents per-model quota/liveness state for a credential.
type ModelState struct {
	CredentialID  string
	ModelID       string
	Status        CredentialStatus
	LastCheckedAt time.Time
	CooldownUntil time.Time
	RetryCount    int
	ErrorCode     int
	ErrorMessage  string
	Quota         QuotaInfo
}

// IsAvailable returns true if the model can be used with this credential.
func (m ModelState) IsAvailable(now time.Time) bool {
	if m.Status == StatusUnhealthy {
		return false
	}
	if m.Status == StatusCooldown && now.Before(m.CooldownUntil) {
		return false
	}
	return true
}

// CheckResult represents the result of a single liveness check.
type CheckResult struct {
	CredentialID string
	Healthy      bool
	CheckedAt    time.Time
	ErrorCode    int
	ErrorMessage string
	Quota        QuotaInfo // Quota info extracted from provider response
	QuotaDetail  *CopilotQuotaSnapshots
	Models       []ModelCheckResult // Per-model results if available
}

// ModelCheckResult represents per-model check result.
type ModelCheckResult struct {
	ModelID      string
	Healthy      bool
	ErrorCode    int
	ErrorMessage string
	Quota        QuotaInfo
}

// CooldownConfig holds default cooldown durations.
type CooldownConfig struct {
	AuthFailureCooldown  time.Duration // 401/403 cooldown (default 30m)
	RateLimitInitial     time.Duration // 429 initial backoff (default 2m)
	RateLimitMaxCooldown time.Duration // 429 max backoff (default 30m)
	NetworkErrorCooldown time.Duration // Network error cooldown after retries (default 5m)
	NetworkMaxRetries    int           // Max retries for network errors (default 3)
}

// DefaultCooldownConfig returns default cooldown configuration.
func DefaultCooldownConfig() CooldownConfig {
	return CooldownConfig{
		AuthFailureCooldown:  30 * time.Minute,
		RateLimitInitial:     2 * time.Minute,
		RateLimitMaxCooldown: 30 * time.Minute,
		NetworkErrorCooldown: 5 * time.Minute,
		NetworkMaxRetries:    3,
	}
}

// CalculateRateLimitCooldown calculates exponential backoff for 429 errors.
func CalculateRateLimitCooldown(retryCount int, cfg CooldownConfig) time.Duration {
	// 2^retryCount * initial, capped at max
	multiplier := 1 << retryCount // 2^retryCount
	cooldown := time.Duration(multiplier) * cfg.RateLimitInitial
	if cooldown > cfg.RateLimitMaxCooldown {
		cooldown = cfg.RateLimitMaxCooldown
	}
	return cooldown
}

// CopilotQuotaSnapshot represents quota details for a specific usage type.
// Matches the GitHub Copilot internal API response structure.
type CopilotQuotaSnapshot struct {
	Entitlement      int64   `json:"entitlement"`
	OverageCount     int64   `json:"overage_count"`
	OveragePermitted bool    `json:"overage_permitted"`
	PercentRemaining float64 `json:"percent_remaining"`
	QuotaID          string  `json:"quota_id"`
	QuotaRemaining   float64 `json:"quota_remaining"`
	Remaining        int64   `json:"remaining"`
	Unlimited        bool    `json:"unlimited"`
	TimestampUTC     string  `json:"timestamp_utc,omitempty"`
}

// CopilotQuotaSnapshots holds all quota types.
type CopilotQuotaSnapshots struct {
	Chat                *CopilotQuotaSnapshot `json:"chat,omitempty"`
	Completions         *CopilotQuotaSnapshot `json:"completions,omitempty"`
	PremiumInteractions *CopilotQuotaSnapshot `json:"premium_interactions,omitempty"`
}

// CopilotEndpoints holds the API endpoints for a Copilot account.
type CopilotEndpoints struct {
	API           string `json:"api,omitempty"`
	OriginTracker string `json:"origin-tracker,omitempty"`
	Proxy         string `json:"proxy,omitempty"`
	Telemetry     string `json:"telemetry,omitempty"`
}

// CopilotUsage represents the full usage response from GitHub Copilot API.
// This mirrors the response from https://api.github.com/copilot_internal/user
type CopilotUsage struct {
	// Credential identification
	CredentialID string `json:"credential_id"`

	// User info
	Login string `json:"login,omitempty"`

	// Account info
	AccessTypeSKU        string `json:"access_type_sku,omitempty"`
	AnalyticsTrackingID  string `json:"analytics_tracking_id,omitempty"`
	AssignedDate         string `json:"assigned_date,omitempty"`
	CanSignupForLimited  bool   `json:"can_signup_for_limited"`
	ChatEnabled          bool   `json:"chat_enabled"`
	CopilotIgnoreEnabled bool   `json:"copilotignore_enabled"`
	CopilotPlan          string `json:"copilot_plan,omitempty"`
	IsMCPEnabled         bool   `json:"is_mcp_enabled"`
	RestrictedTelemetry  bool   `json:"restricted_telemetry"`

	// Organization info
	OrganizationLoginList []string `json:"organization_login_list,omitempty"`

	// Endpoints
	Endpoints *CopilotEndpoints `json:"endpoints,omitempty"`

	// Quota info
	QuotaResetDate    string                 `json:"quota_reset_date,omitempty"`
	QuotaResetDateUTC string                 `json:"quota_reset_date_utc,omitempty"`
	QuotaSnapshots    *CopilotQuotaSnapshots `json:"quota_snapshots,omitempty"`

	// Metadata
	FetchedAt time.Time `json:"fetched_at"`
}
