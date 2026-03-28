package artifacts

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type PackageRequest struct {
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
}

type Artifact struct {
	ID           int64             `json:"id"`
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	URI          string            `json:"uri"`
	ManifestURI  string            `json:"manifest_uri"`
	Checksum     string            `json:"checksum"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
	Status       string            `json:"status"`
	CreatedAt    time.Time         `json:"created_at"`
}

type Service struct {
	repo          Repository
	mu            sync.Mutex
	nextJobID     int64
	jobToArtifact map[int64]int64
}

func NewService() *Service {
	return NewServiceWithRepository(nil)
}

func NewServiceWithRepository(repo Repository) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo, nextJobID: 1, jobToArtifact: make(map[int64]int64)}
}

func (s *Service) CreatePackageJob(in PackageRequest) (int64, int64, error) {
	if in.DatasetID <= 0 || in.SnapshotID <= 0 {
		return 0, 0, errors.New("dataset_id and snapshot_id are required")
	}
	if in.Format == "" {
		return 0, 0, errors.New("format is required")
	}
	if in.Version == "" {
		in.Version = fmt.Sprintf("v%d", in.SnapshotID)
	}

	a := Artifact{
		DatasetID:    in.DatasetID,
		SnapshotID:   in.SnapshotID,
		Format:       in.Format,
		Version:      in.Version,
		URI:          fmt.Sprintf("s3://artifacts/%d/%d/package.%s.tar.gz", in.DatasetID, in.SnapshotID, in.Format),
		ManifestURI:  fmt.Sprintf("s3://artifacts/%d/%d/manifest.json", in.DatasetID, in.SnapshotID),
		Checksum:     "pending",
		LabelMapJSON: in.LabelMapJSON,
		Status:       "pending",
	}
	createdArtifact, err := s.repo.Create(a)
	if err != nil {
		return 0, 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	jobID := s.nextJobID
	s.nextJobID++
	s.jobToArtifact[jobID] = createdArtifact.ID
	return jobID, createdArtifact.ID, nil
}

func (s *Service) GetArtifact(id int64) (Artifact, error) {
	a, ok := s.repo.Get(id)
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %d not found", id)
	}
	return a, nil
}

func (s *Service) ResolveArtifact(format, version string) (Artifact, error) {
	a, ok := s.repo.FindByFormatVersion(format, version)
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %s@%s not found", format, version)
	}
	return a, nil
}

func (s *Service) PresignArtifact(id int64, ttlSeconds int) (string, error) {
	a, err := s.GetArtifact(id)
	if err != nil {
		return "", err
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	return fmt.Sprintf("https://signed.local/artifacts/%d?ttl=%d&uri=%s", a.ID, ttlSeconds, a.URI), nil
}
