package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_UsesEnvOverrideFromDotEnv(t *testing.T) {
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

	envPath := filepath.Join(rootDir, ".env")
	if err := os.WriteFile(envPath, []byte("LLMPOOL_SERVER_PORT=19082\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	defer func() {
		_ = os.Remove(envPath)
		_ = os.Unsetenv("LLMPOOL_SERVER_PORT")
	}()

	if err := os.Unsetenv("LLMPOOL_SERVER_PORT"); err != nil {
		t.Fatalf("unset env: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 19082 {
		t.Fatalf("expected port 19082 from .env, got %d", cfg.Server.Port)
	}
}
