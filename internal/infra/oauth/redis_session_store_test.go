package oauth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	redislib "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*RedisSessionStore, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())

	client := redislib.NewClient(&redislib.Options{
		Addr: mr.Addr(),
	})

	store := NewRedisSessionStore(client, 5*time.Minute)

	return store, mr
}

func TestCreatePending(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	session := domainoauth.OAuthSession{
		SessionID:    "test-session-123",
		State:        domainoauth.StatePending,
		PKCEVerifier: "verifier-xyz",
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Verify session exists in Redis
	retrieved, err := store.GetStatus(ctx, session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, session.SessionID, retrieved.SessionID)
	assert.Equal(t, domainoauth.StatePending, retrieved.State)
	assert.Equal(t, session.PKCEVerifier, retrieved.PKCEVerifier)
	assert.Equal(t, session.Provider, retrieved.Provider)
}

func TestGetStatus_NotFound(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()

	_, err := store.GetStatus(ctx, "nonexistent-session")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestMarkComplete(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	session := domainoauth.OAuthSession{
		SessionID:    "test-session-456",
		State:        domainoauth.StatePending,
		PKCEVerifier: "verifier-abc",
		Provider:     "codex",
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Mark as complete
	err = store.MarkComplete(ctx, session.SessionID, domainoauth.ConnectionSummary{AccountID: "account-789"})
	require.NoError(t, err)

	// Verify state change
	retrieved, err := store.GetStatus(ctx, session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, domainoauth.StateOK, retrieved.State)
	assert.Equal(t, "account-789", retrieved.AccountID)
	require.NotNil(t, retrieved.Connection)
	assert.Equal(t, "account-789", retrieved.Connection.AccountID)
	assert.NotNil(t, retrieved.CompletedAt)
}

func TestMarkError(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	session := domainoauth.OAuthSession{
		SessionID:    "test-session-error",
		State:        domainoauth.StatePending,
		PKCEVerifier: "verifier-def",
		Provider:     "codex",
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Mark as error
	err = store.MarkError(ctx, session.SessionID, "auth_failed", "Authorization was denied")
	require.NoError(t, err)

	// Verify state change
	retrieved, err := store.GetStatus(ctx, session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, domainoauth.StateError, retrieved.State)
	assert.Equal(t, "auth_failed", retrieved.ErrorCode)
	assert.Equal(t, "Authorization was denied", retrieved.ErrorMessage)
	assert.NotNil(t, retrieved.CompletedAt)
}

func TestConsume_SingleUse(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	session := domainoauth.OAuthSession{
		SessionID:    "test-session-consume",
		State:        domainoauth.StatePending,
		PKCEVerifier: "verifier-ghi",
		Provider:     "codex",
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// First consume should succeed
	consumed, err := store.Consume(ctx, session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, session.SessionID, consumed.SessionID)
	assert.Equal(t, domainoauth.StatePending, consumed.State)

	// Second consume should fail (already consumed)
	_, err = store.Consume(ctx, session.SessionID)
	assert.ErrorIs(t, err, ErrSessionAlreadyConsumed)
}

func TestConsume_NotFound(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()

	_, err := store.Consume(ctx, "nonexistent-session")
	assert.ErrorIs(t, err, ErrSessionAlreadyConsumed)
}

func TestTTLExpiry(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redislib.NewClient(&redislib.Options{
		Addr: mr.Addr(),
	})

	// Set short TTL for testing
	store := NewRedisSessionStore(client, 2*time.Second)

	ctx := context.Background()
	session := domainoauth.OAuthSession{
		SessionID:    "test-session-ttl",
		State:        domainoauth.StatePending,
		PKCEVerifier: "verifier-jkl",
		Provider:     "codex",
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Session should exist immediately
	_, err = store.GetStatus(ctx, session.SessionID)
	require.NoError(t, err)

	// Fast-forward time in miniredis
	mr.FastForward(3 * time.Second)

	// Session should be expired now
	_, err = store.GetStatus(ctx, session.SessionID)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestStateTransitions(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()

	tests := []struct {
		name           string
		initialState   domainoauth.OAuthState
		operation      func(sessionID string) error
		expectedState  domainoauth.OAuthState
		checkAccountID bool
		expectedAccID  string
	}{
		{
			name:         "pending to ok",
			initialState: domainoauth.StatePending,
			operation: func(sessionID string) error {
				return store.MarkComplete(ctx, sessionID, domainoauth.ConnectionSummary{AccountID: "account-success"})
			},
			expectedState:  domainoauth.StateOK,
			checkAccountID: true,
			expectedAccID:  "account-success",
		},
		{
			name:         "pending to error",
			initialState: domainoauth.StatePending,
			operation: func(sessionID string) error {
				return store.MarkError(ctx, sessionID, "timeout", "Request timed out")
			},
			expectedState:  domainoauth.StateError,
			checkAccountID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID := "session-" + tt.name
			session := domainoauth.OAuthSession{
				SessionID:    sessionID,
				State:        tt.initialState,
				PKCEVerifier: "verifier-test",
				Provider:     "codex",
				CreatedAt:    time.Now(),
			}

			err := store.CreatePending(ctx, session)
			require.NoError(t, err)

			// Perform operation
			err = tt.operation(sessionID)
			require.NoError(t, err)

			// Verify final state
			retrieved, err := store.GetStatus(ctx, sessionID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedState, retrieved.State)
			assert.NotNil(t, retrieved.CompletedAt)

			if tt.checkAccountID {
				assert.Equal(t, tt.expectedAccID, retrieved.AccountID)
			}
		})
	}
}

func TestUpdateSessionPreservesTTL(t *testing.T) {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redislib.NewClient(&redislib.Options{
		Addr: mr.Addr(),
	})

	store := NewRedisSessionStore(client, 10*time.Minute)

	ctx := context.Background()
	session := domainoauth.OAuthSession{
		SessionID:    "test-session-ttl-preserve",
		State:        domainoauth.StatePending,
		PKCEVerifier: "verifier-mno",
		Provider:     "codex",
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Fast-forward 5 minutes
	mr.FastForward(5 * time.Minute)

	// Mark as complete (should preserve remaining TTL)
	err = store.MarkComplete(ctx, session.SessionID, domainoauth.ConnectionSummary{AccountID: "account-ttl-test"})
	require.NoError(t, err)

	// Session should still exist
	retrieved, err := store.GetStatus(ctx, session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, domainoauth.StateOK, retrieved.State)

	// Fast-forward another 6 minutes (total 11 minutes, exceeds original TTL)
	mr.FastForward(6 * time.Minute)

	// Session should be expired now
	_, err = store.GetStatus(ctx, session.SessionID)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}
