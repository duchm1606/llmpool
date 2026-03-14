package provider

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	redismock "github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestRedisAccountRateLimiter_GetUsage(t *testing.T) {
	t.Parallel()

	client, mock := redismock.NewClientMock()
	limiter := NewRedisAccountRateLimiter(client, AccountRateLimitConfig{
		RequestsPerMinute:       5,
		RequestsPer5HourSession: 30,
	}, zap.NewNop())

	now := time.Date(2026, 3, 13, 10, 15, 30, 0, time.UTC)
	minuteKey := buildMinuteRateLimitKey("copilot", "acct-1", now)
	sessionKey := buildSessionRateLimitKey("copilot", "acct-1")
	sessionPayload := `{"window_start":"2026-03-13T08:00:00Z","request_count":12,"user_initiator_used":true}`

	mock.ExpectGet(minuteKey).SetVal("3")
	mock.ExpectGet(sessionKey).SetVal(sessionPayload)

	usage, err := limiter.GetUsage(context.Background(), "copilot", "acct-1", now)
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.RequestsThisMinute != 3 || usage.RemainingThisMinute != 2 {
		t.Fatalf("unexpected minute usage: %+v", usage)
	}
	if usage.RequestsThisSession != 12 || usage.RemainingThisSession != 18 {
		t.Fatalf("unexpected session usage: %+v", usage)
	}
	if !usage.FirstInitiatorUsed {
		t.Fatal("expected first initiator to be marked as used")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRedisAccountRateLimiter_GetUsage_ReturnsNilForEmptyAccount(t *testing.T) {
	t.Parallel()

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	defer func() { _ = client.Close() }()

	limiter := NewRedisAccountRateLimiter(client, AccountRateLimitConfig{
		RequestsPerMinute:       5,
		RequestsPer5HourSession: 30,
	}, zap.NewNop())

	usage, err := limiter.GetUsage(context.Background(), "copilot", "", time.Now())
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if usage != nil {
		t.Fatal("expected nil usage for empty account")
	}
}

func TestRedisAccountRateLimiter_Reserve_UsesUserInitiatorOnlyOncePerSession(t *testing.T) {
	t.Parallel()

	client, mock := redismock.NewClientMock()
	limiter := NewRedisAccountRateLimiter(client, AccountRateLimitConfig{
		RequestsPerMinute:       5,
		RequestsPer5HourSession: 30,
	}, zap.NewNop())

	now := time.Date(2026, 3, 14, 10, 15, 30, 0, time.UTC)
	minuteKey := buildMinuteRateLimitKey("copilot", "acct-1", now)
	sessionKey := buildSessionRateLimitKey("copilot", "acct-1")
	minuteTTL := ttlUntilNextMinuteWindow(now)
	sessionTTL := ttlUntilSessionWindowEnd(now, now)
	firstStatePayload, err := json.Marshal(accountSessionState{
		WindowStart:       now,
		RequestCount:      1,
		UserInitiatorUsed: true,
	})
	if err != nil {
		t.Fatalf("marshal first state payload: %v", err)
	}
	secondStatePayload, err := json.Marshal(accountSessionState{
		WindowStart:       now,
		RequestCount:      2,
		UserInitiatorUsed: true,
	})
	if err != nil {
		t.Fatalf("marshal second state payload: %v", err)
	}

	mock.ExpectWatch(minuteKey, sessionKey)
	mock.ExpectGet(minuteKey).RedisNil()
	mock.ExpectGet(sessionKey).RedisNil()
	mock.ExpectTxPipeline()
	mock.ExpectSet(minuteKey, 1, minuteTTL).SetVal("OK")
	mock.ExpectSet(sessionKey, firstStatePayload, sessionTTL).SetVal("OK")
	mock.ExpectTxPipelineExec()

	decision, err := limiter.Reserve(context.Background(), "copilot", "acct-1", now, true)
	if err != nil {
		t.Fatalf("Reserve() first error = %v", err)
	}
	if !decision.Allowed {
		t.Fatal("expected first reserve to be allowed")
	}
	if decision.Initiator != userAccountInitiator {
		t.Fatalf("unexpected first initiator: got %q want %q", decision.Initiator, userAccountInitiator)
	}

	mock.ExpectWatch(minuteKey, sessionKey)
	mock.ExpectGet(minuteKey).SetVal("1")
	mock.ExpectGet(sessionKey).SetVal(`{"window_start":"2026-03-14T10:15:30Z","request_count":1,"user_initiator_used":true}`)
	mock.ExpectTxPipeline()
	mock.ExpectSet(minuteKey, 2, minuteTTL).SetVal("OK")
	mock.ExpectSet(sessionKey, secondStatePayload, sessionTTL).SetVal("OK")
	mock.ExpectTxPipelineExec()

	decision, err = limiter.Reserve(context.Background(), "copilot", "acct-1", now, true)
	if err != nil {
		t.Fatalf("Reserve() second error = %v", err)
	}
	if !decision.Allowed {
		t.Fatal("expected second reserve to be allowed")
	}
	if decision.Initiator != defaultAccountInitiator {
		t.Fatalf("unexpected second initiator: got %q want %q", decision.Initiator, defaultAccountInitiator)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
