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

// CodexProvider implements OAuthProvider for OpenAI/Codex OAuth
type CodexProvider struct {
	config     OAuthCodexConfig
	httpClient *http.Client
}

// OAuthCodexConfig holds Codex OAuth configuration
type OAuthCodexConfig struct {
	AuthURL     string
	TokenURL    string
	RedirectURI string
	ClientID    string
	Timeout     time.Duration
}

// NewCodexProvider creates a new Codex OAuth provider
func NewCodexProvider(cfg OAuthCodexConfig) *CodexProvider {
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
	defer resp.Body.Close()

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
	// TODO: Implement in T16
	return domainoauth.TokenPayload{}, fmt.Errorf("refresh token not implemented")
}

// StartDeviceFlow initiates device authorization flow
func (p *CodexProvider) StartDeviceFlow(ctx context.Context) (domainoauth.DeviceFlowResponse, error) {
	// TODO: Implement in T17
	return domainoauth.DeviceFlowResponse{}, fmt.Errorf("device flow not implemented")
}

// PollDevice polls for device authorization completion
func (p *CodexProvider) PollDevice(ctx context.Context, deviceCode string) (domainoauth.TokenPayload, error) {
	// TODO: Implement in T17
	return domainoauth.TokenPayload{}, fmt.Errorf("device flow not implemented")
}
