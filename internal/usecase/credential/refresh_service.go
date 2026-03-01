package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type refreshService struct {
	repo       Repository
	refreshers map[string]Refresher
	encryptor  Encryptor
	now        func() time.Time
}

func NewRefreshService(repo Repository, refreshers map[string]Refresher, encryptor Encryptor) RefreshService {
	return &refreshService{repo: repo, refreshers: refreshers, encryptor: encryptor, now: time.Now}
}

func (s *refreshService) RefreshDue(ctx context.Context) error {
	profiles, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}

	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}

		if !isDue(profile, s.now()) {
			continue
		}

		refresher, ok := s.refreshers[profile.Type]
		if !ok {
			continue
		}

		oldPayload, err := s.decryptProfile(profile.EncryptedProfile)
		if err != nil {
			return fmt.Errorf("decrypt existing profile %s: %w", profile.ID, err)
		}

		newEncryptedSecret, expiredAt, refreshErr := refresher.Refresh(ctx, profile)
		now := s.now().UTC()

		if refreshErr != nil {
			profile.Enabled = false
			if _, updateErr := s.repo.Update(ctx, profile); updateErr != nil {
				return fmt.Errorf("disable profile after refresh failure: %w", updateErr)
			}
			continue
		}

		oldPayload.AccessToken = newEncryptedSecret
		if oldPayload.RefreshToken != "" {
			oldPayload.RefreshToken = newEncryptedSecret
		}
		if oldPayload.IDToken != "" {
			oldPayload.IDToken = newEncryptedSecret
		}
		oldPayload.Expired = expiredAt.UTC()
		oldPayload.Enabled = &[]bool{true}[0]
		oldPayload.LastRefresh = now

		raw, marshalErr := json.Marshal(oldPayload)
		if marshalErr != nil {
			return fmt.Errorf("marshal refreshed profile json: %w", marshalErr)
		}

		encProfile, encryptErr := s.encryptor.Encrypt(string(raw))
		if encryptErr != nil {
			return fmt.Errorf("encrypt refreshed profile json: %w", encryptErr)
		}

		profile.EncryptedProfile = encProfile
		profile.Expired = expiredAt
		profile.Enabled = true
		profile.LastRefreshAt = now

		if _, updateErr := s.repo.Update(ctx, profile); updateErr != nil {
			return fmt.Errorf("update profile after refresh: %w", updateErr)
		}
	}

	return nil
}

func (s *refreshService) decryptProfile(encrypted string) (CredentialProfile, error) {
	decrypted, err := s.encryptor.Decrypt(encrypted)
	if err != nil {
		return CredentialProfile{}, fmt.Errorf("decrypt profile: %w", err)
	}

	var payload CredentialProfile
	if err := json.Unmarshal([]byte(decrypted), &payload); err != nil {
		return CredentialProfile{}, fmt.Errorf("unmarshal profile json: %w", err)
	}

	return payload, nil
}

func isDue(profile domaincredential.Profile, now time.Time) bool {
	if profile.Expired.IsZero() {
		return false
	}

	margin := 5 * time.Minute
	return profile.Expired.Before(now.Add(margin))
}
