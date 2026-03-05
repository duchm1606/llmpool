package quota

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"go.uber.org/zap"
)

const (
	defaultSamplePercent = 0.20 // 20% sample
	defaultStateTTL      = 2 * time.Hour
	minSampleSize        = 1
)

// ServiceConfig holds configuration for the liveness service.
type ServiceConfig struct {
	SamplePercent float64
	StateTTL      time.Duration
	Cooldown      domainquota.CooldownConfig
}

// DefaultServiceConfig returns default service configuration.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		SamplePercent: defaultSamplePercent,
		StateTTL:      defaultStateTTL,
		Cooldown:      domainquota.DefaultCooldownConfig(),
	}
}

// Service implements LivenessService.
type Service struct {
	repo                CredentialRepository
	encryptor           Encryptor
	checker             ProviderChecker
	cache               StateCache
	logger              *zap.Logger
	cfg                 ServiceConfig
	copilotUsageFetcher CopilotUsageFetcher // Optional: for fetching full copilot usage
}

type authPayload struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

// NewService creates a new liveness service.
func NewService(
	repo CredentialRepository,
	encryptor Encryptor,
	checker ProviderChecker,
	cache StateCache,
	logger *zap.Logger,
	cfg ServiceConfig,
) *Service {
	return &Service{
		repo:      repo,
		encryptor: encryptor,
		checker:   checker,
		cache:     cache,
		logger:    logger,
		cfg:       cfg,
	}
}

// SetCopilotUsageFetcher sets the optional copilot usage fetcher.
func (s *Service) SetCopilotUsageFetcher(fetcher CopilotUsageFetcher) {
	s.copilotUsageFetcher = fetcher
}

// CheckSample performs a sample-based liveness check (20% of credentials).
func (s *Service) CheckSample(ctx context.Context) error {
	count, err := s.repo.CountEnabled(ctx)
	if err != nil {
		return err
	}

	if count == 0 {
		s.logger.Debug("no enabled credentials to check")
		return nil
	}

	sampleSize := int(float64(count) * s.cfg.SamplePercent)
	if sampleSize < minSampleSize {
		sampleSize = minSampleSize
	}

	seed := time.Now().UnixNano()
	profiles, err := s.repo.RandomSample(ctx, sampleSize, seed)
	if err != nil {
		return err
	}

	s.logger.Info("checking sample",
		zap.Int("sample_size", len(profiles)),
		zap.Int64("total_enabled", count),
	)

	return s.checkProfiles(ctx, profiles)
}

// CheckAll performs a full sweep of all credentials.
func (s *Service) CheckAll(ctx context.Context) error {
	profiles, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}

	s.logger.Info("checking all credentials", zap.Int("count", len(profiles)))
	return s.checkProfiles(ctx, profiles)
}

// CheckCredential performs a liveness check for a single credential.
func (s *Service) CheckCredential(ctx context.Context, credentialID string) error {
	// For single check, we need to get the credential from the full list
	// In a real implementation, we'd have GetByID in the repo
	profiles, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}

	for _, p := range profiles {
		if p.ID == credentialID {
			return s.checkProfiles(ctx, []domaincredential.Profile{p})
		}
	}

	s.logger.Warn("credential not found for check", zap.String("credential_id", credentialID))
	return nil
}

// GetCredentialState retrieves current state for a credential.
func (s *Service) GetCredentialState(ctx context.Context, credentialID string) (*domainquota.CredentialState, error) {
	return s.cache.GetCredentialState(ctx, credentialID)
}

// NeedsRehydration returns true if cache is empty/unavailable and needs full sweep.
func (s *Service) NeedsRehydration(ctx context.Context) (bool, error) {
	if err := s.cache.Ping(ctx); err != nil {
		s.logger.Warn("cache unavailable, needs rehydration", zap.Error(err))
		return true, nil
	}

	count, err := s.cache.CountCredentialStates(ctx)
	if err != nil {
		s.logger.Warn("failed to count cache states", zap.Error(err))
		return true, nil
	}

	if count == 0 {
		s.logger.Info("cache empty, needs rehydration")
		return true, nil
	}

	return false, nil
}

// checkProfiles checks multiple profiles with limited concurrency.
func (s *Service) checkProfiles(ctx context.Context, profiles []domaincredential.Profile) error {
	const maxConcurrency = 10

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, p := range profiles {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(profile domaincredential.Profile) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.checkSingleProfile(ctx, profile); err != nil {
				s.logger.Error("check profile failed",
					zap.String("credential_id", profile.ID),
					zap.Error(err),
				)
			}
		}(p)
	}

	wg.Wait()
	return nil
}

// checkSingleProfile checks a single profile and updates cache.
func (s *Service) checkSingleProfile(ctx context.Context, profile domaincredential.Profile) error {
	s.logger.Debug("starting credential liveness check",
		zap.String("credential_id", profile.ID),
		zap.String("provider", profile.Type),
		zap.String("account_id", profile.AccountID),
		zap.String("email", profile.Email),
	)

	// Decrypt credential payload
	payload, err := s.extractAuthPayload(profile.EncryptedProfile)
	if err != nil {
		s.logger.Error("decrypt credential failed",
			zap.String("credential_id", profile.ID),
			zap.String("provider", profile.Type),
			zap.String("account_id", profile.AccountID),
			zap.String("email", profile.Email),
			zap.Error(err),
		)
		// Mark as unhealthy if we can't decrypt
		state := domainquota.CredentialState{
			CredentialID:  profile.ID,
			Status:        domainquota.StatusUnhealthy,
			LastCheckedAt: time.Now(),
			ErrorMessage:  "decrypt_failed",
		}
		return s.cache.SetCredentialState(ctx, state, s.cfg.StateTTL)
	}

	if strings.TrimSpace(payload.AccessToken) == "" {
		// Keep validation for non-Copilot providers in place.
		// Copilot liveness checks use GitHub token (refresh_token) when available.
		if profile.Type != "copilot" {
			s.logger.Warn("credential has empty access token",
				zap.String("credential_id", profile.ID),
				zap.String("provider", profile.Type),
				zap.String("account_id", profile.AccountID),
				zap.String("email", profile.Email),
			)
			state := domainquota.CredentialState{
				CredentialID:  profile.ID,
				Status:        domainquota.StatusUnhealthy,
				LastCheckedAt: time.Now(),
				ErrorMessage:  "missing_access_token",
			}
			return s.cache.SetCredentialState(ctx, state, s.cfg.StateTTL)
		}
	}

	checkToken, tokenKind := selectCheckToken(profile.Type, payload)
	if strings.TrimSpace(checkToken) == "" {
		s.logger.Warn("credential missing token for liveness check",
			zap.String("credential_id", profile.ID),
			zap.String("provider", profile.Type),
			zap.String("account_id", profile.AccountID),
			zap.String("email", profile.Email),
			zap.String("expected_token", tokenKind),
		)
		state := domainquota.CredentialState{
			CredentialID:  profile.ID,
			Status:        domainquota.StatusUnhealthy,
			LastCheckedAt: time.Now(),
			ErrorMessage:  "missing_access_token",
		}
		return s.cache.SetCredentialState(ctx, state, s.cfg.StateTTL)
	}

	accountID := strings.TrimSpace(payload.AccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(profile.AccountID)
	}

	// Perform provider check with retry logic
	result, err := s.checkWithRetry(ctx, profile.Type, profile.ID, checkToken, accountID)
	if err != nil {
		s.logger.Error("check with retry failed",
			zap.String("credential_id", profile.ID),
			zap.String("provider", profile.Type),
			zap.String("account_id", profile.AccountID),
			zap.String("email", profile.Email),
			zap.Error(err),
		)
	}

	// Build state from result
	state := s.buildStateFromResult(ctx, profile.ID, result, err)

	// Update cache
	if cacheErr := s.cache.SetCredentialState(ctx, state, s.cfg.StateTTL); cacheErr != nil {
		s.logger.Error("update cache failed",
			zap.String("credential_id", profile.ID),
			zap.String("provider", profile.Type),
			zap.String("account_id", profile.AccountID),
			zap.String("email", profile.Email),
			zap.Error(cacheErr),
		)
	}

	s.logger.Debug("credential liveness check completed",
		zap.String("credential_id", profile.ID),
		zap.String("provider", profile.Type),
		zap.String("account_id", profile.AccountID),
		zap.String("email", profile.Email),
		zap.String("status", string(state.Status)),
		zap.Int("error_code", state.ErrorCode),
		zap.String("error_message", state.ErrorMessage),
		zap.Time("last_checked_at", state.LastCheckedAt),
		zap.Time("cooldown_until", state.CooldownUntil),
		zap.Int64("quota_remaining", state.Quota.Remaining),
		zap.Int64("quota_limit", state.Quota.Limit),
		zap.Float64("quota_ratio", state.Quota.Ratio),
	)

	// Update per-model states if available
	for _, modelResult := range result.Models {
		modelState := domainquota.ModelState{
			CredentialID:  profile.ID,
			ModelID:       modelResult.ModelID,
			Status:        domainquota.StatusHealthy,
			LastCheckedAt: result.CheckedAt,
			Quota:         modelResult.Quota,
		}
		if !modelResult.Healthy {
			modelState.Status = domainquota.StatusUnhealthy
			modelState.ErrorCode = modelResult.ErrorCode
			modelState.ErrorMessage = modelResult.ErrorMessage
		}
		if cacheErr := s.cache.SetModelState(ctx, modelState, s.cfg.StateTTL); cacheErr != nil {
			s.logger.Error("update model cache failed",
				zap.String("credential_id", profile.ID),
				zap.String("provider", profile.Type),
				zap.String("account_id", profile.AccountID),
				zap.String("email", profile.Email),
				zap.String("model_id", modelResult.ModelID),
				zap.Error(cacheErr),
			)
		}
	}

	// Fetch and cache full Copilot usage for copilot credentials
	if profile.Type == "copilot" && s.copilotUsageFetcher != nil && result.Healthy {
		s.fetchAndCacheCopilotUsage(ctx, profile.ID, checkToken)
	}

	return nil
}

// selectCheckToken chooses which token should be used for provider liveness checks.
// For Copilot usage API checks, GitHub token (refresh_token) is preferred.
func selectCheckToken(profileType string, payload authPayload) (token, kind string) {
	if profileType == "copilot" {
		if refresh := strings.TrimSpace(payload.RefreshToken); refresh != "" {
			return refresh, "refresh_token"
		}
		return strings.TrimSpace(payload.AccessToken), "access_token"
	}

	return strings.TrimSpace(payload.AccessToken), "access_token"
}

// extractAuthPayload decrypts the credential payload and extracts auth fields.
func (s *Service) extractAuthPayload(encryptedProfile string) (authPayload, error) {
	decrypted, err := s.encryptor.Decrypt(encryptedProfile)
	if err != nil {
		return authPayload{}, err
	}

	var payload authPayload
	if err := json.Unmarshal([]byte(decrypted), &payload); err != nil {
		return authPayload{}, err
	}

	return payload, nil
}

// checkWithRetry performs check with retry logic for network errors.
func (s *Service) checkWithRetry(ctx context.Context, credType, credID, accessToken, accountID string) (domainquota.CheckResult, error) {
	var lastErr error
	var result domainquota.CheckResult

	for attempt := 0; attempt <= s.cfg.Cooldown.NetworkMaxRetries; attempt++ {
		result, lastErr = s.checker.Check(ctx, credType, accessToken, accountID)
		if lastErr == nil {
			return result, nil
		}

		// Check if it's a retriable network error
		if !isNetworkError(lastErr) {
			return result, lastErr
		}

		if attempt < s.cfg.Cooldown.NetworkMaxRetries {
			// Add jitter: 100-500ms (weak random is acceptable for jitter)
			jitter := time.Duration(100+rand.IntN(400)) * time.Millisecond //nolint:gosec // Jitter doesn't need crypto random
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(jitter):
			}
		}
	}

	return result, lastErr
}

// buildStateFromResult creates a CredentialState from check result.
// ctx is used for retrieving existing state during rate-limit backoff calculation.
func (s *Service) buildStateFromResult(ctx context.Context, credID string, result domainquota.CheckResult, checkErr error) domainquota.CredentialState {
	now := time.Now()
	state := domainquota.CredentialState{
		CredentialID:  credID,
		LastCheckedAt: now,
		Quota:         result.Quota, // Persist quota from check result
	}

	// Update deprecated AvailableQuota for backwards compatibility
	if result.Quota.IsKnown() {
		state.AvailableQuota = result.Quota.Remaining
	} else {
		state.AvailableQuota = -1
	}

	if checkErr != nil && isNetworkError(checkErr) {
		// Network error after retries
		state.Status = domainquota.StatusCooldown
		state.CooldownUntil = now.Add(s.cfg.Cooldown.NetworkErrorCooldown)
		state.ErrorMessage = "network_error"
		return state
	}

	if result.Healthy {
		state.Status = domainquota.StatusHealthy
		// Reset retry count on success
		state.RetryCount = 0
		return state
	}

	// Handle error codes
	state.ErrorCode = result.ErrorCode
	state.ErrorMessage = result.ErrorMessage

	switch result.ErrorCode {
	case 401, 403:
		// Auth failure - long cooldown
		state.Status = domainquota.StatusCooldown
		state.CooldownUntil = now.Add(s.cfg.Cooldown.AuthFailureCooldown)
	case 429:
		// Rate limit - exponential backoff
		state.Status = domainquota.StatusCooldown
		// Get existing retry count from cache to continue backoff
		existingState, _ := s.cache.GetCredentialState(ctx, credID)
		retryCount := 0
		if existingState != nil && existingState.ErrorCode == 429 {
			retryCount = existingState.RetryCount + 1
		}
		state.RetryCount = retryCount
		cooldown := domainquota.CalculateRateLimitCooldown(retryCount, s.cfg.Cooldown)
		state.CooldownUntil = now.Add(cooldown)
	default:
		// Other errors - mark unhealthy
		state.Status = domainquota.StatusUnhealthy
	}

	return state
}

// isNetworkError checks if the error is a network-related error using proper error unwrapping.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for net.Error (timeout, temporary errors)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check for URL errors (which wrap network errors)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// URL errors wrap the underlying error
		return isNetworkError(urlErr.Err)
	}

	// Check for common syscall errors
	var sysErr syscall.Errno
	if errors.As(err, &sysErr) {
		switch sysErr {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED,
			syscall.ENETUNREACH, syscall.EHOSTUNREACH, syscall.ETIMEDOUT:
			return true
		}
	}

	// Check for OpError (lower level network operations)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	return false
}

// fetchAndCacheCopilotUsage fetches full Copilot usage and caches it.
func (s *Service) fetchAndCacheCopilotUsage(ctx context.Context, credentialID, accessToken string) {
	usage, err := s.copilotUsageFetcher.FetchUsage(ctx, credentialID, accessToken)
	if err != nil {
		s.logger.Warn("failed to fetch copilot usage",
			zap.String("credential_id", credentialID),
			zap.Error(err),
		)
		return
	}

	if err := s.cache.SetCopilotUsage(ctx, *usage, s.cfg.StateTTL); err != nil {
		s.logger.Warn("failed to cache copilot usage",
			zap.String("credential_id", credentialID),
			zap.Error(err),
		)
	} else {
		s.logger.Debug("cached copilot usage",
			zap.String("credential_id", credentialID),
			zap.String("login", usage.Login),
			zap.String("copilot_plan", usage.CopilotPlan),
			zap.String("quota_reset_date", usage.QuotaResetDate),
		)
	}
}
