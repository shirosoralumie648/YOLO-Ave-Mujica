package datahub

import (
	"context"
	"errors"
)

// PresignFunc resolves a short-lived object URL for a dataset item.
type PresignFunc func(datasetID int64, objectKey string, ttlSeconds int) (string, error)

type ScannedObject struct {
	Key    string
	ETag   string
	Size   int64
	Mime   string
	Width  int
	Height int
}

type ObjectScanner interface {
	ListObjects(bucket, prefix string) ([]ScannedObject, error)
}

// Service currently uses in-memory maps to keep MVP behavior deterministic in tests.
// The public method contracts are designed to map directly to DB-backed storage later.
type Service struct {
	repo    Repository
	presign PresignFunc
	scanner ObjectScanner
}

// Dataset is the minimal dataset record exposed by the MVP control plane.
type Dataset struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
}

// Snapshot represents a logical dataset version that downstream jobs and
// artifact exports can reference.
type Snapshot struct {
	ID              int64  `json:"id"`
	DatasetID       int64  `json:"dataset_id"`
	Version         string `json:"version"`
	BasedOnSnapshot *int64 `json:"based_on_snapshot_id,omitempty"`
	Note            string `json:"note,omitempty"`
}

// CreateDatasetInput contains the fields required to register a new dataset.
type CreateDatasetInput struct {
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
}

// CreateSnapshotInput describes optional ancestry and notes for a new snapshot.
type CreateSnapshotInput struct {
	BasedOnSnapshotID *int64 `json:"based_on_snapshot_id,omitempty"`
	Note              string `json:"note,omitempty"`
}

// PresignInput identifies which dataset object should receive a short-lived URL.
type PresignInput struct {
	DatasetID  int64  `json:"dataset_id"`
	ObjectKey  string `json:"object_key"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// DatasetItem is the stored object metadata returned by dataset listing endpoints.
type DatasetItem struct {
	ID        int64  `json:"id"`
	DatasetID int64  `json:"dataset_id"`
	ObjectKey string `json:"object_key"`
	ETag      string `json:"etag"`
}

// NewService builds the Data Hub service with the default in-memory repository.
func NewService(presign PresignFunc) *Service {
	return NewServiceWithRepository(presign, nil)
}

// NewServiceWithRepository builds the Data Hub service with an explicit repository.
func NewServiceWithRepository(presign PresignFunc, repo Repository) *Service {
	return NewServiceWithRepositoryAndScanner(presign, repo, nil)
}

func NewServiceWithRepositoryAndScanner(presign PresignFunc, repo Repository, scanner ObjectScanner) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{
		repo:    repo,
		presign: presign,
		scanner: scanner,
	}
}

// CreateDataset validates and persists a new dataset record.
func (s *Service) CreateDataset(in CreateDatasetInput) (Dataset, error) {
	if in.ProjectID <= 0 {
		return Dataset{}, errors.New("project_id must be > 0")
	}
	if in.Name == "" || in.Bucket == "" {
		return Dataset{}, errors.New("name and bucket are required")
	}
	return s.repo.CreateDataset(context.Background(), in)
}

// CreateSnapshot persists a new logical version for the given dataset.
func (s *Service) CreateSnapshot(datasetID int64, in CreateSnapshotInput) (Snapshot, error) {
	return s.repo.CreateSnapshot(context.Background(), datasetID, in)
}

// ListSnapshots returns all known snapshots for a dataset.
func (s *Service) ListSnapshots(datasetID int64) ([]Snapshot, error) {
	return s.repo.ListSnapshots(context.Background(), datasetID)
}

// ScanDataset records object keys discovered under a dataset prefix.
func (s *Service) ScanDataset(datasetID int64, objectKeys []string) (int, error) {
	if len(objectKeys) > 0 {
		return s.repo.InsertItems(context.Background(), datasetID, objectKeys)
	}
	if s.scanner != nil {
		dataset, err := s.repo.GetDataset(context.Background(), datasetID)
		if err != nil {
			return 0, err
		}
		objects, err := s.scanner.ListObjects(dataset.Bucket, dataset.Prefix)
		if err != nil {
			return 0, err
		}
		return s.repo.UpsertScannedItems(context.Background(), datasetID, objects)
	}
	return s.repo.InsertItems(context.Background(), datasetID, objectKeys)
}

// ListItems returns the stored objects associated with a dataset.
func (s *Service) ListItems(datasetID int64) ([]DatasetItem, error) {
	return s.repo.ListItems(context.Background(), datasetID)
}

// PresignObject produces a short-lived object URL for dataset consumers.
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
