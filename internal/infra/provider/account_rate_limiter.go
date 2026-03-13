package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	defaultAccountInitiator = "agent"
	userAccountInitiator    = "user"
	minuteWindow            = time.Minute
	fiveHourSessionWindow   = 5 * time.Hour
	rateLimitRetryCount     = 5
	rateLimitKeyBuffer      = time.Minute
)

type AccountRateLimitConfig struct {
	RequestsPerMinute       int
	RequestsPer5HourSession int
}

type AccountRateLimitDecision struct {
	Allowed   bool
	Initiator string
}

type AccountRateLimiter interface {
	Reserve(ctx context.Context, providerType, accountID string, now time.Time, consumeSessionQuota bool) (AccountRateLimitDecision, error)
	GetUsage(ctx context.Context, providerType, accountID string, now time.Time) (*domainquota.SessionQuotaUsage, error)
}

type redisAccountRateLimiter struct {
	client *redis.Client
	cfg    AccountRateLimitConfig
	logger *zap.Logger
}

type redisKeyReader interface {
	Get(ctx context.Context, key string) *redis.StringCmd
}

type accountSessionState struct {
	WindowStart       time.Time `json:"window_start"`
	RequestCount      int       `json:"request_count"`
	UserInitiatorUsed bool      `json:"user_initiator_used"`
}

func NewRedisAccountRateLimiter(
	client *redis.Client,
	cfg AccountRateLimitConfig,
	logger *zap.Logger,
) AccountRateLimiter {
	return &redisAccountRateLimiter{
		client: client,
		cfg:    cfg,
		logger: logger,
	}
}

func (l *redisAccountRateLimiter) Reserve(
	ctx context.Context,
	providerType, accountID string,
	now time.Time,
	consumeSessionQuota bool,
) (AccountRateLimitDecision, error) {
	trimmedAccountID := strings.TrimSpace(accountID)
	if trimmedAccountID == "" || l.cfg.RequestsPerMinute <= 0 || l.cfg.RequestsPer5HourSession <= 0 {
		return AccountRateLimitDecision{Allowed: true, Initiator: defaultAccountInitiator}, nil
	}

	minuteKey := buildMinuteRateLimitKey(providerType, trimmedAccountID, now)
	sessionKey := buildSessionRateLimitKey(providerType, trimmedAccountID)

	var decision AccountRateLimitDecision
	for attempt := 0; attempt < rateLimitRetryCount; attempt++ {
		err := l.client.Watch(ctx, func(tx *redis.Tx) error {
			minuteCount, err := readMinuteCount(ctx, tx, minuteKey)
			if err != nil {
				return err
			}

			sessionState, err := readSessionState(ctx, tx, sessionKey, now)
			if err != nil {
				return err
			}

			if minuteCount >= l.cfg.RequestsPerMinute {
				decision = AccountRateLimitDecision{Allowed: false, Initiator: defaultAccountInitiator}
				return nil
			}
			if consumeSessionQuota && sessionState.RequestCount >= l.cfg.RequestsPer5HourSession {
				decision = AccountRateLimitDecision{Allowed: false, Initiator: defaultAccountInitiator}
				return nil
			}

			decision = AccountRateLimitDecision{Allowed: true, Initiator: defaultAccountInitiator}
			if consumeSessionQuota {
				sessionState.RequestCount++
			}
			if consumeSessionQuota && !sessionState.UserInitiatorUsed {
				sessionState.UserInitiatorUsed = true
				decision.Initiator = userAccountInitiator
			}

			sessionPayload, err := json.Marshal(sessionState)
			if err != nil {
				return fmt.Errorf("marshal session state: %w", err)
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, minuteKey, minuteCount+1, ttlUntilNextMinuteWindow(now))
				pipe.Set(ctx, sessionKey, sessionPayload, ttlUntilSessionWindowEnd(sessionState.WindowStart, now))
				return nil
			})
			return err
		}, minuteKey, sessionKey)
		if err == nil {
			return decision, nil
		}
		if err != redis.TxFailedErr {
			return AccountRateLimitDecision{}, fmt.Errorf("reserve account rate limit: %w", err)
		}
	}

	return AccountRateLimitDecision{}, fmt.Errorf("reserve account rate limit: transaction conflict")
}

func (l *redisAccountRateLimiter) GetUsage(
	ctx context.Context,
	providerType, accountID string,
	now time.Time,
) (*domainquota.SessionQuotaUsage, error) {
	trimmedAccountID := strings.TrimSpace(accountID)
	if trimmedAccountID == "" || l.cfg.RequestsPerMinute <= 0 || l.cfg.RequestsPer5HourSession <= 0 {
		return nil, nil
	}

	minuteKey := buildMinuteRateLimitKey(providerType, trimmedAccountID, now)
	sessionKey := buildSessionRateLimitKey(providerType, trimmedAccountID)

	minuteCount, err := readMinuteCount(ctx, l.client, minuteKey)
	if err != nil {
		return nil, fmt.Errorf("get minute usage: %w", err)
	}

	sessionState, err := readSessionState(ctx, l.client, sessionKey, now)
	if err != nil {
		return nil, fmt.Errorf("get session usage: %w", err)
	}

	minuteStart := now.UTC().Truncate(minuteWindow)
	minuteEnd := minuteStart.Add(minuteWindow)
	windowStart := sessionState.WindowStart.UTC()
	windowEnd := windowStart.Add(fiveHourSessionWindow)

	return &domainquota.SessionQuotaUsage{
		WindowStartUTC:       windowStart,
		WindowEndUTC:         windowEnd,
		MinuteWindowStartUTC: minuteStart,
		MinuteWindowEndUTC:   minuteEnd,
		RequestsPerMinute:    l.cfg.RequestsPerMinute,
		RequestsThisMinute:   minuteCount,
		RemainingThisMinute:  clampRemaining(l.cfg.RequestsPerMinute, minuteCount),
		RequestsPerSession:   l.cfg.RequestsPer5HourSession,
		RequestsThisSession:  sessionState.RequestCount,
		RemainingThisSession: clampRemaining(l.cfg.RequestsPer5HourSession, sessionState.RequestCount),
		FirstInitiatorUsed:   sessionState.UserInitiatorUsed,
	}, nil
}

func buildMinuteRateLimitKey(providerType, accountID string, now time.Time) string {
	bucket := now.UTC().Truncate(minuteWindow).Unix()
	return fmt.Sprintf("rate_limit:account:minute:%s:%s:%d", providerType, accountID, bucket)
}

func buildSessionRateLimitKey(providerType, accountID string) string {
	return fmt.Sprintf("rate_limit:account:session:%s:%s", providerType, accountID)
}

func readMinuteCount(ctx context.Context, reader redisKeyReader, key string) (int, error) {
	count, err := reader.Get(ctx, key).Int()
	if err == nil {
		return count, nil
	}
	if err == redis.Nil {
		return 0, nil
	}
	return 0, fmt.Errorf("read minute count: %w", err)
}

func readSessionState(ctx context.Context, reader redisKeyReader, key string, now time.Time) (accountSessionState, error) {
	payload, err := reader.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return newAccountSessionState(now), nil
	}
	if err != nil {
		return accountSessionState{}, fmt.Errorf("read session state: %w", err)
	}

	var state accountSessionState
	if err := json.Unmarshal(payload, &state); err != nil {
		return accountSessionState{}, fmt.Errorf("unmarshal session state: %w", err)
	}
	if state.WindowStart.IsZero() || !now.Before(state.WindowStart.Add(fiveHourSessionWindow)) {
		return newAccountSessionState(now), nil
	}
	return state, nil
}

func newAccountSessionState(now time.Time) accountSessionState {
	return accountSessionState{
		WindowStart:       now.UTC(),
		RequestCount:      0,
		UserInitiatorUsed: false,
	}
}

func ttlUntilNextMinuteWindow(now time.Time) time.Duration {
	bucketEnd := now.UTC().Truncate(minuteWindow).Add(minuteWindow)
	return positiveTTL(bucketEnd.Sub(now.UTC()) + rateLimitKeyBuffer)
}

func ttlUntilSessionWindowEnd(windowStart, now time.Time) time.Duration {
	windowEnd := windowStart.UTC().Add(fiveHourSessionWindow)
	return positiveTTL(windowEnd.Sub(now.UTC()) + rateLimitKeyBuffer)
}

func positiveTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}

func clampRemaining(limit, used int) int {
	remaining := limit - used
	if remaining < 0 {
		return 0
	}
	return remaining
}
