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
	"go.uber.org/zap"
)

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
	router        Router
	registry      ProviderRegistry
	healthTracker ProviderHealthTracker
	client        ProviderClient
	config        ServiceConfig
	logger        *zap.Logger
}

// NewService creates a new completion service.
func NewService(
	router Router,
	registry ProviderRegistry,
	healthTracker ProviderHealthTracker,
	client ProviderClient,
	config ServiceConfig,
	logger *zap.Logger,
) CompletionService {
	return &service{
		router:        router,
		registry:      registry,
		healthTracker: healthTracker,
		client:        client,
		config:        config,
		logger:        logger,
	}
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

	var excludeProviders []domainprovider.ProviderID
	var lastAPIErr *domaincompletion.APIError

	for attempt := 0; attempt < s.config.MaxFallbackAttempts; attempt++ {
		// Route to a provider
		decision, err := s.router.RouteWithFallback(ctx, req.Model, excludeProviders)
		if err != nil {
			// No more providers available
			if lastAPIErr != nil {
				return nil, lastAPIErr
			}
			var apiErr *domaincompletion.APIError
			if errors.As(err, &apiErr) {
				return nil, apiErr
			}
			return nil, domaincompletion.ErrInternalServer(err.Error())
		}

		s.logger.Debug("attempting provider",
			zap.String("provider", string(decision.ProviderID)),
			zap.String("model", req.Model),
			zap.Int("attempt", attempt+1),
		)

		// Execute request
		startTime := time.Now()
		resp, err := s.client.ChatCompletion(ctx, *decision, req)
		duration := time.Since(startTime)

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
			return resp, nil
		}

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
			return nil, apiErr
		}

		// Unknown error - mark failure and try next
		s.healthTracker.MarkFailure(decision.ProviderID, 0, err)
		excludeProviders = append(excludeProviders, decision.ProviderID)
	}

	// All attempts failed
	if lastAPIErr != nil {
		return nil, lastAPIErr
	}
	return nil, domaincompletion.ErrAllProvidersFailed(req.Model, s.config.MaxFallbackAttempts)
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

	for attempt := 0; attempt < s.config.MaxFallbackAttempts; attempt++ {
		// Route to a provider
		decision, err := s.router.RouteWithFallback(ctx, req.Model, excludeProviders)
		if err != nil {
			var apiErr *domaincompletion.APIError
			if errors.As(err, &apiErr) {
				return apiErr
			}
			return domaincompletion.ErrInternalServer(err.Error())
		}

		s.logger.Debug("attempting streaming to provider",
			zap.String("provider", string(decision.ProviderID)),
			zap.String("model", req.Model),
			zap.Int("attempt", attempt+1),
		)

		// Execute streaming request
		chunks, err := s.client.ChatCompletionStream(ctx, *decision, req)
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
			return nil
		}

		// Stream failed
		lastErr = streamErr
		s.healthTracker.MarkFailure(decision.ProviderID, 0, streamErr)
		excludeProviders = append(excludeProviders, decision.ProviderID)
	}

	// All attempts failed
	if lastErr != nil {
		return domaincompletion.ErrAllProvidersFailed(req.Model, s.config.MaxFallbackAttempts)
	}
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
