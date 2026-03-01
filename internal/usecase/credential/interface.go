package credential

import (
	"context"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type Encryptor interface {
	Encrypt(plain string) (string, error)
	Decrypt(cipher string) (string, error)
}

type Repository interface {
	Save(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
	List(ctx context.Context) ([]domaincredential.Profile, error)
	Update(ctx context.Context, profile domaincredential.Profile) (domaincredential.Profile, error)
}

type Refresher interface {
	Refresh(ctx context.Context, profile domaincredential.Profile) (string, time.Time, error)
}

type ImportService interface {
	Import(ctx context.Context, profile CredentialProfile) (domaincredential.Profile, error)
}

type RefreshService interface {
	RefreshDue(ctx context.Context) error
}
