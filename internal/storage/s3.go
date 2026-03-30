package storage

import (
	"bytes"
	"context"
	"fmt"
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

type ObjectInfo struct {
	Key  string
	ETag string
	Size int64
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

func UploadURI(client *minio.Client, objectURI string, body []byte, contentType string) (int64, error) {
	bucket, objectKey, err := parseS3URI(objectURI)
	if err != nil {
		return 0, err
	}

	info, err := client.PutObject(
		context.Background(),
		bucket,
		objectKey,
		bytes.NewReader(body),
		int64(len(body)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func PresignURI(client *minio.Client, objectURI string, ttl time.Duration) (string, error) {
	bucket, objectKey, err := parseS3URI(objectURI)
	if err != nil {
		return "", err
	}
	return PresignURLString(client, bucket, objectKey, ttl)
}

func ListObjects(client *minio.Client, bucket, prefix string) ([]ObjectInfo, error) {
	ctx := context.Background()
	items := []ObjectInfo{}
	for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if object.Err != nil {
			return nil, object.Err
		}
		items = append(items, ObjectInfo{
			Key:  object.Key,
			ETag: object.ETag,
			Size: object.Size,
		})
	}
	return items, nil
}

func parseS3URI(objectURI string) (string, string, error) {
	u, err := url.Parse(objectURI)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "s3" {
		return "", "", fmt.Errorf("unsupported object uri: %s", objectURI)
	}
	bucket := u.Host
	objectKey := strings.TrimPrefix(u.Path, "/")
	if bucket == "" || objectKey == "" {
		return "", "", fmt.Errorf("invalid object uri: %s", objectURI)
	}
	return bucket, objectKey, nil
}
