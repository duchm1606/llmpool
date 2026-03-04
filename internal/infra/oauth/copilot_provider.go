package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/duchoang/llmpool/internal/infra/config"
	loggerinfra "github.com/duchoang/llmpool/internal/infra/logger"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
	"go.uber.org/zap"
)

var _ usecaseoauth.OAuthProvider = (*CopilotProvider)(nil)

var copilotLog = loggerinfra.ForModuleLazy("infra.oauth.copilot")

// Copilot API version constants - aligned with reference implementation
const (
	// copilotChatVersion is the copilot-chat plugin version
	copilotChatVersion = "0.26.7"
	// copilotEditorPluginVersion is the editor plugin version header
	copilotEditorPluginVersion = "copilot-chat/" + copilotChatVersion
	// copilotUserAgent is the user agent for Copilot requests
	copilotUserAgent = "GitHubCopilotChat/" + copilotChatVersion
	// defaultVSCodeVersion is the fallback VSCode version
	defaultVSCodeVersion = "1.104.3"
	// copilotGitHubAPIVersion is the GitHub API version for Copilot
	copilotGitHubAPIVersion = "2025-04-01"
)

// CopilotProvider implements OAuthProvider for GitHub Copilot OAuth using device flow.
type CopilotProvider struct {
	config     config.CopilotOAuthConfig
	httpClient *http.Client
}

// NewCopilotProvider creates a new Copilot OAuth provider.
func NewCopilotProvider(cfg config.CopilotOAuthConfig) *CopilotProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &CopilotProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// githubDeviceResponse represents GitHub device code endpoint response.
type githubDeviceResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// githubTokenResponse represents GitHub token endpoint response.
type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// copilotTokenResponse represents Copilot token endpoint response.
type copilotTokenResponse struct {
	Token     string         `json:"token"`
	ExpiresAt int64          `json:"expires_at"`
	RefreshIn int64          `json:"refresh_in"` // Seconds until refresh recommended
	ErrorCode string         `json:"error_code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Endpoints map[string]any `json:"endpoints,omitempty"`
}

// githubUserResponse represents GitHub user info endpoint response.
type githubUserResponse struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// BuildAuthURL is not supported for device flow - returns error.
func (p *CopilotProvider) BuildAuthURL(_ context.Context, _, _ string) (domainoauth.AuthorizationURL, error) {
	return domainoauth.AuthorizationURL{}, fmt.Errorf("copilot provider uses device flow only")
}

// ExchangeCode is not supported for device flow - returns error.
func (p *CopilotProvider) ExchangeCode(_ context.Context, _, _ string) (domainoauth.TokenPayload, error) {
	return domainoauth.TokenPayload{}, fmt.Errorf("copilot provider uses device flow only")
}

// RefreshToken refreshes the Copilot session token using the stored GitHub access token.
// Note: This method does NOT return user identity (accountID/email) because the refresh
// is called from the credential refresh service which already has this information stored.
// The returned TokenPayload will have empty AccountID - callers should preserve existing identity.
func (p *CopilotProvider) RefreshToken(ctx context.Context, githubToken string) (domainoauth.TokenPayload, error) {
	// Exchange GitHub token for new Copilot session token
	copilotToken, expiresAt, err := p.exchangeForCopilotToken(ctx, githubToken)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("exchange for copilot token: %w", err)
	}

	// Note: We intentionally do NOT fetch user info here during refresh.
	// The refresh service already has the credential profile with account identity.
	// Fetching user info on every refresh is unnecessary and can fail due to rate limits.
	// The RefreshResult only needs tokens and expiry - identity is preserved from the existing profile.

	return domainoauth.TokenPayload{
		AccessToken:  copilotToken,
		RefreshToken: githubToken, // GitHub token is stored as refresh token
		ExpiresAt:    expiresAt,
		TokenType:    "Bearer",
		Scope:        p.config.Scope,
		// AccountID and Email intentionally left empty - refresh service preserves existing values
	}, nil
}

// StartDeviceFlow initiates GitHub device authorization flow.
// Uses JSON body format as per reference implementation.
func (p *CopilotProvider) StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error) {
	if p.config.DeviceCodeURL == "" {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("device code URL not configured")
	}

	// Build JSON body - aligned with reference implementation
	requestBody := map[string]string{
		"client_id": p.config.ClientID,
		"scope":     p.config.Scope,
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("marshal device request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.DeviceCodeURL, bytes.NewReader(jsonBody))
	if err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("create device request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("execute device request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("device endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var deviceResp githubDeviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("decode device response: %w", err)
	}

	copilotLog.Info("copilot device flow started",
		zap.String("user_code", deviceResp.UserCode),
		zap.String("verification_uri", deviceResp.VerificationURI),
		zap.Int("expires_in", deviceResp.ExpiresIn),
	)

	return domainoauth.DeviceFlowResponse{
		DeviceCode:      deviceResp.DeviceCode,
		UserCode:        deviceResp.UserCode,
		VerificationURI: deviceResp.VerificationURI,
		ExpiresIn:       deviceResp.ExpiresIn,
		Interval:        deviceResp.Interval,
	}, nil
}

// PollDevice polls for device authorization completion and exchanges tokens.
func (p *CopilotProvider) PollDevice(ctx context.Context, deviceCode string) (domainoauth.TokenPayload, error) {
	if p.config.TokenURL == "" {
		return domainoauth.TokenPayload{}, fmt.Errorf("token URL not configured")
	}

	// Step 1: Poll GitHub for access token
	githubToken, err := p.pollGitHubToken(ctx, deviceCode)
	if err != nil {
		return domainoauth.TokenPayload{}, err
	}

	// Step 2: Exchange GitHub token for Copilot session token
	copilotToken, expiresAt, err := p.exchangeForCopilotToken(ctx, githubToken)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("exchange for copilot token: %w", err)
	}

	// Step 3: Fetch GitHub user info for account identity
	userInfo, err := p.fetchUserInfo(ctx, githubToken)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("fetch user info: %w", err)
	}

	copilotLog.Info("copilot device flow completed",
		zap.String("account_id", userInfo.accountID()),
		zap.String("login", userInfo.Login),
		zap.Time("expires_at", expiresAt),
	)

	return domainoauth.TokenPayload{
		AccessToken:  copilotToken,
		RefreshToken: githubToken, // Store GitHub token for session refresh
		ExpiresAt:    expiresAt,
		AccountID:    userInfo.accountID(),
		Email:        userInfo.Email,
		TokenType:    "Bearer",
		Scope:        p.config.Scope,
	}, nil
}

// pollGitHubToken polls GitHub token endpoint for access token.
// Uses JSON body format as per reference implementation.
func (p *CopilotProvider) pollGitHubToken(ctx context.Context, deviceCode string) (string, error) {
	// Build JSON body - aligned with reference implementation
	requestBody := map[string]string{
		"client_id":   p.config.ClientID,
		"device_code": deviceCode,
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal poll request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.TokenURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create poll request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute poll request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var tokenResp githubTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	// Check for OAuth errors
	if tokenResp.Error != "" {
		switch tokenResp.Error {
		case "authorization_pending":
			return "", fmt.Errorf("authorization pending")
		case "slow_down":
			return "", fmt.Errorf("slow down")
		case "expired_token":
			return "", fmt.Errorf("expired token")
		case "access_denied":
			return "", fmt.Errorf("access denied: %s", tokenResp.ErrorDesc)
		default:
			return "", fmt.Errorf("oauth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
		}
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	return tokenResp.AccessToken, nil
}

// exchangeForCopilotToken exchanges GitHub token for Copilot session token.
// Headers aligned with reference implementation (api-config.ts githubHeaders).
func (p *CopilotProvider) exchangeForCopilotToken(ctx context.Context, githubToken string) (string, time.Time, error) {
	if p.config.CopilotTokenURL == "" {
		return "", time.Time{}, fmt.Errorf("copilot token URL not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.CopilotTokenURL, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create copilot token request: %w", err)
	}

	// Set headers aligned with reference implementation (githubHeaders in api-config.ts)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Editor-Version", "vscode/"+defaultVSCodeVersion)
	req.Header.Set("Editor-Plugin-Version", copilotEditorPluginVersion)
	req.Header.Set("User-Agent", copilotUserAgent)
	req.Header.Set("X-Github-Api-Version", copilotGitHubAPIVersion)
	req.Header.Set("X-Vscode-User-Agent-Library-Version", "electron-fetch")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("execute copilot token request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusUnauthorized {
		return "", time.Time{}, fmt.Errorf("github token unauthorized for copilot")
	}

	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("copilot access forbidden (subscription required?): %s", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("copilot token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp copilotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("decode copilot token response: %w", err)
	}

	if tokenResp.ErrorCode != "" {
		return "", time.Time{}, fmt.Errorf("copilot error: %s - %s", tokenResp.ErrorCode, tokenResp.Message)
	}

	if tokenResp.Token == "" {
		return "", time.Time{}, fmt.Errorf("empty copilot token in response")
	}

	// Log refresh_in for debugging (indicates recommended refresh interval)
	if tokenResp.RefreshIn > 0 {
		copilotLog.Debug("copilot token received",
			zap.Int64("refresh_in_seconds", tokenResp.RefreshIn),
			zap.Int64("expires_at", tokenResp.ExpiresAt),
		)
	}

	expiresAt := time.Unix(tokenResp.ExpiresAt, 0)
	return tokenResp.Token, expiresAt, nil
}

// fetchUserInfo fetches GitHub user info for account identity.
func (p *CopilotProvider) fetchUserInfo(ctx context.Context, githubToken string) (*githubUserResponse, error) {
	if p.config.UserInfoURL == "" {
		return &githubUserResponse{}, fmt.Errorf("user info URL not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.UserInfoURL, nil)
	if err != nil {
		return &githubUserResponse{}, fmt.Errorf("create user info request: %w", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LLMPool/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return &githubUserResponse{}, fmt.Errorf("execute user info request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &githubUserResponse{}, fmt.Errorf("user info endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var userResp githubUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		return &githubUserResponse{}, fmt.Errorf("decode user info response: %w", err)
	}

	return &userResp, nil
}

// accountID returns the account identifier for the GitHub user.
func (u *githubUserResponse) accountID() string {
	if u == nil {
		return ""
	}
	if u.Login != "" {
		return u.Login
	}
	if u.ID > 0 {
		return fmt.Sprintf("%d", u.ID)
	}
	return u.Email
}
