package credential

import (
	"context"
	"errors"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

var ErrCredentialNotFound = errors.New("credential profile not found")

type Encryptor interface {
	Encrypt(plain string) (ciphertext, iv, tag string, err error)
	Decrypt(cipher, iv, tag string) (string, error)
	ShouldEncrypt() bool
}

type Repository interface {
	Save(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
	List(ctx context.Context) ([]domaincredential.Profile, error)
	GetByID(ctx context.Context, id string) (*domaincredential.Profile, error)
	Update(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
	UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)

	// ListEnabled returns all credentials where enabled=true.
	ListEnabled(ctx context.Context) ([]domaincredential.Profile, error)
	// CountEnabled returns the count of enabled credentials.
	CountEnabled(ctx context.Context) (int64, error)
	// RandomSample returns a deterministic random sample of enabled credentials.
	// The seed is used for deterministic ordering via hash(id, seed).
	RandomSample(ctx context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error)
}

type RefreshResult struct {
	AccessToken  string
	RefreshToken string // May be new rotated token
	ExpiresAt    time.Time
}

type Refresher interface {
	Refresh(ctx context.Context, refreshToken string) (RefreshResult, error)
}

type ImportService interface {
	Import(ctx context.Context, profile CredentialProfile) (domaincredential.Profile, error)
}

// RegistryRefresher refreshes provider registry state after credential mutations.
type RegistryRefresher func(ctx context.Context, profileType, accountID string)

type ListService interface {
	List(ctx context.Context) ([]domaincredential.Profile, error)
}

type StatusService interface {
	SetEnabled(ctx context.Context, credentialID string, enabled bool) (domaincredential.Profile, error)
}

type RefreshService interface {
	RefreshDue(ctx context.Context) error
	RefreshCredential(ctx context.Context, credentialID string) error
}

type OAuthCompletionService interface {
	CompleteOAuth(ctx context.Context, accountID string, tokenPayload domainoauth.TokenPayload) (domaincredential.Profile, error)
}
