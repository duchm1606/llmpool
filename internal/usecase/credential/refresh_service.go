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
		if err := s.refreshProfile(ctx, profile, false); err != nil {
			return err
		}
	}

	return nil
}

// RefreshCredential refreshes a specific credential immediately, bypassing due checks.
func (s *refreshService) RefreshCredential(ctx context.Context, credentialID string) error {
	profile, err := s.repo.GetByID(ctx, credentialID)
	if err != nil {
		return fmt.Errorf("get profile: %w", err)
	}
	if profile == nil {
		return fmt.Errorf("credential %s not found", credentialID)
	}

	return s.refreshProfile(ctx, *profile, true)
}

func (s *refreshService) refreshProfile(ctx context.Context, profile domaincredential.Profile, force bool) error {
	if !profile.Enabled {
		return nil
	}

	if !force && !isDue(profile, s.now()) {
		return nil
	}

	refresher, ok := s.refreshers[profile.Type]
	if !ok {
		return nil
	}

	oldPayload, err := s.decryptProfile(profile)
	if err != nil {
		return fmt.Errorf("decrypt existing profile %s: %w", profile.ID, err)
	}

	// Get the current refresh token for the refresh operation
	currentRefreshToken := oldPayload.RefreshToken
	if currentRefreshToken == "" {
		if force {
			return fmt.Errorf("refresh token missing for credential %s", profile.ID)
		}
		// No refresh token available, skip this profile in scheduled flow
		return nil
	}

	result, refreshErr := refresher.Refresh(ctx, currentRefreshToken)
	now := s.now().UTC()

	if refreshErr != nil {
		if force {
			return fmt.Errorf("refresh credential %s: %w", profile.ID, refreshErr)
		}

		profile.Enabled = false
		if _, updateErr := s.repo.Update(ctx, profile); updateErr != nil {
			return fmt.Errorf("disable profile after refresh failure: %w", updateErr)
		}
		return nil
	}

	// Safe update: Only update access token and expiry
	// Only update refresh token if provider returned a new one (rotation)
	oldPayload.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		oldPayload.RefreshToken = result.RefreshToken
	}
	oldPayload.Expired = result.ExpiresAt.UTC()
	oldPayload.Enabled = &[]bool{true}[0]
	oldPayload.LastRefresh = now

	raw, marshalErr := json.Marshal(oldPayload)
	if marshalErr != nil {
		return fmt.Errorf("marshal refreshed profile json: %w", marshalErr)
	}

	encProfile, iv, tag, encryptErr := s.encryptor.Encrypt(string(raw))
	if encryptErr != nil {
		return fmt.Errorf("encrypt refreshed profile json: %w", encryptErr)
	}

	profile.EncryptedProfile = encProfile
	profile.EncryptedIV = stringPtrOrNil(iv)
	profile.EncryptedTag = stringPtrOrNil(tag)
	profile.Expired = result.ExpiresAt
	profile.Enabled = true
	profile.LastRefreshAt = now

	if _, updateErr := s.repo.Update(ctx, profile); updateErr != nil {
		return fmt.Errorf("update profile after refresh: %w", updateErr)
	}

	return nil
}

func (s *refreshService) decryptProfile(profile domaincredential.Profile) (CredentialProfile, error) {
	decrypted, err := s.encryptor.Decrypt(profile.EncryptedProfile, stringValue(profile.EncryptedIV), stringValue(profile.EncryptedTag))
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
