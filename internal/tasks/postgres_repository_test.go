package tasks

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

type fakeTaskScanner struct {
	values []any
	err    error
}

func (s fakeTaskScanner) Scan(dest ...any) error {
	if s.err != nil {
		return s.err
	}
	if len(dest) != len(s.values) {
		return errors.New("unexpected scan destination count")
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *int64:
			*d = s.values[i].(int64)
		case *string:
			*d = s.values[i].(string)
		case *time.Time:
			*d = s.values[i].(time.Time)
		case *sql.NullInt64:
			*d = s.values[i].(sql.NullInt64)
		case *sql.NullTime:
			*d = s.values[i].(sql.NullTime)
		default:
			return errors.New("unsupported scan destination type")
		}
	}
	return nil
}

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
		SnapshotID: snapshotID,
		Title:      "Mismatched snapshot project",
		Kind:       KindReview,
		Status:     StatusQueued,
		Priority:   PriorityHigh,
		Assignee:   "reviewer-1",
	})
	if err == nil || !strings.Contains(err.Error(), "project") || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("expected clear project/snapshot mismatch error, got %v", err)
	}
}

func TestScanTaskHandlesNullSnapshotIDAndDueAt(t *testing.T) {
	now := time.Now().UTC()

	task, err := scanTask(fakeTaskScanner{
		values: []any{
			int64(42),
			int64(7),
			sql.NullInt64{},
			"Task without snapshot",
			KindAnnotation,
			StatusQueued,
			PriorityNormal,
			"annotator-1",
			sql.NullTime{},
			"",
			now,
			now,
			now,
		},
	})
	if err != nil {
		t.Fatalf("scan task: %v", err)
	}
	if task.SnapshotID != nil {
		t.Fatalf("expected nil snapshot id, got %v", *task.SnapshotID)
	}
	if task.DueAt != nil {
		t.Fatalf("expected nil due at, got %v", *task.DueAt)
	}
}
