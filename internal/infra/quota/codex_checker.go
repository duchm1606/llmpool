package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
)

const (
	defaultCodexUsageURL = "https://chatgpt.com/backend-api/wham/usage"
)

// CodexCheckerConfig holds configuration for the Codex/OpenAI checker.
type CodexCheckerConfig struct {
	UsageURL string        // Usage API URL (defaults to ChatGPT usage endpoint)
	Timeout  time.Duration // HTTP request timeout
}

// DefaultCodexCheckerConfig returns default configuration.
func DefaultCodexCheckerConfig() CodexCheckerConfig {
	return CodexCheckerConfig{
		UsageURL: defaultCodexUsageURL,
		Timeout:  10 * time.Second,
	}
}

// CodexChecker performs liveness checks against Codex OAuth usage API.
type CodexChecker struct {
	client *http.Client
	cfg    CodexCheckerConfig
}

type codexUsageResponse struct {
	RateLimit struct {
		PrimaryWindow struct {
			UsedPercent *float64 `json:"used_percent"`
		} `json:"primary_window"`
		SecondaryWindow struct {
			UsedPercent *float64 `json:"used_percent"`
		} `json:"secondary_window"`
	} `json:"rate_limit"`
	Credits struct {
		HasCredits *bool `json:"has_credits"`
		Unlimited  *bool `json:"unlimited"`
	} `json:"credits"`
}

// NewCodexChecker creates a new Codex/OpenAI liveness checker.
func NewCodexChecker(cfg CodexCheckerConfig) *CodexChecker {
	defaults := DefaultCodexCheckerConfig()
	if strings.TrimSpace(cfg.UsageURL) == "" {
		cfg.UsageURL = defaults.UsageURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}

	return &CodexChecker{
		client: &http.Client{Timeout: cfg.Timeout},
		cfg:    cfg,
	}
}

// Check performs a lightweight liveness check by calling the Codex usage endpoint.
// For OAuth tokens, this endpoint is more reliable than /models for auth validation.
func (c *CodexChecker) Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error) {
	result := domainquota.CheckResult{
		CheckedAt: time.Now(),
		Quota:     domainquota.NewQuotaInfoUnknown(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(c.cfg.UsageURL), nil)
	if err != nil {
		return result, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "llmpool/1.0")
	if strings.TrimSpace(accountID) != "" {
		req.Header.Set("ChatGPT-Account-Id", strings.TrimSpace(accountID))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return result, fmt.Errorf("read response body: %w", readErr)
	}

	result.Quota = c.extractQuotaFromHeaders(resp.Header)
	usageQuota, usageKnown, exhausted := c.extractQuotaFromUsageBody(body)
	if usageKnown {
		if !result.Quota.IsKnown() || usageQuota.Ratio < result.Quota.Ratio {
			result.Quota = usageQuota
		}
	}
	result = c.mapStatusToResult(result, resp.StatusCode)

	if resp.StatusCode == http.StatusOK && exhausted {
		result.Healthy = false
		result.ErrorCode = http.StatusTooManyRequests
		result.ErrorMessage = "rate_limited"
		if !result.Quota.IsKnown() {
			result.Quota = domainquota.NewQuotaInfo(0, 1)
		}
	}

	return result, nil
}

func (c *CodexChecker) extractQuotaFromUsageBody(body []byte) (domainquota.QuotaInfo, bool, bool) {
	if len(body) == 0 {
		return domainquota.NewQuotaInfoUnknown(), false, false
	}

	var usage codexUsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return domainquota.NewQuotaInfoUnknown(), false, false
	}

	exhausted := false
	usedPercent, hasPercent := firstNonNilFloat64(
		usage.RateLimit.PrimaryWindow.UsedPercent,
		usage.RateLimit.SecondaryWindow.UsedPercent,
	)

	if isQuotaExhaustedFromCredits(usage.Credits.Unlimited, usage.Credits.HasCredits) {
		exhausted = true
	}

	if !hasPercent {
		return domainquota.NewQuotaInfoUnknown(), false, exhausted
	}

	used := math.Max(0, math.Min(100, usedPercent))
	if used >= 100 {
		exhausted = true
	}

	remaining := int64(math.Round((1 - used/100) * 1000))
	if remaining < 0 {
		remaining = 0
	}

	return domainquota.NewQuotaInfo(remaining, 1000), true, exhausted
}

func firstNonNilFloat64(vals ...*float64) (float64, bool) {
	for _, v := range vals {
		if v != nil {
			return *v, true
		}
	}
	return 0, false
}

func isQuotaExhaustedFromCredits(unlimited, hasCredits *bool) bool {
	if unlimited != nil && *unlimited {
		return false
	}
	if hasCredits != nil {
		return !*hasCredits
	}
	return false
}

// extractQuotaFromHeaders extracts rate limit information from response headers.
func (c *CodexChecker) extractQuotaFromHeaders(headers http.Header) domainquota.QuotaInfo {
	remaining := c.parseHeaderInt64(headers, "x-ratelimit-remaining-requests")
	limit := c.parseHeaderInt64(headers, "x-ratelimit-limit-requests")

	if remaining < 0 || limit < 0 {
		remaining = c.parseHeaderInt64(headers, "x-ratelimit-remaining-tokens")
		limit = c.parseHeaderInt64(headers, "x-ratelimit-limit-tokens")
	}

	if remaining >= 0 && limit > 0 {
		return domainquota.NewQuotaInfo(remaining, limit)
	}

	return domainquota.NewQuotaInfoUnknown()
}

func (c *CodexChecker) parseHeaderInt64(headers http.Header, key string) int64 {
	val := headers.Get(key)
	if val == "" {
		return -1
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func (c *CodexChecker) mapStatusToResult(result domainquota.CheckResult, statusCode int) domainquota.CheckResult {
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
