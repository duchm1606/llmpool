package quota

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	domainquota "github.com/duchoang/llmpool/internal/domain/quota"
	"github.com/go-redis/redismock/v9"
)

func TestRedisStateCache_GetCredentialState_CacheMiss(t *testing.T) {
	client, mock := redismock.NewClientMock()
	cache := NewRedisStateCache(client)

	mock.ExpectGet("quota:cred:cred-1").RedisNil()

	state, err := cache.GetCredentialState(context.Background(), "cred-1")
	if err != nil {
		t.Fatalf("GetCredentialState() error = %v", err)
	}
	if state != nil {
		t.Error("expected nil state for cache miss")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRedisStateCache_SetAndGetCredentialState(t *testing.T) {
	client, mock := redismock.NewClientMock()
	cache := NewRedisStateCache(client)

	now := time.Now().Truncate(time.Second)
	state := domainquota.CredentialState{
		CredentialID:  "cred-1",
		Status:        domainquota.StatusHealthy,
		LastCheckedAt: now,
	}

	data, _ := json.Marshal(state)
	mock.ExpectSet("quota:cred:cred-1", data, 2*time.Hour).SetVal("OK")

	err := cache.SetCredentialState(context.Background(), state, 2*time.Hour)
	if err != nil {
		t.Fatalf("SetCredentialState() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRedisStateCache_SetModelState(t *testing.T) {
	client, mock := redismock.NewClientMock()
	cache := NewRedisStateCache(client)

	now := time.Now().Truncate(time.Second)
	state := domainquota.ModelState{
		CredentialID:  "cred-1",
		ModelID:       "gpt-4",
		Status:        domainquota.StatusHealthy,
		LastCheckedAt: now,
	}

	data, _ := json.Marshal(state)
	mock.ExpectSet("quota:model:cred-1:gpt-4", data, 2*time.Hour).SetVal("OK")

	err := cache.SetModelState(context.Background(), state, 2*time.Hour)
	if err != nil {
		t.Fatalf("SetModelState() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRedisStateCache_GetModelState_CacheMiss(t *testing.T) {
	client, mock := redismock.NewClientMock()
	cache := NewRedisStateCache(client)

	mock.ExpectGet("quota:model:cred-1:gpt-4").RedisNil()

	state, err := cache.GetModelState(context.Background(), "cred-1", "gpt-4")
	if err != nil {
		t.Fatalf("GetModelState() error = %v", err)
	}
	if state != nil {
		t.Error("expected nil state for cache miss")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRedisStateCache_Ping(t *testing.T) {
	client, mock := redismock.NewClientMock()
	cache := NewRedisStateCache(client)

	mock.ExpectPing().SetVal("PONG")

	err := cache.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRedisStateCache_CountCredentialStates(t *testing.T) {
	client, mock := redismock.NewClientMock()
	cache := NewRedisStateCache(client)

	// SCAN is called with cursor 0, pattern, and count
	mock.ExpectScan(0, "quota:cred:*", 100).SetVal([]string{
		"quota:cred:cred-1",
		"quota:cred:cred-2",
		"quota:cred:cred-3",
	}, 0) // cursor 0 means done

	count, err := cache.CountCredentialStates(context.Background())
	if err != nil {
		t.Fatalf("CountCredentialStates() error = %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRedisStateCache_ListCredentialStates_Empty(t *testing.T) {
	client, mock := redismock.NewClientMock()
	cache := NewRedisStateCache(client)

	// SCAN returns empty with cursor 0 (done)
	mock.ExpectScan(0, "quota:cred:*", 100).SetVal([]string{}, 0)

	states, err := cache.ListCredentialStates(context.Background())
	if err != nil {
		t.Fatalf("ListCredentialStates() error = %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected empty slice, got %d states", len(states))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
