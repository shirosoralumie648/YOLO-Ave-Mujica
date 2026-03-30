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
	if _, err := pool.Exec(ctx, `insert into projects (name, owner) values ($1, 'test-owner')`, projectName); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	datasetName := "artifact-resolve-" + time.Now().UTC().Format("20060102150405.000000000")
	var datasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values (1, $1, 'platform-dev', 'train')
		returning id
	`, datasetName).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	repo := NewPostgresRepository(pool)
	created, err := repo.Create(Artifact{
		ProjectID:    1,
		DatasetID:    datasetID,
		SnapshotID:   1,
		ArtifactType: "package",
		Format:       "yolo",
		Version:      "v1",
		URI:          fmt.Sprintf("s3://artifacts/%d/1/package.yolo.tar.gz", datasetID),
		ManifestURI:  fmt.Sprintf("s3://artifacts/%d/1/manifest.json", datasetID),
		Checksum:     "pending",
		Status:       "pending",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	resolved, ok := repo.FindByDatasetFormatVersion(datasetName, "yolo", "v1")
	if !ok {
		t.Fatalf("expected artifact lookup by dataset name to succeed")
	}
	if resolved.ID != created.ID || resolved.DatasetID != datasetID {
		t.Fatalf("unexpected resolved artifact: %+v created=%+v", resolved, created)
	}
}
