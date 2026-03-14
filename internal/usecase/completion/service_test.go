package completion

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	"go.uber.org/zap"
)

type mockServiceRouter struct {
	decisions  []*domainprovider.RoutingDecision
	routeErr   error
	callCount  int
	quotaModes []SessionQuotaMode
}

func (m *mockServiceRouter) Route(ctx context.Context, model string) (*domainprovider.RoutingDecision, error) {
	return m.RouteWithHint(ctx, model, "", SessionQuotaConsume, nil)
}

func (m *mockServiceRouter) RouteWithFallback(
	ctx context.Context,
	model string,
	quotaMode SessionQuotaMode,
	excludeProviders []domainprovider.ProviderID,
) (*domainprovider.RoutingDecision, error) {
	return m.RouteWithHint(ctx, model, "", quotaMode, excludeProviders)
}

func (m *mockServiceRouter) RouteWithHint(
	_ context.Context,
	_ string,
	_ string,
	quotaMode SessionQuotaMode,
	_ []domainprovider.ProviderID,
) (*domainprovider.RoutingDecision, error) {
	m.quotaModes = append(m.quotaModes, quotaMode)
	if m.routeErr != nil {
		return nil, m.routeErr
	}
	if m.callCount >= len(m.decisions) {
		return nil, domaincompletion.ErrNoAvailableProvider("gpt-5")
	}
	decision := m.decisions[m.callCount]
	m.callCount++
	return decision, nil
}

func TestSessionQuotaModeForRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  domaincompletion.ChatCompletionRequest
		want SessionQuotaMode
	}{
		{name: "empty request defaults to consume", req: domaincompletion.ChatCompletionRequest{}, want: SessionQuotaConsume},
		{name: "last user message consumes", req: domaincompletion.ChatCompletionRequest{Messages: []domaincompletion.Message{{Role: "tool", Content: "result"}, {Role: "user", Content: "continue"}}}, want: SessionQuotaConsume},
		{name: "last tool message bypasses", req: domaincompletion.ChatCompletionRequest{Messages: []domaincompletion.Message{{Role: "assistant", Content: "tool_call"}, {Role: "tool", Content: "result"}}}, want: SessionQuotaBypass},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SessionQuotaModeForRequest(tt.req); got != tt.want {
				t.Fatalf("SessionQuotaModeForRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestService_ValidateRequest_UsesQuotaBypass(t *testing.T) {
	t.Parallel()

	router := &mockServiceRouter{
		decisions: []*domainprovider.RoutingDecision{{
			ProviderID: domainprovider.ProviderCopilot,
			BaseURL:    "https://api.githubcopilot.com",
			Token:      "copilot-token",
		}},
	}

	svc := NewService(
		router,
		&mockServiceRegistry{},
		&mockServiceHealthTracker{},
		&mockServiceClient{},
		&mockCredentialRefresher{},
		DefaultServiceConfig(),
		zap.NewNop(),
	)

	err := svc.ValidateRequest(context.Background(), domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if len(router.quotaModes) != 1 {
		t.Fatalf("expected exactly one routing call, got %d", len(router.quotaModes))
	}
	if router.quotaModes[0] != SessionQuotaBypass {
		t.Fatalf("unexpected quota mode: got %q want %q", router.quotaModes[0], SessionQuotaBypass)
	}
}

type mockServiceRegistry struct{}

func (m *mockServiceRegistry) GetProvider(id domainprovider.ProviderID) (*domainprovider.Provider, bool) {
	return nil, false
}

func (m *mockServiceRegistry) ListProviders() []domainprovider.Provider {
	return nil
}

func (m *mockServiceRegistry) GetProvidersForModel(model string) []domainprovider.ProviderID {
	return nil
}

func (m *mockServiceRegistry) GetAllModels() []domaincompletion.Model {
	return nil
}

type mockServiceHealthTracker struct {
	markSuccessCount int
	markFailureCount int
}

func (m *mockServiceHealthTracker) GetHealth(id domainprovider.ProviderID) domainprovider.ProviderHealth {
	return domainprovider.ProviderHealth{ProviderID: id, Healthy: true}
}

func (m *mockServiceHealthTracker) MarkSuccess(id domainprovider.ProviderID) {
	m.markSuccessCount++
}

func (m *mockServiceHealthTracker) MarkFailure(id domainprovider.ProviderID, statusCode int, err error) {
	m.markFailureCount++
}

func (m *mockServiceHealthTracker) MarkRateLimited(id domainprovider.ProviderID, resetAt string) {}

type mockServiceClient struct {
	responses []*domaincompletion.ChatCompletionResponse
	errs      []error
	callCount int
}

func (m *mockServiceClient) ChatCompletion(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	if m.callCount >= len(m.responses) {
		if m.callCount < len(m.errs) {
			err := m.errs[m.callCount]
			m.callCount++
			return nil, err
		}
		return nil, errors.New("unexpected client call")
	}
	resp := m.responses[m.callCount]
	var err error
	if m.callCount < len(m.errs) {
		err = m.errs[m.callCount]
	}
	m.callCount++
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (m *mockServiceClient) ChatCompletionStream(
	ctx context.Context,
	decision domainprovider.RoutingDecision,
	req domaincompletion.ChatCompletionRequest,
) (<-chan StreamChunk, error) {
	return nil, errors.New("not implemented")
}

type mockCredentialRefresher struct {
	err      error
	called   bool
	lastCred string
}

func (m *mockCredentialRefresher) RefreshCredential(ctx context.Context, credentialID string) error {
	m.called = true
	m.lastCred = credentialID
	return m.err
}

func TestService_Copilot401RefreshAndRetry(t *testing.T) {
	router := &mockServiceRouter{
		decisions: []*domainprovider.RoutingDecision{
			{ProviderID: domainprovider.ProviderCopilot, CredentialID: "cred-old", CredentialType: "copilot", CredentialAccountID: "acct-1", BaseURL: "https://api.githubcopilot.com", Token: "old-token"},
			{ProviderID: domainprovider.ProviderCopilot, CredentialID: "cred-new", CredentialType: "copilot", CredentialAccountID: "acct-1", BaseURL: "https://api.githubcopilot.com", Token: "new-token"},
		},
	}

	health := &mockServiceHealthTracker{}
	client := &mockServiceClient{
		responses: []*domaincompletion.ChatCompletionResponse{
			nil,
			{
				ID:      "chatcmpl-1",
				Object:  "chat.completion",
				Created: 1,
				Model:   "gpt-5",
				Choices: []domaincompletion.Choice{{
					Index: 0,
					Message: &domaincompletion.Message{
						Role:    "assistant",
						Content: "ok",
					},
					FinishReason: "stop",
				}},
			},
		},
		errs: []error{
			domaincompletion.NewAPIError(http.StatusUnauthorized, domaincompletion.ErrorTypeAuthentication, "access token expired"),
			nil,
		},
	}
	refresher := &mockCredentialRefresher{}

	svc := NewService(
		router,
		&mockServiceRegistry{},
		health,
		client,
		refresher,
		DefaultServiceConfig(),
		zap.NewNop(),
	)

	resp, err := svc.ChatCompletion(context.Background(), domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if !refresher.called {
		t.Fatal("expected inline refresh to be called")
	}
	if refresher.lastCred != "cred-old" {
		t.Fatalf("refresher credential = %q, want cred-old", refresher.lastCred)
	}
	if client.callCount != 2 {
		t.Fatalf("client call count = %d, want 2", client.callCount)
	}
}

func TestService_NonCopilot401DoesNotRefresh(t *testing.T) {
	router := &mockServiceRouter{
		decisions: []*domainprovider.RoutingDecision{{
			ProviderID:          domainprovider.ProviderCodex,
			CredentialID:        "cred-codex",
			CredentialType:      "codex",
			CredentialAccountID: "acct-2",
			BaseURL:             "https://chatgpt.com/backend-api/codex",
			Token:               "token",
		}},
	}

	health := &mockServiceHealthTracker{}
	client := &mockServiceClient{
		responses: []*domaincompletion.ChatCompletionResponse{nil},
		errs: []error{
			domaincompletion.NewAPIError(http.StatusUnauthorized, domaincompletion.ErrorTypeAuthentication, "unauthorized"),
		},
	}
	refresher := &mockCredentialRefresher{}

	svc := NewService(
		router,
		&mockServiceRegistry{},
		health,
		client,
		refresher,
		DefaultServiceConfig(),
		zap.NewNop(),
	)

	_, err := svc.ChatCompletion(context.Background(), domaincompletion.ChatCompletionRequest{
		Model:    "gpt-5",
		Messages: []domaincompletion.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if refresher.called {
		t.Fatal("did not expect inline refresh for non-copilot provider")
	}
}

// Ensure mocks satisfy interfaces used by service construction.
var _ Router = (*mockServiceRouter)(nil)
var _ ProviderRegistry = (*mockServiceRegistry)(nil)
var _ ProviderHealthTracker = (*mockServiceHealthTracker)(nil)
var _ ProviderClient = (*mockServiceClient)(nil)
var _ CredentialRefresher = (*mockCredentialRefresher)(nil)
var _ io.Writer = (*nopWriter)(nil)

type nopWriter struct{}

func (n *nopWriter) Write(p []byte) (int, error) { return len(p), nil }
