package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainoauth "github.com/duchoang/llmpool/internal/domain/oauth"
	"github.com/duchoang/llmpool/internal/infra/config"
	usecaseoauth "github.com/duchoang/llmpool/internal/usecase/oauth"
)

var _ usecaseoauth.OAuthProvider = (*CodexProvider)(nil)

// CodexProvider implements OAuthProvider for OpenAI/Codex OAuth
type CodexProvider struct {
	config     config.CodexOAuthConfig
	httpClient *http.Client
}

// NewCodexProvider creates a new Codex OAuth provider
func NewCodexProvider(cfg config.CodexOAuthConfig) *CodexProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &CodexProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// BuildAuthURL generates the OAuth authorization URL with PKCE
func (p *CodexProvider) BuildAuthURL(ctx context.Context, state, verifier string) (domainoauth.AuthorizationURL, error) {
	if p.config.AuthURL == "" {
		return domainoauth.AuthorizationURL{}, fmt.Errorf("auth URL not configured")
	}

	challenge := GenerateChallenge(verifier)

	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", p.config.RedirectURI)
	params.Set("state", state)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("scope", "openid email profile offline_access")
	params.Set("prompt", "login")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")

	authURL := p.config.AuthURL + "?" + params.Encode()

	return domainoauth.AuthorizationURL{
		URL:   authURL,
		State: state,
	}, nil
}

// tokenResponse represents the OAuth token endpoint response
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// ExchangeCode exchanges authorization code for tokens
func (p *CodexProvider) ExchangeCode(ctx context.Context, code, verifier string) (domainoauth.TokenPayload, error) {
	if p.config.TokenURL == "" {
		return domainoauth.TokenPayload{}, fmt.Errorf("token URL not configured")
	}

	params := url.Values{}
	params.Set("grant_type", "authorization_code")
	params.Set("code", code)
	params.Set("redirect_uri", p.config.RedirectURI)
	params.Set("client_id", p.config.ClientID)
	params.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("execute token request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return domainoauth.TokenPayload{}, fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("decode token response: %w", err)
	}

	return domainoauth.TokenPayload{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
	}, nil
}

// RefreshToken refreshes an access token using refresh token
func (p *CodexProvider) RefreshToken(ctx context.Context, refreshToken string) (domainoauth.TokenPayload, error) {
	if p.config.TokenURL == "" {
		return domainoauth.TokenPayload{}, fmt.Errorf("token URL not configured")
	}

	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)
	params.Set("client_id", p.config.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("create refresh token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("execute refresh token request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return domainoauth.TokenPayload{}, fmt.Errorf("refresh token endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("decode refresh token response: %w", err)
	}

	return domainoauth.TokenPayload{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
	}, nil
}

// deviceAuthResponse represents the device authorization endpoint response
type deviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// StartDeviceFlow initiates device authorization flow
func (p *CodexProvider) StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error) {
	if p.config.DeviceURL == "" {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("device URL not configured")
	}

	params := url.Values{}
	params.Set("client_id", p.config.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.DeviceURL, strings.NewReader(params.Encode()))
	if err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("create device request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("execute device request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("device endpoint returned status %d", resp.StatusCode)
	}

	var deviceResp deviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return domainoauth.DeviceFlowResponse{}, fmt.Errorf("decode device response: %w", err)
	}

	return domainoauth.DeviceFlowResponse{
		DeviceCode:      deviceResp.DeviceCode,
		UserCode:        deviceResp.UserCode,
		VerificationURI: deviceResp.VerificationURI,
		ExpiresIn:       deviceResp.ExpiresIn,
		Interval:        deviceResp.Interval,
	}, nil
}

// PollDevice polls for device authorization completion
func (p *CodexProvider) PollDevice(ctx context.Context, deviceCode string) (domainoauth.TokenPayload, error) {
	if p.config.TokenURL == "" {
		return domainoauth.TokenPayload{}, fmt.Errorf("token URL not configured")
	}

	params := url.Values{}
	params.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	params.Set("device_code", deviceCode)
	params.Set("client_id", p.config.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("create poll request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("execute poll request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusBadRequest {
		// Check for specific error codes in response body
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			switch errResp.Error {
			case "authorization_pending":
				return domainoauth.TokenPayload{}, fmt.Errorf("authorization pending")
			case "slow_down":
				return domainoauth.TokenPayload{}, fmt.Errorf("slow down")
			case "expired_token":
				return domainoauth.TokenPayload{}, fmt.Errorf("expired token")
			}
		}
		return domainoauth.TokenPayload{}, fmt.Errorf("poll endpoint returned status 400")
	}

	if resp.StatusCode != http.StatusOK {
		return domainoauth.TokenPayload{}, fmt.Errorf("poll endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("decode token response: %w", err)
	}

	return domainoauth.TokenPayload{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
	}, nil
}
