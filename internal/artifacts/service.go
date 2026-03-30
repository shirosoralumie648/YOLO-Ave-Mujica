package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	ProjectID    int64             `json:"project_id"`
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	ArtifactType string            `json:"artifact_type"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	URI          string            `json:"uri"`
	ManifestURI  string            `json:"manifest_uri"`
	Checksum     string            `json:"checksum"`
	Size         int64             `json:"size"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
	Entries      []BundleEntry     `json:"entries,omitempty"`
	Status       string            `json:"status"`
	CreatedAt    time.Time         `json:"created_at"`
}

type UploadObjectFunc func(uri string, body []byte, contentType string) (int64, error)
type PresignObjectFunc func(uri string, ttlSeconds int) (string, error)

type Service struct {
	repo          Repository
	bucket        string
	upload        UploadObjectFunc
	presign       PresignObjectFunc
	mu            sync.Mutex
	nextJobID     int64
	jobToArtifact map[int64]int64
}

func NewService() *Service {
	return NewServiceWithRepositoryAndStorageAndBucket(nil, "artifacts", nil, nil)
}

func NewServiceWithRepository(repo Repository) *Service {
	return NewServiceWithRepositoryAndStorageAndBucket(repo, "artifacts", nil, nil)
}

func NewServiceWithRepositoryAndStorage(repo Repository, upload UploadObjectFunc, presign PresignObjectFunc) *Service {
	return NewServiceWithRepositoryAndStorageAndBucket(repo, "artifacts", upload, presign)
}

func NewServiceWithRepositoryAndStorageAndBucket(repo Repository, bucket string, upload UploadObjectFunc, presign PresignObjectFunc) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	if bucket == "" {
		bucket = "artifacts"
	}
	return &Service{
		repo:          repo,
		bucket:        bucket,
		upload:        upload,
		presign:       presign,
		nextJobID:     1,
		jobToArtifact: make(map[int64]int64),
	}
}

func (s *Service) CreateArtifact(in PackageRequest, status string) (Artifact, error) {
	if in.DatasetID <= 0 || in.SnapshotID <= 0 {
		return Artifact{}, errors.New("dataset_id and snapshot_id are required")
	}
	if in.Format == "" {
		return Artifact{}, errors.New("format is required")
	}
	if in.Version == "" {
		in.Version = fmt.Sprintf("v%d", in.SnapshotID)
	}
	if status == "" {
		status = "pending"
	}

	a := Artifact{
		ProjectID:    1,
		DatasetID:    in.DatasetID,
		SnapshotID:   in.SnapshotID,
		ArtifactType: "package",
		Format:       in.Format,
		Version:      in.Version,
		URI:          fmt.Sprintf("s3://%s/artifacts/%d/%d/%s/package.json", s.bucket, in.DatasetID, in.SnapshotID, in.Version),
		ManifestURI:  fmt.Sprintf("s3://%s/artifacts/%d/%d/%s/manifest.json", s.bucket, in.DatasetID, in.SnapshotID, in.Version),
		Checksum:     "pending",
		Size:         0,
		LabelMapJSON: in.LabelMapJSON,
		Status:       status,
	}
	return s.repo.Create(a)
}

func (s *Service) CreatePackageJob(in PackageRequest) (int64, int64, error) {
	createdArtifact, err := s.CreateArtifact(in, "pending")
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

func (s *Service) ResolveArtifact(dataset, format, version string) (Artifact, error) {
	a, ok := s.repo.FindByDatasetFormatVersion(dataset, format, version)
	if !ok {
		if dataset != "" {
			return Artifact{}, fmt.Errorf("artifact %s/%s@%s not found", dataset, format, version)
		}
		return Artifact{}, fmt.Errorf("artifact %s@%s not found", format, version)
	}
	return a, nil
}

func (s *Service) CompleteArtifact(id int64, entries []BundleEntry) (Artifact, error) {
	if len(entries) == 0 {
		return Artifact{}, errors.New("entries are required")
	}
	if s.upload == nil {
		return Artifact{}, errors.New("artifact storage upload is not configured")
	}

	artifact, err := s.GetArtifact(id)
	if err != nil {
		return Artifact{}, err
	}

	pulled := struct {
		ArtifactID int64         `json:"artifact_id"`
		Version    string        `json:"version"`
		Entries    []BundleEntry `json:"entries"`
	}{
		ArtifactID: artifact.ID,
		Version:    artifact.Version,
		Entries:    entries,
	}
	packageBody, err := json.MarshalIndent(pulled, "", "  ")
	if err != nil {
		return Artifact{}, err
	}

	manifestEntries := make([]ManifestEntry, 0, len(entries))
	for _, entry := range entries {
		manifestEntries = append(manifestEntries, ManifestEntry{
			Path:     entry.Path,
			Checksum: entry.Checksum,
		})
	}
	manifestBody, err := BuildManifest(artifact.Version, manifestEntries)
	if err != nil {
		return Artifact{}, err
	}

	size, err := s.upload(artifact.URI, packageBody, "application/json")
	if err != nil {
		return Artifact{}, err
	}
	if _, err := s.upload(artifact.ManifestURI, manifestBody, "application/json"); err != nil {
		return Artifact{}, err
	}

	sum := sha256.Sum256(packageBody)
	if err := s.MarkArtifactReady(artifact.ID, artifact.URI, artifact.ManifestURI, hex.EncodeToString(sum[:]), size); err != nil {
		return Artifact{}, err
	}
	return s.GetArtifact(artifact.ID)
}

func (s *Service) MarkArtifactReady(id int64, uri, manifestURI, checksum string, size int64) error {
	if uri == "" {
		return errors.New("uri is required")
	}
	if manifestURI == "" {
		return errors.New("manifest_uri is required")
	}
	if checksum == "" {
		return errors.New("checksum is required")
	}
	if size < 0 {
		return errors.New("size must be >= 0")
	}
	return s.repo.UpdateReady(id, uri, manifestURI, checksum, size)
}

func (s *Service) PresignArtifact(id int64, ttlSeconds int) (string, error) {
	a, err := s.GetArtifact(id)
	if err != nil {
		return "", err
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	if a.Status == "ready" && s.presign != nil {
		return s.presign(a.URI, ttlSeconds)
	}
	return fmt.Sprintf("https://signed.local/artifacts/%d?ttl=%d&uri=%s", a.ID, ttlSeconds, a.URI), nil
}
