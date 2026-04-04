package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// PackageRequest describes a dataset-export build request accepted by the API.
type PackageRequest struct {
	ProjectID    int64             `json:"project_id"`
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
}

// Artifact tracks the lifecycle and downloadable metadata of an export package.
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
	ErrorMsg     string            `json:"error_msg,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

type UploadObjectFunc func(uri string, body []byte, contentType string) (int64, error)
type PresignObjectFunc func(uri string, ttlSeconds int) (string, error)

type Service struct {
	repo    Repository
	query   *ExportQuery
	builder *Builder
	storage ArtifactStorage
	runner  *BuildRunner
	bucket  string
	upload  UploadObjectFunc
	presign PresignObjectFunc
}

// NewService builds an artifact service backed by in-memory defaults.
func NewService() *Service {
	return NewServiceWithDependencies(nil, nil, nil, nil)
}

// NewServiceWithRepository builds an artifact service with an explicit repository.
func NewServiceWithRepository(repo Repository) *Service {
	return NewServiceWithDependencies(repo, nil, nil, nil)
}

// NewServiceWithDependencies wires the artifact service with explicit build and storage dependencies.
func NewServiceWithDependencies(repo Repository, query *ExportQuery, builder *Builder, storage ArtifactStorage) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{
		repo:    repo,
		query:   query,
		builder: builder,
		storage: storage,
		bucket:  "artifacts",
	}
}

func NewServiceWithRepositoryAndStorage(repo Repository, upload UploadObjectFunc, presign PresignObjectFunc) *Service {
	return NewServiceWithRepositoryAndStorageAndBucket(repo, "artifacts", upload, presign)
}

func NewServiceWithRepositoryAndStorageAndBucket(repo Repository, bucket string, upload UploadObjectFunc, presign PresignObjectFunc) *Service {
	svc := NewServiceWithDependencies(repo, nil, nil, nil)
	if bucket != "" {
		svc.bucket = bucket
	}
	svc.upload = upload
	svc.presign = presign
	return svc
}

// StartBuildRunner enables asynchronous artifact builds inside the API process.
func (s *Service) StartBuildRunner(ctx context.Context, concurrency int) {
	s.runner = NewBuildRunner(concurrency, s.buildArtifact)
	s.runner.Start(ctx)
}

// MarkStaleBuildsFailed converts interrupted builds into a terminal failure state.
func (s *Service) MarkStaleBuildsFailed(ctx context.Context, reason string) (int64, error) {
	return s.repo.MarkStaleBuildsFailed(ctx, reason)
}

func (s *Service) CreateArtifact(in PackageRequest, status string) (Artifact, error) {
	if in.DatasetID <= 0 || in.SnapshotID <= 0 {
		return Artifact{}, errors.New("dataset_id and snapshot_id are required")
	}
	if in.Format == "" {
		return Artifact{}, errors.New("format is required")
	}
	if in.Format != "yolo" {
		return Artifact{}, fmt.Errorf("unsupported format: %s", in.Format)
	}
	if in.Version == "" {
		in.Version = fmt.Sprintf("v%d", in.SnapshotID)
	}
	if in.ProjectID <= 0 {
		in.ProjectID = 1
	}
	if status == "" {
		status = StatusPending
	}

	return s.repo.Create(context.Background(), Artifact{
		ProjectID:    in.ProjectID,
		DatasetID:    in.DatasetID,
		SnapshotID:   in.SnapshotID,
		ArtifactType: "dataset-export",
		Format:       in.Format,
		Version:      in.Version,
		URI:          fmt.Sprintf("s3://%s/artifacts/%d/%d/%s/package.%s.tar.gz", s.bucket, in.DatasetID, in.SnapshotID, in.Version, in.Format),
		ManifestURI:  fmt.Sprintf("s3://%s/artifacts/%d/%d/%s/manifest.json", s.bucket, in.DatasetID, in.SnapshotID, in.Version),
		Checksum:     "pending",
		Size:         0,
		LabelMapJSON: in.LabelMapJSON,
		Status:       status,
	})
}

// CreatePackageJob validates the request and records a new artifact build job.
func (s *Service) CreatePackageJob(in PackageRequest) (Artifact, error) {
	artifact, err := s.CreateArtifact(in, StatusPending)
	if err != nil {
		return Artifact{}, err
	}

	if s.runner != nil {
		s.runner.Enqueue(artifact.ID)
	}
	return artifact, nil
}

// GetArtifact loads a single artifact by identifier.
func (s *Service) GetArtifact(id int64) (Artifact, error) {
	a, ok, err := s.repo.Get(context.Background(), id)
	if err != nil {
		return Artifact{}, err
	}
	if !ok {
		return Artifact{}, wrapArtifactNotFound(id)
	}
	return a, nil
}

// ResolveArtifact finds the ready artifact published under a dataset/format/version tuple.
func (s *Service) ResolveArtifact(dataset, format, version string) (Artifact, error) {
	var (
		a   Artifact
		ok  bool
		err error
	)
	if dataset != "" {
		a, ok, err = s.repo.FindReadyByDatasetFormatVersion(context.Background(), dataset, format, version)
	} else {
		a, ok, err = s.repo.FindReadyByFormatVersion(context.Background(), format, version)
	}
	if err != nil {
		return Artifact{}, err
	}
	if !ok {
		if dataset != "" {
			return Artifact{}, fmt.Errorf("artifact %s/%s@%s not found", dataset, format, version)
		}
		return Artifact{}, fmt.Errorf("artifact %s@%s not found", format, version)
	}
	return a, nil
}

func (s *Service) CompleteArtifact(id int64, entries []BundleEntry) (Artifact, error) {
	artifact, err := s.GetArtifact(id)
	if err != nil {
		return Artifact{}, err
	}
	switch artifact.Status {
	case StatusReady:
		return artifact, nil
	case StatusPending, StatusQueued, StatusBuilding:
		// Continue with completion.
	case StatusFailed:
		return Artifact{}, artifactStateError{artifactID: artifact.ID, status: artifact.Status, action: "completed"}
	default:
		return Artifact{}, artifactStateError{artifactID: artifact.ID, status: artifact.Status, action: "completed"}
	}
	if len(entries) == 0 {
		return Artifact{}, errors.New("entries are required")
	}
	if s.upload == nil {
		return Artifact{}, errors.New("artifact storage upload is not configured")
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
	if err := s.MarkArtifactReady(artifact.ID, artifact.URI, artifact.ManifestURI, NormalizeSHA256Checksum(hex.EncodeToString(sum[:])), size); err != nil {
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

	_, err := s.repo.UpdateBuildResult(context.Background(), id, BuildResult{
		Status:      StatusReady,
		URI:         uri,
		ManifestURI: manifestURI,
		Checksum:    NormalizeSHA256Checksum(checksum),
		Size:        size,
	})
	return err
}

// PresignArtifact returns a short-lived download URL for an existing artifact.
func (s *Service) PresignArtifact(id int64, ttlSeconds int) (string, error) {
	a, err := s.GetArtifact(id)
	if err != nil {
		return "", err
	}
	if a.Status != StatusReady {
		return "", artifactStateError{artifactID: a.ID, status: a.Status, action: "downloaded"}
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	if s.presign != nil {
		return s.presign(a.URI, ttlSeconds)
	}
	return fmt.Sprintf("https://signed.local/artifacts/%d?ttl=%d&uri=%s", a.ID, ttlSeconds, a.URI), nil
}

// OpenArtifactArchive opens a ready artifact archive for HTTP download.
func (s *Service) OpenArtifactArchive(ctx context.Context, id int64) (ReadSeekCloser, int64, Artifact, error) {
	artifact, err := s.GetArtifact(id)
	if err != nil {
		return nil, 0, Artifact{}, err
	}
	if artifact.Status != StatusReady {
		return nil, 0, Artifact{}, artifactStateError{artifactID: artifact.ID, status: artifact.Status, action: "downloaded"}
	}
	if s.storage == nil {
		return nil, 0, Artifact{}, fmt.Errorf("artifact storage is not configured")
	}
	reader, size, err := s.storage.OpenArchive(ctx, artifact.URI)
	if err != nil {
		return nil, 0, Artifact{}, err
	}
	return reader, size, artifact, nil
}

func (s *Service) buildArtifact(ctx context.Context, artifactID int64) error {
	if s.query == nil || s.builder == nil || s.storage == nil {
		return s.failBuild(ctx, artifactID, errors.New("artifact build dependencies are not configured"))
	}

	artifact, ok, err := s.repo.Get(ctx, artifactID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("artifact %d not found", artifactID)
	}

	// Move to the building state before touching the filesystem so callers can
	// distinguish queued records from active build attempts.
	if _, err := s.repo.UpdateStatus(ctx, artifactID, StatusBuilding, ""); err != nil {
		return err
	}

	workdir, err := os.MkdirTemp("", "artifact-build-*")
	if err != nil {
		return s.failBuild(ctx, artifactID, err)
	}
	defer os.RemoveAll(workdir)

	bundle, err := s.query.LoadSnapshotBundle(ctx, artifact.DatasetID, artifact.SnapshotID, artifact.Version)
	if err != nil {
		return s.failBuild(ctx, artifactID, err)
	}
	if len(artifact.LabelMapJSON) > 0 {
		bundle.Categories = ApplyLabelMap(bundle.Categories, artifact.LabelMapJSON)
	}

	buildOut, err := s.builder.Build(ctx, workdir, bundle)
	if err != nil {
		return s.failBuild(ctx, artifactID, err)
	}

	stored, err := s.storage.StoreBuild(ctx, StoreRequest{
		Version:      artifact.Version,
		ArchivePath:  buildOut.ArchivePath,
		ManifestPath: buildOut.ManifestPath,
		PackageDir:   buildOut.RootDir,
	})
	if err != nil {
		return s.failBuild(ctx, artifactID, err)
	}

	_, err = s.repo.UpdateBuildResult(ctx, artifactID, BuildResult{
		Status:      StatusReady,
		URI:         stored.ArchiveURI,
		ManifestURI: stored.ManifestURI,
		Checksum:    buildOut.ArchiveSHA256,
		Size:        stored.ArchiveSize,
	})
	return err
}

func (s *Service) failBuild(ctx context.Context, artifactID int64, buildErr error) error {
	if buildErr == nil {
		return nil
	}
	if _, err := s.repo.UpdateBuildResult(ctx, artifactID, BuildResult{
		Status:   StatusFailed,
		ErrorMsg: buildErr.Error(),
	}); err != nil {
		return err
	}
	return buildErr
}
