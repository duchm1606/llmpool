package quota

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"go.uber.org/zap"
)

// Mock implementations for testing

type mockCredentialRepository struct {
	profiles      []domaincredential.Profile
	count         int64
	countErr      error
	listErr       error
	randomSamples []domaincredential.Profile
	randomErr     error
}

func (m *mockCredentialRepository) ListEnabled(ctx context.Context) ([]domaincredential.Profile, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.profiles, nil
}

func (m *mockCredentialRepository) CountEnabled(ctx context.Context) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.count, nil
}

func (m *mockCredentialRepository) RandomSample(ctx context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error) {
	if m.randomErr != nil {
		return nil, m.randomErr
	}
	return m.randomSamples, nil
}

type mockEncryptor struct {
	decrypted  string
	decryptErr error
}

func (m *mockEncryptor) Decrypt(cipher string) (string, error) {
	if m.decryptErr != nil {
		return "", m.decryptErr
	}
	return m.decrypted, nil
}

type mockProviderChecker struct {
	result   domainquota.CheckResult
	checkErr error

	lastCredentialType string
	lastAccessToken    string
	lastAccountID      string
}

func (m *mockProviderChecker) Check(ctx context.Context, credentialType, accessToken, accountID string) (domainquota.CheckResult, error) {
	m.lastCredentialType = credentialType
	m.lastAccessToken = accessToken
	m.lastAccountID = accountID

	if m.checkErr != nil {
		return domainquota.CheckResult{}, m.checkErr
	}
	return m.result, nil
}

type mockStateCache struct {
	mu            sync.RWMutex
	states        map[string]*domainquota.CredentialState
	modelStates   map[string]*domainquota.ModelState
	copilotUsages map[string]*domainquota.CopilotUsage
	pingErr       error
	setErr        error
	stateCount    int64
}

func newMockStateCache() *mockStateCache {
	return &mockStateCache{
		states:        make(map[string]*domainquota.CredentialState),
		modelStates:   make(map[string]*domainquota.ModelState),
		copilotUsages: make(map[string]*domainquota.CopilotUsage),
	}
}

func (m *mockStateCache) GetCredentialState(ctx context.Context, credentialID string) (*domainquota.CredentialState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.states[credentialID]
	if !ok {
		return nil, nil
	}
	return state, nil
}

func (m *mockStateCache) SetCredentialState(ctx context.Context, state domainquota.CredentialState, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.states[state.CredentialID] = &state
	return nil
}

func (m *mockStateCache) GetModelState(ctx context.Context, credentialID, modelID string) (*domainquota.ModelState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := credentialID + ":" + modelID
	state, ok := m.modelStates[key]
	if !ok {
		return nil, nil
	}
	return state, nil
}

func (m *mockStateCache) SetModelState(ctx context.Context, state domainquota.ModelState, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	key := state.CredentialID + ":" + state.ModelID
	m.modelStates[key] = &state
	return nil
}

func (m *mockStateCache) ListCredentialStates(ctx context.Context) ([]domainquota.CredentialState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	states := make([]domainquota.CredentialState, 0, len(m.states))
	for _, state := range m.states {
		states = append(states, *state)
	}
	return states, nil
}

func (m *mockStateCache) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockStateCache) CountCredentialStates(ctx context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.pingErr != nil {
		return 0, m.pingErr
	}
	return m.stateCount, nil
}

func (m *mockStateCache) GetCopilotUsage(ctx context.Context, credentialID string) (*domainquota.CopilotUsage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	usage, ok := m.copilotUsages[credentialID]
	if !ok {
		return nil, nil
	}
	return usage, nil
}

func (m *mockStateCache) SetCopilotUsage(ctx context.Context, usage domainquota.CopilotUsage, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.copilotUsages[usage.CredentialID] = &usage
	return nil
}

func (m *mockStateCache) ListCopilotUsages(ctx context.Context) ([]domainquota.CopilotUsage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]domainquota.CopilotUsage, 0, len(m.copilotUsages))
	for _, v := range m.copilotUsages {
		result = append(result, *v)
	}
	return result, nil
}

func TestService_CheckSample(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "openai", EncryptedProfile: "encrypted1"},
		{ID: "cred-2", Type: "openai", EncryptedProfile: "encrypted2"},
	}

	repo := &mockCredentialRepository{
		count:         10,
		randomSamples: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
		},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckSample(ctx)
	if err != nil {
		t.Fatalf("CheckSample() error = %v", err)
	}

	// Verify states were cached
	if len(cache.states) != 2 {
		t.Errorf("expected 2 cached states, got %d", len(cache.states))
	}

	for _, p := range profiles {
		state, ok := cache.states[p.ID]
		if !ok {
			t.Errorf("state for %s not found in cache", p.ID)
			continue
		}
		if state.Status != domainquota.StatusHealthy {
			t.Errorf("state for %s: status = %v, want healthy", p.ID, state.Status)
		}
	}
}

func TestService_CheckAll(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "openai", EncryptedProfile: "encrypted1"},
		{ID: "cred-2", Type: "anthropic", EncryptedProfile: "encrypted2"},
		{ID: "cred-3", Type: "gemini", EncryptedProfile: "encrypted3"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
		},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	if len(cache.states) != 3 {
		t.Errorf("expected 3 cached states, got %d", len(cache.states))
	}
}

func TestService_NeedsRehydration_CacheEmpty(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	cache := newMockStateCache()
	cache.stateCount = 0

	svc := NewService(nil, nil, nil, cache, logger, DefaultServiceConfig())

	needs, err := svc.NeedsRehydration(ctx)
	if err != nil {
		t.Fatalf("NeedsRehydration() error = %v", err)
	}
	if !needs {
		t.Error("expected NeedsRehydration() = true for empty cache")
	}
}

func TestService_NeedsRehydration_CacheHasData(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	cache := newMockStateCache()
	cache.stateCount = 10

	svc := NewService(nil, nil, nil, cache, logger, DefaultServiceConfig())

	needs, err := svc.NeedsRehydration(ctx)
	if err != nil {
		t.Fatalf("NeedsRehydration() error = %v", err)
	}
	if needs {
		t.Error("expected NeedsRehydration() = false when cache has data")
	}
}

func TestService_NeedsRehydration_CacheUnavailable(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	cache := newMockStateCache()
	cache.pingErr = errors.New("connection refused")

	svc := NewService(nil, nil, nil, cache, logger, DefaultServiceConfig())

	needs, err := svc.NeedsRehydration(ctx)
	if err != nil {
		t.Fatalf("NeedsRehydration() error = %v", err)
	}
	if !needs {
		t.Error("expected NeedsRehydration() = true when cache unavailable")
	}
}

func TestService_HandleUnhealthyCredential(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "openai", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:      false,
			CheckedAt:    time.Now(),
			ErrorCode:    401,
			ErrorMessage: "unauthorized",
		},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found in cache")
	}

	if state.Status != domainquota.StatusCooldown {
		t.Errorf("status = %v, want cooldown for 401", state.Status)
	}
	if state.ErrorCode != 401 {
		t.Errorf("error_code = %d, want 401", state.ErrorCode)
	}
	// Should have 30m cooldown for auth failure
	expectedCooldown := 30 * time.Minute
	actualCooldown := state.CooldownUntil.Sub(state.LastCheckedAt)
	if actualCooldown < expectedCooldown-time.Second || actualCooldown > expectedCooldown+time.Second {
		t.Errorf("cooldown = %v, want ~%v", actualCooldown, expectedCooldown)
	}
}

func TestService_HandleRateLimitWithBackoff(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "openai", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:      false,
			CheckedAt:    time.Now(),
			ErrorCode:    429,
			ErrorMessage: "rate limited",
		},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	// First check - should get 2m cooldown
	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found in cache")
	}

	if state.Status != domainquota.StatusCooldown {
		t.Errorf("status = %v, want cooldown", state.Status)
	}
	if state.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0", state.RetryCount)
	}
}

func TestService_DecryptError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "openai", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decryptErr: errors.New("decrypt failed"),
	}
	checker := &mockProviderChecker{}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found in cache")
	}

	if state.Status != domainquota.StatusUnhealthy {
		t.Errorf("status = %v, want unhealthy for decrypt error", state.Status)
	}
	if state.ErrorMessage != "decrypt_failed" {
		t.Errorf("error_message = %s, want decrypt_failed", state.ErrorMessage)
	}
}

func TestService_PersistsQuotaFields(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "codex", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}

	// Checker returns quota info
	quota := domainquota.NewQuotaInfo(75, 100)
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
			Quota:     quota,
		},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found in cache")
	}

	// Verify quota was persisted
	if state.Quota.Remaining != 75 {
		t.Errorf("quota.remaining = %d, want 75", state.Quota.Remaining)
	}
	if state.Quota.Limit != 100 {
		t.Errorf("quota.limit = %d, want 100", state.Quota.Limit)
	}
	expectedRatio := 0.75
	if state.Quota.Ratio < expectedRatio-0.01 || state.Quota.Ratio > expectedRatio+0.01 {
		t.Errorf("quota.ratio = %f, want ~%f", state.Quota.Ratio, expectedRatio)
	}
	if !state.Quota.IsKnown() {
		t.Error("expected quota to be known")
	}

	// Verify deprecated AvailableQuota is also set
	if state.AvailableQuota != 75 {
		t.Errorf("available_quota = %d, want 75", state.AvailableQuota)
	}
}

func TestService_PersistsUnknownQuota(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "anthropic", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}

	// Checker returns unknown quota
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
			Quota:     domainquota.NewQuotaInfoUnknown(),
		},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found in cache")
	}

	// Verify quota is unknown
	if state.Quota.IsKnown() {
		t.Error("expected quota to be unknown")
	}
	if state.Quota.Remaining != -1 {
		t.Errorf("quota.remaining = %d, want -1", state.Quota.Remaining)
	}
	if state.Quota.Limit != -1 {
		t.Errorf("quota.limit = %d, want -1", state.Quota.Limit)
	}
	if state.Quota.Ratio != domainquota.QuotaUnknown {
		t.Errorf("quota.ratio = %f, want %f", state.Quota.Ratio, domainquota.QuotaUnknown)
	}

	// Verify deprecated AvailableQuota is -1
	if state.AvailableQuota != -1 {
		t.Errorf("available_quota = %d, want -1", state.AvailableQuota)
	}
}

func TestService_PersistsModelQuota(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "codex", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}

	// Checker returns per-model quota info
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
			Quota:     domainquota.NewQuotaInfo(100, 200),
			Models: []domainquota.ModelCheckResult{
				{
					ModelID: "gpt-4",
					Healthy: true,
					Quota:   domainquota.NewQuotaInfo(50, 100),
				},
				{
					ModelID: "gpt-3.5-turbo",
					Healthy: true,
					Quota:   domainquota.NewQuotaInfo(500, 1000),
				},
			},
		},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	// Verify model states have quota
	gpt4State := cache.modelStates["cred-1:gpt-4"]
	if gpt4State == nil {
		t.Fatal("gpt-4 model state not found in cache")
	}
	if gpt4State.Quota.Remaining != 50 {
		t.Errorf("gpt-4 quota.remaining = %d, want 50", gpt4State.Quota.Remaining)
	}
	if gpt4State.Quota.Ratio != 0.5 {
		t.Errorf("gpt-4 quota.ratio = %f, want 0.5", gpt4State.Quota.Ratio)
	}

	gpt35State := cache.modelStates["cred-1:gpt-3.5-turbo"]
	if gpt35State == nil {
		t.Fatal("gpt-3.5-turbo model state not found in cache")
	}
	if gpt35State.Quota.Remaining != 500 {
		t.Errorf("gpt-3.5-turbo quota.remaining = %d, want 500", gpt35State.Quota.Remaining)
	}
	if gpt35State.Quota.Ratio != 0.5 {
		t.Errorf("gpt-3.5-turbo quota.ratio = %f, want 0.5", gpt35State.Quota.Ratio)
	}
}

func TestService_NetworkError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "openai", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}

	// Create a network error
	checker := &mockProviderChecker{
		checkErr: &mockNetError{msg: "connection refused", timeout: false, temporary: false},
	}
	cache := newMockStateCache()

	cfg := DefaultServiceConfig()
	cfg.Cooldown.NetworkMaxRetries = 0 // No retries for test speed

	svc := NewService(repo, encryptor, checker, cache, logger, cfg)

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found in cache")
	}

	if state.Status != domainquota.StatusCooldown {
		t.Errorf("status = %v, want cooldown for network error", state.Status)
	}
	if state.ErrorMessage != "network_error" {
		t.Errorf("error_message = %s, want network_error", state.ErrorMessage)
	}
}

func TestService_RetryCountResetOnSuccess(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "openai", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{
		profiles: profiles,
	}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}

	cache := newMockStateCache()
	// Pre-seed cache with a state that has retry count from previous 429
	cache.states["cred-1"] = &domainquota.CredentialState{
		CredentialID: "cred-1",
		Status:       domainquota.StatusCooldown,
		ErrorCode:    429,
		RetryCount:   5,
	}

	// Now the credential becomes healthy
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{
			Healthy:   true,
			CheckedAt: time.Now(),
		},
	}

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())

	err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found in cache")
	}

	if state.Status != domainquota.StatusHealthy {
		t.Errorf("status = %v, want healthy", state.Status)
	}
	if state.RetryCount != 0 {
		t.Errorf("retry_count = %d, want 0 (should reset on success)", state.RetryCount)
	}
}

func TestService_UsesPayloadAccountIDForChecker(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "codex", AccountID: "db-account", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{profiles: profiles}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token","account_id":"payload-account"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{Healthy: true, CheckedAt: time.Now()},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())
	if err := svc.CheckAll(ctx); err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	if checker.lastAccountID != "payload-account" {
		t.Fatalf("checker account_id = %q, want payload-account", checker.lastAccountID)
	}
	if checker.lastAccessToken != "test-token" {
		t.Fatalf("checker access_token mismatch")
	}
}

func TestService_FallsBackToProfileAccountIDForChecker(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "codex", AccountID: "db-account", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{profiles: profiles}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"test-token"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{Healthy: true, CheckedAt: time.Now()},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())
	if err := svc.CheckAll(ctx); err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	if checker.lastAccountID != "db-account" {
		t.Fatalf("checker account_id = %q, want db-account", checker.lastAccountID)
	}
}

func TestService_CopilotUsesRefreshTokenForChecker(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "copilot", AccountID: "gh-user", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{profiles: profiles}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"copilot-session-token","refresh_token":"github-token"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{Healthy: true, CheckedAt: time.Now()},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())
	if err := svc.CheckAll(ctx); err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	if checker.lastCredentialType != "copilot" {
		t.Fatalf("checker credential type = %q, want copilot", checker.lastCredentialType)
	}
	if checker.lastAccessToken != "github-token" {
		t.Fatalf("checker token = %q, want github-token", checker.lastAccessToken)
	}
}

func TestService_CopilotFallsBackToAccessTokenWhenRefreshTokenMissing(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{
		{ID: "cred-1", Type: "copilot", AccountID: "gh-user", EncryptedProfile: "encrypted1"},
	}

	repo := &mockCredentialRepository{profiles: profiles}
	encryptor := &mockEncryptor{
		decrypted: `{"access_token":"copilot-session-token"}`,
	}
	checker := &mockProviderChecker{
		result: domainquota.CheckResult{Healthy: true, CheckedAt: time.Now()},
	}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())
	if err := svc.CheckAll(ctx); err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	if checker.lastAccessToken != "copilot-session-token" {
		t.Fatalf("checker token = %q, want copilot-session-token", checker.lastAccessToken)
	}
}

func TestService_CopilotMissingBothTokensMarksUnhealthy(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{{ID: "cred-1", Type: "copilot", EncryptedProfile: "encrypted1"}}
	repo := &mockCredentialRepository{profiles: profiles}
	encryptor := &mockEncryptor{decrypted: `{"account_id":"acct-1"}`}
	checker := &mockProviderChecker{}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())
	if err := svc.CheckAll(ctx); err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found")
	}
	if state.Status != domainquota.StatusUnhealthy {
		t.Fatalf("status = %v, want unhealthy", state.Status)
	}
	if state.ErrorMessage != "missing_access_token" {
		t.Fatalf("error_message = %q, want missing_access_token", state.ErrorMessage)
	}
}

func TestService_MissingAccessTokenMarksUnhealthy(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	profiles := []domaincredential.Profile{{ID: "cred-1", Type: "codex", EncryptedProfile: "encrypted1"}}
	repo := &mockCredentialRepository{profiles: profiles}
	encryptor := &mockEncryptor{decrypted: `{"account_id":"acct-1"}`}
	checker := &mockProviderChecker{}
	cache := newMockStateCache()

	svc := NewService(repo, encryptor, checker, cache, logger, DefaultServiceConfig())
	if err := svc.CheckAll(ctx); err != nil {
		t.Fatalf("CheckAll() error = %v", err)
	}

	state := cache.states["cred-1"]
	if state == nil {
		t.Fatal("state not found")
	}
	if state.Status != domainquota.StatusUnhealthy {
		t.Fatalf("status = %v, want unhealthy", state.Status)
	}
	if state.ErrorMessage != "missing_access_token" {
		t.Fatalf("error_message = %q, want missing_access_token", state.ErrorMessage)
	}
}

// mockNetError implements net.Error for testing
type mockNetError struct {
	msg       string
	timeout   bool
	temporary bool
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }
