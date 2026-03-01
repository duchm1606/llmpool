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
	repo      Repository
	encryptor Encryptor
	now       func() time.Time
}

func NewImportService(repo Repository, encryptor Encryptor) ImportService {
	return &importService{repo: repo, encryptor: encryptor, now: time.Now}
}

func (s *importService) Import(ctx context.Context, profileInput CredentialProfile) (domaincredential.Profile, error) {
	rawProfile, err := json.Marshal(profileInput)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("marshal credential profile: %w", err)
	}

	encProfile, err := s.encryptor.Encrypt(string(rawProfile))
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("encrypt profile json: %w", err)
	}

	enabled := true
	if profileInput.Enabled != nil {
		enabled = *profileInput.Enabled
	}

	profile := domaincredential.Profile{
		ID:               uuid.NewString(),
		Type:             strings.TrimSpace(profileInput.Type),
		AccountID:        strings.TrimSpace(profileInput.AccountID),
		Enabled:          enabled,
		Email:            strings.TrimSpace(profileInput.Email),
		Expired:          profileInput.Expired,
		LastRefreshAt:    profileInput.LastRefresh,
		EncryptedProfile: encProfile,
	}

	return s.repo.Save(ctx, profile)
}
