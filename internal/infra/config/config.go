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
}
