package artifacts

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Repository interface {
	Create(ctx context.Context, a Artifact) (Artifact, error)
	Get(ctx context.Context, id int64) (Artifact, bool, error)
	FindReadyByFormatVersion(ctx context.Context, format, version string) (Artifact, bool, error)
	FindReadyByDatasetFormatVersion(ctx context.Context, dataset, format, version string) (Artifact, bool, error)
	LinkJob(ctx context.Context, artifactID, jobID int64) (Artifact, error)
	UpdateStatus(ctx context.Context, id int64, status string, errorMsg string) (Artifact, error)
	UpdateBuildResult(ctx context.Context, id int64, result BuildResult) (Artifact, error)
	MarkStaleBuildsFailed(ctx context.Context, reason string) (int64, error)
}

const (
	StatusPending  = "pending"
	StatusQueued   = "queued"
	StatusBuilding = "building"
	StatusReady    = "ready"
	StatusFailed   = "failed"
)

type BuildResult struct {
	Status      string
	URI         string
	ManifestURI string
	Checksum    string
	Size        int64
	ErrorMsg    string
}

type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	byID   map[int64]Artifact
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{nextID: 1, byID: make(map[int64]Artifact)}
}

func (r *InMemoryRepository) Create(_ context.Context, a Artifact) (Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a.ID = r.nextID
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	r.nextID++
	r.byID[a.ID] = a
	return a, nil
}

func (r *InMemoryRepository) Get(_ context.Context, id int64) (Artifact, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	return a, ok, nil
}

func (r *InMemoryRepository) FindReadyByFormatVersion(_ context.Context, format, version string) (Artifact, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, artifact := range r.byID {
		if artifact.Format == format && artifact.Version == version && artifact.Status == StatusReady {
			return artifact, true, nil
		}
	}
	return Artifact{}, false, nil
}

func (r *InMemoryRepository) FindReadyByDatasetFormatVersion(_ context.Context, dataset, format, version string) (Artifact, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, artifact := range r.byID {
		if artifact.Format != format || artifact.Version != version || artifact.Status != StatusReady {
			continue
		}
		if dataset != "" && dataset != fmt.Sprintf("%d", artifact.DatasetID) {
			continue
		}
		return artifact, true, nil
	}
	return Artifact{}, false, nil
}

func (r *InMemoryRepository) LinkJob(_ context.Context, artifactID, jobID int64) (Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	artifact, ok := r.byID[artifactID]
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %d not found", artifactID)
	}
	artifact.CreatedByJobID = &jobID
	r.byID[artifactID] = artifact
	return artifact, nil
}

func (r *InMemoryRepository) UpdateStatus(_ context.Context, id int64, status string, errorMsg string) (Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	artifact, ok := r.byID[id]
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %d not found", id)
	}
	artifact.Status = status
	artifact.ErrorMsg = errorMsg
	r.byID[id] = artifact
	return artifact, nil
}

func (r *InMemoryRepository) UpdateBuildResult(_ context.Context, id int64, result BuildResult) (Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	artifact, ok := r.byID[id]
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %d not found", id)
	}
	artifact.Status = result.Status
	artifact.URI = result.URI
	artifact.ManifestURI = result.ManifestURI
	artifact.Checksum = result.Checksum
	artifact.Size = result.Size
	artifact.ErrorMsg = result.ErrorMsg
	r.byID[id] = artifact
	return artifact, nil
}

func (r *InMemoryRepository) MarkStaleBuildsFailed(_ context.Context, reason string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var affected int64
	for id, artifact := range r.byID {
		if artifact.Status != StatusPending && artifact.Status != StatusQueued && artifact.Status != StatusBuilding {
			continue
		}
		artifact.Status = StatusFailed
		artifact.ErrorMsg = reason
		r.byID[id] = artifact
		affected++
	}
	return affected, nil
}

func (r *InMemoryRepository) MustGet(ctx context.Context, id int64) (Artifact, error) {
	a, ok, err := r.Get(ctx, id)
	if err != nil {
		return Artifact{}, err
	}
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %d not found", id)
	}
	return a, nil
}
