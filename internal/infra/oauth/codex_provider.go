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
)

// CodexConfig holds configuration for the Codex OAuth provider
type CodexConfig struct {
	ClientID    string
	AuthURL     string
	TokenURL    string
	RedirectURI string
	Timeout     int // seconds
}

// CodexProvider implements OAuthProvider for OpenAI/Codex
type CodexProvider struct {
	config CodexConfig
	client HTTPClient
}

// HTTPClient interface for testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewCodexProvider creates a new Codex OAuth provider
func NewCodexProvider(config CodexConfig, client HTTPClient) *CodexProvider {
	if client == nil {
		client = &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		}
	}
	return &CodexProvider{
		config: config,
		client: client,
	}
}

// BuildAuthURL generates the OAuth authorization URL with PKCE parameters
func (p *CodexProvider) BuildAuthURL(ctx context.Context, state string, verifier string) (domainoauth.AuthorizationURL, error) {
	// Generate S256 challenge from verifier
	challenge := GenerateChallenge(verifier)

	// Build URL with query parameters
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("response_type", "code")
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	params.Set("redirect_uri", p.config.RedirectURI)
	params.Set("scope", "openid email profile offline_access")
	params.Set("prompt", "login")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")

	fullURL := p.config.AuthURL + "?" + params.Encode()

	return domainoauth.AuthorizationURL{
		URL:   fullURL,
		State: state,
	}, nil
}

// tokenResponse represents the JSON response from token endpoint
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// ExchangeCode exchanges authorization code for tokens
func (p *CodexProvider) ExchangeCode(ctx context.Context, code string, verifier string) (domainoauth.TokenPayload, error) {
	// Build form data
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("code_verifier", verifier)
	data.Set("redirect_uri", p.config.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("decode response: %w", err)
	}

	// Check for OAuth error
	if tokenResp.Error != "" {
		return domainoauth.TokenPayload{}, fmt.Errorf("oauth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return domainoauth.TokenPayload{}, fmt.Errorf("token exchange failed: HTTP %d", resp.StatusCode)
	}

	payload := domainoauth.TokenPayload{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	return payload, nil
}

// RefreshToken refreshes an access token using refresh token
func (p *CodexProvider) RefreshToken(ctx context.Context, refreshToken string) (domainoauth.TokenPayload, error) {
	// Build form data
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return domainoauth.TokenPayload{}, fmt.Errorf("decode response: %w", err)
	}

	if tokenResp.Error != "" {
		return domainoauth.TokenPayload{}, fmt.Errorf("oauth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if resp.StatusCode != http.StatusOK {
		return domainoauth.TokenPayload{}, fmt.Errorf("token refresh failed: HTTP %d", resp.StatusCode)
	}

	payload := domainoauth.TokenPayload{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	return payload, nil
}

// StartDeviceFlow initiates device authorization flow
func (p *CodexProvider) StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error) {
	// TODO: Implement in Session 7
	return domainoauth.DeviceFlowResponse{}, fmt.Errorf("not implemented")
}

// PollDevice polls for device authorization completion
func (p *CodexProvider) PollDevice(ctx context.Context, deviceCode string) (domainoauth.TokenPayload, error) {
	// TODO: Implement in Session 7
	return domainoauth.TokenPayload{}, fmt.Errorf("not implemented")
}
