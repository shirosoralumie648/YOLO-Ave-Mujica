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
