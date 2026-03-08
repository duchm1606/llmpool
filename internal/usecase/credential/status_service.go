package credential

import (
	"context"
	"fmt"
	"strings"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type statusService struct {
	repo Repository
}

func NewStatusService(repo Repository) StatusService {
	return &statusService{repo: repo}
}

func (s *statusService) SetEnabled(ctx context.Context, credentialID string, enabled bool) (domaincredential.Profile, error) {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return domaincredential.Profile{}, fmt.Errorf("credential_id is required")
	}

	profile, err := s.repo.GetByID(ctx, credentialID)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("get credential profile: %w", err)
	}
	if profile == nil {
		return domaincredential.Profile{}, ErrCredentialNotFound
	}

	profile.Enabled = enabled

	updated, err := s.repo.Update(ctx, *profile)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("update credential profile: %w", err)
	}

	return updated, nil
}
