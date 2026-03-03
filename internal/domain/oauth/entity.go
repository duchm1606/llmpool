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
	DeviceCode      string // For device flow
	UserCode        string // For device flow
	VerificationURI string // For device flow
	Interval        int    // Device flow polling interval in seconds
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
