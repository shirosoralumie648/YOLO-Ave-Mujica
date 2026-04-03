package tasks

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositoryRoundTripCreateListGetWithSnapshotContext(t *testing.T) {
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

	ts := time.Now().UTC().UnixNano()
	projectName := fmt.Sprintf("tasks-project-%d", ts)
	otherProjectName := fmt.Sprintf("tasks-project-other-%d", ts)
	datasetName := fmt.Sprintf("tasks-dataset-%d", ts)
	snapshotVersion := "v1"

	var projectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, $2)
		returning id
	`, projectName, "integration-test").Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	var datasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id
	`, projectID, datasetName, "platform-dev", fmt.Sprintf("tasks/%d", ts)).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	var snapshotID int64
	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, $2, $3, $4)
		returning id
	`, datasetID, snapshotVersion, "integration-test", "tasks round-trip").Scan(&snapshotID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	repo := NewPostgresRepository(pool)
	created, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID:  projectID,
		SnapshotID: &snapshotID,
		Title:      fmt.Sprintf("task-%d", ts),
		Kind:       KindReview,
		Status:     StatusReady,
		Priority:   PriorityHigh,
		Assignee:   "reviewer-a",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("expected task id > 0, got %d", created.ID)
	}
	if created.LastActivityAt.IsZero() {
		t.Fatal("expected non-zero last_activity_at for snapshot task")
	}
	if created.SnapshotVersion != snapshotVersion {
		t.Fatalf("expected snapshot version %q, got %q", snapshotVersion, created.SnapshotVersion)
	}
	if created.DatasetID != datasetID {
		t.Fatalf("expected dataset_id %d, got %d", datasetID, created.DatasetID)
	}
	if created.DatasetName != datasetName {
		t.Fatalf("expected dataset_name %q, got %q", datasetName, created.DatasetName)
	}

	listed, err := repo.ListTasks(ctx, projectID, ListTasksFilter{SnapshotID: &snapshotID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 task in snapshot-filtered list, got %d", len(listed))
	}

	found := false
	for _, item := range listed {
		if item.ID == created.ID {
			found = true
			if item.SnapshotVersion != snapshotVersion {
				t.Fatalf("expected listed snapshot version %q, got %q", snapshotVersion, item.SnapshotVersion)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected created task %d in list", created.ID)
	}

	got, err := repo.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected get id %d, got %d", created.ID, got.ID)
	}
	if got.SnapshotVersion != snapshotVersion {
		t.Fatalf("expected get snapshot version %q, got %q", snapshotVersion, got.SnapshotVersion)
	}

	withoutSnapshot, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID: projectID,
		Title:     fmt.Sprintf("task-no-snapshot-%d", ts),
		Kind:      KindOps,
		Status:    StatusQueued,
		Priority:  PriorityNormal,
	})
	if err != nil {
		t.Fatalf("create task without snapshot: %v", err)
	}
	if withoutSnapshot.SnapshotID != nil {
		t.Fatalf("expected nil snapshot_id, got %+v", withoutSnapshot.SnapshotID)
	}
	if withoutSnapshot.LastActivityAt.IsZero() {
		t.Fatal("expected non-zero last_activity_at without snapshot")
	}
	if withoutSnapshot.SnapshotVersion != "" {
		t.Fatalf("expected empty snapshot_version without snapshot, got %q", withoutSnapshot.SnapshotVersion)
	}
	if withoutSnapshot.DatasetID != 0 {
		t.Fatalf("expected dataset_id=0 without snapshot, got %d", withoutSnapshot.DatasetID)
	}
	if withoutSnapshot.DatasetName != "" {
		t.Fatalf("expected empty dataset_name without snapshot, got %q", withoutSnapshot.DatasetName)
	}

	gotWithoutSnapshot, err := repo.GetTask(ctx, withoutSnapshot.ID)
	if err != nil {
		t.Fatalf("get task without snapshot: %v", err)
	}
	if gotWithoutSnapshot.SnapshotID != nil {
		t.Fatalf("expected nil snapshot_id on get without snapshot, got %+v", gotWithoutSnapshot.SnapshotID)
	}
	if gotWithoutSnapshot.SnapshotVersion != "" {
		t.Fatalf("expected empty snapshot_version on get without snapshot, got %q", gotWithoutSnapshot.SnapshotVersion)
	}

	all, err := repo.ListTasks(ctx, projectID, ListTasksFilter{})
	if err != nil {
		t.Fatalf("list all tasks: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected exactly 2 tasks for hermetic project, got %d", len(all))
	}

	var otherProjectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, $2)
		returning id
	`, otherProjectName, "integration-test").Scan(&otherProjectID); err != nil {
		t.Fatalf("seed other project: %v", err)
	}

	var otherDatasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id
	`, otherProjectID, fmt.Sprintf("tasks-dataset-other-%d", ts), "platform-dev", fmt.Sprintf("tasks-other/%d", ts)).Scan(&otherDatasetID); err != nil {
		t.Fatalf("seed other dataset: %v", err)
	}

	var otherSnapshotID int64
	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, $2, $3, $4)
		returning id
	`, otherDatasetID, "v1", "integration-test", "other project snapshot").Scan(&otherSnapshotID); err != nil {
		t.Fatalf("seed other snapshot: %v", err)
	}

	_, err = pool.Exec(ctx, `
		insert into tasks (
			project_id, snapshot_id, title, kind, status, priority, assignee, blocker_reason, last_activity_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, now())
	`, projectID, otherSnapshotID, "invalid direct insert", KindAnnotation, StatusQueued, PriorityNormal, "", "")
	if err == nil {
		t.Fatal("expected direct insert with cross-project snapshot to fail at database level")
	}
	if !strings.Contains(err.Error(), "belongs to project") {
		t.Fatalf("expected trigger error to mention project mismatch, got: %v", err)
	}

	_, err = repo.CreateTask(ctx, CreateTaskInput{
		ProjectID:  projectID,
		SnapshotID: &otherSnapshotID,
		Title:      "invalid cross-project snapshot",
		Kind:       KindAnnotation,
		Status:     StatusQueued,
		Priority:   PriorityNormal,
	})
	if err == nil {
		t.Fatal("expected cross-project snapshot create to fail")
	}
}

func TestPostgresRepositoryTransitionTaskRoundTrip(t *testing.T) {
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

	ts := time.Now().UTC().UnixNano()
	projectName := fmt.Sprintf("tasks-transition-project-%d", ts)

	var projectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, $2)
		returning id
	`, projectName, "integration-test").Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	repo := NewPostgresRepository(pool)
	created, err := repo.CreateTask(ctx, CreateTaskInput{
		ProjectID: projectID,
		Title:     "Review imported night batch",
		Kind:      KindReview,
		Status:    StatusQueued,
		Priority:  PriorityHigh,
		Assignee:  "reviewer-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	updated, err := repo.TransitionTask(ctx, created.ID, TransitionTaskInput{
		Status:        StatusBlocked,
		BlockerReason: "waiting for ontology fix",
	})
	if err != nil {
		t.Fatalf("transition task: %v", err)
	}
	if updated.Status != StatusBlocked || updated.BlockerReason != "waiting for ontology fix" {
		t.Fatalf("unexpected updated task: %+v", updated)
	}
}
