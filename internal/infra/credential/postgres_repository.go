package credential

import (
	"context"
	"fmt"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	"github.com/duchoang/llmpool/internal/infra/credential/sqlcdb"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	queries sqlcdb.Querier
}

func NewCredentialRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{queries: sqlcdb.New(pool)}
}

func (r *PostgresRepository) Save(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	row, err := r.queries.CreateCredentialProfile(ctx, sqlcdb.CreateCredentialProfileParams{
		ID:               profile.ID,
		Type:             profile.Type,
		AccountID:        profile.AccountID,
		Enabled:          profile.Enabled,
		Email:            profile.Email,
		Expired:          toTimestamptz(profile.Expired),
		LastRefreshAt:    toTimestamptz(profile.LastRefreshAt),
		EncryptedProfile: profile.EncryptedProfile,
	})
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("create credential profile: %w", err)
	}

	return toDomainProfile(row), nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]domaincredential.Profile, error) {
	rows, err := r.queries.ListCredentialProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credential profiles: %w", err)
	}

	out := make([]domaincredential.Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainProfile(row))
	}

	return out, nil
}

func (r *PostgresRepository) Update(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	row, err := r.queries.UpdateCredentialProfile(ctx, sqlcdb.UpdateCredentialProfileParams{
		ID:               profile.ID,
		Type:             profile.Type,
		AccountID:        profile.AccountID,
		Enabled:          profile.Enabled,
		Email:            profile.Email,
		Expired:          toTimestamptz(profile.Expired),
		LastRefreshAt:    toTimestamptz(profile.LastRefreshAt),
		EncryptedProfile: profile.EncryptedProfile,
	})
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("update credential profile: %w", err)
	}

	return toDomainProfile(row), nil
}

func (r *PostgresRepository) UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	row, err := r.queries.UpsertCredentialProfileByTypeAccount(ctx, sqlcdb.UpsertCredentialProfileByTypeAccountParams{
		ID:               profile.ID,
		Type:             profile.Type,
		AccountID:        profile.AccountID,
		Enabled:          profile.Enabled,
		Email:            profile.Email,
		Expired:          toTimestamptz(profile.Expired),
		LastRefreshAt:    toTimestamptz(profile.LastRefreshAt),
		EncryptedProfile: profile.EncryptedProfile,
	})
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("upsert credential profile: %w", err)
	}

	return toDomainProfile(row), nil
}

func (r *PostgresRepository) ListEnabled(ctx context.Context) ([]domaincredential.Profile, error) {
	rows, err := r.queries.ListEnabledCredentialProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled credential profiles: %w", err)
	}

	out := make([]domaincredential.Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainProfile(row))
	}

	return out, nil
}

func (r *PostgresRepository) CountEnabled(ctx context.Context) (int64, error) {
	count, err := r.queries.CountEnabledCredentialProfiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("count enabled credential profiles: %w", err)
	}

	return count, nil
}

func (r *PostgresRepository) RandomSample(ctx context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error) {
	// Bound sampleSize to valid int32 range for DB query
	var limit int32
	switch {
	case sampleSize < 0:
		limit = 0
	case sampleSize > 2147483647: // math.MaxInt32
		limit = 2147483647
	default:
		limit = int32(sampleSize) //nolint:gosec // bounds checked above
	}

	rows, err := r.queries.RandomSampleEnabledCredentialProfiles(ctx, sqlcdb.RandomSampleEnabledCredentialProfilesParams{
		Limit:   limit,
		Column2: fmt.Sprintf("%d", seed),
	})
	if err != nil {
		return nil, fmt.Errorf("random sample enabled credential profiles: %w", err)
	}

	out := make([]domaincredential.Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainProfile(row))
	}

	return out, nil
}

func toDomainProfile(row sqlcdb.CredentialProfile) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
	}
}

func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func fromTimestamptz(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}

	return ts.Time
}
