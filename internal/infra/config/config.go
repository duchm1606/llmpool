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

	if err := v.BindEnv("oauth.codex.auth_url"); err != nil {
		return nil, fmt.Errorf("bind env oauth.codex.auth_url: %w", err)
	}
	if err := v.BindEnv("oauth.codex.token_url"); err != nil {
		return nil, fmt.Errorf("bind env oauth.codex.token_url: %w", err)
	}
	if err := v.BindEnv("oauth.codex.redirect_uri"); err != nil {
		return nil, fmt.Errorf("bind env oauth.codex.redirect_uri: %w", err)
	}
	if err := v.BindEnv("oauth.codex.device_url"); err != nil {
		return nil, fmt.Errorf("bind env oauth.codex.device_url: %w", err)
	}
	if err := v.BindEnv("oauth.codex.poll_url"); err != nil {
		return nil, fmt.Errorf("bind env oauth.codex.poll_url: %w", err)
	}
	if err := v.BindEnv("oauth.codex.timeout"); err != nil {
		return nil, fmt.Errorf("bind env oauth.codex.timeout: %w", err)
	}
	if err := v.BindEnv("oauth.codex.session_ttl"); err != nil {
		return nil, fmt.Errorf("bind env oauth.codex.session_ttl: %w", err)
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

	if cfg.Orchestrator.LBStrategy != "round-robin" && cfg.Orchestrator.LBStrategy != "fill-first" {
		return nil, fmt.Errorf("orchestrator.lb_strategy must be one of: round-robin, fill-first")
	}

	if cfg.Security.EncryptionKey == "" {
		return nil, fmt.Errorf("security.encryption_key is required")
	}

	if cfg.Credential.RefreshInterval <= 0 {
		return nil, fmt.Errorf("credential.refresh_interval must be > 0")
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
	v.SetDefault("log.development", true)

	v.SetDefault("postgres.dsn", "postgres://postgres:postgres@postgres:5432/llmpool?sslmode=disable")

	v.SetDefault("redis.addr", "redis:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	v.SetDefault("orchestrator.lb_strategy", "round-robin")

	v.SetDefault("oauth.codex.auth_url", "https://auth.openai.com/authorize")
	v.SetDefault("oauth.codex.token_url", "https://auth.openai.com/token")
	v.SetDefault("oauth.codex.redirect_uri", "http://localhost:8080/oauth/callback")
	v.SetDefault("oauth.codex.device_url", "https://auth.openai.com/device/code")
	v.SetDefault("oauth.codex.poll_url", "https://auth.openai.com/device/poll")
	v.SetDefault("oauth.codex.timeout", "30s")
	v.SetDefault("oauth.codex.session_ttl", "600s")
}
