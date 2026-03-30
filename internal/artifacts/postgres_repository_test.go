package artifacts

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositoryFindByDatasetFormatVersion(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	projectName := "artifacts-project-" + time.Now().UTC().Format("20060102150405.000000000")
	var projectID int64
	if err := pool.QueryRow(ctx, `insert into projects (name, owner) values ($1, 'test-owner') returning id`, projectName).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	datasetName := "artifact-resolve-" + time.Now().UTC().Format("20060102150405.000000000")
	var datasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, 'platform-dev', 'train')
		returning id
	`, projectID, datasetName).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	repo := NewPostgresRepository(pool)
	created, err := repo.Create(ctx, Artifact{
		ProjectID:    projectID,
		DatasetID:    datasetID,
		SnapshotID:   1,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v1",
		Checksum:     "pending",
		Status:       StatusPending,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	if _, ok, err := repo.FindReadyByDatasetFormatVersion(ctx, datasetName, "yolo", "v1"); err != nil || ok {
		t.Fatalf("expected pending artifact to be hidden from dataset resolve, ok=%v err=%v", ok, err)
	}

	updated, err := repo.UpdateBuildResult(ctx, created.ID, BuildResult{
		Status:      StatusReady,
		URI:         fmt.Sprintf("artifact://%s/package.yolo.tar.gz", datasetName),
		ManifestURI: fmt.Sprintf("artifact://%s/manifest.json", datasetName),
		Checksum:    "sha256:dataset-ready",
		Size:        123,
	})
	if err != nil {
		t.Fatalf("update build result: %v", err)
	}
	if updated.Status != StatusReady {
		t.Fatalf("expected ready status, got %s", updated.Status)
	}

	resolved, ok, err := repo.FindReadyByDatasetFormatVersion(ctx, datasetName, "yolo", "v1")
	if err != nil || !ok || resolved.ID != created.ID || resolved.DatasetID != datasetID {
		t.Fatalf("expected dataset-specific ready artifact, got ok=%v err=%v artifact=%+v", ok, err, resolved)
	}
}

func TestPostgresRepositoryCreateUpdateAndResolveReadyArtifact(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)
	artifact, err := repo.Create(ctx, Artifact{
		ProjectID:    1,
		DatasetID:    1,
		SnapshotID:   1,
		ArtifactType: "dataset-export",
		Format:       "yolo",
		Version:      "v-artifact-repo",
		Status:       StatusPending,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	if _, ok, err := repo.FindReadyByFormatVersion(ctx, "yolo", "v-artifact-repo"); err != nil || ok {
		t.Fatalf("expected pending artifact to be hidden from resolve, ok=%v err=%v", ok, err)
	}

	updated, err := repo.UpdateBuildResult(ctx, artifact.ID, BuildResult{
		Status:      StatusReady,
		URI:         "artifact://ready/package.yolo.tar.gz",
		ManifestURI: "artifact://ready/manifest.json",
		Checksum:    "sha256:abc123",
		Size:        123,
	})
	if err != nil {
		t.Fatalf("update build result: %v", err)
	}
	if updated.Status != StatusReady {
		t.Fatalf("expected ready status, got %s", updated.Status)
	}

	resolved, ok, err := repo.FindReadyByFormatVersion(ctx, "yolo", "v-artifact-repo")
	if err != nil || !ok || resolved.ID != artifact.ID {
		t.Fatalf("expected resolved ready artifact, got ok=%v err=%v artifact=%+v", ok, err, resolved)
	}
}

func TestInMemoryRepositoryMarksQueuedArtifactsFailedOnStartupRecovery(t *testing.T) {
	repo := NewInMemoryRepository()
	artifact, err := repo.Create(context.Background(), Artifact{
		ProjectID:  1,
		DatasetID:  1,
		SnapshotID: 1,
		Format:     "yolo",
		Version:    "v-stale",
		Status:     StatusBuilding,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	affected, err := repo.MarkStaleBuildsFailed(context.Background(), "startup_recovery")
	if err != nil {
		t.Fatalf("mark stale failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected one stale artifact to be marked failed, got %d", affected)
	}

	got, ok, err := repo.Get(context.Background(), artifact.ID)
	if err != nil || !ok || got.Status != StatusFailed {
		t.Fatalf("expected failed artifact after recovery, got ok=%v err=%v artifact=%+v", ok, err, got)
	}
}
