package artifacts

import (
	"context"
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
	Status       string            `json:"status"`
	ErrorMsg     string            `json:"error_msg,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// Service coordinates artifact creation, background building, and archive access.
type Service struct {
	repo    Repository
	query   *ExportQuery
	builder *Builder
	storage ArtifactStorage
	runner  *BuildRunner
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
	}
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

// CreatePackageJob validates the request and records a new artifact build job.
func (s *Service) CreatePackageJob(in PackageRequest) (Artifact, error) {
	if in.DatasetID <= 0 || in.SnapshotID <= 0 {
		return Artifact{}, errors.New("dataset_id and snapshot_id are required")
	}
	if in.Format == "" {
		return Artifact{}, errors.New("format is required")
	}
	if in.Version == "" {
		in.Version = fmt.Sprintf("v%d", in.SnapshotID)
	}
	if in.ProjectID <= 0 {
		in.ProjectID = 1
	}

	artifact, err := s.repo.Create(context.Background(), Artifact{
		ProjectID:    in.ProjectID,
		DatasetID:    in.DatasetID,
		SnapshotID:   in.SnapshotID,
		ArtifactType: "dataset-export",
		Format:       in.Format,
		Version:      in.Version,
		Checksum:     "pending",
		LabelMapJSON: in.LabelMapJSON,
		Status:       StatusPending,
	})
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
		return Artifact{}, fmt.Errorf("artifact %d not found", id)
	}
	return a, nil
}

// ResolveArtifact finds the ready artifact published under a format/version pair.
func (s *Service) ResolveArtifact(format, version string) (Artifact, error) {
	a, ok, err := s.repo.FindReadyByFormatVersion(context.Background(), format, version)
	if err != nil {
		return Artifact{}, err
	}
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %s@%s not found", format, version)
	}
	return a, nil
}

// PresignArtifact returns a short-lived download URL for an existing artifact.
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

// OpenArtifactArchive opens a ready artifact archive for HTTP download.
func (s *Service) OpenArtifactArchive(ctx context.Context, id int64) (ReadSeekCloser, int64, Artifact, error) {
	if s.storage == nil {
		return nil, 0, Artifact{}, fmt.Errorf("artifact storage is not configured")
	}
	artifact, err := s.GetArtifact(id)
	if err != nil {
		return nil, 0, Artifact{}, err
	}
	if artifact.Status != StatusReady {
		return nil, 0, Artifact{}, fmt.Errorf("artifact %d is not ready", id)
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
