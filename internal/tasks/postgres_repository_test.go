package tasks

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositoryCreateListAndGetTask(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new postgres pool: %v", err)
	}
	defer pool.Close()

	projectName := "tasks-project-" + time.Now().UTC().Format("20060102150405.000000000")
	var projectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, 'test-owner')
		returning id
	`, projectName).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	datasetName := "tasks-dataset-" + time.Now().UTC().Format("20060102150405.000000000")
	var datasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, 'platform-dev', 'train')
		returning id
	`, projectID, datasetName).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	var snapshotID int64
	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, 'v1', 'test-owner', 'task seed')
		returning id
	`, datasetID).Scan(&snapshotID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	repo := NewPostgresRepository(pool)
	lastActivityAt := time.Now().UTC().Add(-2 * time.Hour)
	task, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID:      projectID,
		DatasetID:      int64Ptr(datasetID),
		SnapshotID:     int64Ptr(snapshotID),
		Title:          "Review imported night batch",
		Description:    "Triaging the oldest pending work",
		Status:         StatusReady,
		Priority:       PriorityHigh,
		Assignee:       "reviewer-1",
		LastActivityAt: lastActivityAt,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	items, err := repo.ListProjectTasks(ctx, projectID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 || items[0].ID != task.ID {
		t.Fatalf("unexpected task list: %+v", items)
	}

	got, ok, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !ok {
		t.Fatalf("expected task to exist")
	}
	if got.Title != task.Title || got.Assignee != "reviewer-1" {
		t.Fatalf("unexpected task: %+v", got)
	}
	if got.DatasetID == nil || *got.DatasetID != datasetID {
		t.Fatalf("expected dataset id %d, got %+v", datasetID, got.DatasetID)
	}
	if got.SnapshotID == nil || *got.SnapshotID != snapshotID {
		t.Fatalf("expected snapshot id %d, got %+v", snapshotID, got.SnapshotID)
	}
}

func TestPostgresRepositoryCreateTaskRejectsSnapshotFromDifferentProject(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new postgres pool: %v", err)
	}
	defer pool.Close()

	firstProjectName := "tasks-project-a-" + time.Now().UTC().Format("20060102150405.000000000")
	var firstProjectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, 'test-owner')
		returning id
	`, firstProjectName).Scan(&firstProjectID); err != nil {
		t.Fatalf("seed first project: %v", err)
	}

	secondProjectName := "tasks-project-b-" + time.Now().UTC().Format("20060102150405.000000000")
	var secondProjectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, 'test-owner')
		returning id
	`, secondProjectName).Scan(&secondProjectID); err != nil {
		t.Fatalf("seed second project: %v", err)
	}

	datasetName := "tasks-dataset-mismatch-" + time.Now().UTC().Format("20060102150405.000000000")
	var datasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, 'platform-dev', 'train')
		returning id
	`, secondProjectID, datasetName).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	var snapshotID int64
	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, 'v1', 'test-owner', 'task mismatch')
		returning id
	`, datasetID).Scan(&snapshotID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	repo := NewPostgresRepository(pool)
	_, err = repo.CreateTask(ctx, CreateTaskInput{
		ProjectID:  firstProjectID,
		SnapshotID: int64Ptr(snapshotID),
		Title:      "Mismatched snapshot project",
		Status:     StatusReady,
		Priority:   PriorityHigh,
		Assignee:   "reviewer-1",
	})
	if err == nil || !strings.Contains(err.Error(), "project") || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("expected clear project/snapshot mismatch error, got %v", err)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
