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
	App          AppConfig          `mapstructure:"app"`
	Server       ServerConfig       `mapstructure:"server"`
	Log          LogConfig          `mapstructure:"log"`
	Postgres     PostgresConfig     `mapstructure:"postgres"`
	Redis        RedisConfig        `mapstructure:"redis"`
	Orchestrator OrchestratorConfig `mapstructure:"orchestrator"`
	Security     SecurityConfig     `mapstructure:"security"`
	Credential   CredentialConfig   `mapstructure:"credential"`
	OAuth        OAuthConfig        `mapstructure:"oauth"`
	Liveness     LivenessConfig     `mapstructure:"liveness"`
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
	Codex CodexOAuthConfig `mapstructure:"codex"`
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
	CheckTimeout         time.Duration `mapstructure:"check_timeout"`
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

	if cfg.Security.EncryptionKey == "" {
		return nil, fmt.Errorf("security.encryption_key is required")
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

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "llmpool")
	v.SetDefault("app.env", "dev")

	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "10s")
	v.SetDefault("server.write_timeout", "10s")
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
	v.SetDefault("liveness.check_timeout", "10s")
}
