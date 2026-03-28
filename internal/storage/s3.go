package storage

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"yolo-ave-mujica/internal/config"
)

type bucketManager interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucket(ctx context.Context, bucketName string, options minio.MakeBucketOptions) error
}

func NewS3Client(cfg config.Config) (*minio.Client, error) {
	endpoint := cfg.S3Endpoint
	secure := false
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		if u, err := url.Parse(endpoint); err == nil {
			endpoint = u.Host
			secure = u.Scheme == "https"
		}
	} else if u, err := url.Parse("http://" + endpoint); err == nil {
		secure = u.Scheme == "https"
	}

	return minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: secure,
	})
}

func EnsureBucket(ctx context.Context, client bucketManager, bucket string) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
}

func PresignGetObject(client *minio.Client, bucket string, objectKey string, ttl time.Duration) (*url.URL, error) {
	return client.PresignedGetObject(context.Background(), bucket, objectKey, ttl, nil)
}

func PresignURLString(client *minio.Client, bucket, objectKey string, ttl time.Duration) (string, error) {
	u, err := PresignGetObject(client, bucket, objectKey, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
