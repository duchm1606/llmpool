package credential

import (
	"context"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

type Encryptor interface {
	Encrypt(plain string) (string, error)
	Decrypt(cipher string) (string, error)
}

type Repository interface {
	Save(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
	List(ctx context.Context) ([]domaincredential.Profile, error)
	Update(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
	UpsertByTypeAccount(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
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

type RefreshService interface {
	RefreshDue(ctx context.Context) error
}

type OAuthCompletionService interface {
	CompleteOAuth(ctx context.Context, accountID string, tokenPayload domainoauth.TokenPayload) (domaincredential.Profile, error)
}
