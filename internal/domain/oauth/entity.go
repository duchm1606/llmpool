package oauth

import "time"

type OAuthState string

const (
	StatePending OAuthState = "pending"
	StateOK      OAuthState = "ok"
	StateError   OAuthState = "error"
)

// OAuthSession represents an OAuth flow session stored in Redis
type OAuthSession struct {
	SessionID       string
	State           OAuthState
	PKCEVerifier    string
	Provider        string
	Expiry          time.Time
	ErrorMessage    string
	ErrorCode       string
	CreatedAt       time.Time
	CompletedAt     *time.Time
	AccountID       string
	Connection      *ConnectionSummary
	DeviceCode      string // For device flow
	UserCode        string // For device flow
	VerificationURI string // For device flow
	Interval        int    // Device flow polling interval in seconds
}

// ConnectionSummary is the non-sensitive cached result of a successful personal-use connection flow.
type ConnectionSummary struct {
	ID            string     `json:"id"`
	AccountID     string     `json:"account_id"`
	Email         string     `json:"email,omitempty"`
	Provider      string     `json:"provider"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	LastRefreshAt *time.Time `json:"last_refresh_at,omitempty"`
	Enabled       bool       `json:"enabled"`
}

// TokenPayload represents OAuth token response data
type TokenPayload struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	Email        string
	ExpiresAt    time.Time
	AccountID    string
	TokenType    string
	Scope        string
}

// AuthorizationURL represents the data needed to build an OAuth authorization URL
type AuthorizationURL struct {
	URL   string
	State string
}

// DeviceFlowResponse represents the initial device flow response
type DeviceFlowResponse struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       int
	Interval        int
}
