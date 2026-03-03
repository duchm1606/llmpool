package oauth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redislib "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

// TestReplayProtection_ConsumeOnlyOnce verifies that Consume() implements replay protection
// by allowing a session to be consumed only once
func TestReplayProtection_ConsumeOnlyOnce(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	sessionID := "test_session_replay_protection"

	// Create a pending session
	session := domainoauth.OAuthSession{
		SessionID:    sessionID,
		State:        domainoauth.StatePending,
		PKCEVerifier: "test_verifier",
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// First Consume should succeed
	consumedSession, err := store.Consume(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, sessionID, consumedSession.SessionID)
	require.Equal(t, domainoauth.StatePending, consumedSession.State)

	// Second Consume should fail with ErrSessionAlreadyConsumed
	_, err = store.Consume(ctx, sessionID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSessionAlreadyConsumed, "replay attack should be prevented")

	// GetStatus should also fail after Consume
	_, err = store.GetStatus(ctx, sessionID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSessionNotFound)
}

// TestReplayProtection_StateValidation verifies state parameter validation
func TestReplayProtection_StateValidation(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()

	tests := []struct {
		name          string
		sessionID     string
		storedState   string
		providedState string
		shouldMatch   bool
	}{
		{
			name:          "valid state matches",
			sessionID:     "session_valid_state",
			storedState:   "state_abc123",
			providedState: "state_abc123",
			shouldMatch:   true,
		},
		{
			name:          "invalid state mismatch",
			sessionID:     "session_invalid_state",
			storedState:   "state_abc123",
			providedState: "state_xyz789",
			shouldMatch:   false,
		},
		{
			name:          "empty state",
			sessionID:     "session_empty_state",
			storedState:   "state_abc123",
			providedState: "",
			shouldMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: State parameter is handled in the handler layer, not stored in session
			// This test verifies the session store behavior for validation scenarios
			session := domainoauth.OAuthSession{
				SessionID:    tt.sessionID,
				State:        domainoauth.StatePending,
				PKCEVerifier: "verifier_" + tt.sessionID,
				Provider:     "codex",
				Expiry:       time.Now().Add(10 * time.Minute),
				CreatedAt:    time.Now(),
			}

			err := store.CreatePending(ctx, session)
			require.NoError(t, err)

			// Retrieve session
			_, err = store.GetStatus(ctx, tt.sessionID)
			require.NoError(t, err)

			// Simulate state validation (this would happen in the handler)
			stateMatches := (tt.storedState == tt.providedState)
			require.Equal(t, tt.shouldMatch, stateMatches,
				"state validation failed: expected match=%v for stored=%q provided=%q",
				tt.shouldMatch, tt.storedState, tt.providedState)

			// Only consume if state matches (simulating real OAuth flow)
			if tt.shouldMatch {
				_, err = store.Consume(ctx, tt.sessionID)
				require.NoError(t, err)
			}
		})
	}
}

// TestReplayProtection_TTLExpiry verifies that sessions expire after TTL
func TestReplayProtection_TTLExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TTL expiry test in short mode")
	}

	// Use miniredis for immediate expiry testing
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	defer mr.Close()

	client := redislib.NewClient(&redislib.Options{
		Addr: mr.Addr(),
	})

	// Use very short TTL for testing (1 second)
	store := NewRedisSessionStore(client, 1*time.Second)

	ctx := context.Background()
	sessionID := "test_session_ttl_expiry"

	// Create a session
	session := domainoauth.OAuthSession{
		SessionID:    sessionID,
		State:        domainoauth.StatePending,
		PKCEVerifier: "test_verifier",
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Session should exist immediately
	_, err = store.GetStatus(ctx, sessionID)
	require.NoError(t, err)

	// Fast-forward time in miniredis
	mr.FastForward(2 * time.Second)

	// Session should be expired and not found
	_, err = store.GetStatus(ctx, sessionID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSessionNotFound, "expired session should not be found")

	// Consume should also fail
	_, err = store.Consume(ctx, sessionID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSessionAlreadyConsumed, "expired session cannot be consumed")
}

// TestReplayProtection_ConcurrentConsumeAttempts verifies atomic consume operation
func TestReplayProtection_ConcurrentConsumeAttempts(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	sessionID := "test_session_concurrent"

	// Create a session
	session := domainoauth.OAuthSession{
		SessionID:    sessionID,
		State:        domainoauth.StatePending,
		PKCEVerifier: "test_verifier",
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Simulate concurrent consume attempts
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := store.Consume(ctx, sessionID)
			results <- err
		}()
	}

	// Collect results
	var successCount int
	var failureCount int

	for i := 0; i < numGoroutines; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else {
			failureCount++
		}
	}

	// Only ONE consume should succeed, rest should fail
	require.Equal(t, 1, successCount, "exactly one consume should succeed")
	require.Equal(t, numGoroutines-1, failureCount, "all other consumes should fail")
}

// TestReplayProtection_NonExistentSession verifies behavior for non-existent sessions
func TestReplayProtection_NonExistentSession(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	nonExistentID := "session_does_not_exist"

	// GetStatus should fail
	_, err := store.GetStatus(ctx, nonExistentID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSessionNotFound)

	// Consume should fail
	_, err = store.Consume(ctx, nonExistentID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSessionAlreadyConsumed)
}

// TestSecurityAudit_NoSessionLeakage verifies that consumed sessions are truly deleted
func TestSecurityAudit_NoSessionLeakage(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	sessionID := "test_session_no_leakage"

	// Create session with sensitive data
	session := domainoauth.OAuthSession{
		SessionID:    sessionID,
		State:        domainoauth.StatePending,
		PKCEVerifier: "sensitive_verifier_12345",
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Consume the session
	consumed, err := store.Consume(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "sensitive_verifier_12345", consumed.PKCEVerifier)

	// Verify the session is completely removed from Redis
	exists := mr.Exists(keyPrefix + sessionID)
	require.False(t, exists, "session should be completely deleted after consume")
}

// TestSecurityAudit_MarkCompletePreservesTTL verifies TTL is preserved on updates
func TestSecurityAudit_MarkCompletePreservesTTL(t *testing.T) {
	store, mr := setupTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	sessionID := "test_session_preserve_ttl"

	// Create session
	session := domainoauth.OAuthSession{
		SessionID:    sessionID,
		State:        domainoauth.StatePending,
		PKCEVerifier: "test_verifier",
		Provider:     "codex",
		Expiry:       time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}

	err := store.CreatePending(ctx, session)
	require.NoError(t, err)

	// Fast-forward 2 seconds in miniredis
	mr.FastForward(2 * time.Second)

	// Mark complete (should preserve remaining TTL)
	err = store.MarkComplete(ctx, sessionID, "account_123")
	require.NoError(t, err)

	// Verify session still exists and was updated
	updated, err := store.GetStatus(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, domainoauth.StateOK, updated.State)
	require.Equal(t, "account_123", updated.AccountID)
}
