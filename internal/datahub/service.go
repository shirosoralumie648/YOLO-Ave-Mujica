package datahub

import (
	"errors"
	"fmt"
	"sync"
)

type PresignFunc func(datasetID int64, objectKey string, ttlSeconds int) (string, error)

type Service struct {
	mu           sync.Mutex
	presign      PresignFunc
	nextDataset  int64
	nextSnapshot int64
	datasets     map[int64]Dataset
	snapshots    map[int64][]Snapshot
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

func NewService(presign PresignFunc) *Service {
	return &Service{
		presign:      presign,
		nextDataset:  1,
		nextSnapshot: 1,
		datasets:     make(map[int64]Dataset),
		snapshots:    make(map[int64][]Snapshot),
	}
}

func (s *Service) CreateDataset(in CreateDatasetInput) (Dataset, error) {
	if in.ProjectID <= 0 {
		return Dataset{}, errors.New("project_id must be > 0")
	}
	if in.Name == "" || in.Bucket == "" {
		return Dataset{}, errors.New("name and bucket are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	d := Dataset{
		ID:        s.nextDataset,
		ProjectID: in.ProjectID,
		Name:      in.Name,
		Bucket:    in.Bucket,
		Prefix:    in.Prefix,
	}
	s.nextDataset++
	s.datasets[d.ID] = d
	return d, nil
}

func (s *Service) CreateSnapshot(datasetID int64, in CreateSnapshotInput) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.datasets[datasetID]; !ok {
		return Snapshot{}, fmt.Errorf("dataset %d not found", datasetID)
	}

	version := fmt.Sprintf("v%d", len(s.snapshots[datasetID])+1)
	snap := Snapshot{
		ID:              s.nextSnapshot,
		DatasetID:       datasetID,
		Version:         version,
		BasedOnSnapshot: in.BasedOnSnapshotID,
		Note:            in.Note,
	}
	s.nextSnapshot++
	s.snapshots[datasetID] = append(s.snapshots[datasetID], snap)
	return snap, nil
}

func (s *Service) ListSnapshots(datasetID int64) ([]Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.datasets[datasetID]; !ok {
		return nil, fmt.Errorf("dataset %d not found", datasetID)
	}

	out := make([]Snapshot, len(s.snapshots[datasetID]))
	copy(out, s.snapshots[datasetID])
	return out, nil
}

func (s *Service) PresignObject(in PresignInput) (string, error) {
	if s.presign == nil {
		return "", errors.New("presign function is not configured")
	}
	if in.DatasetID <= 0 || in.ObjectKey == "" {
		return "", errors.New("dataset_id and object_key are required")
	}
	if in.TTLSeconds <= 0 {
		in.TTLSeconds = 120
	}
	return s.presign(in.DatasetID, in.ObjectKey, in.TTLSeconds)
}
