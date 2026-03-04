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

func TestCopilotCompletionService_CompleteOAuth_Success(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	// Override the time function for deterministic testing
	svc := service.(*copilotCompletionService)
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedTime }

	accountID := "testuser"
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Date(2024, 1, 1, 0, 30, 0, 0, time.UTC),
		Email:        "test@example.com",
		TokenType:    "Bearer",
		Scope:        "read:user",
	}

	expectedEncrypted := "encrypted-copilot-profile"
	mockEnc.On("Encrypt", mock.Anything).Return(expectedEncrypted, nil)

	expectedProfile := domaincredential.Profile{
		ID:               "generated-uuid",
		Type:             "copilot",
		AccountID:        accountID,
		Enabled:          true,
		Email:            "test@example.com",
		Expired:          tokenPayload.ExpiresAt,
		LastRefreshAt:    fixedTime,
		EncryptedProfile: expectedEncrypted,
	}
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.MatchedBy(func(p domaincredential.Profile) bool {
		return p.Type == "copilot" &&
			p.AccountID == accountID &&
			p.Enabled == true &&
			p.Email == "test@example.com" &&
			p.EncryptedProfile == expectedEncrypted &&
			p.ID != "" // UUID should be generated
	})).Return(expectedProfile, nil)

	result, err := service.CompleteOAuth(context.Background(), accountID, tokenPayload)

	assert.NoError(t, err)
	assert.Equal(t, "copilot", result.Type)
	assert.Equal(t, accountID, result.AccountID)
	assert.Equal(t, "test@example.com", result.Email)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestCopilotCompletionService_CompleteOAuth_EmptyAccountID(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	// Empty account ID should fail validation
	_, err := service.CompleteOAuth(context.Background(), "", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account_id is required")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestCopilotCompletionService_CompleteOAuth_WhitespaceAccountID(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	// Whitespace-only account ID should fail validation
	_, err := service.CompleteOAuth(context.Background(), "   ", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account_id is required")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestCopilotCompletionService_CompleteOAuth_EmptyAccessToken(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "", // Empty access token
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	_, err := service.CompleteOAuth(context.Background(), "testuser", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access_token is required")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestCopilotCompletionService_CompleteOAuth_EmptyRefreshToken(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "", // Empty refresh token (GitHub token)
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	_, err := service.CompleteOAuth(context.Background(), "testuser", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refresh_token")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestCopilotCompletionService_CompleteOAuth_EncryptError(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	mockEnc.On("Encrypt", mock.Anything).Return("", errors.New("encryption failed"))

	_, err := service.CompleteOAuth(context.Background(), "testuser", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encrypt profile")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertNotCalled(t, "UpsertByTypeAccount")
}

func TestCopilotCompletionService_CompleteOAuth_UpsertError(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted", nil)
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.Anything).Return(domaincredential.Profile{}, errors.New("upsert failed"))

	_, err := service.CompleteOAuth(context.Background(), "testuser", tokenPayload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upsert failed")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestCopilotCompletionService_CompleteOAuth_TrimsAccountIDAndEmail(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	svc := service.(*copilotCompletionService)
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedTime }

	// Account ID and email with leading/trailing whitespace
	accountID := "  testuser  "
	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
		Email:        "  test@example.com  ",
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted", nil)
	mockRepo.On("UpsertByTypeAccount", mock.Anything, mock.MatchedBy(func(p domaincredential.Profile) bool {
		// Should be trimmed
		return p.AccountID == "testuser" && p.Email == "test@example.com"
	})).Return(domaincredential.Profile{
		AccountID: "testuser",
		Email:     "test@example.com",
	}, nil)

	result, err := service.CompleteOAuth(context.Background(), accountID, tokenPayload)

	assert.NoError(t, err)
	assert.Equal(t, "testuser", result.AccountID)
	assert.Equal(t, "test@example.com", result.Email)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestCopilotCompletionService_CompleteOAuth_GeneratesUUID(t *testing.T) {
	mockRepo := new(mockRepository)
	mockEnc := new(mockEncryptor)
	service := NewCopilotCompletionService(mockRepo, mockEnc)

	tokenPayload := domainoauth.TokenPayload{
		AccessToken:  "copilot-session-token",
		RefreshToken: "github-access-token",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}

	mockEnc.On("Encrypt", mock.Anything).Return("encrypted", nil)

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
