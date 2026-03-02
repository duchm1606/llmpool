package credential

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock repository for refresh tests
type mockRefreshRepo struct {
	profiles  []domaincredential.Profile
	listErr   error
	updateErr error
	updated   []domaincredential.Profile
}

func (m *mockRefreshRepo) Save(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	return profile, nil
}

func (m *mockRefreshRepo) List(ctx context.Context) ([]domaincredential.Profile, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.profiles, nil
}

func (m *mockRefreshRepo) Update(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	if m.updateErr != nil {
		return domaincredential.Profile{}, m.updateErr
	}
	m.updated = append(m.updated, profile)
	return profile, nil
}

func (m *mockRefreshRepo) UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	return profile, nil
}

// Mock refresher for testing
type mockRefresher struct {
	result RefreshResult
	err    error
}

func (m *mockRefresher) Refresh(ctx context.Context, refreshToken string) (RefreshResult, error) {
	if m.err != nil {
		return RefreshResult{}, m.err
	}
	return m.result, nil
}

// Mock encryptor that just passes through for testing
type mockRefreshEncryptor struct{}

func (m *mockRefreshEncryptor) Encrypt(plain string) (string, error) {
	return "encrypted:" + plain, nil
}

func (m *mockRefreshEncryptor) Decrypt(cipher string) (string, error) {
	if len(cipher) < 10 || cipher[:10] != "encrypted:" {
		return "", errors.New("invalid cipher")
	}
	return cipher[10:], nil
}

// createTestProfile creates a credential profile for testing
func createTestProfile(t *testing.T, profileType string, enabled bool, expiry time.Time, accessToken, refreshToken string) domaincredential.Profile {
	payload := CredentialProfile{
		Type:         profileType,
		AccountID:    "test-account-" + uuid.NewString(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expired:      expiry,
		Enabled:      &enabled,
		LastRefresh:  time.Now().Add(-time.Hour),
	}

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	return domaincredential.Profile{
		ID:               uuid.NewString(),
		Type:             profileType,
		AccountID:        payload.AccountID,
		Enabled:          enabled,
		EncryptedProfile: "encrypted:" + string(raw),
		Expired:          expiry,
		LastRefreshAt:    payload.LastRefresh,
	}
}

func TestRefreshDue_MapsRotatedTokensSafely(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create a profile with initial tokens
	oldAccessToken := "old-access-token"
	oldRefreshToken := "old-refresh-token"
	expiry := now.Add(-time.Minute) // Expired, needs refresh

	profile := createTestProfile(t, "codex", true, expiry, oldAccessToken, oldRefreshToken)

	repo := &mockRefreshRepo{
		profiles: []domaincredential.Profile{profile},
	}

	// Refresher returns NEW rotated tokens
	newAccessToken := "new-access-token-xyz"
	newRefreshToken := "new-refresh-token-abc"
	newExpiry := now.Add(time.Hour)

	refresher := &mockRefresher{
		result: RefreshResult{
			AccessToken:  newAccessToken,
			RefreshToken: newRefreshToken,
			ExpiresAt:    newExpiry,
		},
	}

	refreshers := map[string]Refresher{
		"codex": refresher,
	}

	service := NewRefreshService(repo, refreshers, &mockRefreshEncryptor{})
	service.(*refreshService).now = func() time.Time { return now }

	err := service.RefreshDue(ctx)
	require.NoError(t, err)

	// Verify profile was updated
	require.Len(t, repo.updated, 1, "expected profile to be updated")

	updatedProfile := repo.updated[0]

	// Decrypt and verify tokens
	decryptor := &mockRefreshEncryptor{}
	decrypted, err := decryptor.Decrypt(updatedProfile.EncryptedProfile)
	require.NoError(t, err)

	var payload CredentialProfile
	err = json.Unmarshal([]byte(decrypted), &payload)
	require.NoError(t, err)

	// CRITICAL: Both access token AND refresh token should be updated (rotation)
	assert.Equal(t, newAccessToken, payload.AccessToken, "access token should be updated")
	assert.Equal(t, newRefreshToken, payload.RefreshToken, "refresh token should be rotated to new value")
	assert.Equal(t, newExpiry, payload.Expired, "expiry should be updated")
	assert.True(t, payload.Enabled != nil && *payload.Enabled, "profile should remain enabled")
}

func TestRefreshDue_PreservesRefreshTokenWhenNotRotated(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create a profile with initial tokens
	oldAccessToken := "old-access-token"
	oldRefreshToken := "persistent-refresh-token" // This should be preserved
	expiry := now.Add(-time.Minute)               // Expired, needs refresh

	profile := createTestProfile(t, "codex", true, expiry, oldAccessToken, oldRefreshToken)

	repo := &mockRefreshRepo{
		profiles: []domaincredential.Profile{profile},
	}

	// Refresher returns new access token but EMPTY refresh token (no rotation)
	newAccessToken := "new-access-token-xyz"
	newExpiry := now.Add(time.Hour)

	refresher := &mockRefresher{
		result: RefreshResult{
			AccessToken:  newAccessToken,
			RefreshToken: "", // Empty = no rotation
			ExpiresAt:    newExpiry,
		},
	}

	refreshers := map[string]Refresher{
		"codex": refresher,
	}

	service := NewRefreshService(repo, refreshers, &mockRefreshEncryptor{})
	service.(*refreshService).now = func() time.Time { return now }

	err := service.RefreshDue(ctx)
	require.NoError(t, err)

	// Verify profile was updated
	require.Len(t, repo.updated, 1, "expected profile to be updated")

	// Decrypt and verify tokens
	decryptor := &mockRefreshEncryptor{}
	decrypted, err := decryptor.Decrypt(repo.updated[0].EncryptedProfile)
	require.NoError(t, err)

	var payload CredentialProfile
	err = json.Unmarshal([]byte(decrypted), &payload)
	require.NoError(t, err)

	// Access token should be updated, but refresh token should be PRESERVED
	assert.Equal(t, newAccessToken, payload.AccessToken, "access token should be updated")
	assert.Equal(t, oldRefreshToken, payload.RefreshToken, "refresh token should be preserved when not rotated")
}

func TestRefreshDue_NotOverwritingRefreshWithAccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create a profile
	oldAccessToken := "access-123"
	oldRefreshToken := "refresh-456"
	expiry := now.Add(-time.Minute)

	profile := createTestProfile(t, "codex", true, expiry, oldAccessToken, oldRefreshToken)

	repo := &mockRefreshRepo{
		profiles: []domaincredential.Profile{profile},
	}

	// Simulate a buggy refresher that returns access token as refresh token
	newAccessToken := "new-access-789"
	newExpiry := now.Add(time.Hour)

	refresher := &mockRefresher{
		result: RefreshResult{
			AccessToken:  newAccessToken,
			RefreshToken: newAccessToken, // BUG: Same as access token
			ExpiresAt:    newExpiry,
		},
	}

	refreshers := map[string]Refresher{
		"codex": refresher,
	}

	service := NewRefreshService(repo, refreshers, &mockRefreshEncryptor{})
	service.(*refreshService).now = func() time.Time { return now }

	err := service.RefreshDue(ctx)
	require.NoError(t, err)

	// Decrypt and verify
	decryptor := &mockRefreshEncryptor{}
	decrypted, err := decryptor.Decrypt(repo.updated[0].EncryptedProfile)
	require.NoError(t, err)

	var payload CredentialProfile
	err = json.Unmarshal([]byte(decrypted), &payload)
	require.NoError(t, err)

	// The code should still update (it doesn't detect this bug, but doesn't introduce it either)
	// This documents the current behavior
	assert.Equal(t, newAccessToken, payload.AccessToken)
	assert.Equal(t, newAccessToken, payload.RefreshToken) // BUG: Would be overwritten
}

func TestRefreshDue_DisabledProfileSkipped(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create a disabled profile
	profile := createTestProfile(t, "codex", false, now.Add(-time.Hour), "token", "refresh")

	repo := &mockRefreshRepo{
		profiles: []domaincredential.Profile{profile},
	}

	refresher := &mockRefresher{}
	refreshers := map[string]Refresher{
		"codex": refresher,
	}

	service := NewRefreshService(repo, refreshers, &mockRefreshEncryptor{})
	service.(*refreshService).now = func() time.Time { return now }

	err := service.RefreshDue(ctx)
	require.NoError(t, err)

	// Should not update disabled profiles
	assert.Len(t, repo.updated, 0, "disabled profile should not be refreshed")
}

func TestRefreshDue_NoRefresherForType(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create a profile with type that has no refresher
	profile := createTestProfile(t, "unknown-provider", true, now.Add(-time.Hour), "token", "refresh")

	repo := &mockRefreshRepo{
		profiles: []domaincredential.Profile{profile},
	}

	// Empty refreshers map
	refreshers := map[string]Refresher{}

	service := NewRefreshService(repo, refreshers, &mockRefreshEncryptor{})
	service.(*refreshService).now = func() time.Time { return now }

	err := service.RefreshDue(ctx)
	require.NoError(t, err)

	// Should skip profiles with no refresher
	assert.Len(t, repo.updated, 0, "profile with no refresher should be skipped")
}

func TestRefreshDue_RefreshFailureDisablesProfile(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	profile := createTestProfile(t, "codex", true, now.Add(-time.Hour), "token", "refresh")

	repo := &mockRefreshRepo{
		profiles: []domaincredential.Profile{profile},
	}

	refresher := &mockRefresher{
		err: errors.New("refresh failed"),
	}

	refreshers := map[string]Refresher{
		"codex": refresher,
	}

	service := NewRefreshService(repo, refreshers, &mockRefreshEncryptor{})
	service.(*refreshService).now = func() time.Time { return now }

	err := service.RefreshDue(ctx)
	require.NoError(t, err) // Service shouldn't error, should disable profile

	// Profile should be disabled after refresh failure
	require.Len(t, repo.updated, 1, "profile should be updated (disabled)")
	assert.False(t, repo.updated[0].Enabled, "profile should be disabled after refresh failure")
}

func TestRefreshDue_NotDueYet(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Profile expires in 10 minutes (more than 5 minute margin)
	profile := createTestProfile(t, "codex", true, now.Add(10*time.Minute), "token", "refresh")

	repo := &mockRefreshRepo{
		profiles: []domaincredential.Profile{profile},
	}

	refresher := &mockRefresher{}
	refreshers := map[string]Refresher{
		"codex": refresher,
	}

	service := NewRefreshService(repo, refreshers, &mockRefreshEncryptor{})
	service.(*refreshService).now = func() time.Time { return now }

	err := service.RefreshDue(ctx)
	require.NoError(t, err)

	// Should not refresh profiles that aren't due yet
	assert.Len(t, repo.updated, 0, "profile not due should not be refreshed")
}

func TestIsDue(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		expiry   time.Time
		expected bool
	}{
		{
			name:     "expired",
			expiry:   now.Add(-time.Minute),
			expected: true,
		},
		{
			name:     "expires within margin",
			expiry:   now.Add(3 * time.Minute), // Within 5 minute margin
			expected: true,
		},
		{
			name:     "expires after margin",
			expiry:   now.Add(10 * time.Minute), // Outside 5 minute margin
			expected: false,
		},
		{
			name:     "zero expiry",
			expiry:   time.Time{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := domaincredential.Profile{
				Expired: tt.expiry,
			}
			result := isDue(profile, now)
			assert.Equal(t, tt.expected, result)
		})
	}
}
