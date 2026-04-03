package review

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
	"github.com/jackc/pgx/v5/pgxpool"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositoryListPublishableCandidatesBySnapshot(t *testing.T) {
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

	mustApplyPublishMigration(t, ctx, pool)
	projectID, snapshotID, acceptedID := seedPublishableCandidateFixture(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	items, err := repo.ListPublishableCandidates(projectID)
	if err != nil {
		t.Fatalf("list publishable candidates: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected exactly 1 accepted publishable candidate, got %d: %+v", len(items), items)
	}
	if items[0].ID != acceptedID {
		t.Fatalf("expected candidate_id=%d, got %+v", acceptedID, items[0])
	}
	if items[0].SnapshotID != snapshotID {
		t.Fatalf("expected snapshot_id=%d, got %+v", snapshotID, items[0])
	}
	if items[0].ReviewStatus != "accepted" {
		t.Fatalf("expected accepted review status, got %+v", items[0])
	}
	if items[0].Summary["snapshot_version"] == "" {
		t.Fatalf("expected snapshot summary metadata, got %+v", items[0].Summary)
	}
}

func TestPostgresRepositoryListPendingIncludesLegacyAndQueuedCandidates(t *testing.T) {
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

	mustApplyPublishMigration(t, ctx, pool)
	_, _, _, pendingCandidateID, queuedCandidateID := seedReviewQueueFixture(t, ctx, pool)

	repo := NewPostgresRepository(pool)
	items, err := repo.ListPending()
	if err != nil {
		t.Fatalf("list pending review candidates: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 queued candidates, got %d: %+v", len(items), items)
	}
	if items[0].ID != pendingCandidateID || items[1].ID != queuedCandidateID {
		t.Fatalf("expected pending+queued fixture candidates, got %+v", items)
	}
	for _, item := range items {
		if item.Status != "queued_for_review" || item.ReviewStatus != "queued_for_review" {
			t.Fatalf("expected normalized queued status, got %+v", item)
		}
		if item.Source.ModelName != "detector-a" || !item.Source.IsPseudo {
			t.Fatalf("expected source metadata, got %+v", item.Source)
		}
		if item.Source.Confidence == nil {
			t.Fatalf("expected source confidence, got %+v", item.Source)
		}
	}
}

func seedPublishableCandidateFixture(t *testing.T, ctx context.Context, pool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}) (projectID, snapshotID, acceptedCandidateID int64) {
	t.Helper()

	ts := time.Now().UTC().UnixNano()

	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, $2)
		returning id
	`, fmt.Sprintf("review-project-%d", ts), "integration-test").Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	var datasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id
	`, projectID, fmt.Sprintf("review-dataset-%d", ts), "platform-dev", fmt.Sprintf("review/%d", ts)).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, $2, $3, $4)
		returning id
	`, datasetID, fmt.Sprintf("v%d", ts), "integration-test", "review fixture").Scan(&snapshotID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	var itemID int64
	if err := pool.QueryRow(ctx, `
		insert into dataset_items (dataset_id, object_key, mime)
		values ($1, $2, $3)
		returning id
	`, datasetID, fmt.Sprintf("images/%d.jpg", ts), "image/jpeg").Scan(&itemID); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	var categoryID int64
	if err := pool.QueryRow(ctx, `
		insert into categories (project_id, name)
		values ($1, $2)
		returning id
	`, projectID, fmt.Sprintf("truck-%d", ts)).Scan(&categoryID); err != nil {
		t.Fatalf("seed category: %v", err)
	}

	var taskID int64
	if err := pool.QueryRow(ctx, `
		insert into tasks (
			project_id, snapshot_id, title, kind, status, priority, assignee, blocker_reason, last_activity_at
		)
		values ($1, $2, $3, 'review', 'ready', 'high', 'reviewer-a', '', now())
		returning id
	`, projectID, snapshotID, fmt.Sprintf("review-task-%d", ts)).Scan(&taskID); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	_ = taskID

	reviewedAt := time.Now().UTC().Truncate(time.Hour)
	if err := pool.QueryRow(ctx, `
		insert into annotation_candidates (
			dataset_id, snapshot_id, item_id, category_id,
			bbox_x, bbox_y, bbox_w, bbox_h,
			confidence, model_name, is_pseudo, review_status, reviewer_id, reviewed_at
		)
		values ($1, $2, $3, $4, 10, 20, 30, 40, 0.88, 'detector-a', true, 'accepted', 'reviewer-a', $5)
		returning id
	`, datasetID, snapshotID, itemID, categoryID, reviewedAt).Scan(&acceptedCandidateID); err != nil {
		t.Fatalf("seed accepted candidate: %v", err)
	}

	var pendingCandidateID int64
	if err := pool.QueryRow(ctx, `
		insert into annotation_candidates (
			dataset_id, snapshot_id, item_id, category_id,
			bbox_x, bbox_y, bbox_w, bbox_h,
			confidence, model_name, is_pseudo, review_status
		)
		values ($1, $2, $3, $4, 11, 21, 31, 41, 0.52, 'detector-a', true, 'pending')
		returning id
	`, datasetID, snapshotID, itemID, categoryID).Scan(&pendingCandidateID); err != nil {
		t.Fatalf("seed pending candidate: %v", err)
	}
	_ = pendingCandidateID

	return projectID, snapshotID, acceptedCandidateID
}

func seedReviewQueueFixture(t *testing.T, ctx context.Context, pool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}) (projectID, snapshotID, itemID, pendingCandidateID, queuedCandidateID int64) {
	t.Helper()

	ts := time.Now().UTC().UnixNano()

	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, $2)
		returning id
	`, fmt.Sprintf("queue-project-%d", ts), "integration-test").Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	var datasetID int64
	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id
	`, projectID, fmt.Sprintf("queue-dataset-%d", ts), "platform-dev", fmt.Sprintf("queue/%d", ts)).Scan(&datasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, $2, $3, $4)
		returning id
	`, datasetID, fmt.Sprintf("v%d", ts), "integration-test", "review queue fixture").Scan(&snapshotID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into dataset_items (dataset_id, object_key, mime)
		values ($1, $2, $3)
		returning id
	`, datasetID, fmt.Sprintf("images/%d.jpg", ts), "image/jpeg").Scan(&itemID); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	var categoryID int64
	if err := pool.QueryRow(ctx, `
		insert into categories (project_id, name)
		values ($1, $2)
		returning id
	`, projectID, fmt.Sprintf("queue-category-%d", ts)).Scan(&categoryID); err != nil {
		t.Fatalf("seed category: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into annotation_candidates (
			dataset_id, snapshot_id, item_id, category_id,
			bbox_x, bbox_y, bbox_w, bbox_h,
			confidence, model_name, is_pseudo, review_status
		)
		values ($1, $2, $3, $4, 10, 20, 30, 40, 0.88, 'detector-a', true, 'pending')
		returning id
	`, datasetID, snapshotID, itemID, categoryID).Scan(&pendingCandidateID); err != nil {
		t.Fatalf("seed pending candidate: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into annotation_candidates (
			dataset_id, snapshot_id, item_id, category_id,
			bbox_x, bbox_y, bbox_w, bbox_h,
			confidence, model_name, is_pseudo, review_status
		)
		values ($1, $2, $3, $4, 11, 21, 31, 41, 0.52, 'detector-a', true, 'queued_for_review')
		returning id
	`, datasetID, snapshotID, itemID, categoryID).Scan(&queuedCandidateID); err != nil {
		t.Fatalf("seed queued candidate: %v", err)
	}

	return projectID, snapshotID, itemID, pendingCandidateID, queuedCandidateID
}

func mustApplyPublishMigration(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}) {
	t.Helper()

	down := mustReadMigrationFile(t, "000004_publish_gate_review_workspace.down.sql")
	if strings.TrimSpace(down) != "" {
		if _, err := pool.Exec(ctx, down); err != nil && !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("apply publish down migration: %v", err)
		}
	}

	up := mustReadMigrationFile(t, "000004_publish_gate_review_workspace.up.sql")
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("apply publish up migration: %v", err)
	}
}

func mustReadMigrationFile(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join("..", "..", "migrations", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(body)
}

var _ interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
} = (*pgxpool.Pool)(nil)
