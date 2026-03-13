package oauth

import (
	"context"
	"fmt"
	"strings"
	"time"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
)

const (
	defaultCopilotPollInterval = 5 * time.Second
	maxCopilotPollInterval     = 30 * time.Second
)

type copilotDeviceFlowService struct {
	provider          OAuthProvider
	sessionStore      OAuthSessionStore
	completionService usecasecredential.OAuthCompletionService
	now               func() time.Time
	sleep             func(ctx context.Context, d time.Duration) error
}

// NewCopilotDeviceFlowService creates a service that orchestrates the Copilot device flow.
func NewCopilotDeviceFlowService(
	provider OAuthProvider,
	sessionStore OAuthSessionStore,
	completionService usecasecredential.OAuthCompletionService,
) DeviceFlowCoordinator {
	if completionService == nil {
		completionService = noopDeviceFlowCompletionService{}
	}

	return &copilotDeviceFlowService{
		provider:          provider,
		sessionStore:      sessionStore,
		completionService: completionService,
		now:               time.Now,
		sleep:             sleepWithContext,
	}
}

func (s *copilotDeviceFlowService) StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error) {
	resp, err := s.provider.StartDeviceFlow(ctx)
	if err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("start device flow: %w", err)
	}

	session := domainoauth.OAuthSession{
		SessionID:       resp.DeviceCode,
		State:           domainoauth.StatePending,
		Provider:        "copilot",
		Expiry:          s.now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		CreatedAt:       s.now(),
		DeviceCode:      resp.DeviceCode,
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		Interval:        resp.Interval,
	}

	if err := s.sessionStore.CreatePending(ctx, session); err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("create pending session: %w", err)
	}

	go s.runPoller(resp)

	return resp, nil
}

func (s *copilotDeviceFlowService) GetDeviceStatus(ctx context.Context, deviceCode string) (*domainoauth.OAuthSession, error) {
	trimmed := strings.TrimSpace(deviceCode)
	if trimmed == "" {
		return nil, fmt.Errorf("device code is required")
	}

	session, err := s.sessionStore.GetStatus(ctx, trimmed)
	if err != nil {
		if isSessionNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get device status: %w", err)
	}

	return &session, nil
}

type noopDeviceFlowCompletionService struct{}

func (noopDeviceFlowCompletionService) CompleteOAuth(_ context.Context, accountID string, _ domainoauth.TokenPayload) (domaincredential.Profile, error) {
	return domaincredential.Profile{Type: "copilot", AccountID: accountID, Enabled: true}, nil
}

func (s *copilotDeviceFlowService) runPoller(resp domainoauth.DeviceFlowResponse) {
	ttl := time.Duration(resp.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), ttl)
	defer cancel()

	interval := intervalSecondsToDuration(resp.Interval)

	for {
		select {
		case <-ctx.Done():
			_ = s.sessionStore.MarkError(context.Background(), resp.DeviceCode, "expired_token", "device flow expired")
			return
		default:
		}

		payload, err := s.provider.PollDevice(ctx, resp.DeviceCode)
		if err != nil {
			action := classifyPollError(err)
			switch action.kind {
			case "pending":
				if sleepErr := s.sleep(ctx, interval); sleepErr != nil {
					return
				}
				continue
			case "slow_down":
				interval += 5 * time.Second
				if interval > maxCopilotPollInterval {
					interval = maxCopilotPollInterval
				}
				if sleepErr := s.sleep(ctx, interval); sleepErr != nil {
					return
				}
				continue
			default:
				_ = s.sessionStore.MarkError(context.Background(), resp.DeviceCode, action.errorCode, action.errorMessage)
				return
			}
		}

		accountID := strings.TrimSpace(payload.AccountID)
		if accountID == "" {
			_ = s.sessionStore.MarkError(context.Background(), resp.DeviceCode, "missing_account_id", "missing account identifier")
			return
		}

		profile, completionErr := s.completionService.CompleteOAuth(ctx, accountID, payload)
		if completionErr != nil {
			_ = s.sessionStore.MarkError(context.Background(), resp.DeviceCode, "completion_failed", "failed to persist credentials")
			return
		}

		_ = s.sessionStore.MarkComplete(context.Background(), resp.DeviceCode, summarizeConnection(profile))
		return
	}
}

type pollAction struct {
	kind         string
	errorCode    string
	errorMessage string
}

func classifyPollError(err error) pollAction {
	if err == nil {
		return pollAction{}
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))

	switch {
	case msg == "authorization pending":
		return pollAction{kind: "pending"}
	case msg == "slow down":
		return pollAction{kind: "slow_down"}
	case msg == "expired token":
		return pollAction{kind: "terminal", errorCode: "expired_token", errorMessage: err.Error()}
	case strings.Contains(msg, "access denied"):
		return pollAction{kind: "terminal", errorCode: "access_denied", errorMessage: err.Error()}
	case strings.Contains(msg, "forbidden") || strings.Contains(msg, "subscription"):
		return pollAction{kind: "terminal", errorCode: "no_subscription", errorMessage: "GitHub Copilot subscription required"}
	default:
		return pollAction{kind: "terminal", errorCode: "poll_failed", errorMessage: err.Error()}
	}
}

func intervalSecondsToDuration(interval int) time.Duration {
	if interval <= 0 {
		return defaultCopilotPollInterval
	}

	d := time.Duration(interval) * time.Second
	if d > maxCopilotPollInterval {
		return maxCopilotPollInterval
	}

	return d
}

func summarizeConnection(profile domaincredential.Profile) domainoauth.ConnectionSummary {
	expiresAt := profile.Expired.UTC()
	lastRefreshAt := profile.LastRefreshAt.UTC()

	return domainoauth.ConnectionSummary{
		ID:            profile.ID,
		AccountID:     profile.AccountID,
		Email:         profile.Email,
		Provider:      profile.Type,
		ExpiresAt:     &expiresAt,
		LastRefreshAt: &lastRefreshAt,
		Enabled:       profile.Enabled,
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isSessionNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "session not found") || strings.Contains(msg, "not found or expired")
}
