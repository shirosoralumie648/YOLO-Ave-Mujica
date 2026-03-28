package datahub

import (
	"context"
	"errors"
)

type PresignFunc func(datasetID int64, objectKey string, ttlSeconds int) (string, error)

// Service currently uses in-memory maps to keep MVP behavior deterministic in tests.
// The public method contracts are designed to map directly to DB-backed storage later.
type Service struct {
	repo    Repository
	presign PresignFunc
}

type Dataset struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
}

type Snapshot struct {
	ID              int64  `json:"id"`
	DatasetID       int64  `json:"dataset_id"`
	Version         string `json:"version"`
	BasedOnSnapshot *int64 `json:"based_on_snapshot_id,omitempty"`
	Note            string `json:"note,omitempty"`
}

type CreateDatasetInput struct {
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
}

type CreateSnapshotInput struct {
	BasedOnSnapshotID *int64 `json:"based_on_snapshot_id,omitempty"`
	Note              string `json:"note,omitempty"`
}

type PresignInput struct {
	DatasetID  int64  `json:"dataset_id"`
	ObjectKey  string `json:"object_key"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type DatasetItem struct {
	ID        int64  `json:"id"`
	DatasetID int64  `json:"dataset_id"`
	ObjectKey string `json:"object_key"`
	ETag      string `json:"etag"`
}

func NewService(presign PresignFunc) *Service {
	return NewServiceWithRepository(presign, nil)
}

func NewServiceWithRepository(presign PresignFunc, repo Repository) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{
		repo:    repo,
		presign: presign,
	}
}

func (s *Service) CreateDataset(in CreateDatasetInput) (Dataset, error) {
	if in.ProjectID <= 0 {
		return Dataset{}, errors.New("project_id must be > 0")
	}
	if in.Name == "" || in.Bucket == "" {
		return Dataset{}, errors.New("name and bucket are required")
	}
	return s.repo.CreateDataset(context.Background(), in)
}

func (s *Service) CreateSnapshot(datasetID int64, in CreateSnapshotInput) (Snapshot, error) {
	return s.repo.CreateSnapshot(context.Background(), datasetID, in)
}

func (s *Service) ListSnapshots(datasetID int64) ([]Snapshot, error) {
	return s.repo.ListSnapshots(context.Background(), datasetID)
}

func (s *Service) ScanDataset(datasetID int64, objectKeys []string) (int, error) {
	return s.repo.InsertItems(context.Background(), datasetID, objectKeys)
}

func (s *Service) ListItems(datasetID int64) ([]DatasetItem, error) {
	return s.repo.ListItems(context.Background(), datasetID)
}

func (s *Service) PresignObject(in PresignInput) (string, error) {
	if s.presign == nil {
		return "", errors.New("presign function is not configured")
	}
	if in.DatasetID <= 0 || in.ObjectKey == "" {
		return "", errors.New("dataset_id and object_key are required")
	}
	// Keep URLs short-lived by default to avoid long-lived object exposure.
	if in.TTLSeconds <= 0 {
		in.TTLSeconds = 120
	}
	return s.presign(in.DatasetID, in.ObjectKey, in.TTLSeconds)
}
