package artifacts

import (
	"context"
	"os"
	"testing"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

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
