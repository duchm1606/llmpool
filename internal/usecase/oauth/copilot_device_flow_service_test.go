package oauth

import (
	"context"
	"errors"
	"testing"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
)

func TestCopilotDeviceFlowService_StartDeviceFlow_CompletesAndCachesSummary(t *testing.T) {
	provider := &oauthProviderStub{
		startResp: domainoauth.DeviceFlowResponse{
			DeviceCode:      "dc-success",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       60,
			Interval:        1,
		},
		pollPayloads: []domainoauth.TokenPayload{{
			AccessToken:  "copilot-token",
			RefreshToken: "github-token",
			AccountID:    "duchoang",
			Email:        "duc@example.com",
			ExpiresAt:    time.Now().Add(30 * time.Minute),
			TokenType:    "Bearer",
			Scope:        "read:user",
		}},
	}
	store := newOAuthSessionStoreStub()
	completion := &oauthCompletionStub{profile: domaincredential.Profile{
		ID:            "profile-1",
		Type:          "copilot",
		AccountID:     "duchoang",
		Email:         "duc@example.com",
		Enabled:       true,
		Expired:       time.Now().Add(30 * time.Minute),
		LastRefreshAt: time.Now(),
	}}

	svc := NewCopilotDeviceFlowService(provider, store, completion).(*copilotDeviceFlowService)
	svc.sleep = func(context.Context, time.Duration) error { return nil }

	resp, err := svc.StartDeviceFlow(context.Background())
	if err != nil {
		t.Fatalf("StartDeviceFlow() unexpected error: %v", err)
	}
	if resp.DeviceCode != "dc-success" {
		t.Fatalf("expected device code dc-success, got %s", resp.DeviceCode)
	}

	status := waitForOAuthSessionState(t, store, "dc-success", func(session domainoauth.OAuthSession) bool {
		return session.State == domainoauth.StateOK
	})

	if status.Connection == nil {
		t.Fatal("expected completed connection summary")
	}
	if status.Connection.AccountID != "duchoang" {
		t.Fatalf("expected account id duchoang, got %s", status.Connection.AccountID)
	}
	if completion.callCount != 1 {
		t.Fatalf("expected completion service call count 1, got %d", completion.callCount)
	}
}

func TestCopilotDeviceFlowService_StartDeviceFlow_TerminalError(t *testing.T) {
	provider := &oauthProviderStub{
		startResp: domainoauth.DeviceFlowResponse{
			DeviceCode:      "dc-error",
			UserCode:        "ERR-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       60,
			Interval:        1,
		},
		pollErrs: []error{errors.New("expired token")},
	}
	store := newOAuthSessionStoreStub()
	svc := NewCopilotDeviceFlowService(provider, store, &oauthCompletionStub{}).(*copilotDeviceFlowService)
	svc.sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := svc.StartDeviceFlow(context.Background()); err != nil {
		t.Fatalf("StartDeviceFlow() unexpected error: %v", err)
	}

	status := waitForOAuthSessionState(t, store, "dc-error", func(session domainoauth.OAuthSession) bool {
		return session.State == domainoauth.StateError
	})

	if status.ErrorCode != "expired_token" {
		t.Fatalf("expected expired_token, got %s", status.ErrorCode)
	}
}

func TestCopilotDeviceFlowService_GetDeviceStatus_NotFound(t *testing.T) {
	store := newOAuthSessionStoreStub()
	svc := NewCopilotDeviceFlowService(&oauthProviderStub{}, store, &oauthCompletionStub{})

	status, err := svc.GetDeviceStatus(context.Background(), "missing-device-code")
	if err != nil {
		t.Fatalf("GetDeviceStatus() unexpected error: %v", err)
	}
	if status != nil {
		t.Fatalf("expected nil status, got %+v", status)
	}
}

type oauthProviderStub struct {
	startResp     domainoauth.DeviceFlowResponse
	startErr      error
	pollPayloads  []domainoauth.TokenPayload
	pollErrs      []error
	pollCallCount int
}

func (f *oauthProviderStub) BuildAuthURL(context.Context, string, string) (domainoauth.AuthorizationURL, error) {
	return domainoauth.AuthorizationURL{}, nil
}

func (f *oauthProviderStub) ExchangeCode(context.Context, string, string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

func (f *oauthProviderStub) RefreshToken(context.Context, string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, nil
}

func (f *oauthProviderStub) StartDeviceFlow(context.Context) (domainoauth.DeviceFlowResponse, error) {
	if f.startErr != nil {
		return domainoauth.DeviceFlowResponse{}, f.startErr
	}
	return f.startResp, nil
}

func (f *oauthProviderStub) PollDevice(context.Context, string) (domainoauth.TokenPayload, error) {
	idx := f.pollCallCount
	f.pollCallCount++
	if idx < len(f.pollErrs) && f.pollErrs[idx] != nil {
		return domainoauth.TokenPayload{}, f.pollErrs[idx]
	}
	if idx < len(f.pollPayloads) {
		return f.pollPayloads[idx], nil
	}
	return domainoauth.TokenPayload{}, errors.New("authorization pending")
}

type oauthCompletionStub struct {
	profile   domaincredential.Profile
	err       error
	callCount int
}

func (s *oauthCompletionStub) CompleteOAuth(_ context.Context, accountID string, _ domainoauth.TokenPayload) (domaincredential.Profile, error) {
	s.callCount++
	if s.err != nil {
		return domaincredential.Profile{}, s.err
	}
	profile := s.profile
	if profile.AccountID == "" {
		profile.AccountID = accountID
	}
	if profile.Type == "" {
		profile.Type = "copilot"
	}
	return profile, nil
}

type oauthSessionStoreStub struct {
	items map[string]domainoauth.OAuthSession
}

func newOAuthSessionStoreStub() *oauthSessionStoreStub {
	return &oauthSessionStoreStub{items: make(map[string]domainoauth.OAuthSession)}
}

func (s *oauthSessionStoreStub) CreatePending(_ context.Context, session domainoauth.OAuthSession) error {
	s.items[session.SessionID] = session
	return nil
}

func (s *oauthSessionStoreStub) GetStatus(_ context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	session, ok := s.items[sessionID]
	if !ok {
		return domainoauth.OAuthSession{}, errors.New("session not found or expired")
	}
	return session, nil
}

func (s *oauthSessionStoreStub) MarkComplete(_ context.Context, sessionID string, summary domainoauth.ConnectionSummary) error {
	session, ok := s.items[sessionID]
	if !ok {
		return errors.New("session not found or expired")
	}
	session.State = domainoauth.StateOK
	session.AccountID = summary.AccountID
	session.Connection = &summary
	s.items[sessionID] = session
	return nil
}

func (s *oauthSessionStoreStub) MarkError(_ context.Context, sessionID string, errorCode string, errorMessage string) error {
	session, ok := s.items[sessionID]
	if !ok {
		return errors.New("session not found or expired")
	}
	session.State = domainoauth.StateError
	session.ErrorCode = errorCode
	session.ErrorMessage = errorMessage
	s.items[sessionID] = session
	return nil
}

func (s *oauthSessionStoreStub) Consume(_ context.Context, sessionID string) (domainoauth.OAuthSession, error) {
	session, ok := s.items[sessionID]
	if !ok {
		return domainoauth.OAuthSession{}, errors.New("session already consumed")
	}
	delete(s.items, sessionID)
	return session, nil
}

func waitForOAuthSessionState(
	t *testing.T,
	store *oauthSessionStoreStub,
	sessionID string,
	predicate func(domainoauth.OAuthSession) bool,
) domainoauth.OAuthSession {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session, err := store.GetStatus(context.Background(), sessionID)
		if err == nil && predicate(session) {
			return session
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for session %s state", sessionID)
	return domainoauth.OAuthSession{}
}
