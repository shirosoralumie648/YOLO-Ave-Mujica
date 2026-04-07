package config

import (
	"os"
	"path/filepath"
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
	if cfg.JobRetryBackoffBase != 5*time.Second {
		t.Fatalf("expected default retry backoff base 5s, got %s", cfg.JobRetryBackoffBase)
	}
	if cfg.JobRetryBackoffMax != time.Minute {
		t.Fatalf("expected default retry backoff max 1m, got %s", cfg.JobRetryBackoffMax)
	}
	if cfg.ArtifactBuildConcurrency != 2 {
		t.Fatalf("expected default artifact build concurrency, got %d", cfg.ArtifactBuildConcurrency)
	}
	if cfg.ArtifactStorageDir != filepath.Join(os.TempDir(), "platform-artifacts") {
		t.Fatalf("expected default artifact storage dir, got %s", cfg.ArtifactStorageDir)
	}
	if cfg.AuthBearerToken != "" {
		t.Fatalf("expected empty auth bearer token by default, got %q", cfg.AuthBearerToken)
	}
	if cfg.MutationRateLimitPerMinute != 60 {
		t.Fatalf("expected default mutation rate limit 60/min, got %d", cfg.MutationRateLimitPerMinute)
	}
}

func TestLoadConfigIncludesAuthBearerToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable")
	t.Setenv("AUTH_BEARER_TOKEN", "secret-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AuthBearerToken != "secret-token" {
		t.Fatalf("expected auth bearer token to be loaded, got %q", cfg.AuthBearerToken)
	}
}

func TestLoadConfigIncludesRetryBackoffSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable")
	t.Setenv("JOB_RETRY_BACKOFF_BASE", "7s")
	t.Setenv("JOB_RETRY_BACKOFF_MAX", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.JobRetryBackoffBase != 7*time.Second {
		t.Fatalf("expected retry backoff base 7s, got %s", cfg.JobRetryBackoffBase)
	}
	if cfg.JobRetryBackoffMax != 45*time.Second {
		t.Fatalf("expected retry backoff max 45s, got %s", cfg.JobRetryBackoffMax)
	}
}

func TestLoadConfigIncludesMutationRateLimitPerMinute(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable")
	t.Setenv("MUTATION_RATE_LIMIT_PER_MINUTE", "15")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.MutationRateLimitPerMinute != 15 {
		t.Fatalf("expected mutation rate limit 15/min, got %d", cfg.MutationRateLimitPerMinute)
	}
}

func TestLoadConfigIncludesDefaultAuthProjectScopes(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.AuthDefaultProjectIDs) != 1 || cfg.AuthDefaultProjectIDs[0] != 1 {
		t.Fatalf("expected default auth project scopes [1], got %+v", cfg.AuthDefaultProjectIDs)
	}
}

func TestLoadConfigIncludesAuthProjectScopesFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable")
	t.Setenv("AUTH_DEFAULT_PROJECT_IDS", "2,5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.AuthDefaultProjectIDs) != 2 || cfg.AuthDefaultProjectIDs[0] != 2 || cfg.AuthDefaultProjectIDs[1] != 5 {
		t.Fatalf("expected auth project scopes [2 5], got %+v", cfg.AuthDefaultProjectIDs)
	}
}
