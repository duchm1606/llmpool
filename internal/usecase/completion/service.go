package completion

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	domaincompletion "github.com/duchoang/llmpool/internal/domain/completion"
	domainprovider "github.com/duchoang/llmpool/internal/domain/provider"
	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	"go.uber.org/zap"
)

// UsagePublisher is used to publish usage records.
type UsagePublisher interface {
	Publish(record domainusage.UsageRecord)
}

// ServiceConfig holds configuration for the completion service.
type ServiceConfig struct {
	MaxFallbackAttempts int
	RequestTimeout      time.Duration
}

// DefaultServiceConfig returns sensible defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		MaxFallbackAttempts: 3,
		RequestTimeout:      120 * time.Second,
	}
}

// service implements the CompletionService interface.
type service struct {
	router         Router
	registry       ProviderRegistry
	healthTracker  ProviderHealthTracker
	client         ProviderClient
	refresher      CredentialRefresher
	usagePublisher UsagePublisher
	config         ServiceConfig
	logger         *zap.Logger
}

// NewService creates a new completion service.
func NewService(
	router Router,
	registry ProviderRegistry,
	healthTracker ProviderHealthTracker,
	client ProviderClient,
	refresher CredentialRefresher,
	config ServiceConfig,
	logger *zap.Logger,
) CompletionService {
	return &service{
		router:        router,
		registry:      registry,
		healthTracker: healthTracker,
		client:        client,
		refresher:     refresher,
		config:        config,
		logger:        logger,
	}
}

// SetUsagePublisher sets the usage publisher for tracking.
// This is optional - if not set, usage tracking is disabled.
func (s *service) SetUsagePublisher(publisher UsagePublisher) {
	s.usagePublisher = publisher
}

// ValidateRequest validates request fields and preflights routing feasibility.
// This is intended for handlers that need to ensure errors are returned before
// committing streaming response headers.
func (s *service) ValidateRequest(ctx context.Context, req domaincompletion.ChatCompletionRequest) error {
	if err := s.validateRequest(req); err != nil {
		return err
	}

	_, err := s.router.RouteWithHint(ctx, req.Model, req.ProviderHint, nil)
	if err != nil {
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		return domaincompletion.ErrInternalServer(err.Error())
	}

	return nil
}

// ChatCompletion handles a chat completion request with routing and fallback.
func (s *service) ChatCompletion(
	ctx context.Context,
	req domaincompletion.ChatCompletionRequest,
) (*domaincompletion.ChatCompletionResponse, error) {
	// Validate request
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	requestStartedAt := time.Now()
	var excludeProviders []domainprovider.ProviderID
	var lastAPIErr *domaincompletion.APIError
	var lastErr error
	var lastDecision *domainprovider.RoutingDecision

	for attempt := 0; attempt < s.config.MaxFallbackAttempts; attempt++ {
		if ctx.Err() != nil {
			err := ctx.Err()
			s.publishUsage(req, lastDecision, nil, err, requestStartedAt, false)
			return nil, domaincompletion.ErrInternalServer(err.Error())
		}

		// Route to a provider (with optional provider hint)
		decision, err := s.router.RouteWithHint(ctx, req.Model, req.ProviderHint, excludeProviders)
		if err != nil {
			// No more providers available
			if lastAPIErr != nil {
				s.publishUsage(req, lastDecision, nil, lastAPIErr, requestStartedAt, false)
				return nil, lastAPIErr
			}
			var apiErr *domaincompletion.APIError
			if errors.As(err, &apiErr) {
				s.publishUsage(req, lastDecision, nil, apiErr, requestStartedAt, false)
				return nil, apiErr
			}
			s.publishUsage(req, lastDecision, nil, err, requestStartedAt, false)
			return nil, domaincompletion.ErrInternalServer(err.Error())
		}
		lastDecision = decision

		s.logger.Debug("attempting provider",
			zap.String("provider", string(decision.ProviderID)),
			zap.String("model", req.Model),
			zap.String("provider_hint", req.ProviderHint),
			zap.Int("attempt", attempt+1),
		)

		// Execute request
		startTime := time.Now()
		resp, err := s.client.ChatCompletion(ctx, *decision, req)
		duration := time.Since(startTime)

		if err != nil {
			refreshed, retryResp, retryErr := s.tryRefreshCopilotAndRetry(ctx, req, *decision, err)
			if refreshed {
				if retryErr == nil {
					s.healthTracker.MarkSuccess(decision.ProviderID)
					s.logger.Info("completion request succeeded after copilot refresh",
						zap.String("provider", string(decision.ProviderID)),
						zap.String("model", req.Model),
						zap.String("credential_id", decision.CredentialID),
						zap.String("credential_type", decision.CredentialType),
						zap.String("credential_account_id", decision.CredentialAccountID),
						zap.Duration("duration", duration),
					)
					// Publish usage for successful retry
					s.publishUsage(req, decision, retryResp, nil, requestStartedAt, false)
					return retryResp, nil
				}
				err = retryErr
			}
		}

		if err == nil {
			// Success
			s.healthTracker.MarkSuccess(decision.ProviderID)
			s.logger.Info("completion request succeeded",
				zap.String("provider", string(decision.ProviderID)),
				zap.String("model", req.Model),
				zap.String("credential_id", decision.CredentialID),
				zap.String("credential_type", decision.CredentialType),
				zap.String("credential_account_id", decision.CredentialAccountID),
				zap.Duration("duration", duration),
			)
			// Publish usage for successful completion
			s.publishUsage(req, decision, resp, nil, requestStartedAt, false)
			return resp, nil
		}
		lastErr = err

		// Handle error
		s.logger.Warn("provider request failed",
			zap.String("provider", string(decision.ProviderID)),
			zap.String("model", req.Model),
			zap.String("credential_id", decision.CredentialID),
			zap.String("credential_type", decision.CredentialType),
			zap.String("credential_account_id", decision.CredentialAccountID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)

		// Check if error is retryable
		var apiErr *domaincompletion.APIError
		if errors.As(err, &apiErr) {
			lastAPIErr = apiErr
			statusCode := apiErr.HTTPStatus

			// Rate limit - mark and fallback
			if statusCode == http.StatusTooManyRequests {
				s.healthTracker.MarkFailure(decision.ProviderID, statusCode, err)
				s.healthTracker.MarkRateLimited(decision.ProviderID, "")
				excludeProviders = append(excludeProviders, decision.ProviderID)
				continue
			}

			// Server errors (5xx) - fallback to next provider
			if statusCode >= 500 {
				s.healthTracker.MarkFailure(decision.ProviderID, statusCode, err)
				excludeProviders = append(excludeProviders, decision.ProviderID)
				continue
			}

			// Client errors (4xx except 429) - don't retry, return error
			s.publishUsage(req, decision, nil, apiErr, requestStartedAt, false)
			return nil, apiErr
		}

		// Unknown error - mark failure and try next
		s.healthTracker.MarkFailure(decision.ProviderID, 0, err)
		excludeProviders = append(excludeProviders, decision.ProviderID)
	}

	// All attempts failed
	if lastAPIErr != nil {
		s.publishUsage(req, lastDecision, nil, lastAPIErr, requestStartedAt, false)
		return nil, lastAPIErr
	}
	if lastErr != nil {
		s.publishUsage(req, lastDecision, nil, lastErr, requestStartedAt, false)
	}
	return nil, domaincompletion.ErrAllProvidersFailed(req.Model, s.config.MaxFallbackAttempts)
}

func (s *service) tryRefreshCopilotAndRetry(
	ctx context.Context,
	req domaincompletion.ChatCompletionRequest,
	decision domainprovider.RoutingDecision,
	err error,
) (bool, *domaincompletion.ChatCompletionResponse, error) {
	if s.refresher == nil {
		return false, nil, err
	}

	if decision.ProviderID != domainprovider.ProviderCopilot {
		return false, nil, err
	}

	if decision.CredentialID == "" {
		return false, nil, err
	}

	apiErr := getAPIError(err)
	if apiErr == nil || apiErr.HTTPStatus != http.StatusUnauthorized {
		return false, nil, err
	}

	if refreshErr := s.refresher.RefreshCredential(ctx, decision.CredentialID); refreshErr != nil {
		s.logger.Warn("copilot inline refresh failed",
			zap.String("credential_id", decision.CredentialID),
			zap.String("credential_type", decision.CredentialType),
			zap.String("credential_account_id", decision.CredentialAccountID),
			zap.Error(refreshErr),
		)
		return true, nil, err
	}

	newDecision, routeErr := s.router.RouteWithHint(
		ctx,
		req.Model,
		string(domainprovider.ProviderCopilot),
		nil,
	)
	if routeErr != nil {
		s.logger.Warn("copilot inline refresh succeeded but reroute failed",
			zap.String("credential_id", decision.CredentialID),
			zap.Error(routeErr),
		)
		return true, nil, err
	}

	retryResp, retryErr := s.client.ChatCompletion(ctx, *newDecision, req)
	if retryErr != nil {
		s.logger.Warn("copilot retry after inline refresh failed",
			zap.String("credential_id", newDecision.CredentialID),
			zap.String("credential_type", newDecision.CredentialType),
			zap.String("credential_account_id", newDecision.CredentialAccountID),
			zap.Error(retryErr),
		)
		return true, nil, retryErr
	}

	s.logger.Info("copilot inline refresh and retry succeeded",
		zap.String("old_credential_id", decision.CredentialID),
		zap.String("new_credential_id", newDecision.CredentialID),
		zap.String("credential_account_id", newDecision.CredentialAccountID),
	)

	return true, retryResp, nil
}

func getAPIError(err error) *domaincompletion.APIError {
	if err == nil {
		return nil
	}
	var apiErr *domaincompletion.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}

func (s *service) tryRefreshCopilotAndRetryStream(
	ctx context.Context,
	req domaincompletion.ChatCompletionRequest,
	decision domainprovider.RoutingDecision,
	err error,
) (bool, <-chan StreamChunk, error) {
	if s.refresher == nil {
		return false, nil, err
	}

	if decision.ProviderID != domainprovider.ProviderCopilot {
		return false, nil, err
	}

	if decision.CredentialID == "" {
		return false, nil, err
	}

	apiErr := getAPIError(err)
	if apiErr == nil || apiErr.HTTPStatus != http.StatusUnauthorized {
		return false, nil, err
	}

	if refreshErr := s.refresher.RefreshCredential(ctx, decision.CredentialID); refreshErr != nil {
		s.logger.Warn("copilot inline refresh failed for stream",
			zap.String("credential_id", decision.CredentialID),
			zap.String("credential_type", decision.CredentialType),
			zap.String("credential_account_id", decision.CredentialAccountID),
			zap.Error(refreshErr),
		)
		return true, nil, err
	}

	newDecision, routeErr := s.router.RouteWithHint(
		ctx,
		req.Model,
		string(domainprovider.ProviderCopilot),
		nil,
	)
	if routeErr != nil {
		s.logger.Warn("copilot inline refresh succeeded but stream reroute failed",
			zap.String("credential_id", decision.CredentialID),
			zap.Error(routeErr),
		)
		return true, nil, err
	}

	retryChunks, retryErr := s.client.ChatCompletionStream(ctx, *newDecision, req)
	if retryErr != nil {
		s.logger.Warn("copilot stream retry after inline refresh failed",
			zap.String("credential_id", newDecision.CredentialID),
			zap.String("credential_type", newDecision.CredentialType),
			zap.String("credential_account_id", newDecision.CredentialAccountID),
			zap.Error(retryErr),
		)
		return true, nil, retryErr
	}

	s.logger.Info("copilot inline refresh and stream retry succeeded",
		zap.String("old_credential_id", decision.CredentialID),
		zap.String("new_credential_id", newDecision.CredentialID),
		zap.String("credential_account_id", newDecision.CredentialAccountID),
	)

	return true, retryChunks, nil
}

// ChatCompletionStream handles a streaming chat completion request.
func (s *service) ChatCompletionStream(
	ctx context.Context,
	req domaincompletion.ChatCompletionRequest,
	writer io.Writer,
) error {
	// Validate request
	if err := s.validateRequest(req); err != nil {
		return err
	}

	var excludeProviders []domainprovider.ProviderID
	var lastErr error
	startTime := time.Now()
	var lastDecision *domainprovider.RoutingDecision

	for attempt := 0; attempt < s.config.MaxFallbackAttempts; attempt++ {
		if ctx.Err() != nil {
			err := ctx.Err()
			s.publishUsage(req, lastDecision, nil, err, startTime, true)
			return domaincompletion.ErrInternalServer(err.Error())
		}

		// Route to a provider (with optional provider hint)
		decision, err := s.router.RouteWithHint(ctx, req.Model, req.ProviderHint, excludeProviders)
		if err != nil {
			var apiErr *domaincompletion.APIError
			if errors.As(err, &apiErr) {
				s.publishUsage(req, lastDecision, nil, apiErr, startTime, true)
				return apiErr
			}
			s.publishUsage(req, lastDecision, nil, err, startTime, true)
			return domaincompletion.ErrInternalServer(err.Error())
		}
		lastDecision = decision

		s.logger.Debug("attempting streaming to provider",
			zap.String("provider", string(decision.ProviderID)),
			zap.String("model", req.Model),
			zap.String("provider_hint", req.ProviderHint),
			zap.Int("attempt", attempt+1),
		)

		// Execute streaming request
		chunks, err := s.client.ChatCompletionStream(ctx, *decision, req)
		if err != nil {
			refreshed, retryChunks, retryErr := s.tryRefreshCopilotAndRetryStream(ctx, req, *decision, err)
			if refreshed {
				if retryErr == nil {
					chunks = retryChunks
					err = nil
				} else {
					err = retryErr
				}
			}
		}
		if err != nil {
			lastErr = err
			s.healthTracker.MarkFailure(decision.ProviderID, 0, err)
			excludeProviders = append(excludeProviders, decision.ProviderID)
			continue
		}

		// Stream chunks to writer
		var streamErr error
		for chunk := range chunks {
			if chunk.Error != nil {
				streamErr = chunk.Error
				break
			}
			if chunk.Done {
				break
			}

			// Write SSE format: "data: {...}\n\n"
			if _, err := fmt.Fprintf(writer, "data: %s\n\n", chunk.Data); err != nil {
				streamErr = err
				break
			}

			// Flush if possible
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}

		if streamErr == nil {
			// Write final done marker
			if _, err := fmt.Fprintf(writer, "data: [DONE]\n\n"); err != nil {
				return err
			}
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}

			s.healthTracker.MarkSuccess(decision.ProviderID)
			// Publish usage for successful stream (token counts not available for streaming)
			s.publishUsage(req, decision, nil, nil, startTime, true)
			return nil
		}

		// Stream failed
		lastErr = streamErr
		s.healthTracker.MarkFailure(decision.ProviderID, 0, streamErr)
		excludeProviders = append(excludeProviders, decision.ProviderID)
	}

	// All attempts failed
	if lastErr != nil {
		s.publishUsage(req, lastDecision, nil, lastErr, startTime, true)
		return domaincompletion.ErrAllProvidersFailed(req.Model, s.config.MaxFallbackAttempts)
	}
	s.publishUsage(req, lastDecision, nil, domaincompletion.ErrNoAvailableProvider(req.Model), startTime, true)
	return domaincompletion.ErrNoAvailableProvider(req.Model)
}

// ListModels returns all available models.
func (s *service) ListModels(ctx context.Context) (*domaincompletion.ModelsResponse, error) {
	models := s.registry.GetAllModels()
	return &domaincompletion.ModelsResponse{
		Object: "list",
		Data:   models,
	}, nil
}

// validateRequest validates a chat completion request.
func (s *service) validateRequest(req domaincompletion.ChatCompletionRequest) error {
	if req.Model == "" {
		return domaincompletion.ErrMissingModel()
	}
	if len(req.Messages) == 0 {
		return domaincompletion.ErrMissingMessages()
	}
	return nil
}

// publishUsage publishes a usage record for tracking.
func (s *service) publishUsage(
	req domaincompletion.ChatCompletionRequest,
	decision *domainprovider.RoutingDecision,
	resp *domaincompletion.ChatCompletionResponse,
	err error,
	startTime time.Time,
	stream bool,
) {
	if s.usagePublisher == nil {
		return
	}

	completedAt := time.Now()

	// Determine status and extract token usage
	var status domainusage.Status
	var errMsg string
	var promptTokens, completionTokens, cachedTokens int

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = domainusage.StatusCanceled
		} else {
			status = domainusage.StatusFailed
		}
		errMsg = err.Error()
	} else if resp != nil && resp.Usage != nil {
		status = domainusage.StatusDone
		cachedTokens = resp.Usage.CachedTokens()
		promptTokens = resp.Usage.PromptTokens
		// OpenAI-compatible APIs typically report prompt_tokens including cached tokens.
		// Normalize to non-cached input tokens when possible.
		if cachedTokens > 0 && promptTokens >= cachedTokens {
			promptTokens -= cachedTokens
		}
		completionTokens = resp.Usage.CompletionTokens
	} else {
		status = domainusage.StatusDone
	}

	credentialID := ""
	credentialType := ""
	credentialAccountID := ""
	providerID := ""
	if decision != nil {
		credentialID = decision.CredentialID
		credentialType = decision.CredentialType
		credentialAccountID = decision.CredentialAccountID
		providerID = string(decision.ProviderID)
	}

	record := domainusage.UsageRecord{
		RequestID:           req.RequestID,
		Model:               req.Model,
		Provider:            providerID,
		CredentialID:        credentialID,
		CredentialType:      credentialType,
		CredentialAccountID: credentialAccountID,
		PromptTokens:        promptTokens,
		CachedTokens:        cachedTokens,
		CompletionTokens:    completionTokens,
		Status:              status,
		ErrorMessage:        errMsg,
		StartedAt:           startTime,
		CompletedAt:         completedAt,
		Stream:              stream,
	}

	s.usagePublisher.Publish(record)
}
