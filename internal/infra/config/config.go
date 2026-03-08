package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	App          AppConfig                 `mapstructure:"app"`
	Server       ServerConfig              `mapstructure:"server"`
	Log          LogConfig                 `mapstructure:"log"`
	Postgres     PostgresConfig            `mapstructure:"postgres"`
	Redis        RedisConfig               `mapstructure:"redis"`
	Orchestrator OrchestratorConfig        `mapstructure:"orchestrator"`
	Security     SecurityConfig            `mapstructure:"security"`
	Credential   CredentialConfig          `mapstructure:"credential"`
	OAuth        OAuthConfig               `mapstructure:"oauth"`
	Liveness     LivenessConfig            `mapstructure:"liveness"`
	Routing      RoutingConfig             `mapstructure:"routing"`
	MessagesAPI  MessagesAPIConfig         `mapstructure:"messages_api"`
	Usage        UsageConfig               `mapstructure:"usage"`
	CORS         CORSConfig                `mapstructure:"cors"`
	Providers    map[string]ProviderConfig `mapstructure:"providers"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

type LogConfig struct {
	Level       string `mapstructure:"level"`
	Format      string `mapstructure:"format"`
	Development bool   `mapstructure:"development"`
}

type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type OrchestratorConfig struct {
	LBStrategy string `mapstructure:"lb_strategy"`
}

type SecurityConfig struct {
	EncryptionKey string `mapstructure:"encryption_key"`
}

type CredentialConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval"`
}

type OAuthConfig struct {
	Codex   CodexOAuthConfig   `mapstructure:"codex"`
	Copilot CopilotOAuthConfig `mapstructure:"copilot"`
}

type CopilotOAuthConfig struct {
	ClientID        string        `mapstructure:"client_id"`
	DeviceCodeURL   string        `mapstructure:"device_code_url"`
	TokenURL        string        `mapstructure:"token_url"`
	CopilotTokenURL string        `mapstructure:"copilot_token_url"`
	UserInfoURL     string        `mapstructure:"user_info_url"`
	Scope           string        `mapstructure:"scope"`
	Timeout         time.Duration `mapstructure:"timeout"`
	SessionTTL      time.Duration `mapstructure:"session_ttl"`
	AccountType     string        `mapstructure:"account_type"`
	// EnterpriseURL is the optional GitHub Enterprise Server URL.
	// When set, the Copilot API base URL becomes https://copilot-api.<normalized_domain>
	// where normalized_domain strips the scheme and trailing slash.
	// Takes precedence over AccountType for URL resolution.
	EnterpriseURL string `mapstructure:"enterprise_url"`
}

type CodexOAuthConfig struct {
	ClientID    string        `mapstructure:"client_id"`
	AuthURL     string        `mapstructure:"auth_url"`
	TokenURL    string        `mapstructure:"token_url"`
	RedirectURI string        `mapstructure:"redirect_uri"`
	DeviceURL   string        `mapstructure:"device_url"`
	PollURL     string        `mapstructure:"poll_url"`
	Timeout     time.Duration `mapstructure:"timeout"`
	SessionTTL  time.Duration `mapstructure:"session_ttl"`
}

type LivenessConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	SampleInterval       time.Duration `mapstructure:"sample_interval"`
	FullSweepInterval    time.Duration `mapstructure:"full_sweep_interval"`
	SamplePercent        float64       `mapstructure:"sample_percent"`
	StateTTL             time.Duration `mapstructure:"state_ttl"`
	AuthFailureCooldown  time.Duration `mapstructure:"auth_failure_cooldown"`
	RateLimitInitial     time.Duration `mapstructure:"rate_limit_initial"`
	RateLimitMaxCooldown time.Duration `mapstructure:"rate_limit_max_cooldown"`
	NetworkErrorCooldown time.Duration `mapstructure:"network_error_cooldown"`
	NetworkMaxRetries    int           `mapstructure:"network_max_retries"`
	CodexUsageURL        string        `mapstructure:"codex_usage_url"`
	CopilotUsageURL      string        `mapstructure:"copilot_usage_url"`
	CheckTimeout         time.Duration `mapstructure:"check_timeout"`
}

// RoutingConfig holds configuration for the completion API routing.
type RoutingConfig struct {
	Enabled          bool           `mapstructure:"enabled"`
	ProviderPriority []string       `mapstructure:"provider_priority"`
	Fallback         FallbackConfig `mapstructure:"fallback"`
	RequestTimeout   time.Duration  `mapstructure:"request_timeout"`
	Health           HealthConfig   `mapstructure:"health"`
}

// FallbackConfig configures fallback behavior.
type FallbackConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	MaxAttempts int  `mapstructure:"max_attempts"`
}

// HealthConfig configures health tracking.
type HealthConfig struct {
	FailureThreshold         int           `mapstructure:"failure_threshold"`
	CooldownDuration         time.Duration `mapstructure:"cooldown_duration"`
	RateLimitDefaultCooldown time.Duration `mapstructure:"rate_limit_default_cooldown"`
}

// ProviderConfig holds configuration for a single LLM provider.
type ProviderConfig struct {
	Enabled  bool              `mapstructure:"enabled"`
	Name     string            `mapstructure:"name"`
	BaseURL  string            `mapstructure:"base_url"`
	AuthType string            `mapstructure:"auth_type"`
	Timeout  time.Duration     `mapstructure:"timeout"`
	Models   []string          `mapstructure:"models"`
	Headers  map[string]string `mapstructure:"headers"`
	// EnableResponsesRouting enables /responses endpoint routing for supported models.
	// When true, GPT-5+ models (except gpt-5-mini) use /responses instead of /chat/completions.
	// This mirrors opencode reference behavior. Default: false for safe rollout.
	EnableResponsesRouting bool `mapstructure:"enable_responses_routing"`
}

// MessagesAPIConfig holds configuration for the Anthropic-style Messages API adapter.
type MessagesAPIConfig struct {
	// SmallModel is the model to use for compact/summarization requests.
	// If empty, compact requests use the originally requested model.
	// Example: "gpt-4o-mini" or "claude-3-haiku"
	SmallModel string `mapstructure:"small_model"`

	// DefaultReasoningEffort is the default reasoning effort for models that support adaptive thinking.
	// Valid values: "low", "medium", "high", "max"
	// Default: "medium"
	DefaultReasoningEffort string `mapstructure:"default_reasoning_effort"`

	// CompactUseSmallModel controls whether compact/summarization requests use SmallModel.
	// Default: false (use originally requested model)
	CompactUseSmallModel bool `mapstructure:"compact_use_small_model"`
}

// UsageConfig holds configuration for usage tracking.
type UsageConfig struct {
	// Enabled enables usage tracking
	Enabled bool `mapstructure:"enabled"`

	// QueueSize is the size of the internal usage queue
	QueueSize int `mapstructure:"queue_size"`

	// BatchSize is the number of records to process in a batch
	BatchSize int `mapstructure:"batch_size"`

	// FlushInterval is how often to flush batches
	FlushInterval time.Duration `mapstructure:"flush_interval"`

	// StatsCacheTTL is how long to cache dashboard stats
	StatsCacheTTL time.Duration `mapstructure:"stats_cache_ttl"`

	// StatsRebuildInterval is how often to rebuild aggregated dashboard stats
	StatsRebuildInterval time.Duration `mapstructure:"stats_rebuild_interval"`

	// RetentionDays is the number of days to retain audit logs
	RetentionDays int `mapstructure:"retention_days"`

	// RetentionCleanupInterval is how often to run retention cleanup
	RetentionCleanupInterval time.Duration `mapstructure:"retention_cleanup_interval"`
}

// CORSConfig holds configuration for CORS.
type CORSConfig struct {
	// Enabled enables CORS
	Enabled bool `mapstructure:"enabled"`

	// AllowedOrigins is the list of allowed origins for CORS
	// Use "*" to allow all origins (not recommended for production)
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	v := viper.New()

	setDefaults(v)

	v.SetConfigType("yml")
	v.SetConfigName("default")
	v.AddConfigPath("configs")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	v.SetEnvPrefix("LLMPOOL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.BindEnv("security.encryption_key"); err != nil {
		return nil, fmt.Errorf("bind env security.encryption_key: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg, func(c *mapstructure.DecoderConfig) {
		c.DecodeHook = mapstructure.StringToTimeDurationHookFunc()
	}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.Server.Host == "" {
		return nil, fmt.Errorf("server.host is required")
	}
	if cfg.Server.Port <= 0 {
		return nil, fmt.Errorf("server.port must be > 0")
	}

	validLogLevels := map[string]struct{}{
		"debug": {},
		"info":  {},
		"warn":  {},
		"error": {},
	}
	if _, ok := validLogLevels[cfg.Log.Level]; !ok {
		return nil, fmt.Errorf("log.level must be one of: debug, info, warn, error")
	}

	validLogFormats := map[string]struct{}{
		"json": {},
		"text": {},
	}
	if _, ok := validLogFormats[cfg.Log.Format]; !ok {
		return nil, fmt.Errorf("log.format must be one of: json, text")
	}

	if cfg.Orchestrator.LBStrategy != "round-robin" && cfg.Orchestrator.LBStrategy != "fill-first" {
		return nil, fmt.Errorf("orchestrator.lb_strategy must be one of: round-robin, fill-first")
	}

	if cfg.Credential.RefreshInterval <= 0 {
		return nil, fmt.Errorf("credential.refresh_interval must be > 0")
	}

	if cfg.OAuth.Codex.ClientID == "" {
		return nil, fmt.Errorf("oauth.codex.client_id is required")
	}
	if cfg.OAuth.Codex.AuthURL == "" {
		return nil, fmt.Errorf("oauth.codex.auth_url is required")
	}
	if cfg.OAuth.Codex.TokenURL == "" {
		return nil, fmt.Errorf("oauth.codex.token_url is required")
	}
	if cfg.OAuth.Codex.RedirectURI == "" {
		return nil, fmt.Errorf("oauth.codex.redirect_uri is required")
	}
	if cfg.OAuth.Codex.DeviceURL == "" {
		return nil, fmt.Errorf("oauth.codex.device_url is required")
	}
	if cfg.OAuth.Codex.PollURL == "" {
		return nil, fmt.Errorf("oauth.codex.poll_url is required")
	}
	if cfg.OAuth.Codex.Timeout <= 0 {
		return nil, fmt.Errorf("oauth.codex.timeout must be > 0")
	}
	if cfg.OAuth.Codex.SessionTTL <= 0 {
		return nil, fmt.Errorf("oauth.codex.session_ttl must be > 0")
	}

	if cfg.OAuth.Copilot.ClientID == "" {
		return nil, fmt.Errorf("oauth.copilot.client_id is required")
	}
	if cfg.OAuth.Copilot.DeviceCodeURL == "" {
		return nil, fmt.Errorf("oauth.copilot.device_code_url is required")
	}
	if cfg.OAuth.Copilot.TokenURL == "" {
		return nil, fmt.Errorf("oauth.copilot.token_url is required")
	}
	if cfg.OAuth.Copilot.CopilotTokenURL == "" {
		return nil, fmt.Errorf("oauth.copilot.copilot_token_url is required")
	}
	if cfg.OAuth.Copilot.UserInfoURL == "" {
		return nil, fmt.Errorf("oauth.copilot.user_info_url is required")
	}
	if cfg.OAuth.Copilot.Scope == "" {
		return nil, fmt.Errorf("oauth.copilot.scope is required")
	}
	if cfg.OAuth.Copilot.Timeout <= 0 {
		return nil, fmt.Errorf("oauth.copilot.timeout must be > 0")
	}
	if cfg.OAuth.Copilot.SessionTTL <= 0 {
		return nil, fmt.Errorf("oauth.copilot.session_ttl must be > 0")
	}
	if cfg.OAuth.Copilot.AccountType == "" {
		return nil, fmt.Errorf("oauth.copilot.account_type is required")
	}

	validCopilotAccountTypes := map[string]struct{}{
		"individual": {},
		"business":   {},
		"enterprise": {},
	}
	if _, ok := validCopilotAccountTypes[strings.ToLower(strings.TrimSpace(cfg.OAuth.Copilot.AccountType))]; !ok {
		return nil, fmt.Errorf("oauth.copilot.account_type must be one of: individual, business, enterprise")
	}

	// Validate liveness config if enabled
	if cfg.Liveness.Enabled {
		if cfg.Liveness.SampleInterval <= 0 {
			return nil, fmt.Errorf("liveness.sample_interval must be > 0 when liveness is enabled")
		}
		if cfg.Liveness.FullSweepInterval <= 0 {
			return nil, fmt.Errorf("liveness.full_sweep_interval must be > 0 when liveness is enabled")
		}
		if cfg.Liveness.SamplePercent <= 0 || cfg.Liveness.SamplePercent > 1 {
			return nil, fmt.Errorf("liveness.sample_percent must be > 0 and <= 1 when liveness is enabled")
		}
		if cfg.Liveness.StateTTL <= 0 {
			return nil, fmt.Errorf("liveness.state_ttl must be > 0 when liveness is enabled")
		}
		if cfg.Liveness.AuthFailureCooldown <= 0 {
			return nil, fmt.Errorf("liveness.auth_failure_cooldown must be > 0 when liveness is enabled")
		}
		if cfg.Liveness.RateLimitInitial <= 0 {
			return nil, fmt.Errorf("liveness.rate_limit_initial must be > 0 when liveness is enabled")
		}
		if cfg.Liveness.RateLimitMaxCooldown <= 0 {
			return nil, fmt.Errorf("liveness.rate_limit_max_cooldown must be > 0 when liveness is enabled")
		}
		if cfg.Liveness.NetworkErrorCooldown <= 0 {
			return nil, fmt.Errorf("liveness.network_error_cooldown must be > 0 when liveness is enabled")
		}
		if cfg.Liveness.NetworkMaxRetries < 0 {
			return nil, fmt.Errorf("liveness.network_max_retries must be >= 0 when liveness is enabled")
		}
	}

	// Validate usage tracking config if enabled
	if cfg.Usage.Enabled {
		if cfg.Usage.QueueSize <= 0 {
			return nil, fmt.Errorf("usage.queue_size must be > 0 when usage tracking is enabled")
		}
		if cfg.Usage.BatchSize <= 0 {
			return nil, fmt.Errorf("usage.batch_size must be > 0 when usage tracking is enabled")
		}
		if cfg.Usage.FlushInterval <= 0 {
			return nil, fmt.Errorf("usage.flush_interval must be > 0 when usage tracking is enabled")
		}
		if cfg.Usage.StatsCacheTTL <= 0 {
			return nil, fmt.Errorf("usage.stats_cache_ttl must be > 0 when usage tracking is enabled")
		}
		if cfg.Usage.StatsRebuildInterval <= 0 {
			return nil, fmt.Errorf("usage.stats_rebuild_interval must be > 0 when usage tracking is enabled")
		}
		if cfg.Usage.RetentionDays <= 0 {
			return nil, fmt.Errorf("usage.retention_days must be > 0 when usage tracking is enabled")
		}
		if cfg.Usage.RetentionCleanupInterval <= 0 {
			return nil, fmt.Errorf("usage.retention_cleanup_interval must be > 0 when usage tracking is enabled")
		}
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "llmpool")
	v.SetDefault("app.env", "dev")

	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "10s")
	v.SetDefault("server.write_timeout", "0s")
	v.SetDefault("server.idle_timeout", "30s")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")
	v.SetDefault("log.development", true)

	v.SetDefault("postgres.dsn", "postgres://postgres:postgres@postgres:5432/llmpool?sslmode=disable")

	v.SetDefault("redis.addr", "redis:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	v.SetDefault("orchestrator.lb_strategy", "round-robin")

	v.SetDefault("credential.refresh_interval", "1m")

	// Copilot OAuth defaults (public client, non-sensitive)
	v.SetDefault("oauth.copilot.client_id", "Iv1.b507a08c87ecfe98")
	v.SetDefault("oauth.copilot.device_code_url", "https://github.com/login/device/code")
	v.SetDefault("oauth.copilot.token_url", "https://github.com/login/oauth/access_token")
	v.SetDefault("oauth.copilot.copilot_token_url", "https://api.github.com/copilot_internal/v2/token")
	v.SetDefault("oauth.copilot.user_info_url", "https://api.github.com/user")
	v.SetDefault("oauth.copilot.scope", "read:user")
	v.SetDefault("oauth.copilot.timeout", "30s")
	v.SetDefault("oauth.copilot.session_ttl", "600s")
	v.SetDefault("oauth.copilot.account_type", "individual")
	v.SetDefault("oauth.copilot.enterprise_url", "")

	// Liveness checker defaults
	v.SetDefault("liveness.enabled", true)
	v.SetDefault("liveness.sample_interval", "5m")
	v.SetDefault("liveness.full_sweep_interval", "60m")
	v.SetDefault("liveness.sample_percent", 0.20)
	v.SetDefault("liveness.state_ttl", "2h")
	v.SetDefault("liveness.auth_failure_cooldown", "30m")
	v.SetDefault("liveness.rate_limit_initial", "2m")
	v.SetDefault("liveness.rate_limit_max_cooldown", "30m")
	v.SetDefault("liveness.network_error_cooldown", "5m")
	v.SetDefault("liveness.network_max_retries", 3)
	v.SetDefault("liveness.codex_usage_url", "https://chatgpt.com/backend-api/wham/usage")
	v.SetDefault("liveness.copilot_usage_url", "https://api.github.com/copilot_internal/user")
	v.SetDefault("liveness.check_timeout", "10s")

	// Routing defaults
	v.SetDefault("routing.enabled", true)
	v.SetDefault("routing.provider_priority", []string{"codex", "copilot", "openai", "anthropic"})
	v.SetDefault("routing.fallback.enabled", true)
	v.SetDefault("routing.fallback.max_attempts", 3)
	v.SetDefault("routing.request_timeout", "120s")
	v.SetDefault("routing.health.failure_threshold", 3)
	v.SetDefault("routing.health.cooldown_duration", "30s")
	v.SetDefault("routing.health.rate_limit_default_cooldown", "60s")

	// Messages API defaults (Anthropic-style adapter)
	v.SetDefault("messages_api.small_model", "")                    // Empty = use requested model
	v.SetDefault("messages_api.default_reasoning_effort", "medium") // low, medium, high, max
	v.SetDefault("messages_api.compact_use_small_model", false)     // Don't override model for compact

	// Usage tracking defaults
	v.SetDefault("usage.enabled", true)
	v.SetDefault("usage.queue_size", 10000)
	v.SetDefault("usage.batch_size", 100)
	v.SetDefault("usage.flush_interval", "5s")
	v.SetDefault("usage.stats_cache_ttl", "5m")
	v.SetDefault("usage.stats_rebuild_interval", "15m")
	v.SetDefault("usage.retention_days", 90)
	v.SetDefault("usage.retention_cleanup_interval", "24h")

	// CORS defaults
	v.SetDefault("cors.enabled", true)
	v.SetDefault("cors.allowed_origins", []string{"http://localhost:3000", "http://localhost:5173"})
}
