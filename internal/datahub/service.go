package datahub

import (
	"context"
	"errors"
)

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

type ImportedAnnotation struct {
	ObjectKey    string
	CategoryName string
	BBoxX        float64
	BBoxY        float64
	BBoxW        float64
	BBoxH        float64
}

type ImportSnapshotInput struct {
	Format    string
	SourceURI string
	Entries   []ImportedAnnotation
}

type ImportSnapshotResult struct {
	DatasetID           int64
	SnapshotID          int64
	ImportedAnnotations int
}

func NewService(presign PresignFunc) *Service {
	return NewServiceWithRepository(presign, nil)
}

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

func (s *Service) GetSnapshot(snapshotID int64) (Snapshot, error) {
	return s.repo.GetSnapshot(context.Background(), snapshotID)
}

func (s *Service) ListSnapshots(datasetID int64) ([]Snapshot, error) {
	return s.repo.ListSnapshots(context.Background(), datasetID)
}

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

func (s *Service) ListItems(datasetID int64) ([]DatasetItem, error) {
	return s.repo.ListItems(context.Background(), datasetID)
}

func (s *Service) ImportSnapshot(snapshotID int64, in ImportSnapshotInput) (ImportSnapshotResult, error) {
	if in.Format == "" {
		return ImportSnapshotResult{}, errors.New("format is required")
	}
	if len(in.Entries) == 0 {
		return ImportSnapshotResult{}, errors.New("entries are required")
	}

	snapshot, err := s.repo.GetSnapshot(context.Background(), snapshotID)
	if err != nil {
		return ImportSnapshotResult{}, err
	}
	dataset, err := s.repo.GetDataset(context.Background(), snapshot.DatasetID)
	if err != nil {
		return ImportSnapshotResult{}, err
	}

	imported := 0
	for _, entry := range in.Entries {
		if entry.ObjectKey == "" || entry.CategoryName == "" {
			return ImportSnapshotResult{}, errors.New("object_key and category_name are required")
		}
		if entry.BBoxW <= 0 || entry.BBoxH <= 0 {
			return ImportSnapshotResult{}, errors.New("bbox_w and bbox_h must be > 0")
		}

		item, err := s.repo.GetItemByObjectKey(context.Background(), dataset.ID, entry.ObjectKey)
		if err != nil {
			return ImportSnapshotResult{}, err
		}
		categoryID, err := s.repo.EnsureCategory(context.Background(), dataset.ProjectID, entry.CategoryName)
		if err != nil {
			return ImportSnapshotResult{}, err
		}
		if err := s.repo.CreateAnnotation(context.Background(), snapshot.ID, dataset.ID, item.ID, item.ObjectKey, categoryID, entry.CategoryName, entry.BBoxX, entry.BBoxY, entry.BBoxW, entry.BBoxH); err != nil {
			return ImportSnapshotResult{}, err
		}
		imported++
	}

	return ImportSnapshotResult{
		DatasetID:           dataset.ID,
		SnapshotID:          snapshot.ID,
		ImportedAnnotations: imported,
	}, nil
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
