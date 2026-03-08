package credential

import (
	"context"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type listService struct {
	repo Repository
}

func NewListService(repo Repository) ListService {
	return &listService{repo: repo}
}

func (s *listService) List(ctx context.Context) ([]domaincredential.Profile, error) {
	return s.repo.List(ctx)
}
