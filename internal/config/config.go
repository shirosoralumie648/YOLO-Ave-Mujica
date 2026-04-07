package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                   string
	AuthBearerToken            string
	AuthDefaultProjectIDs      []int64
	MutationRateLimitPerMinute int
	DatabaseURL                string
	RedisAddr                  string
	S3Endpoint                 string
	S3AccessKey                string
	S3SecretKey                string
	S3Bucket                   string
	APIBaseURL                 string
	ArtifactStorageDir         string
	ArtifactBuildConcurrency   int
	ShutdownTimeout            time.Duration
	SweeperInterval            time.Duration
	LeaseSeconds               int
	JobRetryBackoffBase        time.Duration
	JobRetryBackoffMax         time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:                   envOrDefault("HTTP_ADDR", ":8080"),
		AuthBearerToken:            os.Getenv("AUTH_BEARER_TOKEN"),
		AuthDefaultProjectIDs:      int64SliceEnvOrDefault("AUTH_DEFAULT_PROJECT_IDS", []int64{1}),
		MutationRateLimitPerMinute: intOrDefault("MUTATION_RATE_LIMIT_PER_MINUTE", 60),
		DatabaseURL:                os.Getenv("DATABASE_URL"),
		RedisAddr:                  envOrDefault("REDIS_ADDR", "localhost:6379"),
		S3Endpoint:                 os.Getenv("S3_ENDPOINT"),
		S3AccessKey:                os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:                os.Getenv("S3_SECRET_KEY"),
		S3Bucket:                   os.Getenv("S3_BUCKET"),
		APIBaseURL:                 envOrDefault("API_BASE_URL", "http://localhost:8080"),
		ArtifactStorageDir:         envOrDefault("ARTIFACT_STORAGE_DIR", filepath.Join(os.TempDir(), "platform-artifacts")),
		ArtifactBuildConcurrency:   intOrDefault("ARTIFACT_BUILD_CONCURRENCY", 2),
		ShutdownTimeout:            durationOrDefault("SHUTDOWN_TIMEOUT", 10*time.Second),
		SweeperInterval:            durationOrDefault("SWEEPER_INTERVAL", 5*time.Second),
		LeaseSeconds:               intOrDefault("LEASE_SECONDS", 30),
		JobRetryBackoffBase:        durationOrDefault("JOB_RETRY_BACKOFF_BASE", 5*time.Second),
		JobRetryBackoffMax:         durationOrDefault("JOB_RETRY_BACKOFF_MAX", time.Minute),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func intOrDefault(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func int64SliceEnvOrDefault(key string, fallback []int64) []int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return append([]int64(nil), fallback...)
	}
	parts := strings.Split(value, ",")
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || parsed <= 0 {
			continue
		}
		out = append(out, parsed)
	}
	if len(out) == 0 {
		return append([]int64(nil), fallback...)
	}
	return out
}
