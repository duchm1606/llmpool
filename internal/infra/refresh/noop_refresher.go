package refresh

import (
	"context"
	"fmt"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type NoopRefresher struct{}

func NewNoopRefresher() *NoopRefresher {
	return &NoopRefresher{}
}

func (n *NoopRefresher) Refresh(_ context.Context, profile domaincredential.Profile) (domaincredential.Secret, error) {
	return domaincredential.Secret{}, fmt.Errorf("refresh is not implemented for provider %q", profile.Provider)
}
