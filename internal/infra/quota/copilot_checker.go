package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

const (
	defaultCopilotUsageURL = "https://api.github.com/copilot_internal/user"
)

// CopilotCheckerConfig holds configuration for the Copilot/GitHub checker.
type CopilotCheckerConfig struct {
	UsageURL string        // Usage API URL (defaults to GitHub Copilot internal user endpoint)
	Timeout  time.Duration // HTTP request timeout
}

// DefaultCopilotCheckerConfig returns default configuration.
func DefaultCopilotCheckerConfig() CopilotCheckerConfig {
	return CopilotCheckerConfig{
		UsageURL: defaultCopilotUsageURL,
		Timeout:  10 * time.Second,
	}
}

// CopilotChecker performs liveness checks against GitHub Copilot usage API.
type CopilotChecker struct {
	client *http.Client
	cfg    CopilotCheckerConfig
}

// copilotUsageResponse represents the full GitHub Copilot internal user API response.
// Based on reference implementation in copilot-api.
type copilotUsageResponse struct {
	// User info
	Login string `json:"login"`

	// Account info
	AccessTypeSKU        string `json:"access_type_sku"`
	AnalyticsTrackingID  string `json:"analytics_tracking_id"`
	AssignedDate         string `json:"assigned_date"`
	CanSignupForLimited  bool   `json:"can_signup_for_limited"`
	ChatEnabled          bool   `json:"chat_enabled"`
	CopilotIgnoreEnabled bool   `json:"copilotignore_enabled"`
	CopilotPlan          string `json:"copilot_plan"`
	IsMCPEnabled         bool   `json:"is_mcp_enabled"`
	RestrictedTelemetry  bool   `json:"restricted_telemetry"`

	// Organization info
	OrganizationLoginList []string `json:"organization_login_list"`

	// Endpoints
	Endpoints *copilotEndpoints `json:"endpoints"`

	// Quota info
	QuotaResetDate    string               `json:"quota_reset_date"`
	QuotaResetDateUTC string               `json:"quota_reset_date_utc"`
	QuotaSnapshots    *copilotQuotaDetails `json:"quota_snapshots"`
}

// copilotEndpoints represents the API endpoints for a Copilot account.
type copilotEndpoints struct {
	API           string `json:"api"`
	OriginTracker string `json:"origin-tracker"`
	Proxy         string `json:"proxy"`
	Telemetry     string `json:"telemetry"`
}

// copilotQuotaDetails holds all quota types.
type copilotQuotaDetails struct {
	Chat                *copilotQuotaDetail `json:"chat"`
	Completions         *copilotQuotaDetail `json:"completions"`
	PremiumInteractions *copilotQuotaDetail `json:"premium_interactions"`
}

// copilotQuotaDetail represents quota details for a specific usage type.
type copilotQuotaDetail struct {
	Entitlement      int64    `json:"entitlement"`
	OverageCount     int64    `json:"overage_count"`
	OveragePermitted bool     `json:"overage_permitted"`
	PercentRemaining *float64 `json:"percent_remaining"`
	QuotaID          string   `json:"quota_id"`
	QuotaRemaining   int64    `json:"quota_remaining"`
	Remaining        int64    `json:"remaining"`
	Unlimited        *bool    `json:"unlimited"`
	TimestampUTC     string   `json:"timestamp_utc"`
}

// NewCopilotChecker creates a new Copilot/GitHub liveness checker.
func NewCopilotChecker(cfg CopilotCheckerConfig) *CopilotChecker {
	defaults := DefaultCopilotCheckerConfig()
	if strings.TrimSpace(cfg.UsageURL) == "" {
		cfg.UsageURL = defaults.UsageURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}

	return &CopilotChecker{
		client: &http.Client{Timeout: cfg.Timeout},
		cfg:    cfg,
	}
}

// Check performs a liveness check by calling the GitHub Copilot internal user endpoint.
// The accessToken is expected to be a GitHub personal access token or OAuth token.
func (c *CopilotChecker) Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error) {
	result := domainquota.CheckResult{
		CheckedAt: time.Now(),
		Quota:     domainquota.NewQuotaInfoUnknown(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(c.cfg.UsageURL), nil)
	if err != nil {
		return result, fmt.Errorf("build request: %w", err)
	}

	// Set headers following the reference implementation (proxypal)
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "llmpool/1.0")
	req.Header.Set("Editor-Version", "vscode/1.91.1")
	req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.26.7")
	req.Header.Set("X-Github-Api-Version", "2025-04-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return result, fmt.Errorf("read response body: %w", readErr)
	}

	// Map HTTP status to result
	result = c.mapStatusToResult(result, resp.StatusCode)

	// If successful, parse quota from response body
	if resp.StatusCode == http.StatusOK {
		quotaInfo, exhausted := c.extractQuotaFromUsageBody(body)
		result.Quota = quotaInfo

		if exhausted {
			result.Healthy = false
			result.ErrorCode = http.StatusTooManyRequests
			result.ErrorMessage = "rate_limited"
			if !result.Quota.IsKnown() {
				result.Quota = domainquota.NewQuotaInfo(0, 1)
			}
		}
	}

	return result, nil
}

// extractQuotaFromUsageBody parses the Copilot usage response and extracts quota information.
// Returns quota info and whether the account is exhausted.
func (c *CopilotChecker) extractQuotaFromUsageBody(body []byte) (domainquota.QuotaInfo, bool) {
	if len(body) == 0 {
		return domainquota.NewQuotaInfoUnknown(), false
	}

	var usage copilotUsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return domainquota.NewQuotaInfoUnknown(), false
	}

	// Handle nil QuotaSnapshots
	if usage.QuotaSnapshots == nil {
		return domainquota.NewQuotaInfoUnknown(), false
	}

	// Priority: premium_interactions > chat > completions
	// These represent the most constrained quotas
	quota := c.extractBestQuota(
		usage.QuotaSnapshots.PremiumInteractions,
		usage.QuotaSnapshots.Chat,
		usage.QuotaSnapshots.Completions,
	)

	exhausted := c.isExhausted(usage.QuotaSnapshots.PremiumInteractions, usage.QuotaSnapshots.Chat)

	return quota, exhausted
}

// extractBestQuota extracts the most restrictive quota from available quota details.
// Uses percent_remaining when available, otherwise computes from remaining/entitlement.
func (c *CopilotChecker) extractBestQuota(details ...*copilotQuotaDetail) domainquota.QuotaInfo {
	var bestQuota domainquota.QuotaInfo
	bestRatio := float64(2) // Start with impossible high ratio

	for _, detail := range details {
		if detail == nil {
			continue
		}

		// Skip unlimited quotas
		if detail.Unlimited != nil && *detail.Unlimited {
			continue
		}

		var remaining, limit int64
		var ratio float64

		if detail.PercentRemaining != nil {
			// Use percent_remaining directly
			percentRemaining := math.Max(0, math.Min(100, *detail.PercentRemaining))
			ratio = percentRemaining / 100
			// Normalize to a 1000-unit scale for consistency
			remaining = int64(math.Round(ratio * 1000))
			limit = 1000
		} else if detail.Entitlement > 0 {
			// Compute from entitlement and remaining
			remaining = detail.Remaining
			limit = detail.Entitlement
			if remaining < 0 {
				remaining = 0
			}
			ratio = float64(remaining) / float64(limit)
		} else {
			continue
		}

		// Use the most restrictive quota (lowest ratio)
		if ratio < bestRatio {
			bestRatio = ratio
			bestQuota = domainquota.NewQuotaInfo(remaining, limit)
		}
	}

	if bestRatio > 1 {
		return domainquota.NewQuotaInfoUnknown()
	}

	return bestQuota
}

// isExhausted checks if the account has exhausted its quota.
func (c *CopilotChecker) isExhausted(premium, chat *copilotQuotaDetail) bool {
	// Check premium interactions first (most common constraint)
	if premium != nil {
		if premium.Unlimited != nil && *premium.Unlimited {
			return false
		}
		if premium.PercentRemaining != nil && *premium.PercentRemaining <= 0 {
			return true
		}
		if premium.Entitlement > 0 && premium.Remaining <= 0 {
			return true
		}
	}

	// Check chat quota
	if chat != nil {
		if chat.Unlimited != nil && *chat.Unlimited {
			return false
		}
		if chat.PercentRemaining != nil && *chat.PercentRemaining <= 0 {
			return true
		}
		if chat.Entitlement > 0 && chat.Remaining <= 0 {
			return true
		}
	}

	return false
}

func (c *CopilotChecker) mapStatusToResult(result domainquota.CheckResult, statusCode int) domainquota.CheckResult {
	switch statusCode {
	case http.StatusOK:
		result.Healthy = true
	case http.StatusUnauthorized:
		result.Healthy = false
		result.ErrorCode = http.StatusUnauthorized
		result.ErrorMessage = "unauthorized"
	case http.StatusForbidden:
		result.Healthy = false
		result.ErrorCode = http.StatusForbidden
		result.ErrorMessage = "forbidden"
	case http.StatusTooManyRequests:
		result.Healthy = false
		result.ErrorCode = http.StatusTooManyRequests
		result.ErrorMessage = "rate_limited"
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		result.Healthy = false
		result.ErrorCode = statusCode
		result.ErrorMessage = "server_error"
	default:
		if statusCode >= 200 && statusCode < 300 {
			result.Healthy = true
		} else {
			result.Healthy = false
			result.ErrorCode = statusCode
			result.ErrorMessage = fmt.Sprintf("http_%d", statusCode)
		}
	}

	return result
}

// FetchUsage fetches the full Copilot usage information for a credential.
// This returns the complete usage response including login, quota_reset_date, and all quota snapshots.
func (c *CopilotChecker) FetchUsage(ctx context.Context, credentialID, accessToken string) (*domainquota.CopilotUsage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(c.cfg.UsageURL), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	// Set headers following the reference implementation (copilot-api)
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GitHubCopilotChat/0.26.7")
	req.Header.Set("Editor-Version", "vscode/1.91.1")
	req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.26.7")
	req.Header.Set("X-Github-Api-Version", "2025-04-01")
	req.Header.Set("X-VSCode-User-Agent-Library-Version", "electron-fetch")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("read response body: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var usage copilotUsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return c.toDomainUsage(credentialID, &usage), nil
}

// toDomainUsage converts the internal usage response to the domain model.
func (c *CopilotChecker) toDomainUsage(credentialID string, resp *copilotUsageResponse) *domainquota.CopilotUsage {
	usage := &domainquota.CopilotUsage{
		CredentialID:          credentialID,
		Login:                 resp.Login,
		AccessTypeSKU:         resp.AccessTypeSKU,
		AnalyticsTrackingID:   resp.AnalyticsTrackingID,
		AssignedDate:          resp.AssignedDate,
		CanSignupForLimited:   resp.CanSignupForLimited,
		ChatEnabled:           resp.ChatEnabled,
		CopilotIgnoreEnabled:  resp.CopilotIgnoreEnabled,
		CopilotPlan:           resp.CopilotPlan,
		IsMCPEnabled:          resp.IsMCPEnabled,
		RestrictedTelemetry:   resp.RestrictedTelemetry,
		OrganizationLoginList: resp.OrganizationLoginList,
		QuotaResetDate:        resp.QuotaResetDate,
		QuotaResetDateUTC:     resp.QuotaResetDateUTC,
		FetchedAt:             time.Now(),
	}

	// Convert endpoints
	if resp.Endpoints != nil {
		usage.Endpoints = &domainquota.CopilotEndpoints{
			API:           resp.Endpoints.API,
			OriginTracker: resp.Endpoints.OriginTracker,
			Proxy:         resp.Endpoints.Proxy,
			Telemetry:     resp.Endpoints.Telemetry,
		}
	}

	// Convert quota snapshots
	if resp.QuotaSnapshots != nil {
		usage.QuotaSnapshots = &domainquota.CopilotQuotaSnapshots{}

		if resp.QuotaSnapshots.Chat != nil {
			usage.QuotaSnapshots.Chat = c.toDomainQuotaSnapshot(resp.QuotaSnapshots.Chat)
		}
		if resp.QuotaSnapshots.Completions != nil {
			usage.QuotaSnapshots.Completions = c.toDomainQuotaSnapshot(resp.QuotaSnapshots.Completions)
		}
		if resp.QuotaSnapshots.PremiumInteractions != nil {
			usage.QuotaSnapshots.PremiumInteractions = c.toDomainQuotaSnapshot(resp.QuotaSnapshots.PremiumInteractions)
		}
	}

	return usage
}

// toDomainQuotaSnapshot converts internal quota detail to domain model.
func (c *CopilotChecker) toDomainQuotaSnapshot(detail *copilotQuotaDetail) *domainquota.CopilotQuotaSnapshot {
	snapshot := &domainquota.CopilotQuotaSnapshot{
		Entitlement:      detail.Entitlement,
		OverageCount:     detail.OverageCount,
		OveragePermitted: detail.OveragePermitted,
		QuotaID:          detail.QuotaID,
		QuotaRemaining:   detail.QuotaRemaining,
		Remaining:        detail.Remaining,
		TimestampUTC:     detail.TimestampUTC,
	}

	if detail.PercentRemaining != nil {
		snapshot.PercentRemaining = *detail.PercentRemaining
	}
	if detail.Unlimited != nil {
		snapshot.Unlimited = *detail.Unlimited
	}

	return snapshot
}
