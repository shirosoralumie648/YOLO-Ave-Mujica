package artifacts

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestExportQueryBuildsSnapshotBundleFromCanonicalAnnotations(t *testing.T) {
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

	projectID := seedArtifactProject(t, ctx, pool)
	datasetID, snapshot1, snapshot2, itemID := seedArtifactDatasetSnapshotAndItem(t, ctx, pool, projectID)
	categoryID := seedArtifactCategory(t, ctx, pool, projectID, "person")
	seedArtifactAnnotation(t, ctx, pool, datasetID, itemID, categoryID, snapshot1, nil)

	query := NewExportQuery(pool)
	bundle, err := query.LoadSnapshotBundle(ctx, datasetID, snapshot2, "v2")
	if err != nil {
		t.Fatalf("load snapshot bundle: %v", err)
	}
	if len(bundle.Categories) != 1 || bundle.Categories[0] != "person" {
		t.Fatalf("unexpected categories: %+v", bundle.Categories)
	}
	if len(bundle.Items) != 1 || len(bundle.Items[0].Boxes) != 1 {
		t.Fatalf("unexpected bundle items: %+v", bundle.Items)
	}
}

func seedArtifactProject(t *testing.T, ctx context.Context, db *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(ctx, `
		insert into projects (name, owner)
		values ('artifact-export-test', 'test-owner')
		returning id
	`).Scan(&id); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return id
}

func seedArtifactDatasetSnapshotAndItem(t *testing.T, ctx context.Context, db *pgxpool.Pool, projectID int64) (int64, int64, int64, int64) {
	t.Helper()

	var datasetID int64
	if err := db.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, 'artifact-dataset', 'platform-dev', 'train')
		returning id
	`, projectID).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	var snapshot1 int64
	if err := db.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, 'v1', 'test', 'initial')
		returning id
	`, datasetID).Scan(&snapshot1); err != nil {
		t.Fatalf("seed snapshot1: %v", err)
	}

	var snapshot2 int64
	if err := db.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, based_on_snapshot_id, created_by, note)
		values ($1, 'v2', $2, 'test', 'next')
		returning id
	`, datasetID, snapshot1).Scan(&snapshot2); err != nil {
		t.Fatalf("seed snapshot2: %v", err)
	}

	var itemID int64
	if err := db.QueryRow(ctx, `
		insert into dataset_items (dataset_id, object_key, etag)
		values ($1, 'train/a.jpg', 'etag-a')
		returning id
	`, datasetID).Scan(&itemID); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	return datasetID, snapshot1, snapshot2, itemID
}

func seedArtifactCategory(t *testing.T, ctx context.Context, db *pgxpool.Pool, projectID int64, name string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(ctx, `
		insert into categories (project_id, name)
		values ($1, $2)
		returning id
	`, projectID, name).Scan(&id); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	return id
}

func seedArtifactAnnotation(t *testing.T, ctx context.Context, db *pgxpool.Pool, datasetID, itemID, categoryID, createdSnapshotID int64, deletedSnapshotID *int64) {
	t.Helper()
	if _, err := db.Exec(ctx, `
		insert into annotations (
			dataset_id, item_id, category_id, bbox_x, bbox_y, bbox_w, bbox_h,
			created_at_snapshot_id, deleted_at_snapshot_id, review_status, is_pseudo
		)
		values ($1, $2, $3, 0.4, 0.4, 0.2, 0.2, $4, $5, 'verified', false)
	`, datasetID, itemID, categoryID, createdSnapshotID, deletedSnapshotID); err != nil {
		t.Fatalf("seed annotation: %v", err)
	}
}
