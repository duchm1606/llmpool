package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	redislib "github.com/redis/go-redis/v9"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

const (
	keyPrefix = "oauth:session:"
)

var (
	// ErrSessionNotFound is returned when session is not found or expired
	ErrSessionNotFound = errors.New("session not found or expired")
	// ErrSessionAlreadyConsumed is returned when attempting to consume an already consumed session
	ErrSessionAlreadyConsumed = errors.New("session already consumed")
)

// RedisSessionStore implements OAuthSessionStore using Redis
type RedisSessionStore struct {
	client *redislib.Client
	ttl    time.Duration
}

// NewRedisSessionStore creates a new Redis-backed session store
func NewRedisSessionStore(client *redislib.Client, ttl time.Duration) *RedisSessionStore {
	return &RedisSessionStore{
		client: client,
		ttl:    ttl,
	}
}

// CreatePending creates a new pending OAuth session
func (s *RedisSessionStore) CreatePending(ctx context.Context, session domainoauth.OAuthSession) error {
	key := keyPrefix + session.SessionID

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	ttl := s.ttl
	if !session.Expiry.IsZero() {
		remaining := time.Until(session.Expiry)
		if remaining > 0 {
			ttl = remaining
		}
	}

	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("store session: %w", err)
	}

	return nil
}

// GetStatus retrieves current session status
func (s *RedisSessionStore) GetStatus(ctx context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	key := keyPrefix + sessionID

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return domainoauth.OAuthSession{}, ErrSessionNotFound
		}
		return domainoauth.OAuthSession{}, fmt.Errorf("get session: %w", err)
	}

	var session domainoauth.OAuthSession
	if err := json.Unmarshal(data, &session); err != nil {
		return domainoauth.OAuthSession{}, fmt.Errorf("unmarshal session: %w", err)
	}

	return session, nil
}

// MarkComplete marks session as successfully completed with account ID
func (s *RedisSessionStore) MarkComplete(ctx context.Context, sessionID string, summary domainoauth.ConnectionSummary) error {
	session, err := s.GetStatus(ctx, sessionID)
	if err != nil {
		return err
	}

	now := time.Now()
	session.State = domainoauth.StateOK
	session.AccountID = summary.AccountID
	session.Connection = &summary
	session.CompletedAt = &now

	return s.updateSession(ctx, sessionID, session)
}

// MarkError marks session as failed with error details
func (s *RedisSessionStore) MarkError(ctx context.Context, sessionID string, errorCode string, errorMessage string) error {
	session, err := s.GetStatus(ctx, sessionID)
	if err != nil {
		return err
	}

	now := time.Now()
	session.State = domainoauth.StateError
	session.ErrorCode = errorCode
	session.ErrorMessage = errorMessage
	session.CompletedAt = &now

	return s.updateSession(ctx, sessionID, session)
}

// Consume retrieves and deletes a session (one-time use)
func (s *RedisSessionStore) Consume(ctx context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	key := keyPrefix + sessionID

	// Use GETDEL to atomically get and delete the key
	data, err := s.client.GetDel(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return domainoauth.OAuthSession{}, ErrSessionAlreadyConsumed
		}
		return domainoauth.OAuthSession{}, fmt.Errorf("consume session: %w", err)
	}

	var session domainoauth.OAuthSession
	if err := json.Unmarshal(data, &session); err != nil {
		return domainoauth.OAuthSession{}, fmt.Errorf("unmarshal session: %w", err)
	}

	return session, nil
}

// updateSession is a helper to update an existing session while preserving TTL
func (s *RedisSessionStore) updateSession(ctx context.Context, sessionID string, session domainoauth.OAuthSession) error {
	key := keyPrefix + sessionID

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	// Get remaining TTL to preserve it
	ttl, err := s.client.TTL(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("get ttl: %w", err)
	}

	// If key doesn't exist or has no expiry, use default TTL
	if ttl < 0 {
		ttl = s.ttl
	}

	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	return nil
}
