package credential

import (
	"context"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	"github.com/duchoang/llmpool/internal/infra/credential/sqlcdb"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockQuerier implements sqlcdb.Querier for testing
type MockQuerier struct {
	createFn func(ctx context.Context, arg sqlcdb.CreateCredentialProfileParams) (sqlcdb.CredentialProfile, error)
	listFn   func(ctx context.Context) ([]sqlcdb.CredentialProfile, error)
	updateFn func(ctx context.Context, arg sqlcdb.UpdateCredentialProfileParams) (sqlcdb.CredentialProfile, error)
	upsertFn func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.CredentialProfile, error)
}

func (m *MockQuerier) CreateCredentialProfile(ctx context.Context, arg sqlcdb.CreateCredentialProfileParams) (sqlcdb.CredentialProfile, error) {
	if m.createFn != nil {
		return m.createFn(ctx, arg)
	}
	return sqlcdb.CredentialProfile{}, nil
}

func (m *MockQuerier) ListCredentialProfiles(ctx context.Context) ([]sqlcdb.CredentialProfile, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *MockQuerier) UpdateCredentialProfile(ctx context.Context, arg sqlcdb.UpdateCredentialProfileParams) (sqlcdb.CredentialProfile, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, arg)
	}
	return sqlcdb.CredentialProfile{}, nil
}

func (m *MockQuerier) UpsertCredentialProfileByTypeAccount(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.CredentialProfile, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, arg)
	}
	return sqlcdb.CredentialProfile{}, nil
}

func TestUpsertByTypeAccount(t *testing.T) {
	tests := []struct {
		name           string
		input          domaincredential.Profile
		mockResponse   sqlcdb.CredentialProfile
		expectedOutput domaincredential.Profile
		expectError    bool
	}{
		{
			name: "successful upsert",
			input: domaincredential.Profile{
				Type:             "oauth",
				AccountID:        "account-123",
				Enabled:          true,
				Email:            "test@example.com",
				Expired:          time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
				LastRefreshAt:    time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
				EncryptedProfile: "encrypted_data_here",
			},
			mockResponse: sqlcdb.CredentialProfile{
				ID:               "profile-456",
				Type:             "oauth",
				AccountID:        "account-123",
				Enabled:          true,
				Email:            "test@example.com",
				Expired:          pgtype.Timestamptz{Time: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), Valid: true},
				LastRefreshAt:    pgtype.Timestamptz{Time: time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC), Valid: true},
				EncryptedProfile: "encrypted_data_here",
				CreatedAt:        pgtype.Timestamptz{Time: time.Date(2025, 3, 2, 10, 0, 0, 0, time.UTC), Valid: true},
				ModifiedAt:       pgtype.Timestamptz{Time: time.Date(2025, 3, 2, 14, 0, 0, 0, time.UTC), Valid: true},
			},
			expectedOutput: domaincredential.Profile{
				ID:               "profile-456",
				Type:             "oauth",
				AccountID:        "account-123",
				Enabled:          true,
				Email:            "test@example.com",
				Expired:          time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
				LastRefreshAt:    time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
				EncryptedProfile: "encrypted_data_here",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockQuerier{
				upsertFn: func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.CredentialProfile, error) {
					// Verify input parameters
					assert.Equal(t, tt.input.Type, arg.Type)
					assert.Equal(t, tt.input.AccountID, arg.AccountID)
					assert.Equal(t, tt.input.Enabled, arg.Enabled)
					assert.Equal(t, tt.input.Email, arg.Email)
					assert.Equal(t, tt.input.EncryptedProfile, arg.EncryptedProfile)

					return tt.mockResponse, nil
				},
			}

			repo := &PostgresRepository{queries: mock}
			ctx := context.Background()

			result, err := repo.UpsertByTypeAccount(ctx, tt.input)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedOutput.ID, result.ID)
				assert.Equal(t, tt.expectedOutput.Type, result.Type)
				assert.Equal(t, tt.expectedOutput.AccountID, result.AccountID)
				assert.Equal(t, tt.expectedOutput.Enabled, result.Enabled)
				assert.Equal(t, tt.expectedOutput.Email, result.Email)
				assert.Equal(t, tt.expectedOutput.EncryptedProfile, result.EncryptedProfile)
			}
		})
	}
}

func TestUpsertByTypeAccountIdempotency(t *testing.T) {
	// Test scenario: upserting same (type, account_id) twice should return same ID
	t.Run("idempotent upsert returns same ID", func(t *testing.T) {
		callCount := 0
		expectedID := "profile-789"

		mock := &MockQuerier{
			upsertFn: func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.CredentialProfile, error) {
				callCount++
				// Both calls should use same type and account_id
				assert.Equal(t, "oauth", arg.Type)
				assert.Equal(t, "account-999", arg.AccountID)

				// Return same ID to simulate idempotent behavior
				return sqlcdb.CredentialProfile{
					ID:               expectedID,
					Type:             arg.Type,
					AccountID:        arg.AccountID,
					Enabled:          arg.Enabled,
					Email:            arg.Email,
					EncryptedProfile: arg.EncryptedProfile,
					CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
					ModifiedAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		ctx := context.Background()

		// First upsert
		profile1 := domaincredential.Profile{
			Type:             "oauth",
			AccountID:        "account-999",
			Enabled:          true,
			Email:            "first@example.com",
			EncryptedProfile: "first_data",
		}
		result1, err1 := repo.UpsertByTypeAccount(ctx, profile1)

		// Second upsert (with updated email)
		profile2 := domaincredential.Profile{
			Type:             "oauth",
			AccountID:        "account-999",
			Enabled:          false,
			Email:            "second@example.com",
			EncryptedProfile: "second_data",
		}
		result2, err2 := repo.UpsertByTypeAccount(ctx, profile2)

		// Verify both succeeded
		require.NoError(t, err1)
		require.NoError(t, err2)

		// Verify same ID returned (idempotent)
		assert.Equal(t, expectedID, result1.ID)
		assert.Equal(t, expectedID, result2.ID)

		// Verify we called the mock twice
		assert.Equal(t, 2, callCount)
	})
}

func TestUpsertByTypeAccountParameterMapping(t *testing.T) {
	// Test scenario: verify all fields are correctly mapped from domain to sqlc params
	t.Run("maps all domain fields to sqlc params", func(t *testing.T) {
		now := time.Now()
		input := domaincredential.Profile{
			Type:             "google",
			AccountID:        "acc-123",
			Enabled:          true,
			Email:            "user@google.com",
			Expired:          now.Add(24 * time.Hour),
			LastRefreshAt:    now,
			EncryptedProfile: "encrypted_google_token",
		}

		var capturedParams sqlcdb.UpsertCredentialProfileByTypeAccountParams
		mock := &MockQuerier{
			upsertFn: func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.CredentialProfile, error) {
				capturedParams = arg
				return sqlcdb.CredentialProfile{
					ID:               "test-id",
					Type:             arg.Type,
					AccountID:        arg.AccountID,
					Enabled:          arg.Enabled,
					Email:            arg.Email,
					Expired:          arg.Expired,
					LastRefreshAt:    arg.LastRefreshAt,
					EncryptedProfile: arg.EncryptedProfile,
					CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
					ModifiedAt:       pgtype.Timestamptz{Time: now, Valid: true},
				}, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		_, err := repo.UpsertByTypeAccount(context.Background(), input)

		require.NoError(t, err)

		// Verify all fields mapped correctly
		assert.Equal(t, input.Type, capturedParams.Type)
		assert.Equal(t, input.AccountID, capturedParams.AccountID)
		assert.Equal(t, input.Enabled, capturedParams.Enabled)
		assert.Equal(t, input.Email, capturedParams.Email)
		assert.Equal(t, input.EncryptedProfile, capturedParams.EncryptedProfile)
		assert.Equal(t, input.Expired, capturedParams.Expired.Time)
		assert.Equal(t, input.LastRefreshAt, capturedParams.LastRefreshAt.Time)
	})
}
