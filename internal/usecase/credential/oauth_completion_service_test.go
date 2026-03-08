package credential

import (
	"context"
	"errors"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations
type mockRepository struct {
	mock.Mock
}

func (m *mockRepository) Save(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	args := m.Called(ctx, profile)
	return args.Get(0).(domaincredential.Profile), args.Error(1)
}

func (m *mockRepository) List(ctx context.Context) ([]domaincredential.Profile, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domaincredential.Profile), args.Error(1)
}

func (m *mockRepository) GetByID(ctx context.Context, id string) (*domaincredential.Profile, error) {
	_ = id
	return nil, nil
}

func (m *mockRepository) Update(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	args := m.Called(ctx, profile)
	return args.Get(0).(domaincredential.Profile), args.Error(1)
}

func (m *mockRepository) UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	args := m.Called(ctx, profile)
	return args.Get(0).(domaincredential.Profile), args.Error(1)
}

func (m *mockRepository) ListEnabled(ctx context.Context) ([]domaincredential.Profile, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domaincredential.Profile), args.Error(1)
}

func (m *mockRepository) CountEnabled(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockRepository) RandomSample(ctx context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error) {
	args := m.Called(ctx, sampleSize, seed)
	return args.Get(0).([]domaincredential.Profile), args.Error(1)
}

type mockEncryptor struct {
	mock.Mock
}

func (m *mockEncryptor) Encrypt(plain string) (string, string, string, error) {
	args := m.Called(plain)
	return args.String(0), args.String(1), args.String(2), args.Error(3)
}

func (m *mockEncryptor) Decrypt(cipher, iv, tag string) (string, error) {
	args := m.Called(cipher, iv, tag)
	return args.String(0), args.Error(1)
}

func (m *mockEncryptor) ShouldEncrypt() bool {
	return true
}

func TestOAuthCompletionService_CompleteOAuth_Success(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc, nil)

	// Override the time function for deterministic testing
	svc := service.(*oAuthCompletionService)
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedTime }

	accountID := "user@example.com"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		TokenType:    "Bearer",
		Scope:        "read write",
	}

	expectedEncrypted := "encrypted-profile-data"
	mockEnc.On("Encrypt", mock.Anything).Return(expectedEncrypted, "iv", "tag", nil)

	expectedProfile := domaincredential.Profile{
		ID:               "profile-id",
		Type:             "codex",
		AccountID:        accountID,
		Enabled:          true,
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    fixedTime,
		EncryptedProfile: expectedEncrypted,
	}
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.MatchedBy(func(p domaincredential.Profile) bool {
		return p.Type == "codex" &&
			p.AccountID == accountID &&
			p.Enabled == true &&
			p.EncryptedProfile == expectedEncrypted
	})).Return(expectedProfile, nil)

	result, err := service.CompleteOAuth(context.Background(), accountID, tokenPayload)

	assert.NoError(t, err)
	assert.Equal(t, expectedProfile.ID, result.ID)
	assert.Equal(t, "codex", result.Type)
	assert.Equal(t, accountID, result.AccountID)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestOAuthCompletionService_CompleteOAuth_EncryptError(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc, nil)

	accountID := "user@example.com"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken: "access-token-123",
	}

	mockEnc.On("Encrypt", mock.Anything).Return("", "", "", errors.New("encryption failed"))

	_, err := service.CompleteOAuth(context.Background(), accountID, tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encrypt profile")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestOAuthCompletionService_CompleteOAuth_UpsertError(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc, nil)

	accountID := "user@example.com"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken: "access-token-123",
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted", "iv", "tag", nil)
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.Anything).Return(domaincredential.Profile{}, errors.New("upsert failed"))

	_, err := service.CompleteOAuth(context.Background(), accountID, tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upsert failed")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestOAuthCompletionService_CompleteOAuth_ReauthIdempotency(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc, nil)

	svc := service.(*oAuthCompletionService)
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedTime }

	accountID := "user@example.com"

	// First auth
	tokenPayload1 := domainoauth.TokenPayload{
		AccessToken:  "access-token-1",
		RefreshToken: "refresh-token-1",
		ExpiresAt:    time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted-1", "iv1", "tag1", nil).Once()
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.MatchedBy(func(p domaincredential.Profile) bool {
		return p.AccountID == accountID && p.EncryptedProfile == "encrypted-1"
	})).Return(domaincredential.Profile{ID: "profile-1"}, nil).Once()

	_, err := service.CompleteOAuth(context.Background(), accountID, tokenPayload1)
	assert.NoError(t, err)

	// Re-auth with same account (should update, not create duplicate)
	tokenPayload2 := domainoauth.TokenPayload{
		AccessToken:  "access-token-2",
		RefreshToken: "refresh-token-2",
		ExpiresAt:    time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted-2", "iv2", "tag2", nil).Once()
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.MatchedBy(func(p domaincredential.Profile) bool {
		return p.AccountID == accountID && p.EncryptedProfile == "encrypted-2"
	})).Return(domaincredential.Profile{ID: "profile-1"}, nil).Once()

	_, err = service.CompleteOAuth(context.Background(), accountID, tokenPayload2)
	assert.NoError(t, err)

	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestOAuthCompletionService_CompleteOAuth_EmptyAccountID(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc, nil)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	// Empty account ID should fail validation
	_, err := service.CompleteOAuth(context.Background(), "", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account_id is required")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestOAuthCompletionService_CompleteOAuth_WhitespaceAccountID(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc, nil)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	// Whitespace-only account ID should fail validation
	_, err := service.CompleteOAuth(context.Background(), "   ", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account_id is required")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestOAuthCompletionService_CompleteOAuth_GeneratesUUID(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc, nil)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted", "iv", "tag", nil)

	var capturedID string
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.MatchedBy(func(p domaincredential.Profile) bool {
		capturedID = p.ID
		// ID should be a non-empty UUID
		return p.ID != "" && len(p.ID) == 36 // UUID format: 8-4-4-4-12 = 36 chars
	})).Return(domaincredential.Profile{ID: "test-uuid"}, nil)

	_, err := service.CompleteOAuth(context.Background(), "testuser", tokenPayload)

	assert.NoError(t, err)
	assert.NotEmpty(t, capturedID)
	assert.Len(t, capturedID, 36) // UUID format
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestOAuthCompletionService_CompleteOAuth_RefreshesRegistryOnSuccess(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)

	var refreshCalled bool
	var refreshedType string
	var refreshedAccountID string
	service := NewOAuthCompletionService(mockRepo, mockEnc, func(_ context.Context, profileType, accountID string) {
		refreshCalled = true
		refreshedType = profileType
		refreshedAccountID = accountID
	})

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted", "iv", "tag", nil)
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.Anything).Return(domaincredential.Profile{
		Type:      "codex",
		AccountID: "testuser",
	}, nil)

	_, err := service.CompleteOAuth(context.Background(), "testuser", tokenPayload)

	assert.NoError(t, err)
	assert.True(t, refreshCalled)
	assert.Equal(t, "codex", refreshedType)
	assert.Equal(t, "testuser", refreshedAccountID)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}
