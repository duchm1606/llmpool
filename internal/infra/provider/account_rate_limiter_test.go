package provider

import (
	"context"
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
		RequestsPer5HourSession: 50,
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
	if usage.RequestsThisSession != 12 || usage.RemainingThisSession != 38 {
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
		RequestsPer5HourSession: 50,
	}, zap.NewNop())

	usage, err := limiter.GetUsage(context.Background(), "copilot", "", time.Now())
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if usage != nil {
		t.Fatal("expected nil usage for empty account")
	}
}
