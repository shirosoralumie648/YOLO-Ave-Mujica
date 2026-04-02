package datahub

import (
	"context"
	"os"
	"testing"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositoryRoundTripDatasetScanAndSnapshots(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	cfg := config.Config{DatabaseURL: databaseURL}
	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `insert into projects (name, owner) values ('integration-project', 'test-owner')`); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	repo := NewPostgresRepository(pool)
	ds, err := repo.CreateDataset(ctx, CreateDatasetInput{
		ProjectID: 1,
		Name:      "round-trip",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	if _, err := repo.InsertItems(ctx, ds.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("insert items: %v", err)
	}

	items, err := repo.ListItems(ctx, ds.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 || items[0].ObjectKey != "train/a.jpg" {
		t.Fatalf("unexpected items: %+v", items)
	}

	snap, err := repo.CreateSnapshot(ctx, ds.ID, CreateSnapshotInput{Note: "initial"})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snap.Version != "v1" {
		t.Fatalf("expected v1 snapshot, got %s", snap.Version)
	}

	got, err := repo.GetSnapshot(ctx, snap.ID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if got.ID != snap.ID || got.DatasetID != ds.ID || got.Version != "v1" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func findDatasetSummary(items []DatasetSummary, datasetID int64) (DatasetSummary, bool) {
	for _, item := range items {
		if item.ID == datasetID {
			return item, true
		}
	}
	return DatasetSummary{}, false
}

func TestPostgresRepositoryBrowseQueries(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	cfg := config.Config{DatabaseURL: databaseURL}
	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	var projectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ('integration-browse-project', 'test-owner')
		returning id
	`).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	repo := NewPostgresRepository(pool)
	dataset, err := repo.CreateDataset(ctx, CreateDatasetInput{
		ProjectID: projectID,
		Name:      "yard-day",
		Bucket:    "platform-dev",
		Prefix:    "train/day",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	if _, err := repo.InsertItems(ctx, dataset.ID, []string{"train/day/a.jpg", "train/day/b.jpg"}); err != nil {
		t.Fatalf("insert items: %v", err)
	}

	parent, err := repo.CreateSnapshot(ctx, dataset.ID, CreateSnapshotInput{Note: "baseline"})
	if err != nil {
		t.Fatalf("create parent snapshot: %v", err)
	}
	child, err := repo.CreateSnapshot(ctx, dataset.ID, CreateSnapshotInput{
		BasedOnSnapshotID: &parent.ID,
		Note:              "relabel batch",
	})
	if err != nil {
		t.Fatalf("create child snapshot: %v", err)
	}

	item, err := repo.GetItemByObjectKey(ctx, dataset.ID, "train/day/a.jpg")
	if err != nil {
		t.Fatalf("get item by object key: %v", err)
	}
	categoryID, err := repo.EnsureCategory(ctx, projectID, "car")
	if err != nil {
		t.Fatalf("ensure category: %v", err)
	}
	if err := repo.CreateAnnotation(ctx, child.ID, dataset.ID, item.ID, item.ObjectKey, categoryID, "car", 0.1, 0.2, 0.3, 0.4); err != nil {
		t.Fatalf("create annotation: %v", err)
	}

	items, err := repo.ListDatasets(ctx, projectID)
	if err != nil {
		t.Fatalf("list datasets: %v", err)
	}
	summary, ok := findDatasetSummary(items, dataset.ID)
	if !ok {
		t.Fatalf("dataset summary not found for dataset_id=%d in %+v", dataset.ID, items)
	}
	if summary.ItemCount != 2 {
		t.Fatalf("expected item_count=2, got %d", summary.ItemCount)
	}
	if summary.SnapshotCount != 2 {
		t.Fatalf("expected snapshot_count=2, got %d", summary.SnapshotCount)
	}
	if summary.ProjectID != projectID {
		t.Fatalf("expected project_id=%d, got %d", projectID, summary.ProjectID)
	}
	if summary.LatestSnapshotID == nil || *summary.LatestSnapshotID != child.ID {
		t.Fatalf("expected latest_snapshot_id=%d, got %+v", child.ID, summary.LatestSnapshotID)
	}
	if summary.LatestSnapshotVersion != child.Version {
		t.Fatalf("expected latest_snapshot_version=%s, got %s", child.Version, summary.LatestSnapshotVersion)
	}

	detail, err := repo.GetDatasetDetail(ctx, dataset.ID)
	if err != nil {
		t.Fatalf("get dataset detail: %v", err)
	}
	if detail.ItemCount != 2 {
		t.Fatalf("expected dataset detail item_count=2, got %d", detail.ItemCount)
	}
	if detail.SnapshotCount != 2 {
		t.Fatalf("expected dataset detail snapshot_count=2, got %d", detail.SnapshotCount)
	}
	if detail.ProjectID != projectID {
		t.Fatalf("expected dataset detail project_id=%d, got %d", projectID, detail.ProjectID)
	}
	if detail.LatestSnapshotID == nil || *detail.LatestSnapshotID != child.ID {
		t.Fatalf("expected dataset detail latest_snapshot_id=%d, got %+v", child.ID, detail.LatestSnapshotID)
	}
	if detail.LatestSnapshotVersion != child.Version {
		t.Fatalf("expected dataset detail latest_snapshot_version=%s, got %s", child.Version, detail.LatestSnapshotVersion)
	}

	snapshot, err := repo.GetSnapshotDetail(ctx, child.ID)
	if err != nil {
		t.Fatalf("get snapshot detail: %v", err)
	}
	if snapshot.DatasetName != "yard-day" {
		t.Fatalf("expected dataset_name=yard-day, got %s", snapshot.DatasetName)
	}
	if snapshot.ProjectID != projectID {
		t.Fatalf("expected snapshot project_id=%d, got %d", projectID, snapshot.ProjectID)
	}
	if snapshot.BasedOnSnapshotID == nil || *snapshot.BasedOnSnapshotID != parent.ID {
		t.Fatalf("expected based_on_snapshot_id=%d, got %+v", parent.ID, snapshot.BasedOnSnapshotID)
	}
	if snapshot.Note != "relabel batch" {
		t.Fatalf("expected note=relabel batch, got %s", snapshot.Note)
	}
	if snapshot.AnnotationCount != 1 {
		t.Fatalf("expected annotation_count=1, got %d", snapshot.AnnotationCount)
	}
}
