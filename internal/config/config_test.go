package config

import (
	"testing"
	"time"
)

func TestLoadConfigRequiresCoreEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}

func TestLoadConfigProvidesRuntimeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable")
	t.Setenv("S3_ENDPOINT", "localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "minioadmin")
	t.Setenv("S3_SECRET_KEY", "minioadmin")
	t.Setenv("S3_BUCKET", "platform-dev")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("expected default :8080, got %s", cfg.HTTPAddr)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("expected 10s shutdown timeout, got %s", cfg.ShutdownTimeout)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Fatalf("expected default redis addr, got %s", cfg.RedisAddr)
	}
	if cfg.APIBaseURL != "http://localhost:8080" {
		t.Fatalf("expected default api base url, got %s", cfg.APIBaseURL)
	}
	if cfg.LeaseSeconds != 30 {
		t.Fatalf("expected default lease seconds, got %d", cfg.LeaseSeconds)
	}
}
