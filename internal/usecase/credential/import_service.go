package credential

import (
	"context"
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

func (s *importService) Import(ctx context.Context, input ImportInput) (domaincredential.Profile, error) {
	rawPayload := input.Payload.ToRawMap()
	if len(rawPayload) == 0 {
		return domaincredential.Profile{}, fmt.Errorf("payload is required")
	}

	provider := detectProvider(input.ProviderHint, input.Label, input.Payload.Provider)
	if provider == "" {
		return domaincredential.Profile{}, fmt.Errorf("unable to detect provider")
	}

	accessToken := input.Payload.AccessToken
	if accessToken == "" {
		return domaincredential.Profile{}, fmt.Errorf("access token is required")
	}

	refreshToken := input.Payload.RefreshToken
	expiresRaw := input.Payload.ExpiresAt
	expiresAt := parseExpiry(expiresRaw)

	encAccess, err := s.encryptor.Encrypt(accessToken)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("encrypt access token: %w", err)
	}

	encRefresh := ""
	if refreshToken != "" {
		encRefresh, err = s.encryptor.Encrypt(refreshToken)
		if err != nil {
			return domaincredential.Profile{}, fmt.Errorf("encrypt refresh token: %w", err)
		}
	}

	now := s.now().UTC()
	profile := domaincredential.Profile{
		ID:              uuid.NewString(),
		Provider:        provider,
		Label:           defaultLabel(input.Label, provider),
		SourcePath:      input.Source,
		Email:           input.Payload.Email,
		AccountID:       input.Payload.AccountID,
		HasRefreshToken: refreshToken != "",
		Status:          "active",
		CreatedAt:       now,
		UpdatedAt:       now,
		Secret: domaincredential.Secret{
			AccessToken:  encAccess,
			RefreshToken: encRefresh,
			ExpiresAt:    expiresAt,
			Raw:          rawPayload,
		},
	}

	saved, err := s.repo.Save(ctx, profile)
	if err != nil {
		return domaincredential.Profile{}, fmt.Errorf("save credential profile: %w", err)
	}

	return saved, nil
}

func detectProvider(providerHint, label, payloadProvider string) string {
	if p := strings.TrimSpace(strings.ToLower(providerHint)); p != "" {
		return p
	}

	if p := strings.TrimSpace(strings.ToLower(payloadProvider)); p != "" {
		return p
	}

	name := strings.ToLower(label)
	prefixes := []struct {
		prefix   string
		provider string
	}{
		{prefix: "codex-", provider: "openai"},
		{prefix: "openai-", provider: "openai"},
		{prefix: "claude-", provider: "anthropic"},
		{prefix: "anthropic-", provider: "anthropic"},
		{prefix: "gemini-", provider: "gemini"},
		{prefix: "vertex-", provider: "vertex"},
		{prefix: "qwen-", provider: "qwen"},
		{prefix: "iflow-", provider: "iflow"},
		{prefix: "antigravity-", provider: "antigravity"},
		{prefix: "kiro-", provider: "kiro"},
		{prefix: "copilot-", provider: "copilot"},
	}

	for _, pair := range prefixes {
		if strings.HasPrefix(name, pair.prefix) {
			return pair.provider
		}
	}

	return ""
}

func defaultLabel(input, provider string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed != "" {
		return trimmed
	}
	return provider + "-profile"
}

func parseExpiry(input string) time.Time {
	if input == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC3339, input)
	if err != nil {
		return time.Time{}
	}

	return parsed
}
