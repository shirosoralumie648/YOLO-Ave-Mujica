package main

import (
	"context"
	"log"
	"os"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/storage"
)

func main() {
	cfg := config.Config{
		S3Endpoint:  envOrDefault("S3_ENDPOINT", "localhost:9000"),
		S3AccessKey: envOrDefault("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey: envOrDefault("S3_SECRET_KEY", "minioadmin"),
		S3Bucket:    envOrDefault("S3_BUCKET", "platform-dev"),
	}

	client, err := storage.NewS3Client(cfg)
	if err != nil {
		log.Fatal(err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		err = storage.EnsureBucket(context.Background(), client, cfg.S3Bucket)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			log.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
