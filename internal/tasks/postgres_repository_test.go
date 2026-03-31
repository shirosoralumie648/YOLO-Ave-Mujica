package tasks

import (
	"context"
	"os"
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
	task, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID:  projectID,
		SnapshotID: snapshotID,
		Title:      "Review imported night batch",
		Kind:       KindReview,
		Status:     StatusQueued,
		Priority:   PriorityHigh,
		Assignee:   "reviewer-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	items, err := repo.ListTasks(ctx, projectID, ListTasksFilter{Assignee: "reviewer-1"})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 || items[0].ID != task.ID {
		t.Fatalf("unexpected task list: %+v", items)
	}

	got, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Title != task.Title || got.Assignee != "reviewer-1" {
		t.Fatalf("unexpected task: %+v", got)
	}
}
