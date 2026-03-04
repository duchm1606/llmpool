package completion

import (
	"fmt"
	"net/http"
)

// Error types matching OpenAI's error contract.
const (
	ErrorTypeInvalidRequest     = "invalid_request_error"
	ErrorTypeAuthentication     = "authentication_error"
	ErrorTypePermission         = "permission_error"
	ErrorTypeNotFound           = "not_found_error"
	ErrorTypeRateLimit          = "rate_limit_error"
	ErrorTypeServer             = "server_error"
	ErrorTypeServiceUnavailable = "service_unavailable"
)

// APIError represents an OpenAI-compatible error.
type APIError struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param,omitempty"`
	Code    *string `json:"code,omitempty"`

	// Internal fields (not serialized)
	HTTPStatus int `json:"-"`
}

func (e *APIError) Error() string {
	return e.Message
}

// APIErrorResponse is the envelope for API errors.
type APIErrorResponse struct {
	Error APIError `json:"error"`
}

// NewAPIError creates a new API error.
func NewAPIError(httpStatus int, errType, message string) *APIError {
	return &APIError{
		HTTPStatus: httpStatus,
		Type:       errType,
		Message:    message,
	}
}

// NewAPIErrorWithCode creates a new API error with an error code.
func NewAPIErrorWithCode(httpStatus int, errType, message, code string) *APIError {
	return &APIError{
		HTTPStatus: httpStatus,
		Type:       errType,
		Message:    message,
		Code:       &code,
	}
}

// Common error constructors.

// ErrInvalidModelID returns an error for invalid model ID format.
func ErrInvalidModelID(model string) *APIError {
	return NewAPIError(
		http.StatusBadRequest,
		ErrorTypeInvalidRequest,
		fmt.Sprintf("Invalid model ID '%s'. Model IDs must not contain '/' prefix.", model),
	)
}

// ErrModelNotFound returns an error when the model is not available.
func ErrModelNotFound(model string) *APIError {
	code := "model_not_found"
	return &APIError{
		HTTPStatus: http.StatusNotFound,
		Type:       ErrorTypeNotFound,
		Message:    fmt.Sprintf("The model '%s' does not exist or you do not have access to it.", model),
		Code:       &code,
	}
}

// ErrNoAvailableProvider returns an error when no provider can serve the request.
func ErrNoAvailableProvider(model string) *APIError {
	return NewAPIError(
		http.StatusServiceUnavailable,
		ErrorTypeServiceUnavailable,
		fmt.Sprintf("No available provider for model '%s'. All providers are either unavailable or rate-limited.", model),
	)
}

// ErrAllProvidersFailed returns an error when all provider attempts failed.
func ErrAllProvidersFailed(model string, attempts int) *APIError {
	return NewAPIError(
		http.StatusBadGateway,
		ErrorTypeServer,
		fmt.Sprintf("All %d provider attempts failed for model '%s'.", attempts, model),
	)
}

// ErrMissingModel returns an error when the model field is missing.
func ErrMissingModel() *APIError {
	param := "model"
	return &APIError{
		HTTPStatus: http.StatusBadRequest,
		Type:       ErrorTypeInvalidRequest,
		Message:    "Missing required parameter: 'model'.",
		Param:      &param,
	}
}

// ErrMissingMessages returns an error when messages field is missing.
func ErrMissingMessages() *APIError {
	param := "messages"
	return &APIError{
		HTTPStatus: http.StatusBadRequest,
		Type:       ErrorTypeInvalidRequest,
		Message:    "Missing required parameter: 'messages'.",
		Param:      &param,
	}
}

// ErrInvalidJSON returns an error for malformed JSON.
func ErrInvalidJSON(detail string) *APIError {
	return NewAPIError(
		http.StatusBadRequest,
		ErrorTypeInvalidRequest,
		fmt.Sprintf("Invalid JSON: %s", detail),
	)
}

// ErrRateLimited returns a rate limit error.
func ErrRateLimited(retryAfter string) *APIError {
	code := "rate_limit_exceeded"
	return &APIError{
		HTTPStatus: http.StatusTooManyRequests,
		Type:       ErrorTypeRateLimit,
		Message:    "Rate limit exceeded. Please retry after " + retryAfter,
		Code:       &code,
	}
}

// ErrAuthentication returns an authentication error.
func ErrAuthentication(message string) *APIError {
	return NewAPIError(
		http.StatusUnauthorized,
		ErrorTypeAuthentication,
		message,
	)
}

// ErrInternalServer returns a generic server error.
func ErrInternalServer(message string) *APIError {
	return NewAPIError(
		http.StatusInternalServerError,
		ErrorTypeServer,
		message,
	)
}

// MapHTTPStatusToErrorType maps HTTP status codes to OpenAI error types.
func MapHTTPStatusToErrorType(status int) string {
	switch {
	case status == http.StatusBadRequest:
		return ErrorTypeInvalidRequest
	case status == http.StatusUnauthorized:
		return ErrorTypeAuthentication
	case status == http.StatusForbidden:
		return ErrorTypePermission
	case status == http.StatusNotFound:
		return ErrorTypeNotFound
	case status == http.StatusTooManyRequests:
		return ErrorTypeRateLimit
	case status >= 500:
		return ErrorTypeServer
	default:
		return ErrorTypeServer
	}
}
