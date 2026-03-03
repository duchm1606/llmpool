package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCredentialRepository is a mock implementation of CredentialRepository
type mockCredentialRepository struct {
	upserted []domaincredential.Profile
	err      error
}

func (m *mockCredentialRepository) UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	if m.err != nil {
		return domaincredential.Profile{}, m.err
	}
	m.upserted = append(m.upserted, profile)
	return profile, nil
}

// mockEncryptor is a mock implementation of Encryptor
type mockEncryptor struct {
	encrypted map[string]string
	err       error
}

func newMockEncryptor() *mockEncryptor {
	return &mockEncryptor{
		encrypted: make(map[string]string),
	}
}

func (m *mockEncryptor) Encrypt(plain string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	cipher := "encrypted_" + plain
	m.encrypted[plain] = cipher
	return cipher, nil
}

func (m *mockEncryptor) Decrypt(cipher string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	for plain, enc := range m.encrypted {
		if enc == cipher {
			return plain, nil
		}
	}
	return "", errors.New("cipher not found")
}

func TestCompletionService_HandleCompletion(t *testing.T) {
	fixedNow := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	expiresAt := fixedNow.Add(1 * time.Hour)

	t.Run("successfully persists encrypted credentials", func(t *testing.T) {
		repo := &mockCredentialRepository{}
		encryptor := newMockEncryptor()

		svc := NewCompletionService(repo, encryptor)
		svc.now = func() time.Time { return fixedNow }

		tokenPayload := domainoauth.TokenPayload{
			AccessToken:  "access_token_12345",
			RefreshToken: "refresh_token_67890",
			ExpiresAt:    expiresAt,
			AccountID:    "account_123",
			TokenType:    "Bearer",
			Scope:        "read write",
		}

		err := svc.HandleCompletion(context.Background(), "session_123", tokenPayload)
		require.NoError(t, err)

		require.Len(t, repo.upserted, 1)
		profile := repo.upserted[0]

		assert.Equal(t, "codex", profile.Type)
		assert.Equal(t, "account_123", profile.AccountID)
		assert.True(t, profile.Enabled)
		assert.Equal(t, expiresAt, profile.Expired)
		assert.Equal(t, fixedNow, profile.LastRefreshAt)

		// Verify encrypted profile contains expected data
		decrypted, err := encryptor.Decrypt(profile.EncryptedProfile)
		require.NoError(t, err)

		var profileData map[string]interface{}
		err = json.Unmarshal([]byte(decrypted), &profileData)
		require.NoError(t, err)

		assert.Equal(t, "access_token_12345", profileData["access_token"])
		assert.Equal(t, "refresh_token_67890", profileData["refresh_token"])
		assert.Equal(t, expiresAt.Format(time.RFC3339), profileData["expires_at"])
		assert.Equal(t, "Bearer", profileData["token_type"])
		assert.Equal(t, "read write", profileData["scope"])
	})

	t.Run("returns error when AccountID is empty", func(t *testing.T) {
		repo := &mockCredentialRepository{}
		encryptor := newMockEncryptor()

		svc := NewCompletionService(repo, encryptor)
		svc.now = func() time.Time { return fixedNow }

		tokenPayload := domainoauth.TokenPayload{
			AccessToken:  "access_token_12345",
			RefreshToken: "refresh_token_67890",
			ExpiresAt:    expiresAt,
			AccountID:    "", // Empty
			TokenType:    "Bearer",
			Scope:        "read write",
		}

		err := svc.HandleCompletion(context.Background(), "session_123", tokenPayload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing account identifier")
		require.Len(t, repo.upserted, 0)
	})

	t.Run("encryption failure returns error", func(t *testing.T) {
		repo := &mockCredentialRepository{}
		encryptor := newMockEncryptor()
		encryptor.err = errors.New("encryption failed")

		svc := NewCompletionService(repo, encryptor)
		svc.now = func() time.Time { return fixedNow }

		tokenPayload := domainoauth.TokenPayload{
			AccessToken:  "access_token_12345",
			RefreshToken: "refresh_token_67890",
			ExpiresAt:    expiresAt,
			AccountID:    "account_123",
		}

		err := svc.HandleCompletion(context.Background(), "session_123", tokenPayload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "encrypt profile json")
	})

	t.Run("repository upsert failure returns error", func(t *testing.T) {
		repo := &mockCredentialRepository{
			err: errors.New("database error"),
		}
		encryptor := newMockEncryptor()

		svc := NewCompletionService(repo, encryptor)
		svc.now = func() time.Time { return fixedNow }

		tokenPayload := domainoauth.TokenPayload{
			AccessToken:  "access_token_12345",
			RefreshToken: "refresh_token_67890",
			ExpiresAt:    expiresAt,
			AccountID:    "account_123",
		}

		err := svc.HandleCompletion(context.Background(), "session_123", tokenPayload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upsert credential profile")
	})

}
