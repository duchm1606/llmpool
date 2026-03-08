package credential

import (
	"context"
	"fmt"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	"github.com/duchoang/llmpool/internal/infra/credential/sqlcdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	queries credentialQueries
}

type credentialQueries interface {
	CreateCredentialProfile(ctx context.Context, arg sqlcdb.CreateCredentialProfileParams) (sqlcdb.CreateCredentialProfileRow, error)
	ListCredentialProfiles(ctx context.Context) ([]sqlcdb.ListCredentialProfilesRow, error)
	GetCredentialProfileByID(ctx context.Context, id string) (sqlcdb.GetCredentialProfileByIDRow, error)
	UpdateCredentialProfile(ctx context.Context, arg sqlcdb.UpdateCredentialProfileParams) (sqlcdb.UpdateCredentialProfileRow, error)
	UpsertCredentialProfileByTypeAccount(ctx context.Context, arg sqlcdb.UpsertCredentialProfileByTypeAccountParams) (sqlcdb.UpsertCredentialProfileByTypeAccountRow, error)
	ListEnabledCredentialProfiles(ctx context.Context) ([]sqlcdb.ListEnabledCredentialProfilesRow, error)
	CountEnabledCredentialProfiles(ctx context.Context) (int64, error)
	RandomSampleEnabledCredentialProfiles(ctx context.Context, arg sqlcdb.RandomSampleEnabledCredentialProfilesParams) ([]sqlcdb.RandomSampleEnabledCredentialProfilesRow, error)
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
		EncryptedIv:      toText(profile.EncryptedIV),
		EncryptedTag:     toText(profile.EncryptedTag),
	})
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("create credential profile: %w", err)
	}

	return toDomainProfileFromCreate(row), nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]domaincredential.Profile, error) {
	rows, err := r.queries.ListCredentialProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credential profiles: %w", err)
	}

	out := make([]domaincredential.Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainProfileFromList(row))
	}

	return out, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*domaincredential.Profile, error) {
	row, err := r.queries.GetCredentialProfileByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get credential profile by id: %w", err)
	}

	profile := toDomainProfileFromGetByID(row)
	return &profile, nil
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
		EncryptedIv:      toText(profile.EncryptedIV),
		EncryptedTag:     toText(profile.EncryptedTag),
	})
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("update credential profile: %w", err)
	}

	return toDomainProfileFromUpdate(row), nil
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
		EncryptedIv:      toText(profile.EncryptedIV),
		EncryptedTag:     toText(profile.EncryptedTag),
	})
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("upsert credential profile: %w", err)
	}

	return toDomainProfileFromUpsert(row), nil
}

func (r *PostgresRepository) ListEnabled(ctx context.Context) ([]domaincredential.Profile, error) {
	rows, err := r.queries.ListEnabledCredentialProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled credential profiles: %w", err)
	}

	out := make([]domaincredential.Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainProfileFromListEnabled(row))
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
		out = append(out, toDomainProfileFromRandomSample(row))
	}

	return out, nil
}

func toDomainProfileFromCreate(row sqlcdb.CreateCredentialProfileRow) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
		EncryptedIV:      fromText(row.EncryptedIv),
		EncryptedTag:     fromText(row.EncryptedTag),
	}
}

func toDomainProfileFromUpdate(row sqlcdb.UpdateCredentialProfileRow) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
		EncryptedIV:      fromText(row.EncryptedIv),
		EncryptedTag:     fromText(row.EncryptedTag),
	}
}

func toDomainProfileFromUpsert(row sqlcdb.UpsertCredentialProfileByTypeAccountRow) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
		EncryptedIV:      fromText(row.EncryptedIv),
		EncryptedTag:     fromText(row.EncryptedTag),
	}
}

func toDomainProfileFromList(row sqlcdb.ListCredentialProfilesRow) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
		EncryptedIV:      fromText(row.EncryptedIv),
		EncryptedTag:     fromText(row.EncryptedTag),
	}
}

func toDomainProfileFromGetByID(row sqlcdb.GetCredentialProfileByIDRow) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
		EncryptedIV:      fromText(row.EncryptedIv),
		EncryptedTag:     fromText(row.EncryptedTag),
	}
}

func toDomainProfileFromListEnabled(row sqlcdb.ListEnabledCredentialProfilesRow) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
		EncryptedIV:      fromText(row.EncryptedIv),
		EncryptedTag:     fromText(row.EncryptedTag),
	}
}

func toDomainProfileFromRandomSample(row sqlcdb.RandomSampleEnabledCredentialProfilesRow) domaincredential.Profile {
	return domaincredential.Profile{
		ID:               row.ID,
		Type:             row.Type,
		AccountID:        row.AccountID,
		Enabled:          row.Enabled,
		Email:            row.Email,
		Expired:          fromTimestamptz(row.Expired),
		LastRefreshAt:    fromTimestamptz(row.LastRefreshAt),
		EncryptedProfile: row.EncryptedProfile,
		EncryptedIV:      fromText(row.EncryptedIv),
		EncryptedTag:     fromText(row.EncryptedTag),
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

func toText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}

	return pgtype.Text{String: *value, Valid: true}
}

func fromText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}

	v := value.String
	return &v
}
