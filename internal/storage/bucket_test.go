package storage

import (
	"context"
	"testing"

	"github.com/minio/minio-go/v7"
)

type fakeBucketManager struct {
	existsCalled bool
	makeCalled   bool
	exists       bool
}

func (f *fakeBucketManager) BucketExists(_ context.Context, _ string) (bool, error) {
	f.existsCalled = true
	return f.exists, nil
}

func (f *fakeBucketManager) MakeBucket(_ context.Context, _ string, _ minio.MakeBucketOptions) error {
	f.makeCalled = true
	return nil
}

func TestEnsureBucketCreatesMissingBucket(t *testing.T) {
	manager := &fakeBucketManager{exists: false}

	if err := EnsureBucket(context.Background(), manager, "platform-dev"); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	if !manager.existsCalled {
		t.Fatal("expected BucketExists to be called")
	}
	if !manager.makeCalled {
		t.Fatal("expected MakeBucket to be called for missing bucket")
	}
}
