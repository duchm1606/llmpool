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

func TestLoad_RequiresEncryptionKeyFromEnv(t *testing.T) {
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
	if err == nil {
		t.Fatalf("expected error when encryption key is missing")
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
	t.Setenv("LLMPOOL_OAUTH_CODEX_CLIENT_ID", "")

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
