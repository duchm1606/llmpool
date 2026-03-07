// Package usage defines domain entities for usage tracking.
package usage

import "time"

// Status represents the status of a usage record.
type Status string

const (
	StatusDone     Status = "done"
	StatusCanceled Status = "canceled"
	StatusFailed   Status = "failed"
)

// AuditLog represents a single usage audit log entry.
type AuditLog struct {
	ID                  string `json:"id"`
	RequestID           string `json:"request_id"`
	Model               string `json:"model"`
	Provider            string `json:"provider"`
	CredentialID        string `json:"credential_id"`
	CredentialType      string `json:"credential_type"`
	CredentialAccountID string `json:"credential_account_id"`

	// Token usage
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`

	// Pricing (in microdollars: $0.01 = 10000)
	InputPriceMicros  int64 `json:"input_price_micros"`
	OutputPriceMicros int64 `json:"output_price_micros"`
	TotalPriceMicros  int64 `json:"total_price_micros"`

	// Status
	Status       Status `json:"status"`
	ErrorMessage string `json:"error_message"`

	// Timing
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	DurationMs  int       `json:"duration_ms"`

	// Metadata
	Stream    bool      `json:"stream"`
	CreatedAt time.Time `json:"created_at"`
}

// UsageRecord is the input for creating an audit log.
// Used by the usage manager to queue records.
type UsageRecord struct {
	RequestID           string `json:"request_id"`
	Model               string `json:"model"`
	Provider            string `json:"provider"`
	CredentialID        string `json:"credential_id"`
	CredentialType      string `json:"credential_type"`
	CredentialAccountID string `json:"credential_account_id"`

	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`

	Status       Status `json:"status"`
	ErrorMessage string `json:"error_message"`

	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Stream      bool      `json:"stream"`
}

// ModelStats holds aggregated stats for a single model.
type ModelStats struct {
	Model            string `json:"model"`
	RequestCount     int64  `json:"request_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	TotalPriceMicros int64  `json:"total_price_micros"`
	SuccessCount     int64  `json:"success_count"`
	FailedCount      int64  `json:"failed_count"`
	CanceledCount    int64  `json:"canceled_count"`
}

// CredentialStats holds aggregated stats for a single credential.
type CredentialStats struct {
	CredentialID        string `json:"credential_id"`
	CredentialType      string `json:"credential_type"`
	CredentialAccountID string `json:"credential_account_id"`
	RequestCount        int64  `json:"request_count"`
	PromptTokens        int64  `json:"prompt_tokens"`
	CompletionTokens    int64  `json:"completion_tokens"`
	TotalTokens         int64  `json:"total_tokens"`
	TotalPriceMicros    int64  `json:"total_price_micros"`
	SuccessCount        int64  `json:"success_count"`
	FailedCount         int64  `json:"failed_count"`
	CanceledCount       int64  `json:"canceled_count"`
}

// HourlyStats holds aggregated stats for one hour.
type HourlyStats struct {
	Hour             time.Time `json:"hour"`
	RequestCount     int64     `json:"request_count"`
	TotalTokens      int64     `json:"total_tokens"`
	TotalPriceMicros int64     `json:"total_price_micros"`
	SuccessCount     int64     `json:"success_count"`
	FailedCount      int64     `json:"failed_count"`
}

// DailyStats holds aggregated stats for one day.
type DailyStats struct {
	Day              time.Time `json:"day"`
	RequestCount     int64     `json:"request_count"`
	TotalTokens      int64     `json:"total_tokens"`
	TotalPriceMicros int64     `json:"total_price_micros"`
	SuccessCount     int64     `json:"success_count"`
	FailedCount      int64     `json:"failed_count"`
}

// Overview holds overall usage statistics.
type Overview struct {
	TotalRequests         int64   `json:"total_requests"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalPriceMicros      int64   `json:"total_price_micros"`
	SuccessCount          int64   `json:"success_count"`
	FailedCount           int64   `json:"failed_count"`
	CanceledCount         int64   `json:"canceled_count"`
	AvgDurationMs         float64 `json:"avg_duration_ms"`
}

// DashboardStats holds all cached stats for the dashboard.
type DashboardStats struct {
	Overview        Overview          `json:"overview"`
	HourlyStats     []HourlyStats     `json:"hourly_stats"`
	DailyStats      []DailyStats      `json:"daily_stats"`
	ModelStats      []ModelStats      `json:"model_stats"`
	CredentialStats []CredentialStats `json:"credential_stats"`
	GeneratedAt     time.Time         `json:"generated_at"`
	Period          StatsPeriod       `json:"period"`
}

// StatsPeriod defines the time period for stats.
type StatsPeriod struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}
