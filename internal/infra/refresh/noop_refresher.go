package refresh

import (
	"context"
	"fmt"

	"github.com/duchoang/llmpool/internal/usecase/credential"
)

type NoopRefresher struct{}

func NewNoopRefresher() *NoopRefresher {
	return &NoopRefresher{}
}

func (n *NoopRefresher) Refresh(ctx context.Context, refreshToken string) (credential.RefreshResult, error) {
	return credential.RefreshResult{}, fmt.Errorf("refresh is not implemented for this provider")
}
