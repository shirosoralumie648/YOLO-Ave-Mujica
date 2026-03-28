package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
	RedisAddr   string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisAddr:   os.Getenv("REDIS_ADDR"),
		S3Endpoint:  os.Getenv("S3_ENDPOINT"),
		S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey: os.Getenv("S3_SECRET_KEY"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	if cfg.RedisAddr == "" {
		cfg.RedisAddr = "localhost:6379"
	}

	return cfg, nil
}
