package annotations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositorySaveDraftIncrementsRevisionAndSubmitMarksSubmitted(t *testing.T) {
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

	mustApplyAnnotationMigrations(t, ctx, pool, true)
	fixture := seedAnnotationFixture(t, ctx, pool)
	repo := NewPostgresRepository(pool)

	if _, err := repo.SaveDraft(ctx, SaveDraftInput{
		TaskID:          fixture.TaskID,
		SnapshotID:      fixture.SnapshotID + 1,
		AssetObjectKey:  fixture.ObjectKey,
		OntologyVersion: "v1",
		Body:            map[string]any{"objects": []any{}},
	}); err == nil {
		t.Fatal("expected initial save with mismatched task context to fail")
	}

	first, err := repo.SaveDraft(ctx, SaveDraftInput{
		TaskID:          fixture.TaskID,
		SnapshotID:      fixture.SnapshotID,
		AssetObjectKey:  fixture.ObjectKey,
		OntologyVersion: "v1",
		Body: map[string]any{
			"objects": []map[string]any{{"id": "box-1", "label": "person"}},
		},
	})
	if err != nil {
		t.Fatalf("save draft #1: %v", err)
	}

	second, err := repo.SaveDraft(ctx, SaveDraftInput{
		TaskID:          fixture.TaskID,
		SnapshotID:      fixture.SnapshotID,
		AssetObjectKey:  fixture.ObjectKey,
		OntologyVersion: "v1",
		BaseRevision:    first.Revision,
		Body: map[string]any{
			"objects": []map[string]any{{"id": "box-1", "label": "person"}, {"id": "box-2", "label": "car"}},
		},
	})
	if err != nil {
		t.Fatalf("save draft #2: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same row id, got %d and %d", first.ID, second.ID)
	}
	if second.Revision != first.Revision+1 {
		t.Fatalf("expected revision increment, got %d -> %d", first.Revision, second.Revision)
	}

	if _, err := repo.SaveDraft(ctx, SaveDraftInput{
		TaskID:          fixture.TaskID,
		SnapshotID:      fixture.SnapshotID + 1,
		AssetObjectKey:  fixture.ObjectKey,
		OntologyVersion: "v1",
		BaseRevision:    second.Revision,
		Body:            map[string]any{"objects": []any{}},
	}); err == nil {
		t.Fatal("expected save with changed context to fail")
	}

	submitted, err := repo.Submit(ctx, fixture.TaskID, "annotator-1")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if submitted.State != StateSubmitted {
		t.Fatalf("expected state %q, got %q", StateSubmitted, submitted.State)
	}
	if submitted.SubmittedBy != "annotator-1" {
		t.Fatalf("expected submitted_by annotator-1, got %q", submitted.SubmittedBy)
	}
	if submitted.SubmittedAt == nil {
		t.Fatal("expected submitted_at to be set")
	}
	if submitted.Revision != second.Revision+1 {
		t.Fatalf("expected submit to increment revision, got %d -> %d", second.Revision, submitted.Revision)
	}

	submittedAgain, err := repo.Submit(ctx, fixture.TaskID, "annotator-2")
	if err != nil {
		t.Fatalf("submit again: %v", err)
	}
	if submittedAgain.Revision != submitted.Revision {
		t.Fatalf("expected idempotent submit revision, got %d -> %d", submitted.Revision, submittedAgain.Revision)
	}
	if submittedAgain.SubmittedBy != submitted.SubmittedBy {
		t.Fatalf("expected submitted_by unchanged, got %q -> %q", submitted.SubmittedBy, submittedAgain.SubmittedBy)
	}
	if submitted.SubmittedAt == nil || submittedAgain.SubmittedAt == nil {
		t.Fatal("expected submitted_at to be set")
	}
	if !submitted.SubmittedAt.Equal(*submittedAgain.SubmittedAt) {
		t.Fatalf("expected submitted_at unchanged, got %s -> %s", submitted.SubmittedAt.Format(time.RFC3339Nano), submittedAgain.SubmittedAt.Format(time.RFC3339Nano))
	}

	if _, err := repo.SaveDraft(ctx, SaveDraftInput{
		TaskID:          fixture.TaskID,
		SnapshotID:      fixture.SnapshotID,
		AssetObjectKey:  fixture.ObjectKey,
		OntologyVersion: "v1",
		BaseRevision:    submitted.Revision,
		Body:            map[string]any{"objects": []any{}},
	}); err == nil {
		t.Fatal("expected save after submit to fail")
	}
}

type annotationFixture struct {
	ProjectID  int64
	DatasetID  int64
	SnapshotID int64
	TaskID     int64
	ObjectKey  string
}

func seedAnnotationFixture(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}) annotationFixture {
	t.Helper()

	ts := time.Now().UTC().UnixNano()
	fixture := annotationFixture{
		ObjectKey: fmt.Sprintf("images/%d.jpg", ts),
	}

	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, $2)
		returning id
	`, fmt.Sprintf("annotations-project-%d", ts), "integration-test").Scan(&fixture.ProjectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id
	`, fixture.ProjectID, fmt.Sprintf("annotations-dataset-%d", ts), "platform-dev", fmt.Sprintf("annotations/%d", ts)).Scan(&fixture.DatasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, $2, $3, $4)
		returning id
	`, fixture.DatasetID, fmt.Sprintf("v%d", ts), "integration-test", "annotation fixture").Scan(&fixture.SnapshotID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into tasks (
			project_id, snapshot_id, title, kind, status, priority, assignee,
			asset_object_key, media_kind, ontology_version, blocker_reason, last_activity_at
		)
		values ($1, $2, $3, 'annotation', 'in_progress', 'high', 'annotator-1', $4, 'image', 'v1', '', now())
		returning id
	`, fixture.ProjectID, fixture.SnapshotID, fmt.Sprintf("annotate-%d", ts), fixture.ObjectKey).Scan(&fixture.TaskID); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	return fixture
}

func mustApplyAnnotationMigrations(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, reset bool) {
	t.Helper()

	if reset {
		mustApplyMigrationDown(t, ctx, pool, "000006_annotation_draft_persistence.down.sql")
		mustApplyMigrationDown(t, ctx, pool, "000005_annotation_workspace_foundation.down.sql")
	}

	mustApplyMigrationUp(t, ctx, pool, "000005_annotation_workspace_foundation.up.sql")
	mustApplyMigrationUp(t, ctx, pool, "000006_annotation_draft_persistence.up.sql")
}

func mustApplyMigrationUp(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, name string) {
	t.Helper()

	up := mustReadAnnotationMigrationFile(t, name)
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("apply migration up %s: %v", name, err)
	}
}

func mustApplyMigrationDown(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, name string) {
	t.Helper()

	down := mustReadAnnotationMigrationFile(t, name)
	if strings.TrimSpace(down) == "" {
		return
	}
	if _, err := pool.Exec(ctx, down); err != nil && !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("apply migration down %s: %v", name, err)
	}
}

func mustReadAnnotationMigrationFile(t *testing.T, name string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(body)
}
