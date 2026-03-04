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

func (m *mockEncryptor) Encrypt(plain string) (string, error) {
	args := m.Called(plain)
	return args.String(0), args.Error(1)
}

func (m *mockEncryptor) Decrypt(cipher string) (string, error) {
	args := m.Called(cipher)
	return args.String(0), args.Error(1)
}

func TestOAuthCompletionService_CompleteOAuth_Success(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc)

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
	mockEnc.On("Encrypt", mock.Anything).Return(expectedEncrypted, nil)

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
	service := NewOAuthCompletionService(mockRepo, mockEnc)

	accountID := "user@example.com"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken: "access-token-123",
	}

	mockEnc.On("Encrypt", mock.Anything).Return("", errors.New("encryption failed"))

	_, err := service.CompleteOAuth(context.Background(), accountID, tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encrypt profile")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestOAuthCompletionService_CompleteOAuth_UpsertError(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewOAuthCompletionService(mockRepo, mockEnc)

	accountID := "user@example.com"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken: "access-token-123",
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted", nil)
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
	service := NewOAuthCompletionService(mockRepo, mockEnc)

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

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted-1", nil).Once()
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

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted-2", nil).Once()
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.MatchedBy(func(p domaincredential.Profile) bool {
		return p.AccountID == accountID && p.EncryptedProfile == "encrypted-2"
	})).Return(domaincredential.Profile{ID: "profile-1"}, nil).Once()

	_, err = service.CompleteOAuth(context.Background(), accountID, tokenPayload2)
	assert.NoError(t, err)

	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}
