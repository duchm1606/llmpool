package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	usecasecompletion "github.com/duchoang/llmpool/internal/usecase/completion"
	"go.uber.org/zap"
)

type stubCredentialRepo struct {
	profiles []domaincredential.Profile
}

func (s *stubCredentialRepo) ListEnabled(ctx context.Context) ([]domaincredential.Profile, error) {
	return s.profiles, nil
}

type stubDecryptor struct{}

func (s *stubDecryptor) Decrypt(cipher, iv, tag string) (string, error) {
	return cipher, nil
}

type stubRateLimiter struct {
	decisions map[string][]AccountRateLimitDecision
	seen      map[string]int
	err       error
	consume   []bool
}

func (s *stubRateLimiter) Reserve(ctx context.Context, providerType, accountID string, now time.Time, consumeSessionQuota bool) (AccountRateLimitDecision, error) {
	if s.err != nil {
		return AccountRateLimitDecision{}, s.err
	}
	s.consume = append(s.consume, consumeSessionQuota)
	if s.seen == nil {
		s.seen = map[string]int{}
	}
	idx := s.seen[accountID]
	s.seen[accountID] = idx + 1
	choices := s.decisions[accountID]
	if len(choices) == 0 {
		return AccountRateLimitDecision{Allowed: true, Initiator: defaultAccountInitiator}, nil
	}
	if idx >= len(choices) {
		return choices[len(choices)-1], nil
	}
	return choices[idx], nil
}

func (s *stubRateLimiter) GetUsage(ctx context.Context, providerType, accountID string, now time.Time) (*domainquota.SessionQuotaUsage, error) {
	return nil, s.err
}

func TestPooledTokenFetcher_GetNextTokenWithInfo_UsesUserInitiatorForFirstSessionRequest(t *testing.T) {
	t.Parallel()

	fetcher := NewPooledTokenFetcher(
		&stubCredentialRepo{profiles: []domaincredential.Profile{newTestProfile("cred-1", "acct-1", "copilot", "user@example.com")}},
		&stubDecryptor{},
		zap.NewNop(),
		PooledTokenFetcherConfig{
			Limiter: &stubRateLimiter{decisions: map[string][]AccountRateLimitDecision{
				"acct-1": {{Allowed: true, Initiator: userAccountInitiator}},
			}},
		},
	)

	_, meta, err := fetcher.GetNextTokenWithInfo(context.Background(), "copilot")
	if err != nil {
		t.Fatalf("GetNextTokenWithInfo() error = %v", err)
	}
	if meta.CredentialID != "cred-1" {
		t.Fatalf("unexpected credential id: %q", meta.CredentialID)
	}
	if meta.AccountID != "acct-1" {
		t.Fatalf("unexpected account id: %q", meta.AccountID)
	}
	if meta.Initiator != userAccountInitiator {
		t.Fatalf("unexpected initiator: got %q want %q", meta.Initiator, userAccountInitiator)
	}
}

func TestPooledTokenFetcher_GetNextTokenWithInfo_UsesAgentInitiatorAfterFirstSessionRequest(t *testing.T) {
	t.Parallel()

	fetcher := NewPooledTokenFetcher(
		&stubCredentialRepo{profiles: []domaincredential.Profile{newTestProfile("cred-1", "acct-1", "copilot", "user@example.com")}},
		&stubDecryptor{},
		zap.NewNop(),
		PooledTokenFetcherConfig{
			Limiter: &stubRateLimiter{decisions: map[string][]AccountRateLimitDecision{
				"acct-1": {
					{Allowed: true, Initiator: userAccountInitiator},
					{Allowed: true, Initiator: defaultAccountInitiator},
				},
			}},
		},
	)

	_, firstMeta, err := fetcher.GetNextTokenWithInfo(context.Background(), "copilot")
	if err != nil {
		t.Fatalf("GetNextTokenWithInfo() first error = %v", err)
	}
	if firstMeta.Initiator != userAccountInitiator {
		t.Fatalf("unexpected first initiator: got %q want %q", firstMeta.Initiator, userAccountInitiator)
	}

	_, secondMeta, err := fetcher.GetNextTokenWithInfo(context.Background(), "copilot")
	if err != nil {
		t.Fatalf("GetNextTokenWithInfo() second error = %v", err)
	}
	if secondMeta.Initiator != defaultAccountInitiator {
		t.Fatalf("unexpected second initiator: got %q want %q", secondMeta.Initiator, defaultAccountInitiator)
	}
}

func TestPooledTokenFetcher_GetNextTokenWithInfo_SkipsRateLimitedAccount(t *testing.T) {
	t.Parallel()

	fetcher := NewPooledTokenFetcher(
		&stubCredentialRepo{profiles: []domaincredential.Profile{
			newTestProfile("cred-1", "acct-1", "copilot", "first@example.com"),
			newTestProfile("cred-2", "acct-2", "copilot", "second@example.com"),
		}},
		&stubDecryptor{},
		zap.NewNop(),
		PooledTokenFetcherConfig{
			Limiter: &stubRateLimiter{decisions: map[string][]AccountRateLimitDecision{
				"acct-1": {{Allowed: false, Initiator: defaultAccountInitiator}},
				"acct-2": {{Allowed: true, Initiator: defaultAccountInitiator}},
			}},
		},
	)

	_, meta, err := fetcher.GetNextTokenWithInfo(context.Background(), "copilot")
	if err != nil {
		t.Fatalf("GetNextTokenWithInfo() error = %v", err)
	}
	if meta.CredentialID != "cred-2" {
		t.Fatalf("unexpected credential id: %q", meta.CredentialID)
	}
	if meta.Initiator != defaultAccountInitiator {
		t.Fatalf("unexpected initiator: got %q want %q", meta.Initiator, defaultAccountInitiator)
	}
}

func TestPooledTokenFetcher_GetNextTokenWithInfo_ReturnsErrorWhenAllAccountsLimited(t *testing.T) {
	t.Parallel()

	fetcher := NewPooledTokenFetcher(
		&stubCredentialRepo{profiles: []domaincredential.Profile{newTestProfile("cred-1", "acct-1", "copilot", "user@example.com")}},
		&stubDecryptor{},
		zap.NewNop(),
		PooledTokenFetcherConfig{
			Limiter: &stubRateLimiter{decisions: map[string][]AccountRateLimitDecision{
				"acct-1": {{Allowed: false, Initiator: defaultAccountInitiator}},
			}},
		},
	)

	_, _, err := fetcher.GetNextTokenWithInfo(context.Background(), "copilot")
	if err == nil {
		t.Fatal("expected error when all accounts are rate limited")
	}
	var apiErr *domaincompletion.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.HTTPStatus != 429 {
		t.Fatalf("unexpected status: got %d want %d", apiErr.HTTPStatus, 429)
	}
}

func TestPooledTokenFetcher_GetNextTokenWithInfo_ReturnsLimiterError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("redis unavailable")
	fetcher := NewPooledTokenFetcher(
		&stubCredentialRepo{profiles: []domaincredential.Profile{newTestProfile("cred-1", "acct-1", "copilot", "user@example.com")}},
		&stubDecryptor{},
		zap.NewNop(),
		PooledTokenFetcherConfig{Limiter: &stubRateLimiter{err: wantErr}},
	)

	_, _, err := fetcher.GetNextTokenWithInfo(context.Background(), "copilot")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected limiter error %v, got %v", wantErr, err)
	}
}

func TestPooledTokenFetcher_GetNextTokenWithInfoForQuotaMode_BypassDoesNotConsumeSessionQuota(t *testing.T) {
	t.Parallel()

	limiter := &stubRateLimiter{decisions: map[string][]AccountRateLimitDecision{
		"acct-1": {{Allowed: true, Initiator: defaultAccountInitiator}},
	}}
	fetcher := NewPooledTokenFetcher(
		&stubCredentialRepo{profiles: []domaincredential.Profile{newTestProfile("cred-1", "acct-1", "copilot", "user@example.com")}},
		&stubDecryptor{},
		zap.NewNop(),
		PooledTokenFetcherConfig{Limiter: limiter},
	)

	_, meta, err := fetcher.GetNextTokenWithInfoForQuotaMode(context.Background(), "copilot", usecasecompletion.SessionQuotaBypass)
	if err != nil {
		t.Fatalf("GetNextTokenWithInfoForQuotaMode() error = %v", err)
	}
	if meta.Initiator != defaultAccountInitiator {
		t.Fatalf("unexpected initiator: got %q want %q", meta.Initiator, defaultAccountInitiator)
	}
	if len(limiter.consume) != 1 || limiter.consume[0] {
		t.Fatalf("expected consumeSessionQuota=false, got %+v", limiter.consume)
	}
}

func TestPooledTokenFetcher_GetNextTokenWithInfo_FallsBackToAnotherAccountWhenSessionQuotaExhausted(t *testing.T) {
	t.Parallel()

	fetcher := NewPooledTokenFetcher(
		&stubCredentialRepo{profiles: []domaincredential.Profile{
			newTestProfile("cred-1", "acct-1", "copilot", "first@example.com"),
			newTestProfile("cred-2", "acct-2", "copilot", "second@example.com"),
		}},
		&stubDecryptor{},
		zap.NewNop(),
		PooledTokenFetcherConfig{
			Limiter: &stubRateLimiter{decisions: map[string][]AccountRateLimitDecision{
				"acct-1": {{Allowed: false, Initiator: defaultAccountInitiator}},
				"acct-2": {{Allowed: true, Initiator: userAccountInitiator}},
			}},
		},
	)

	_, meta, err := fetcher.GetNextTokenWithInfo(context.Background(), "copilot")
	if err != nil {
		t.Fatalf("GetNextTokenWithInfo() error = %v", err)
	}
	if meta.CredentialID != "cred-2" {
		t.Fatalf("unexpected credential id: %q", meta.CredentialID)
	}
	if meta.Initiator != userAccountInitiator {
		t.Fatalf("unexpected initiator: got %q want %q", meta.Initiator, userAccountInitiator)
	}
}

func newTestProfile(id, accountID, profileType, email string) domaincredential.Profile {
	payload, _ := json.Marshal(DecryptedCredential{
		AccessToken: "value-for-" + id,
		AccountID:   accountID,
		Email:       email,
		Type:        profileType,
	})

	return domaincredential.Profile{
		ID:               id,
		AccountID:        accountID,
		Type:             profileType,
		EncryptedProfile: string(payload),
	}
}
