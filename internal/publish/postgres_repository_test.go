package publish

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

func TestPostgresRepositoryCreateBatchAndItems(t *testing.T) {
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

	mustApplyPublishMigration(t, ctx, pool, true)

	fixture := seedPublishFixture(t, ctx, pool, time.Now().UTC().Truncate(time.Hour))
	repo := NewPostgresRepository(pool)

	batch, err := repo.CreateBatch(ctx, CreateBatchInput{
		ProjectID:  fixture.ProjectID,
		SnapshotID: fixture.SnapshotID,
		Source:     SourceSuggested,
		RuleSummary: map[string]any{
			"grouping": "risk-window",
		},
		Items: []CreateBatchItemInput{
			{
				CandidateID: fixture.AcceptedCandidateIDs[0],
				TaskID:      fixture.TaskID,
				DatasetID:   fixture.DatasetID,
				SnapshotID:  fixture.SnapshotID,
				ItemPayload: map[string]any{
					"task":     map[string]any{"id": fixture.TaskID, "title": "review lane 4"},
					"snapshot": map[string]any{"id": fixture.SnapshotID, "version": fixture.SnapshotVersion},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if batch.Status != StatusDraft {
		t.Fatalf("expected draft status, got %s", batch.Status)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("expected 1 batch item, got %d", len(batch.Items))
	}
	if batch.Items[0].CandidateID != fixture.AcceptedCandidateIDs[0] {
		t.Fatalf("expected candidate_id=%d, got %d", fixture.AcceptedCandidateIDs[0], batch.Items[0].CandidateID)
	}
	if batch.Items[0].ItemPayload["task"] == nil {
		t.Fatalf("expected frozen task payload, got %+v", batch.Items[0].ItemPayload)
	}
}

func TestPostgresRepositoryOwnerEditInvalidatesReviewApproval(t *testing.T) {
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

	mustApplyPublishMigration(t, ctx, pool, true)

	fixture := seedPublishFixture(t, ctx, pool, time.Now().UTC().Truncate(time.Hour))
	repo := NewPostgresRepository(pool)

	batch, err := repo.CreateBatch(ctx, CreateBatchInput{
		ProjectID:  fixture.ProjectID,
		SnapshotID: fixture.SnapshotID,
		Source:     SourceManual,
		Items: []CreateBatchItemInput{{
			CandidateID: fixture.AcceptedCandidateIDs[0],
			TaskID:      fixture.TaskID,
			DatasetID:   fixture.DatasetID,
			SnapshotID:  fixture.SnapshotID,
			ItemPayload: map[string]any{"task": map[string]any{"id": fixture.TaskID}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := repo.ApplyReviewDecision(ctx, batch.ID, ReviewDecisionApprove, "reviewer-1", nil); err != nil {
		t.Fatalf("review approve: %v", err)
	}

	updated, err := repo.ReplaceBatchItems(ctx, batch.ID, ReplaceBatchItemsInput{
		Actor: "owner-1",
		Items: []CreateBatchItemInput{{
			CandidateID: fixture.AcceptedCandidateIDs[1],
			TaskID:      fixture.TaskID,
			DatasetID:   fixture.DatasetID,
			SnapshotID:  fixture.SnapshotID,
			ItemPayload: map[string]any{"task": map[string]any{"id": fixture.TaskID}},
		}},
	})
	if err != nil {
		t.Fatalf("replace batch items: %v", err)
	}
	if updated.Status != StatusOwnerChangesRequested {
		t.Fatalf("expected owner_changes_requested, got %s", updated.Status)
	}
	if updated.ReviewApprovedBy != "" {
		t.Fatalf("expected review approval cleared, got %q", updated.ReviewApprovedBy)
	}
	if updated.ReviewApprovedAt != nil {
		t.Fatalf("expected review approval timestamp cleared, got %+v", updated.ReviewApprovedAt)
	}
}

func TestPostgresRepositoryListSuggestedCandidatesGroupsByRules(t *testing.T) {
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

	mustApplyPublishMigration(t, ctx, pool, true)

	acceptedAt := time.Now().UTC().Truncate(time.Hour)
	fixture := seedPublishFixture(t, ctx, pool, acceptedAt)
	otherModelCandidateID := seedAcceptedCandidate(t, ctx, pool, fixture, "detector-b", acceptedAt)

	repo := NewPostgresRepository(pool)
	groups, err := repo.ListSuggestedCandidates(ctx, fixture.ProjectID)
	if err != nil {
		t.Fatalf("list suggested candidates: %v", err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 suggestion groups, got %d: %+v", len(groups), groups)
	}

	var sameModelGroup SuggestedCandidate
	var otherModelGroup SuggestedCandidate
	for _, group := range groups {
		if group.SnapshotID <= 0 {
			t.Fatalf("expected snapshot-scoped suggestion, got %+v", group)
		}
		if group.SuggestionKey == "" {
			t.Fatalf("expected suggestion key, got %+v", group)
		}
		if len(group.Items) == 0 {
			t.Fatalf("expected grouped items, got %+v", group)
		}
		if group.Summary["source_model"] == "detector-a" {
			sameModelGroup = group
		}
		if group.Summary["source_model"] == "detector-b" {
			otherModelGroup = group
		}
	}

	if len(sameModelGroup.Items) != 2 {
		t.Fatalf("expected detector-a group to contain 2 items, got %+v", sameModelGroup)
	}
	if len(otherModelGroup.Items) != 1 || otherModelGroup.Items[0].CandidateID != otherModelCandidateID {
		t.Fatalf("expected detector-b group to contain the alternate candidate, got %+v", otherModelGroup)
	}
}

func TestPostgresRepositoryOwnerApproveCreatesPublishRecord(t *testing.T) {
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

	mustApplyPublishMigration(t, ctx, pool, true)

	fixture := seedPublishFixture(t, ctx, pool, time.Now().UTC().Truncate(time.Hour))
	repo := NewPostgresRepository(pool)

	batch, err := repo.CreateBatch(ctx, CreateBatchInput{
		ProjectID:  fixture.ProjectID,
		SnapshotID: fixture.SnapshotID,
		Source:     SourceSuggested,
		Items: []CreateBatchItemInput{{
			CandidateID: fixture.AcceptedCandidateIDs[0],
			TaskID:      fixture.TaskID,
			DatasetID:   fixture.DatasetID,
			SnapshotID:  fixture.SnapshotID,
			ItemPayload: map[string]any{"task": map[string]any{"id": fixture.TaskID}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := repo.ApplyReviewDecision(ctx, batch.ID, ReviewDecisionApprove, "reviewer-1", nil); err != nil {
		t.Fatalf("review approve: %v", err)
	}

	record, err := repo.ApplyOwnerDecision(ctx, batch.ID, OwnerDecisionApprove, "owner-1", nil)
	if err != nil {
		t.Fatalf("owner approve: %v", err)
	}
	if record.PublishBatchID != batch.ID {
		t.Fatalf("expected publish_batch_id=%d, got %+v", batch.ID, record)
	}
	if record.ApprovedByOwner != "owner-1" {
		t.Fatalf("expected approved_by_owner=owner-1, got %+v", record)
	}
}

type publishFixture struct {
	ProjectID            int64
	DatasetID            int64
	SnapshotID           int64
	SnapshotVersion      string
	ItemID               int64
	CategoryID           int64
	TaskID               int64
	AcceptedCandidateIDs []int64
}

func seedPublishFixture(t *testing.T, ctx context.Context, pool queryRower, acceptedAt time.Time) publishFixture {
	t.Helper()
	return seedPublishFixtureForProject(t, ctx, pool, 0, "detector-a", acceptedAt, 2)
}

func seedPublishFixtureForProject(t *testing.T, ctx context.Context, pool queryRower, projectID int64, model string, acceptedAt time.Time, acceptedCandidates int) publishFixture {
	t.Helper()

	ts := time.Now().UTC().UnixNano()
	fixture := publishFixture{}

	if projectID == 0 {
		if err := pool.QueryRow(ctx, `
			insert into projects (name, owner)
			values ($1, $2)
			returning id
		`, fmt.Sprintf("publish-project-%d", ts), "integration-test").Scan(&fixture.ProjectID); err != nil {
			t.Fatalf("seed project: %v", err)
		}
	} else {
		fixture.ProjectID = projectID
	}

	if err := pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id
	`, fixture.ProjectID, fmt.Sprintf("publish-dataset-%d", ts), "platform-dev", fmt.Sprintf("publish/%d", ts)).Scan(&fixture.DatasetID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}

	fixture.SnapshotVersion = fmt.Sprintf("v%d", ts)
	if err := pool.QueryRow(ctx, `
		insert into dataset_snapshots (dataset_id, version, created_by, note)
		values ($1, $2, $3, $4)
		returning id
	`, fixture.DatasetID, fixture.SnapshotVersion, "integration-test", "publish fixture").Scan(&fixture.SnapshotID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into dataset_items (dataset_id, object_key, mime)
		values ($1, $2, $3)
		returning id
	`, fixture.DatasetID, fmt.Sprintf("images/%d.jpg", ts), "image/jpeg").Scan(&fixture.ItemID); err != nil {
		t.Fatalf("seed dataset item: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into categories (project_id, name)
		values ($1, $2)
		returning id
	`, fixture.ProjectID, fmt.Sprintf("car-%d", ts)).Scan(&fixture.CategoryID); err != nil {
		t.Fatalf("seed category: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		insert into tasks (
			project_id, snapshot_id, title, kind, status, priority, assignee, blocker_reason, last_activity_at
		)
		values ($1, $2, $3, 'review', 'ready', 'high', 'reviewer-a', '', now())
		returning id
	`, fixture.ProjectID, fixture.SnapshotID, fmt.Sprintf("review-lane-%d", ts)).Scan(&fixture.TaskID); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	fixture.AcceptedCandidateIDs = make([]int64, 0, acceptedCandidates)
	for i := 0; i < acceptedCandidates; i++ {
		var candidateID int64
		if err := pool.QueryRow(ctx, `
			insert into annotation_candidates (
				dataset_id, snapshot_id, item_id, category_id,
				bbox_x, bbox_y, bbox_w, bbox_h,
				confidence, model_name, is_pseudo, review_status, reviewer_id, reviewed_at
			)
			values ($1, $2, $3, $4, 1, 2, 3, 4, 0.95, $5, true, 'accepted', 'reviewer-a', $6)
			returning id
		`, fixture.DatasetID, fixture.SnapshotID, fixture.ItemID, fixture.CategoryID, model, acceptedAt).Scan(&candidateID); err != nil {
			t.Fatalf("seed accepted candidate: %v", err)
		}
		fixture.AcceptedCandidateIDs = append(fixture.AcceptedCandidateIDs, candidateID)
	}

	return fixture
}

func seedAcceptedCandidate(t *testing.T, ctx context.Context, pool queryRower, fixture publishFixture, model string, acceptedAt time.Time) int64 {
	t.Helper()

	var candidateID int64
	if err := pool.QueryRow(ctx, `
		insert into annotation_candidates (
			dataset_id, snapshot_id, item_id, category_id,
			bbox_x, bbox_y, bbox_w, bbox_h,
			confidence, model_name, is_pseudo, review_status, reviewer_id, reviewed_at
		)
		values ($1, $2, $3, $4, 1, 2, 3, 4, 0.95, $5, true, 'accepted', 'reviewer-a', $6)
		returning id
	`, fixture.DatasetID, fixture.SnapshotID, fixture.ItemID, fixture.CategoryID, model, acceptedAt).Scan(&candidateID); err != nil {
		t.Fatalf("seed accepted candidate: %v", err)
	}

	return candidateID
}

type queryRower interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func mustApplyPublishMigration(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}, reset bool) {
	t.Helper()

	if reset {
		down := mustReadMigrationFile(t, "000004_publish_gate_review_workspace.down.sql")
		if strings.TrimSpace(down) != "" {
			if _, err := pool.Exec(ctx, down); err != nil && !strings.Contains(err.Error(), "does not exist") {
				t.Fatalf("apply publish down migration: %v", err)
			}
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

var _ queryRower = (*pgxpool.Pool)(nil)
