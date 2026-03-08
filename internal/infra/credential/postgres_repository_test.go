package credential

import (
	"context"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	"github.com/duchoang/llmpool/internal/infra/credential/sqlcdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockQuerier implements sqlcdb.Querier for testing
type MockQuerier struct {
	createFn       func(ctx context.Context, arg sqlcdb.CreateCredentialProfileParams) (sqlcdb.CreateCredentialProfileRow, error)
	listFn         func(ctx context.Context) ([]sqlcdb.ListCredentialProfilesRow, error)
	getByIDFn      func(ctx context.Context, id string) (sqlcdb.GetCredentialProfileByIDRow, error)
	updateFn       func(ctx context.Context, arg sqlcdb.UpdateCredentialProfileParams) (sqlcdb.UpdateCredentialProfileRow, error)
	upsertFn       func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.UpsertCredentialProfileByTypeAccountRow, error)
	listEnabledFn  func(ctx context.Context) ([]sqlcdb.ListEnabledCredentialProfilesRow, error)
	countEnabledFn func(ctx context.Context) (int64, error)
	randomSampleFn func(ctx context.Context, arg sqlcdb.RandomSampleEnabledCredentialProfilesParams) ([]sqlcdb.RandomSampleEnabledCredentialProfilesRow, error)
}

func (m *MockQuerier) CreateCredentialProfile(ctx context.Context, arg sqlcdb.CreateCredentialProfileParams) (sqlcdb.CreateCredentialProfileRow, error) {
	if m.createFn != nil {
		return m.createFn(ctx, arg)
	}
	return sqlcdb.CreateCredentialProfileRow{}, nil
}

func (m *MockQuerier) ListCredentialProfiles(ctx context.Context) ([]sqlcdb.ListCredentialProfilesRow, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *MockQuerier) GetCredentialProfileByID(ctx context.Context, id string) (sqlcdb.GetCredentialProfileByIDRow, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return sqlcdb.GetCredentialProfileByIDRow{}, pgx.ErrNoRows
}

func (m *MockQuerier) UpdateCredentialProfile(ctx context.Context, arg sqlcdb.UpdateCredentialProfileParams) (sqlcdb.UpdateCredentialProfileRow, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, arg)
	}
	return sqlcdb.UpdateCredentialProfileRow{}, nil
}

func (m *MockQuerier) UpsertCredentialProfileByTypeAccount(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.UpsertCredentialProfileByTypeAccountRow, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, arg)
	}
	return sqlcdb.UpsertCredentialProfileByTypeAccountRow{}, nil
}

func (m *MockQuerier) ListEnabledCredentialProfiles(ctx context.Context) ([]sqlcdb.ListEnabledCredentialProfilesRow, error) {
	if m.listEnabledFn != nil {
		return m.listEnabledFn(ctx)
	}
	return []sqlcdb.ListEnabledCredentialProfilesRow{}, nil
}

func (m *MockQuerier) CountEnabledCredentialProfiles(ctx context.Context) (int64, error) {
	if m.countEnabledFn != nil {
		return m.countEnabledFn(ctx)
	}
	return 0, nil
}

func (m *MockQuerier) RandomSampleEnabledCredentialProfiles(ctx context.Context, arg sqlcdb.RandomSampleEnabledCredentialProfilesParams) ([]sqlcdb.RandomSampleEnabledCredentialProfilesRow, error) {
	if m.randomSampleFn != nil {
		return m.randomSampleFn(ctx, arg)
	}
	return []sqlcdb.RandomSampleEnabledCredentialProfilesRow{}, nil
}

func TestUpsertByTypeAccount(t *testing.T) {
	tests := []struct {
		name           string
		input          domaincredential.Profile
		mockResponse   sqlcdb.UpsertCredentialProfileByTypeAccountRow
		expectedOutput domaincredential.Profile
		expectError    bool
	}{
		{
			name: "successful upsert",
			input: domaincredential.Profile{
				ID:               "input-profile-123",
				Type:             "oauth",
				AccountID:        "account-123",
				Enabled:          true,
				Email:            "test@example.com",
				Expired:          time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
				LastRefreshAt:    time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
				EncryptedProfile: "encrypted_data_here",
			},
			mockResponse: sqlcdb.UpsertCredentialProfileByTypeAccountRow{
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
				upsertFn: func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.UpsertCredentialProfileByTypeAccountRow, error) {
					// Verify input parameters
					assert.Equal(t, tt.input.ID, arg.ID)
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
			upsertFn: func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.UpsertCredentialProfileByTypeAccountRow, error) {
				callCount++
				// Both calls should use same type and account_id
				assert.Equal(t, "oauth", arg.Type)
				assert.Equal(t, "account-999", arg.AccountID)

				// Return same ID to simulate idempotent behavior
				return sqlcdb.UpsertCredentialProfileByTypeAccountRow{
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
			ID:               "profile-input-1",
			Type:             "oauth",
			AccountID:        "account-999",
			Enabled:          true,
			Email:            "first@example.com",
			EncryptedProfile: "first_data",
		}
		result1, err1 := repo.UpsertByTypeAccount(ctx, profile1)

		// Second upsert (with updated email)
		profile2 := domaincredential.Profile{
			ID:               "profile-input-2",
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
			ID:               "profile-input-3",
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
			upsertFn: func(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.UpsertCredentialProfileByTypeAccountRow, error) {
				capturedParams = arg
				return sqlcdb.UpsertCredentialProfileByTypeAccountRow{
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
		assert.Equal(t, input.ID, capturedParams.ID)
		assert.Equal(t, input.Type, capturedParams.Type)
		assert.Equal(t, input.AccountID, capturedParams.AccountID)
		assert.Equal(t, input.Enabled, capturedParams.Enabled)
		assert.Equal(t, input.Email, capturedParams.Email)
		assert.Equal(t, input.EncryptedProfile, capturedParams.EncryptedProfile)
		assert.Equal(t, input.Expired, capturedParams.Expired.Time)
		assert.Equal(t, input.LastRefreshAt, capturedParams.LastRefreshAt.Time)
	})
}

func TestListEnabled(t *testing.T) {
	t.Run("returns enabled profiles only", func(t *testing.T) {
		now := time.Now()
		mockProfiles := []sqlcdb.ListEnabledCredentialProfilesRow{
			{
				ID:               "profile-1",
				Type:             "codex",
				AccountID:        "account-1",
				Enabled:          true,
				Email:            "user1@example.com",
				Expired:          pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true},
				LastRefreshAt:    pgtype.Timestamptz{Time: now, Valid: true},
				EncryptedProfile: "encrypted_1",
				CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
				ModifiedAt:       pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				ID:               "profile-2",
				Type:             "codex",
				AccountID:        "account-2",
				Enabled:          true,
				Email:            "user2@example.com",
				Expired:          pgtype.Timestamptz{Time: now.Add(2 * time.Hour), Valid: true},
				LastRefreshAt:    pgtype.Timestamptz{Time: now, Valid: true},
				EncryptedProfile: "encrypted_2",
				CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
				ModifiedAt:       pgtype.Timestamptz{Time: now, Valid: true},
			},
		}

		mock := &MockQuerier{
			listEnabledFn: func(ctx context.Context) ([]sqlcdb.ListEnabledCredentialProfilesRow, error) {
				return mockProfiles, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		result, err := repo.ListEnabled(context.Background())

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "profile-1", result[0].ID)
		assert.Equal(t, "profile-2", result[1].ID)
		assert.True(t, result[0].Enabled)
		assert.True(t, result[1].Enabled)
	})

	t.Run("returns empty slice when no enabled profiles", func(t *testing.T) {
		mock := &MockQuerier{
			listEnabledFn: func(ctx context.Context) ([]sqlcdb.ListEnabledCredentialProfilesRow, error) {
				return []sqlcdb.ListEnabledCredentialProfilesRow{}, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		result, err := repo.ListEnabled(context.Background())

		require.NoError(t, err)
		assert.Len(t, result, 0)
	})
}

func TestCountEnabled(t *testing.T) {
	t.Run("returns count of enabled profiles", func(t *testing.T) {
		mock := &MockQuerier{
			countEnabledFn: func(ctx context.Context) (int64, error) {
				return 42, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		count, err := repo.CountEnabled(context.Background())

		require.NoError(t, err)
		assert.Equal(t, int64(42), count)
	})

	t.Run("returns zero when no enabled profiles", func(t *testing.T) {
		mock := &MockQuerier{
			countEnabledFn: func(ctx context.Context) (int64, error) {
				return 0, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		count, err := repo.CountEnabled(context.Background())

		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})
}

func TestRandomSample(t *testing.T) {
	t.Run("returns sampled profiles with correct params", func(t *testing.T) {
		now := time.Now()
		mockProfiles := []sqlcdb.RandomSampleEnabledCredentialProfilesRow{
			{
				ID:               "profile-sampled-1",
				Type:             "codex",
				AccountID:        "account-1",
				Enabled:          true,
				Email:            "user1@example.com",
				Expired:          pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true},
				LastRefreshAt:    pgtype.Timestamptz{Time: now, Valid: true},
				EncryptedProfile: "encrypted_1",
				CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
				ModifiedAt:       pgtype.Timestamptz{Time: now, Valid: true},
			},
		}

		var capturedParams sqlcdb.RandomSampleEnabledCredentialProfilesParams
		mock := &MockQuerier{
			randomSampleFn: func(ctx context.Context, arg sqlcdb.RandomSampleEnabledCredentialProfilesParams) ([]sqlcdb.RandomSampleEnabledCredentialProfilesRow, error) {
				capturedParams = arg
				return mockProfiles, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		result, err := repo.RandomSample(context.Background(), 5, 12345)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "profile-sampled-1", result[0].ID)

		// Verify params passed correctly
		assert.Equal(t, int32(5), capturedParams.Limit)
		assert.Equal(t, "12345", capturedParams.Column2) // seed as string
	})

	t.Run("seed affects ordering deterministically", func(t *testing.T) {
		var capturedSeeds []string
		mock := &MockQuerier{
			randomSampleFn: func(ctx context.Context, arg sqlcdb.RandomSampleEnabledCredentialProfilesParams) ([]sqlcdb.RandomSampleEnabledCredentialProfilesRow, error) {
				capturedSeeds = append(capturedSeeds, arg.Column2)
				return []sqlcdb.RandomSampleEnabledCredentialProfilesRow{}, nil
			},
		}

		repo := &PostgresRepository{queries: mock}

		_, _ = repo.RandomSample(context.Background(), 10, 111)
		_, _ = repo.RandomSample(context.Background(), 10, 222)

		require.Len(t, capturedSeeds, 2)
		assert.Equal(t, "111", capturedSeeds[0])
		assert.Equal(t, "222", capturedSeeds[1])
	})

	t.Run("returns empty slice when no profiles match", func(t *testing.T) {
		mock := &MockQuerier{
			randomSampleFn: func(ctx context.Context, arg sqlcdb.RandomSampleEnabledCredentialProfilesParams) ([]sqlcdb.RandomSampleEnabledCredentialProfilesRow, error) {
				return []sqlcdb.RandomSampleEnabledCredentialProfilesRow{}, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		result, err := repo.RandomSample(context.Background(), 10, 999)

		require.NoError(t, err)
		assert.Len(t, result, 0)
	})
}

func TestGetByID(t *testing.T) {
	t.Run("returns profile when found", func(t *testing.T) {
		now := time.Now().UTC()
		mock := &MockQuerier{
			getByIDFn: func(ctx context.Context, id string) (sqlcdb.GetCredentialProfileByIDRow, error) {
				assert.Equal(t, "profile-1", id)
				return sqlcdb.GetCredentialProfileByIDRow{
					ID:               "profile-1",
					Type:             "copilot",
					AccountID:        "octocat",
					Enabled:          true,
					Email:            "octocat@example.com",
					Expired:          pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true},
					LastRefreshAt:    pgtype.Timestamptz{Time: now, Valid: true},
					EncryptedProfile: "enc",
					EncryptedIv:      pgtype.Text{String: "iv", Valid: true},
					EncryptedTag:     pgtype.Text{String: "tag", Valid: true},
				}, nil
			},
		}

		repo := &PostgresRepository{queries: mock}
		profile, err := repo.GetByID(context.Background(), "profile-1")

		require.NoError(t, err)
		require.NotNil(t, profile)
		assert.Equal(t, "profile-1", profile.ID)
		assert.Equal(t, "copilot", profile.Type)
		assert.Equal(t, "octocat", profile.AccountID)
	})

	t.Run("returns nil when not found", func(t *testing.T) {
		mock := &MockQuerier{
			getByIDFn: func(ctx context.Context, id string) (sqlcdb.GetCredentialProfileByIDRow, error) {
				return sqlcdb.GetCredentialProfileByIDRow{}, pgx.ErrNoRows
			},
		}

		repo := &PostgresRepository{queries: mock}
		profile, err := repo.GetByID(context.Background(), "missing")

		require.NoError(t, err)
		assert.Nil(t, profile)
	})
}
