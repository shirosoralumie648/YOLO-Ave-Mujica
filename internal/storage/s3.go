package storage

import (
	"context"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"yolo-ave-mujica/internal/config"
)

func NewS3Client(cfg config.Config) (*minio.Client, error) {
	secure := false
	if u, err := url.Parse("http://" + cfg.S3Endpoint); err == nil {
		secure = u.Scheme == "https"
	}

	return minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: secure,
	})
}

func PresignGetObject(client *minio.Client, bucket string, objectKey string, ttl time.Duration) (*url.URL, error) {
	return client.PresignedGetObject(context.Background(), bucket, objectKey, ttl, nil)
}
