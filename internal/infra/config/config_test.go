package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_UsesEnvironmentOverride(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SERVER_PORT", "19082")
	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 19082 {
		t.Fatalf("expected port 19082 from env, got %d", cfg.Server.Port)
	}
}

func TestLoad_AllowsMissingEncryptionKeyFromEnv(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SERVER_PORT", "19082")
	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")

	_, err = Load()
	if err != nil {
		t.Fatalf("expected config to load without encryption key, got: %v", err)
	}
}

func TestLoad_OAuthConfigEnvironmentOverride(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_OAUTH_CODEX_AUTH_URL", "https://custom.auth.example.com/authorize")
	t.Setenv("LLMPOOL_OAUTH_CODEX_TOKEN_URL", "https://custom.auth.example.com/token")
	t.Setenv("LLMPOOL_OAUTH_CODEX_REDIRECT_URI", "http://custom:9090/callback")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.OAuth.Codex.AuthURL != "https://custom.auth.example.com/authorize" {
		t.Fatalf("expected custom auth_url from env, got %s", cfg.OAuth.Codex.AuthURL)
	}
	if cfg.OAuth.Codex.TokenURL != "https://custom.auth.example.com/token" {
		t.Fatalf("expected custom token_url from env, got %s", cfg.OAuth.Codex.TokenURL)
	}
	if cfg.OAuth.Codex.RedirectURI != "http://custom:9090/callback" {
		t.Fatalf("expected custom redirect_uri from env, got %s", cfg.OAuth.Codex.RedirectURI)
	}
}

func TestLoad_LogFormatEnvironmentOverride(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_LOG_FORMAT", "json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Log.Format != "json" {
		t.Fatalf("expected log format json, got %q", cfg.Log.Format)
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_LOG_LEVEL", "verbose")

	_, err = Load()
	if err == nil {
		t.Fatalf("expected error when log.level is invalid")
	}
	if !strings.Contains(err.Error(), "log.level must be one of") {
		t.Fatalf("unexpected error, got: %v", err)
	}
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_LOG_FORMAT", "yaml")

	_, err = Load()
	if err == nil {
		t.Fatalf("expected error when log.format is invalid")
	}
	if !strings.Contains(err.Error(), "log.format must be one of") {
		t.Fatalf("unexpected error, got: %v", err)
	}
}

func TestLoad_OAuthCodexUsesDefaults(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.OAuth.Codex.AuthURL != "https://auth.openai.com/oauth/authorize" {
		t.Fatalf("expected default auth_url, got %s", cfg.OAuth.Codex.AuthURL)
	}
	if cfg.OAuth.Codex.TokenURL != "https://auth.openai.com/oauth/token" {
		t.Fatalf("expected default token_url, got %s", cfg.OAuth.Codex.TokenURL)
	}
	if cfg.OAuth.Codex.RedirectURI != "http://localhost:1455/auth/callback" {
		t.Fatalf("expected default redirect_uri, got %s", cfg.OAuth.Codex.RedirectURI)
	}
	if cfg.OAuth.Codex.DeviceURL != "https://auth.openai.com/device/code" {
		t.Fatalf("expected default device_url, got %s", cfg.OAuth.Codex.DeviceURL)
	}
	if cfg.OAuth.Codex.PollURL != "https://auth.openai.com/device/poll" {
		t.Fatalf("expected default poll_url, got %s", cfg.OAuth.Codex.PollURL)
	}
}

func TestLoad_RequiresOAuthCodexTimeout(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_TIMEOUT", "0")

	_, err = Load()
	if err == nil {
		t.Fatalf("expected error when oauth.codex.timeout is zero")
	}
}

func TestLoad_RequiresOAuthCodexSessionTTL(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_SESSION_TTL", "0")

	_, err = Load()
	if err == nil {
		t.Fatalf("expected error when oauth.codex.session_ttl is zero")
	}
}

func TestLoad_RequiresOAuthCodexClientID(t *testing.T) {
	// Create a temp directory with a config file that has empty client_id
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}

	// Write a minimal config with empty client_id
	configContent := `
server:
  host: 0.0.0.0
  port: 8080
log:
  level: info
  format: text
orchestrator:
  lb_strategy: round-robin
credential:
  refresh_interval: 1m
oauth:
  codex:
    client_id: ""
    auth_url: https://auth.openai.com/oauth/authorize
    token_url: https://auth.openai.com/oauth/token
    redirect_uri: http://localhost:1455/auth/callback
    device_url: https://auth.openai.com/device/code
    poll_url: https://auth.openai.com/device/poll
    timeout: 30s
    session_ttl: 600s
`
	if err := os.WriteFile(filepath.Join(configDir, "default.yml"), []byte(configContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}

	// Ensure env var doesn't override (in case a parallel test sets it)
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "")
	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")

	_, err = Load()
	if err == nil {
		t.Fatalf("expected error when oauth.codex.client_id is missing")
	}
	if !strings.Contains(err.Error(), "oauth.codex.client_id is required") {
		t.Fatalf("unexpected error, got: %v", err)
	}

}

func TestLoad_OAuthCodexClientIDEnvironmentOverride(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id-12345")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.OAuth.Codex.ClientID != "test-client-id-12345" {
		t.Fatalf("expected client_id 'test-client-id-12345' from env, got %s", cfg.OAuth.Codex.ClientID)
	}

}

func TestLoad_LivenessConfigValidation(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	tests := []struct {
		name        string
		envKey      string
		envValue    string
		expectError string
	}{
		{
			name:        "invalid sample_interval",
			envKey:      "LLMPOOL_LIVENESS_SAMPLE_INTERVAL",
			envValue:    "0",
			expectError: "liveness.sample_interval must be > 0",
		},
		{
			name:        "invalid full_sweep_interval",
			envKey:      "LLMPOOL_LIVENESS_FULL_SWEEP_INTERVAL",
			envValue:    "0",
			expectError: "liveness.full_sweep_interval must be > 0",
		},
		{
			name:        "sample_percent too high",
			envKey:      "LLMPOOL_LIVENESS_SAMPLE_PERCENT",
			envValue:    "1.5",
			expectError: "liveness.sample_percent must be > 0 and <= 1",
		},
		{
			name:        "sample_percent zero",
			envKey:      "LLMPOOL_LIVENESS_SAMPLE_PERCENT",
			envValue:    "0",
			expectError: "liveness.sample_percent must be > 0 and <= 1",
		},
		{
			name:        "invalid state_ttl",
			envKey:      "LLMPOOL_LIVENESS_STATE_TTL",
			envValue:    "0",
			expectError: "liveness.state_ttl must be > 0",
		},
		{
			name:        "invalid auth_failure_cooldown",
			envKey:      "LLMPOOL_LIVENESS_AUTH_FAILURE_COOLDOWN",
			envValue:    "0",
			expectError: "liveness.auth_failure_cooldown must be > 0",
		},
		{
			name:        "invalid rate_limit_initial",
			envKey:      "LLMPOOL_LIVENESS_RATE_LIMIT_INITIAL",
			envValue:    "0",
			expectError: "liveness.rate_limit_initial must be > 0",
		},
		{
			name:        "invalid network_max_retries",
			envKey:      "LLMPOOL_LIVENESS_NETWORK_MAX_RETRIES",
			envValue:    "-1",
			expectError: "liveness.network_max_retries must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set base required env vars
			t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
			t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
			t.Setenv("LLMPOOL_LIVENESS_ENABLED", "true")
			// Set the invalid value
			t.Setenv(tt.envKey, tt.envValue)

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Fatalf("expected error containing %q, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestLoad_LivenessDisabledSkipsValidation(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_LIVENESS_ENABLED", "false")
	// Invalid values that should be ignored when disabled
	t.Setenv("LLMPOOL_LIVENESS_SAMPLE_INTERVAL", "0")
	t.Setenv("LLMPOOL_LIVENESS_SAMPLE_PERCENT", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error when liveness is disabled, got: %v", err)
	}

	if cfg.Liveness.Enabled {
		t.Fatal("expected liveness to be disabled")
	}
}

func TestLoad_RequiresOAuthCopilotAccountTypeFromAllowedSet(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_OAUTH_COPILOT_ACCOUNT_TYPE", "team")

	_, err = Load()
	if err == nil {
		t.Fatalf("expected error when oauth.copilot.account_type is invalid")
	}
	if !strings.Contains(err.Error(), "oauth.copilot.account_type must be one of") {
		t.Fatalf("unexpected error, got: %v", err)
	}
}

func TestLoad_AllowsOAuthCopilotAccountTypeBusiness(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_OAUTH_COPILOT_ACCOUNT_TYPE", "business")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.OAuth.Copilot.AccountType != "business" {
		t.Fatalf("expected oauth.copilot.account_type=business, got %q", cfg.OAuth.Copilot.AccountType)
	}
}

func TestLoad_AccountRateLimitDefaults(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Routing.AccountRateLimit.RequestsPerMinute != 5 {
		t.Fatalf("expected requests_per_minute default 5, got %d", cfg.Routing.AccountRateLimit.RequestsPerMinute)
	}
	if cfg.Routing.AccountRateLimit.RequestsPer5HourSession != 50 {
		t.Fatalf("expected requests_per_5hour_session default 50 from YAML config, got %d", cfg.Routing.AccountRateLimit.RequestsPer5HourSession)
	}
}

func TestLoad_InvalidAccountRateLimitConfig(t *testing.T) {
	rootDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve root dir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLMPOOL_SECURITY_ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "test-client-id")
	t.Setenv("LLMPOOL_ROUTING_ACCOUNT_RATE_LIMIT_REQUESTS_PER_MINUTE", "0")

	_, err = Load()
	if err == nil {
		t.Fatal("expected validation error for zero requests_per_minute")
	}
	if !strings.Contains(err.Error(), "routing.account_rate_limit.requests_per_minute must be > 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}
