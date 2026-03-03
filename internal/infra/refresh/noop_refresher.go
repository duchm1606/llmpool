package refresh

import (
	"context"
	"fmt"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type NoopRefresher struct{}

func NewNoopRefresher() *NoopRefresher {
	return &NoopRefresher{}
}

func (n *NoopRefresher) Refresh(_ context.Context, profile domaincredential.Profile) (string, time.Time, error) {
	return "", time.Time{}, fmt.Errorf("refresh is not implemented for provider %q", profile.Type)
}
