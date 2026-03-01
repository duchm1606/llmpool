package config

import (
	"os"
	"path/filepath"
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

	_, err = Load()
	if err == nil {
		t.Fatalf("expected error when encryption key is missing")
	}
}
