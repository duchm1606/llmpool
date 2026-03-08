package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	"github.com/google/uuid"
)

type importService struct {
	repo              Repository
	encryptor         Encryptor
	now               func() time.Time
	registryRefresher RegistryRefresher
}

func NewImportService(repo Repository, encryptor Encryptor, registryRefresher RegistryRefresher) ImportService {
	return &importService{
		repo:              repo,
		encryptor:         encryptor,
		now:               time.Now,
		registryRefresher: registryRefresher,
	}
}

func (s *importService) Import(ctx context.Context, profileInput CredentialProfile) (domaincredential.Profile, error) {
	profileType := strings.TrimSpace(profileInput.Type)
	accountID := strings.TrimSpace(profileInput.AccountID)
	if profileType == "" {
		return domaincredential.Profile{}, fmt.Errorf("type is required")
	}
	if accountID == "" {
		return domaincredential.Profile{}, fmt.Errorf("account_id is required")
	}

	rawProfile, err := json.Marshal(profileInput)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("marshal credential profile: %w", err)
	}

	encProfile, iv, tag, err := s.encryptor.Encrypt(string(rawProfile))
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("encrypt profile json: %w", err)
	}

	enabled := true
	if profileInput.Enabled != nil {
		enabled = *profileInput.Enabled
	}

	profile := domaincredential.Profile{
		ID:               uuid.NewString(),
		Type:             profileType,
		AccountID:        accountID,
		Enabled:          enabled,
		Email:            strings.TrimSpace(profileInput.Email),
		Expired:          profileInput.Expired,
		LastRefreshAt:    profileInput.LastRefresh,
		EncryptedProfile: encProfile,
		EncryptedIV:      stringPtrOrNil(iv),
		EncryptedTag:     stringPtrOrNil(tag),
	}

	savedProfile, err := s.repo.UpsertByTypeAccount(ctx, profile)
	if err != nil {
		return domaincredential.Profile{}, err
	}

	if s.registryRefresher != nil {
		s.registryRefresher(ctx, savedProfile.Type, savedProfile.AccountID)
	}

	return savedProfile, nil
}
