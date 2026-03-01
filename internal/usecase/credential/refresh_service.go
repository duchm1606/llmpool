package credential

import (
	"context"
	"fmt"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type refreshService struct {
	repo       Repository
	refreshers map[string]Refresher
	now        func() time.Time
}

func NewRefreshService(repo Repository, refreshers map[string]Refresher) RefreshService {
	return &refreshService{repo: repo, refreshers: refreshers, now: time.Now}
}

func (s *refreshService) RefreshDue(ctx context.Context) error {
	profiles, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}

	for _, profile := range profiles {
		if !isDue(profile, s.now()) {
			continue
		}

		refresher, ok := s.refreshers[profile.Provider]
		if !ok {
			continue
		}

		secret, refreshErr := refresher.Refresh(ctx, profile)
		now := s.now().UTC()

		if refreshErr != nil {
			profile.Status = "refresh_failed"
			profile.RefreshError = refreshErr.Error()
			profile.UpdatedAt = now
			_, _ = s.repo.Update(ctx, profile)
			continue
		}

		profile.Secret.AccessToken = secret.AccessToken
		profile.Secret.RefreshToken = secret.RefreshToken
		profile.Secret.ExpiresAt = secret.ExpiresAt
		profile.Secret.Raw = secret.Raw
		profile.Status = "active"
		profile.RefreshError = ""
		profile.UpdatedAt = now
		profile.LastRefreshAt = &now

		if _, updateErr := s.repo.Update(ctx, profile); updateErr != nil {
			return fmt.Errorf("update profile after refresh: %w", updateErr)
		}
	}

	return nil
}

func isDue(profile domaincredential.Profile, now time.Time) bool {
	if !profile.HasRefreshToken {
		return false
	}

	if profile.Secret.ExpiresAt.IsZero() {
		return false
	}

	margin := 5 * time.Minute
	return profile.Secret.ExpiresAt.Before(now.Add(margin))
}
