# Publish Gate And Review Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the V1 Phase 2 review-to-publish slice: suggested publish batches, reviewer/owner double approval, structured feedback, publish records, automatic downstream tasks, and the first complete review workspace in the web shell.

**Architecture:** Add a new `internal/publish` domain that owns batch, feedback, record, and approval state transitions while consuming read-only review candidate context from `internal/review` and creating downstream work through `internal/tasks`. Extend `apps/web` with `Review Queue`, `Publish Candidates`, `Publish Batch Detail`, and `Review Workspace`, keeping batch/workspace reads backend-aggregated so the React client does not assemble publish semantics with N+1 requests.

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, React 19, TypeScript, React Router, TanStack Query, Vitest, Testing Library.

---

## File Structure

**Create**

- `migrations/000004_publish_gate_review_workspace.up.sql`
- `migrations/000004_publish_gate_review_workspace.down.sql`
- `internal/publish/model.go`
- `internal/publish/repository.go`
- `internal/publish/postgres_repository.go`
- `internal/publish/service.go`
- `internal/publish/handler.go`
- `internal/publish/service_test.go`
- `internal/publish/handler_test.go`
- `internal/publish/postgres_repository_test.go`
- `internal/review/postgres_repository_test.go`
- `apps/web/src/features/publish/api.ts`
- `apps/web/src/features/publish/publish-candidates-page.tsx`
- `apps/web/src/features/publish/publish-batch-detail-page.tsx`
- `apps/web/src/features/publish/publish-pages.test.tsx`
- `apps/web/src/features/review/review-queue-page.tsx`
- `apps/web/src/features/review/review-workspace-page.tsx`
- `apps/web/src/features/review/review-workspace.test.tsx`

**Modify**

- `internal/review/repository.go`
- `internal/review/postgres_repository.go`
- `internal/tasks/model.go`
- `internal/tasks/service.go`
- `internal/tasks/service_test.go`
- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `cmd/api-server/main.go`
- `cmd/api-server/main_test.go`
- `apps/web/src/app/layout/app-shell.tsx`
- `apps/web/src/app/router.tsx`
- `apps/web/src/app/styles.css`

## Task 1: Add Publish Gate Schema, Models, And Repository Semantics

**Files:**
- Create: `migrations/000004_publish_gate_review_workspace.up.sql`
- Create: `migrations/000004_publish_gate_review_workspace.down.sql`
- Create: `internal/publish/model.go`
- Create: `internal/publish/repository.go`
- Create: `internal/publish/postgres_repository.go`
- Create: `internal/publish/postgres_repository_test.go`
- Create: `internal/review/postgres_repository_test.go`
- Modify: `internal/review/repository.go`
- Modify: `internal/review/postgres_repository.go`

- [ ] **Step 1: Add the failing repository tests for publish batch lifecycle, rule-grouped suggestions, and frozen item payloads**

Append focused tests to `internal/publish/postgres_repository_test.go` that prove:

```go
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

	repo := NewPostgresRepository(pool)

	batch, err := repo.CreateBatch(ctx, CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 11,
		Source:     SourceSuggested,
		RuleSummary: map[string]any{
			"grouping": "risk-window",
		},
		Items: []CreateBatchItemInput{
			{
				CandidateID: 301,
				TaskID:      44,
				DatasetID:   7,
				SnapshotID:  11,
				ItemPayload: map[string]any{
					"task":     map[string]any{"id": 44, "title": "review lane 4"},
					"snapshot": map[string]any{"id": 11, "version": "v4"},
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
	if batch.Items[0].CandidateID != 301 {
		t.Fatalf("expected candidate_id=301, got %d", batch.Items[0].CandidateID)
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

	repo := NewPostgresRepository(pool)
	batch, err := repo.CreateBatch(ctx, CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 12,
		Source:     SourceManual,
		Items: []CreateBatchItemInput{{
			CandidateID: 302,
			TaskID:      45,
			DatasetID:   7,
			SnapshotID:  12,
			ItemPayload: map[string]any{"task": map[string]any{"id": 45}},
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
			CandidateID: 303,
			TaskID:      46,
			DatasetID:   7,
			SnapshotID:  12,
			ItemPayload: map[string]any{"task": map[string]any{"id": 46}},
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

	repo := NewPostgresRepository(pool)
	groups, err := repo.ListSuggestedCandidates(ctx, 1)
	if err != nil {
		t.Fatalf("list suggested candidates: %v", err)
	}

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
	}
}
```

- [ ] **Step 2: Add the failing review query test for publish candidate suggestions**

Create `internal/review/postgres_repository_test.go` with a focused integration test that proves the review module can expose publishable candidate rows without mutating review state:

```go
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

	repo := NewPostgresRepository(pool)
	items, err := repo.ListPublishableCandidates(1)
	if err != nil {
		t.Fatalf("list publishable candidates: %v", err)
	}

	for _, item := range items {
		if item.SnapshotID <= 0 {
			t.Fatalf("expected snapshot-scoped candidate, got %+v", item)
		}
		if item.ReviewStatus != "accepted" {
			t.Fatalf("expected accepted review status, got %+v", item)
		}
	}
}
```

- [ ] **Step 3: Add the migration for publish batches, items, feedback, and records**

Create `migrations/000004_publish_gate_review_workspace.up.sql` with:

```sql
create table publish_batches (
  id bigserial primary key,
  project_id bigint not null,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  source text not null,
  status text not null,
  rule_summary_json jsonb not null default '{}'::jsonb,
  owner_edit_version integer not null default 0,
  review_approved_at timestamptz,
  review_approved_by text not null default '',
  owner_decided_at timestamptz,
  owner_decided_by text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table publish_batch_items (
  id bigserial primary key,
  publish_batch_id bigint not null references publish_batches(id) on delete cascade,
  candidate_id bigint not null,
  task_id bigint not null,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  dataset_id bigint not null references datasets(id) on delete cascade,
  item_payload_json jsonb not null,
  position integer not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table publish_feedback (
  id bigserial primary key,
  publish_batch_id bigint not null references publish_batches(id) on delete cascade,
  publish_batch_item_id bigint references publish_batch_items(id) on delete cascade,
  scope text not null,
  stage text not null,
  action text not null,
  reason_code text not null,
  severity text not null,
  influence_weight numeric(5,2) not null default 1.00,
  comment text not null default '',
  created_by text not null,
  created_at timestamptz not null default now()
);

create table publish_records (
  id bigserial primary key,
  project_id bigint not null,
  snapshot_id bigint not null references dataset_snapshots(id) on delete cascade,
  publish_batch_id bigint not null references publish_batches(id) on delete cascade,
  status text not null,
  summary_json jsonb not null default '{}'::jsonb,
  approved_by_owner text not null,
  approved_at timestamptz not null,
  created_at timestamptz not null default now()
);

create index idx_publish_batches_snapshot_status on publish_batches(snapshot_id, status);
create index idx_publish_batch_items_batch_position on publish_batch_items(publish_batch_id, position);
create index idx_publish_feedback_batch on publish_feedback(publish_batch_id, created_at desc);
create index idx_publish_records_snapshot on publish_records(snapshot_id, approved_at desc);
```

Create `migrations/000004_publish_gate_review_workspace.down.sql` with:

```sql
drop table if exists publish_records;
drop table if exists publish_feedback;
drop table if exists publish_batch_items;
drop table if exists publish_batches;
```

- [ ] **Step 4: Add publish models and repository interfaces**

Create `internal/publish/model.go` with the core types and constants:

```go
package publish

import "time"

const (
	SourceSuggested = "suggested"
	SourceManual    = "manual"

	StatusDraft                 = "draft"
	StatusReviewPending         = "review_pending"
	StatusReviewApproved        = "review_approved"
	StatusOwnerPending          = "owner_pending"
	StatusOwnerChangesRequested = "owner_changes_requested"
	StatusRejected              = "rejected"
	StatusPublished             = "published"
	StatusSuperseded            = "superseded"

	FeedbackScopeBatch = "batch"
	FeedbackScopeItem  = "item"
	FeedbackStageReview = "review"
	FeedbackStageOwner  = "owner"
	FeedbackActionReject = "reject"
	FeedbackActionRework = "rework"
	FeedbackActionComment = "comment"
	ReviewDecisionApprove = "approve"
	ReviewDecisionReject  = "reject"
	ReviewDecisionRework  = "rework"
	OwnerDecisionApprove  = "approve"
	OwnerDecisionReject   = "reject"
	OwnerDecisionRework   = "rework"
)

type Batch struct {
	ID               int64          `json:"id"`
	ProjectID        int64          `json:"project_id"`
	SnapshotID       int64          `json:"snapshot_id"`
	Source           string         `json:"source"`
	Status           string         `json:"status"`
	RuleSummary      map[string]any `json:"rule_summary"`
	OwnerEditVersion int            `json:"owner_edit_version"`
	ReviewApprovedAt *time.Time     `json:"review_approved_at,omitempty"`
	ReviewApprovedBy string         `json:"review_approved_by,omitempty"`
	OwnerDecidedAt   *time.Time     `json:"owner_decided_at,omitempty"`
	OwnerDecidedBy   string         `json:"owner_decided_by,omitempty"`
	Items            []BatchItem    `json:"items,omitempty"`
	Feedback         []Feedback     `json:"feedback,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type BatchItem struct {
	ID            int64          `json:"id"`
	PublishBatchID int64         `json:"publish_batch_id"`
	CandidateID   int64          `json:"candidate_id"`
	TaskID        int64          `json:"task_id"`
	DatasetID     int64          `json:"dataset_id"`
	SnapshotID    int64          `json:"snapshot_id"`
	ItemPayload   map[string]any `json:"item_payload"`
	Position      int            `json:"position"`
}

type Feedback struct {
	ID                 int64      `json:"id"`
	PublishBatchID     int64      `json:"publish_batch_id"`
	PublishBatchItemID int64      `json:"publish_batch_item_id,omitempty"`
	Scope              string     `json:"scope"`
	Stage              string     `json:"stage"`
	Action             string     `json:"action"`
	ReasonCode         string     `json:"reason_code"`
	Severity           string     `json:"severity"`
	InfluenceWeight    float64    `json:"influence_weight"`
	Comment            string     `json:"comment"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
}

type Record struct {
	ID              int64          `json:"id"`
	ProjectID       int64          `json:"project_id"`
	SnapshotID      int64          `json:"snapshot_id"`
	PublishBatchID  int64          `json:"publish_batch_id"`
	Status          string         `json:"status"`
	Summary         map[string]any `json:"summary"`
	ApprovedByOwner string         `json:"approved_by_owner"`
	ApprovedAt      time.Time      `json:"approved_at"`
	CreatedAt       time.Time      `json:"created_at"`
}

type SuggestedCandidate struct {
	SnapshotID    int64                  `json:"snapshot_id"`
	SuggestionKey string                 `json:"suggestion_key"`
	Summary       map[string]any         `json:"summary"`
	Items         []SuggestedCandidateItem `json:"items"`
}

type SuggestedCandidateItem struct {
	CandidateID int64          `json:"candidate_id"`
	TaskID      int64          `json:"task_id"`
	DatasetID   int64          `json:"dataset_id"`
	ItemPayload map[string]any `json:"item_payload"`
}

type TimelineEntry struct {
	Stage  string    `json:"stage"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	At     time.Time `json:"at"`
}

type CreateBatchInput struct {
	ProjectID   int64                  `json:"project_id"`
	SnapshotID  int64                  `json:"snapshot_id"`
	Source      string                 `json:"source"`
	RuleSummary map[string]any         `json:"rule_summary"`
	Items       []CreateBatchItemInput `json:"items"`
}

type CreateBatchItemInput struct {
	CandidateID int64          `json:"candidate_id"`
	TaskID      int64          `json:"task_id"`
	DatasetID   int64          `json:"dataset_id"`
	SnapshotID  int64          `json:"snapshot_id"`
	ItemPayload map[string]any `json:"item_payload"`
}

type ReplaceBatchItemsInput struct {
	Actor string                 `json:"actor"`
	Items []CreateBatchItemInput `json:"items"`
}

type CreateFeedbackInput struct {
	Scope           string  `json:"scope"`
	Stage           string  `json:"stage"`
	Action          string  `json:"action"`
	ReasonCode      string  `json:"reason_code"`
	Severity        string  `json:"severity"`
	InfluenceWeight float64 `json:"influence_weight"`
	Comment         string  `json:"comment"`
	Actor           string  `json:"actor"`
}

type ApprovalInput struct {
	Actor    string                `json:"actor"`
	Feedback []CreateFeedbackInput `json:"feedback"`
}
```

Create `internal/publish/repository.go` with:

```go
type Repository interface {
	CreateBatch(ctx context.Context, in CreateBatchInput) (Batch, error)
	GetBatch(ctx context.Context, batchID int64) (Batch, error)
	GetRecord(ctx context.Context, recordID int64) (Record, error)
	ReplaceBatchItems(ctx context.Context, batchID int64, in ReplaceBatchItemsInput) (Batch, error)
	ApplyReviewDecision(ctx context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) error
	ApplyOwnerDecision(ctx context.Context, batchID int64, decision, actor string, feedback []CreateFeedbackInput) (Record, error)
	AddBatchFeedback(ctx context.Context, batchID int64, in CreateFeedbackInput) (Feedback, error)
	AddItemFeedback(ctx context.Context, batchID, itemID int64, in CreateFeedbackInput) (Feedback, error)
	ListSuggestedCandidates(ctx context.Context, projectID int64) ([]SuggestedCandidate, error)
	BuildWorkspace(ctx context.Context, batchID int64) (Workspace, error)
}

type InMemoryRepository struct {
	mu      sync.Mutex
	nextIDs struct {
		batch    int64
		item     int64
		feedback int64
		record   int64
	}
	batches map[int64]Batch
	records map[int64]Record
}

func NewInMemoryRepository() *InMemoryRepository {
	repo := &InMemoryRepository{
		batches: make(map[int64]Batch),
		records: make(map[int64]Record),
	}
	repo.nextIDs.batch = 1
	repo.nextIDs.item = 1
	repo.nextIDs.feedback = 1
	repo.nextIDs.record = 1
	return repo
}
```

- [ ] **Step 5: Implement PostgreSQL repository with frozen item payload and owner invalidation logic**

In `internal/publish/postgres_repository.go`, implement:

```go
func (r *PostgresRepository) ReplaceBatchItems(ctx context.Context, batchID int64, in ReplaceBatchItemsInput) (Batch, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Batch{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		delete from publish_batch_items
		where publish_batch_id = $1
	`, batchID); err != nil {
		return Batch{}, err
	}

	for i, item := range in.Items {
		if _, err := tx.Exec(ctx, `
			insert into publish_batch_items (
				publish_batch_id, candidate_id, task_id, snapshot_id, dataset_id, item_payload_json, position
			) values ($1, $2, $3, $4, $5, $6, $7)
		`, batchID, item.CandidateID, item.TaskID, item.SnapshotID, item.DatasetID, item.ItemPayload, i); err != nil {
			return Batch{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
		update publish_batches
		set status = $2,
		    owner_edit_version = owner_edit_version + 1,
		    review_approved_at = null,
		    review_approved_by = '',
		    updated_at = now()
		where id = $1
	`, batchID, StatusOwnerChangesRequested); err != nil {
		return Batch{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Batch{}, err
	}
	return r.GetBatch(ctx, batchID)
}

func (r *PostgresRepository) ListSuggestedCandidates(ctx context.Context, projectID int64) ([]SuggestedCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		select
			c.id,
			coalesce(t.id, 0) as task_id,
			c.dataset_id,
			c.snapshot_id,
			coalesce(t.priority, 'normal') as risk_level,
			coalesce(c.model_name, '') as source_model,
			date_trunc('hour', c.reviewed_at) as accepted_window,
			jsonb_build_object(
				"task", jsonb_build_object("id", coalesce(t.id, 0), "title", coalesce(t.title, '')),
				"snapshot", jsonb_build_object("id", s.id, "version", s.version)
			) as item_payload
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		join dataset_snapshots s on s.id = c.snapshot_id
		left join tasks t on t.snapshot_id = c.snapshot_id and t.kind = 'review'
		where d.project_id = $1 and c.review_status = 'accepted'
		order by c.snapshot_id asc, accepted_window asc, c.id asc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := map[string]*SuggestedCandidate{}
	order := make([]string, 0)
	for rows.Next() {
		var (
			candidateID     int64
			taskID          int64
			datasetID       int64
			snapshotID      int64
			riskLevel       string
			sourceModel     string
			acceptedWindow  time.Time
			itemPayload     map[string]any
		)
		if err := rows.Scan(&candidateID, &taskID, &datasetID, &snapshotID, &riskLevel, &sourceModel, &acceptedWindow, &itemPayload); err != nil {
			return nil, err
		}

		key := fmt.Sprintf("%d:%s:%s:%s", snapshotID, riskLevel, sourceModel, acceptedWindow.Format(time.RFC3339))
		if grouped[key] == nil {
			grouped[key] = &SuggestedCandidate{
				SnapshotID:    snapshotID,
				SuggestionKey: key,
				Summary: map[string]any{
					"risk_level":      riskLevel,
					"source_model":    sourceModel,
					"accepted_window": acceptedWindow.Format(time.RFC3339),
				},
			}
			order = append(order, key)
		}

		grouped[key].Items = append(grouped[key].Items, SuggestedCandidateItem{
			CandidateID: candidateID,
			TaskID:      taskID,
			DatasetID:   datasetID,
			ItemPayload: itemPayload,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]SuggestedCandidate, 0, len(order))
	for _, key := range order {
		out = append(out, *grouped[key])
	}
	return out, nil
}
```

- [ ] **Step 6: Extend review repository with publishable-candidate query**

Modify `internal/review/repository.go` and `internal/review/postgres_repository.go` to add a read-only query:

```go
type Repository interface {
	ListPending() ([]Candidate, error)
	Accept(candidateID int64, reviewer string) error
	Reject(candidateID int64, reviewer string) error
	ListPublishableCandidates(projectID int64) ([]PublishableCandidate, error)
}

type PublishableCandidate struct {
	ID           int64          `json:"id"`
	ProjectID    int64          `json:"project_id"`
	DatasetID    int64          `json:"dataset_id"`
	SnapshotID   int64          `json:"snapshot_id"`
	ItemID       int64          `json:"item_id"`
	TaskID       int64          `json:"task_id"`
	ReviewStatus string         `json:"review_status"`
	RiskLevel    string         `json:"risk_level"`
	SourceModel  string         `json:"source_model"`
	AcceptedAt   time.Time      `json:"accepted_at"`
	Summary      map[string]any `json:"summary"`
}

func (r *PostgresRepository) ListPublishableCandidates(projectID int64) ([]PublishableCandidate, error) {
	rows, err := r.pool.Query(context.Background(), `
		select
			c.id,
			d.project_id,
			c.dataset_id,
			c.snapshot_id,
			c.item_id,
			coalesce(t.id, 0) as task_id,
			c.review_status,
			coalesce(t.priority, 'normal') as risk_level,
			coalesce(c.model_name, '') as source_model,
			c.reviewed_at,
			jsonb_build_object(
				"dataset_name", d.name,
				"snapshot_version", s.version,
				"task_title", coalesce(t.title, ''),
				"reviewer_id", coalesce(c.reviewer_id, '')
			) as summary
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		join dataset_snapshots s on s.id = c.snapshot_id
		left join tasks t on t.snapshot_id = c.snapshot_id and t.kind = 'review'
		where d.project_id = $1 and c.review_status = 'accepted'
		order by c.snapshot_id asc, c.reviewed_at asc, c.id asc
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PublishableCandidate
	for rows.Next() {
		var item PublishableCandidate
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.DatasetID,
			&item.SnapshotID,
			&item.ItemID,
			&item.TaskID,
			&item.ReviewStatus,
			&item.RiskLevel,
			&item.SourceModel,
			&item.AcceptedAt,
			&item.Summary,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
```

- [ ] **Step 7: Run focused backend tests**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/publish ./internal/review -count=1 -v
```

Expected:

```text
ok  	yolo-ave-mujica/internal/publish
ok  	yolo-ave-mujica/internal/review
```

- [ ] **Step 8: Commit Task 1**

```bash
git add migrations/000004_publish_gate_review_workspace.up.sql migrations/000004_publish_gate_review_workspace.down.sql internal/publish internal/review/repository.go internal/review/postgres_repository.go
git commit -m "feat: add publish batch persistence"
```

### Task 2: Add Publish Service, Approval APIs, And Downstream Task Creation

**Files:**
- Create: `internal/publish/service.go`
- Create: `internal/publish/service_test.go`
- Create: `internal/publish/handler.go`
- Create: `internal/publish/handler_test.go`
- Modify: `internal/tasks/model.go`
- Modify: `internal/tasks/service.go`
- Modify: `internal/tasks/service_test.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/main_test.go`

- [ ] **Step 1: Write failing service tests for reviewer/owner approval and publish record output**

Create `internal/publish/service_test.go` with:

```go
func TestServiceOwnerApproveCreatesPublishRecordAndDownstreamTask(t *testing.T) {
	repo := NewInMemoryRepository()
	taskRepo := tasks.NewInMemoryRepository()
	svc := NewService(repo, tasks.NewService(taskRepo))

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 15,
		Source:     SourceSuggested,
		Items: []CreateBatchItemInput{{
			CandidateID: 401,
			TaskID:      51,
			DatasetID:   9,
			SnapshotID:  15,
			ItemPayload: map[string]any{"task": map[string]any{"id": 51}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := svc.ReviewApprove(context.Background(), batch.ID, ApprovalInput{Actor: "reviewer-1"}); err != nil {
		t.Fatalf("review approve: %v", err)
	}

	record, err := svc.OwnerApprove(context.Background(), batch.ID, ApprovalInput{Actor: "owner-1"})
	if err != nil {
		t.Fatalf("owner approve: %v", err)
	}
	if record.PublishBatchID != batch.ID {
		t.Fatalf("expected publish_batch_id=%d, got %d", batch.ID, record.PublishBatchID)
	}

	created, err := tasks.NewService(taskRepo).ListTasks(context.Background(), 1, tasks.ListTasksFilter{Kind: tasks.KindTrainingCandidate})
	if err != nil {
		t.Fatalf("list downstream tasks: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 downstream task, got %d", len(created))
	}
}

func TestServiceOwnerEditRequiresReviewApprovalAgain(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil)

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 16,
		Source:     SourceManual,
		Items: []CreateBatchItemInput{{
			CandidateID: 402,
			TaskID:      52,
			DatasetID:   9,
			SnapshotID:  16,
			ItemPayload: map[string]any{"task": map[string]any{"id": 52}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := svc.ReviewApprove(context.Background(), batch.ID, ApprovalInput{Actor: "reviewer-1"}); err != nil {
		t.Fatalf("review approve: %v", err)
	}
	if _, err := svc.ReplaceBatchItems(context.Background(), batch.ID, ReplaceBatchItemsInput{
		Actor: "owner-1",
		Items: []CreateBatchItemInput{{
			CandidateID: 403,
			TaskID:      53,
			DatasetID:   9,
			SnapshotID:  16,
			ItemPayload: map[string]any{"task": map[string]any{"id": 53}},
		}},
	}); err != nil {
		t.Fatalf("replace batch items: %v", err)
	}

	if _, err := svc.OwnerApprove(context.Background(), batch.ID, ApprovalInput{Actor: "owner-1"}); err == nil {
		t.Fatal("expected owner approve to fail before renewed reviewer approval")
	}
}
```

- [ ] **Step 2: Write failing handler tests for publish APIs**

Create `internal/publish/handler_test.go` with request coverage for:

```go
func TestHandlerReviewApproveAndOwnerApprove(t *testing.T) {
	repo := NewInMemoryRepository()
	taskRepo := tasks.NewInMemoryRepository()
	svc := NewService(repo, tasks.NewService(taskRepo))
	handler := NewHandler(svc)
	server := server.NewHTTPServerWithModules(server.Modules{
		Publish: server.PublishRoutes{
			ListCandidates:   handler.ListSuggestedCandidates,
			CreateBatch:      handler.CreateBatch,
			GetBatch:         handler.GetBatch,
			ReplaceBatchItems: handler.ReplaceBatchItems,
			ReviewApprove:    handler.ReviewApprove,
			OwnerApprove:     handler.OwnerApprove,
			AddBatchFeedback: handler.AddBatchFeedback,
			AddItemFeedback:  handler.AddItemFeedback,
			GetWorkspace:     handler.GetWorkspace,
			GetRecord:        handler.GetRecord,
		},
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/publish/batches", strings.NewReader(`{
		"project_id": 1,
		"snapshot_id": 15,
		"source": "suggested",
		"items": [{
			"candidate_id": 401,
			"task_id": 51,
			"dataset_id": 9,
			"snapshot_id": 15,
			"item_payload": {"task": {"id": 51}}
		}]
	}`))
	createRec := httptest.NewRecorder()
	server.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create batch 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	reviewReq := httptest.NewRequest(http.MethodPost, "/v1/publish/batches/1/review-approve", strings.NewReader(`{"actor":"reviewer-1"}`))
	reviewRec := httptest.NewRecorder()
	server.Handler.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("expected review approve 200, got %d body=%s", reviewRec.Code, reviewRec.Body.String())
	}

	ownerReq := httptest.NewRequest(http.MethodPost, "/v1/publish/batches/1/owner-approve", strings.NewReader(`{"actor":"owner-1"}`))
	ownerRec := httptest.NewRecorder()
	server.Handler.ServeHTTP(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("expected owner approve 200, got %d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	if !strings.Contains(ownerRec.Body.String(), `"publish_record_id":`) {
		t.Fatalf("expected publish_record_id in response, got %s", ownerRec.Body.String())
	}
}
```

- [ ] **Step 3: Extend task kinds for publish downstream work**

Modify `internal/tasks/model.go` to add:

```go
const (
	KindAnnotation        = "annotation"
	KindReview            = "review"
	KindQA                = "qa"
	KindOps               = "ops"
	KindTrainingCandidate = "training_candidate"
	KindPromotionReview   = "promotion_review"
)
```

Update `internal/tasks/service.go` validation so these kinds are accepted, then add focused assertions in `internal/tasks/service_test.go`.

- [ ] **Step 4: Implement publish service state machine**

In `internal/publish/service.go`, add:

```go
type TaskCreator interface {
	CreateTask(ctx context.Context, in tasks.CreateTaskInput) (tasks.Task, error)
}

func (s *Service) OwnerApprove(ctx context.Context, batchID int64, in ApprovalInput) (Record, error) {
	batch, err := s.repo.GetBatch(ctx, batchID)
	if err != nil {
		return Record{}, err
	}
	if batch.Status != StatusReviewApproved && batch.Status != StatusOwnerPending {
		return Record{}, fmt.Errorf("batch %d is not ready for owner approval", batchID)
	}

	record, err := s.repo.ApplyOwnerDecision(ctx, batchID, OwnerDecisionApprove, in.Actor, in.Feedback)
	if err != nil {
		return Record{}, err
	}
	if s.tasks != nil {
		_, err = s.tasks.CreateTask(ctx, tasks.CreateTaskInput{
			ProjectID:  batch.ProjectID,
			SnapshotID: &batch.SnapshotID,
			Title:      fmt.Sprintf("Evaluate publish record %d", record.ID),
			Kind:       tasks.KindTrainingCandidate,
			Status:     tasks.StatusQueued,
			Priority:   tasks.PriorityHigh,
			Assignee:   "ml-engineer",
		})
		if err != nil {
			return Record{}, err
		}
	}
	return record, nil
}
```

- [ ] **Step 5: Add publish HTTP routes and route wiring**

Extend `internal/server/http_server.go` with a new route group:

```go
type PublishRoutes struct {
	ListCandidates    http.HandlerFunc
	CreateBatch       http.HandlerFunc
	GetBatch          http.HandlerFunc
	ReplaceBatchItems http.HandlerFunc
	ReviewApprove     http.HandlerFunc
	ReviewReject      http.HandlerFunc
	ReviewRework      http.HandlerFunc
	OwnerApprove      http.HandlerFunc
	OwnerReject       http.HandlerFunc
	OwnerRework       http.HandlerFunc
	AddBatchFeedback  http.HandlerFunc
	AddItemFeedback   http.HandlerFunc
	GetWorkspace      http.HandlerFunc
	GetRecord         http.HandlerFunc
}
```

Register:

```go
r.Get("/publish/candidates", handlerOrNotImplemented(m.Publish.ListCandidates))
r.Post("/publish/batches", handlerOrNotImplemented(m.Publish.CreateBatch))
r.Get("/publish/batches/{id}", handlerOrNotImplemented(m.Publish.GetBatch))
r.Post("/publish/batches/{id}/items", handlerOrNotImplemented(m.Publish.ReplaceBatchItems))
r.Post("/publish/batches/{id}/review-approve", handlerOrNotImplemented(m.Publish.ReviewApprove))
r.Post("/publish/batches/{id}/review-reject", handlerOrNotImplemented(m.Publish.ReviewReject))
r.Post("/publish/batches/{id}/review-rework", handlerOrNotImplemented(m.Publish.ReviewRework))
r.Post("/publish/batches/{id}/owner-approve", handlerOrNotImplemented(m.Publish.OwnerApprove))
r.Post("/publish/batches/{id}/owner-reject", handlerOrNotImplemented(m.Publish.OwnerReject))
r.Post("/publish/batches/{id}/owner-rework", handlerOrNotImplemented(m.Publish.OwnerRework))
r.Post("/publish/batches/{id}/feedback", handlerOrNotImplemented(m.Publish.AddBatchFeedback))
r.Post("/publish/batches/{id}/items/{itemId}/feedback", handlerOrNotImplemented(m.Publish.AddItemFeedback))
r.Get("/publish/batches/{id}/workspace", handlerOrNotImplemented(m.Publish.GetWorkspace))
r.Get("/publish/records/{id}", handlerOrNotImplemented(m.Publish.GetRecord))
```

Wire the new service and handler in `cmd/api-server/main.go`:

```go
publishRepo := publish.NewPostgresRepository(pool)
publishSvc := publish.NewService(publishRepo, taskSvc)
publishHandler := publish.NewHandler(publishSvc)

modules.Publish = server.PublishRoutes{
	ListCandidates:    publishHandler.ListSuggestedCandidates,
	CreateBatch:       publishHandler.CreateBatch,
	GetBatch:          publishHandler.GetBatch,
	ReplaceBatchItems: publishHandler.ReplaceBatchItems,
	ReviewApprove:     publishHandler.ReviewApprove,
	ReviewReject:      publishHandler.ReviewReject,
	ReviewRework:      publishHandler.ReviewRework,
	OwnerApprove:      publishHandler.OwnerApprove,
	OwnerReject:       publishHandler.OwnerReject,
	OwnerRework:       publishHandler.OwnerRework,
	AddBatchFeedback:  publishHandler.AddBatchFeedback,
	AddItemFeedback:   publishHandler.AddItemFeedback,
	GetWorkspace:      publishHandler.GetWorkspace,
	GetRecord:         publishHandler.GetRecord,
}
```

- [ ] **Step 6: Run backend publish/task/server tests**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/publish ./internal/tasks ./internal/server ./cmd/api-server -count=1 -v
```

Expected:

```text
ok  	yolo-ave-mujica/internal/publish
ok  	yolo-ave-mujica/internal/tasks
ok  	yolo-ave-mujica/internal/server
ok  	yolo-ave-mujica/cmd/api-server
```

- [ ] **Step 7: Commit Task 2**

```bash
git add internal/publish internal/tasks/model.go internal/tasks/service.go internal/tasks/service_test.go internal/server/http_server.go internal/server/http_server_routes_test.go cmd/api-server/main.go cmd/api-server/main_test.go
git commit -m "feat: add publish approval APIs"
```

### Task 3: Add Review Queue, Publish Candidates, And Publish Batch Detail Pages

**Files:**
- Create: `apps/web/src/features/publish/api.ts`
- Create: `apps/web/src/features/publish/publish-candidates-page.tsx`
- Create: `apps/web/src/features/publish/publish-batch-detail-page.tsx`
- Create: `apps/web/src/features/publish/publish-pages.test.tsx`
- Create: `apps/web/src/features/review/review-queue-page.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/layout/app-shell.tsx`
- Modify: `apps/web/src/app/styles.css`

- [ ] **Step 1: Write failing page tests for queue, candidates, and batch detail**

Create `apps/web/src/features/publish/publish-pages.test.tsx` with:

```tsx
it("renders review queue with publish links", async () => {
  vi.mocked(global.fetch).mockResolvedValueOnce(
    jsonResponse({
      items: [
        { id: 71, title: "Publish lane 4", status: "owner_pending", snapshot_id: 15 },
      ],
    }),
  );

  renderPage("/review");

  expect(await screen.findByRole("heading", { name: "Review Queue" })).toBeInTheDocument();
  expect(screen.getByRole("link", { name: /Publish lane 4/i })).toHaveAttribute(
    "href",
    "/publish/batches/71",
  );
});

it("renders suggested publish candidates and create-batch controls", async () => {
  vi.mocked(global.fetch).mockResolvedValueOnce(
    jsonResponse({
      items: [
        {
          snapshot_id: 15,
          suggestion_key: "risk-high-window-1",
          summary: { reason: "same-risk-window" },
          items: [
            {
              candidate_id: 401,
              task_id: 51,
              dataset_id: 9,
              item_payload: { task: { id: 51 }, snapshot: { id: 15, version: "v5" } },
            },
          ],
        },
      ],
    }),
  );

  renderPage("/publish/candidates");

  expect(await screen.findByRole("heading", { name: "Publish Candidates" })).toBeInTheDocument();
  expect(screen.getByText(/same-risk-window/i)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Create Publish Batch/i })).toBeInTheDocument();
});

it("renders publish batch detail with feedback and owner actions", async () => {
  vi.mocked(global.fetch).mockResolvedValueOnce(
    jsonResponse({
      id: 71,
      snapshot_id: 15,
      status: "owner_pending",
      items: [{ id: 801, candidate_id: 401, task_id: 51, dataset_id: 9, snapshot_id: 15, item_payload: {} }],
      feedback: [{
        id: 1,
        scope: "batch",
        stage: "review",
        action: "comment",
        reason_code: "ready_for_publish",
        severity: "low",
        influence_weight: 1,
        comment: "",
      }],
    }),
  );

  renderPage("/publish/batches/71");

  expect(await screen.findByRole("heading", { name: "Publish Batch #71" })).toBeInTheDocument();
  expect(screen.getByText(/owner_pending/i)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Owner Approve/i })).toBeInTheDocument();
});
```

- [ ] **Step 2: Add publish API client**

Create `apps/web/src/features/publish/api.ts`:

```ts
import { getJSON, postJSON } from "../shared/http";

export interface SuggestedPublishGroup {
  snapshot_id: number;
  suggestion_key: string;
  summary: Record<string, unknown>;
  items: Array<{
    candidate_id: number;
    task_id: number;
    dataset_id: number;
    item_payload: Record<string, unknown>;
  }>;
}

export interface PublishBatchItem {
  id: number;
  candidate_id: number;
  task_id: number;
  dataset_id: number;
  snapshot_id: number;
  item_payload: Record<string, unknown>;
}

export interface PublishFeedback {
  id: number;
  scope: string;
  stage: string;
  action: string;
  reason_code: string;
  severity: string;
  influence_weight: number;
  comment: string;
  publish_batch_item_id?: number;
}

export interface PublishWorkspace {
  batch: PublishBatch;
  items: Array<{
    item_id: number;
    candidate_id: number;
    task_id: number;
    overlay: Record<string, unknown>;
    diff: Record<string, number>;
    context: Record<string, unknown>;
    feedback: PublishFeedback[];
  }>;
  history: Array<{ stage: string; actor: string; action: string; at?: string }>;
}

export interface CreateFeedbackPayload {
  stage: string;
  action: string;
  reason_code: string;
  severity: string;
  influence_weight: number;
  comment?: string;
  actor: string;
}

export interface PublishBatch {
  id: number;
  snapshot_id: number;
  project_id: number;
  status: string;
  source: string;
  rule_summary: Record<string, unknown>;
  items: PublishBatchItem[];
  feedback: PublishFeedback[];
}

export function listPublishCandidates() {
  return getJSON<{ items: SuggestedPublishGroup[] }>("/v1/publish/candidates");
}

export function createPublishBatch(payload: unknown) {
  return postJSON<PublishBatch>("/v1/publish/batches", payload);
}

export function getPublishBatch(batchId: number | string) {
  return getJSON<PublishBatch>(`/v1/publish/batches/${batchId}`);
}

export function getPublishWorkspace(batchId: number | string) {
  return getJSON<PublishWorkspace>(`/v1/publish/batches/${batchId}/workspace`);
}

export function reviewApprove(batchId: number | string, actor: string) {
  return postJSON<{ ok: true }>(`/v1/publish/batches/${batchId}/review-approve`, { actor });
}

export function ownerApprove(batchId: number | string, actor: string) {
  return postJSON<{ publish_record_id: number }>(`/v1/publish/batches/${batchId}/owner-approve`, { actor });
}

export function addBatchFeedback(batchId: number | string, payload: CreateFeedbackPayload) {
  return postJSON<PublishFeedback>(`/v1/publish/batches/${batchId}/feedback`, payload);
}

export function addItemFeedback(batchId: number | string, itemId: number | string, payload: CreateFeedbackPayload) {
  return postJSON<PublishFeedback>(`/v1/publish/batches/${batchId}/items/${itemId}/feedback`, payload);
}
```

- [ ] **Step 3: Implement Review Queue and Publish Candidates pages**

Create `apps/web/src/features/review/review-queue-page.tsx` and `apps/web/src/features/publish/publish-candidates-page.tsx`:

```tsx
export function ReviewQueuePage() {
  const queueQuery = useQuery({
    queryKey: ["review-queue", 1],
    queryFn: () => listTasks(1, { kind: "review" }),
  });

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Review Queue</h1>
          <p className="page-summary">Process review and publish work without leaving the shell.</p>
        </div>
      </header>
      {queueQuery.data?.items.map((task) => (
        <Link key={task.id} className="stack-item" to={`/publish/batches/${task.id}`}>
          <strong>{task.title}</strong>
          <span>{task.status}</span>
        </Link>
      ))}
    </section>
  );
}

export function PublishCandidatesPage() {
  const queryClient = useQueryClient();
  const candidatesQuery = useQuery({
    queryKey: ["publish-candidates", 1],
    queryFn: listPublishCandidates,
  });
  const createBatchMutation = useMutation({
    mutationFn: (group: SuggestedPublishGroup) =>
      createPublishBatch({
        project_id: 1,
        snapshot_id: group.snapshot_id,
        source: "suggested",
        rule_summary: group.summary,
        items: group.items.map((item) => ({
          candidate_id: item.candidate_id,
          task_id: item.task_id,
          dataset_id: item.dataset_id,
          snapshot_id: group.snapshot_id,
          item_payload: item.item_payload,
        })),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["publish-candidates", 1] });
    },
  });

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Publish Candidates</h1>
          <p className="page-summary">Review rule-engine suggestions before creating a formal publish batch.</p>
        </div>
      </header>

      {candidatesQuery.isLoading ? <p>Loading publish candidates.</p> : null}
      {candidatesQuery.isError ? (
        <p role="alert">Failed to load publish candidates: {candidatesQuery.error.message}</p>
      ) : null}

      {candidatesQuery.data ? (
        <section className="panel">
          <div className="panel-header">
            <h2>Suggested groups</h2>
            <span>{candidatesQuery.data.items.length}</span>
          </div>
          <div className="stack-list">
            {candidatesQuery.data.items.map((group) => (
              <article className="stack-item" key={group.suggestion_key}>
                <strong>{group.suggestion_key}</strong>
                <span>{String(group.summary.reason ?? "rules-based suggestion")}</span>
                <span>{group.items.length} candidates</span>
                <button type="button" onClick={() => void createBatchMutation.mutateAsync(group)}>
                  Create Publish Batch
                </button>
              </article>
            ))}
          </div>
        </section>
      ) : null}
    </section>
  );
}
```

- [ ] **Step 4: Implement Publish Batch Detail page**

Create `apps/web/src/features/publish/publish-batch-detail-page.tsx`:

```tsx
export function PublishBatchDetailPage() {
  const { batchId = "" } = useParams();
  const batchQuery = useQuery({
    queryKey: ["publish-batch", batchId],
    queryFn: () => getPublishBatch(batchId),
  });
  const queryClient = useQueryClient();

  const ownerApproveMutation = useMutation({
    mutationFn: () => ownerApprove(batchId, "owner-1"),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["publish-batch", batchId] });
    },
  });
  const reviewApproveMutation = useMutation({
    mutationFn: () => reviewApprove(batchId, "reviewer-1"),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["publish-batch", batchId] });
    },
  });

  if (batchQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Publish Batch</h1>
        <p>Loading publish batch.</p>
      </section>
    );
  }

  if (batchQuery.isError || !batchQuery.data) {
    return (
      <section className="page-stack">
        <h1>Publish Batch</h1>
        <p role="alert">Failed to load publish batch.</p>
      </section>
    );
  }

  const batch = batchQuery.data;

  return (
    <section className="page-stack">
      <header className="page-hero page-hero--compact">
        <div>
          <p className="page-kicker">Snapshot {batch.snapshot_id}</p>
          <h1>Publish Batch #{batch.id}</h1>
          <p className="page-summary">Track reviewer approval, owner approval, frozen item payloads, and structured feedback.</p>
        </div>
        <div className="hero-meter">
          <span>Status</span>
          <strong>{batch.status}</strong>
          <small>{batch.items.length} items</small>
        </div>
      </header>

      <section className="panel">
        <div className="panel-header">
          <h2>Batch items</h2>
          <Link to={`/review/workspace/${batch.id}`}>Open Review Workspace</Link>
        </div>
        <div className="stack-list">
          {batch.items.map((item) => (
            <article className="stack-item" key={item.id}>
              <strong>Candidate #{item.candidate_id}</strong>
              <span>Task #{item.task_id}</span>
              <span>Dataset #{item.dataset_id}</span>
            </article>
          ))}
        </div>
      </section>

      <section className="panel panel-accent">
        <div className="panel-header">
          <h2>Structured feedback</h2>
          <span>{batch.feedback.length}</span>
        </div>
        <div className="stack-list">
          {batch.feedback.map((entry) => (
            <article className="stack-item" key={entry.id}>
              <strong>{entry.scope} · {entry.stage}</strong>
              <span>{entry.reason_code}</span>
              <span>{entry.comment || "No comment"}</span>
            </article>
          ))}
        </div>
        <div className="action-row">
          <button type="button" onClick={() => void reviewApproveMutation.mutateAsync()}>
            Reviewer Approve
          </button>
          <button type="button" onClick={() => void ownerApproveMutation.mutateAsync()}>
            Owner Approve
          </button>
        </div>
      </section>
    </section>
  );
}
```

- [ ] **Step 5: Add routes and shell navigation**

Modify `apps/web/src/app/router.tsx`:

```tsx
{ path: "review", element: <ReviewQueuePage /> },
{ path: "publish/candidates", element: <PublishCandidatesPage /> },
{ path: "publish/batches/:batchId", element: <PublishBatchDetailPage /> },
```

Modify `apps/web/src/app/layout/app-shell.tsx`:

```tsx
<NavLink to="/review">Review</NavLink>
<NavLink to="/publish/candidates">Publish</NavLink>
```

- [ ] **Step 6: Run focused frontend tests**

Run:

```bash
cd apps/web && npm test -- src/features/publish/publish-pages.test.tsx
```

Expected:

```text
Test Files  1 passed
Tests       3 passed
```

- [ ] **Step 7: Commit Task 3**

```bash
git add apps/web/src/features/publish apps/web/src/features/review/review-queue-page.tsx apps/web/src/app/router.tsx apps/web/src/app/layout/app-shell.tsx apps/web/src/app/styles.css
git commit -m "feat: add publish queue pages"
```

### Task 4: Add Review Workspace Query And Complete Review Workspace UI

**Files:**
- Create: `apps/web/src/features/review/review-workspace-page.tsx`
- Create: `apps/web/src/features/review/review-workspace.test.tsx`
- Modify: `internal/publish/repository.go`
- Modify: `internal/publish/postgres_repository.go`
- Modify: `internal/publish/service.go`
- Modify: `internal/publish/handler.go`
- Modify: `apps/web/src/features/publish/api.ts`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/styles.css`

- [ ] **Step 1: Write the failing workspace query/service tests**

Append to `internal/publish/service_test.go`:

```go
func TestServiceBuildWorkspaceReturnsBatchItemsDiffAndFeedback(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil)

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 17,
		Source:     SourceSuggested,
		Items: []CreateBatchItemInput{{
			CandidateID: 501,
			TaskID:      61,
			DatasetID:   10,
			SnapshotID:  17,
			ItemPayload: map[string]any{
				"overlay": map[string]any{"boxes": []any{map[string]any{"label": "car"}}},
				"diff":    map[string]any{"added": 1, "updated": 0, "removed": 0},
			},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	workspace, err := svc.GetWorkspace(context.Background(), batch.ID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if len(workspace.Items) != 1 {
		t.Fatalf("expected 1 workspace item, got %d", len(workspace.Items))
	}
	if workspace.Items[0].Overlay == nil {
		t.Fatalf("expected overlay metadata, got %+v", workspace.Items[0])
	}
}
```

- [ ] **Step 2: Implement backend workspace query**

Extend `internal/publish/repository.go` and `internal/publish/postgres_repository.go` with a typed workspace payload:

```go
type Workspace struct {
	Batch   Batch           `json:"batch"`
	Items   []WorkspaceItem `json:"items"`
	History []TimelineEntry `json:"history"`
}

type WorkspaceItem struct {
	ItemID      int64          `json:"item_id"`
	CandidateID int64          `json:"candidate_id"`
	TaskID      int64          `json:"task_id"`
	Overlay     map[string]any `json:"overlay"`
	Diff        map[string]any `json:"diff"`
	Context     map[string]any `json:"context"`
	Feedback    []Feedback     `json:"feedback"`
}

func (r *PostgresRepository) BuildWorkspace(ctx context.Context, batchID int64) (Workspace, error) {
	batch, err := r.GetBatch(ctx, batchID)
	if err != nil {
		return Workspace{}, err
	}

	items := make([]WorkspaceItem, 0, len(batch.Items))
	for _, item := range batch.Items {
		overlay, _ := item.ItemPayload["overlay"].(map[string]any)
		diffPayload, _ := item.ItemPayload["diff"].(map[string]any)
		contextPayload, _ := item.ItemPayload["context"].(map[string]any)

		itemFeedback := make([]Feedback, 0)
		for _, entry := range batch.Feedback {
			if entry.Scope == FeedbackScopeItem && entry.PublishBatchItemID == item.ID {
				itemFeedback = append(itemFeedback, entry)
			}
		}

		items = append(items, WorkspaceItem{
			ItemID:      item.ID,
			CandidateID: item.CandidateID,
			TaskID:      item.TaskID,
			Overlay:     overlay,
			Diff:        diffPayload,
			Context:     contextPayload,
			Feedback:    itemFeedback,
		})
	}

	history := make([]TimelineEntry, 0, 4)
	if batch.ReviewApprovedBy != "" && batch.ReviewApprovedAt != nil {
		history = append(history, TimelineEntry{
			Stage:  FeedbackStageReview,
			Actor:  batch.ReviewApprovedBy,
			Action: ReviewDecisionApprove,
			At:     *batch.ReviewApprovedAt,
		})
	}
	if batch.OwnerDecidedBy != "" && batch.OwnerDecidedAt != nil {
		history = append(history, TimelineEntry{
			Stage:  FeedbackStageOwner,
			Actor:  batch.OwnerDecidedBy,
			Action: batch.Status,
			At:     *batch.OwnerDecidedAt,
		})
	}

	return Workspace{
		Batch:   batch,
		Items:   items,
		History: history,
	}, nil
}
```

- [ ] **Step 3: Write the failing Review Workspace page tests**

Create `apps/web/src/features/review/review-workspace.test.tsx`:

```tsx
it("renders overlay, diff stats, and item feedback controls", async () => {
  vi.mocked(global.fetch).mockResolvedValue(
    jsonResponse({
      batch: { id: 71, snapshot_id: 15, status: "owner_pending" },
      items: [
        {
          item_id: 801,
          candidate_id: 401,
          task_id: 51,
          overlay: { boxes: [{ label: "car", x: 0.1, y: 0.2, w: 0.3, h: 0.4 }] },
          diff: { added: 1, updated: 0, removed: 0 },
          feedback: [],
        },
      ],
      history: [{ stage: "review", actor: "reviewer-1", action: "approve" }],
    }),
  );

  renderPage("/review/workspace/71");

  expect(await screen.findByRole("heading", { name: "Review Workspace" })).toBeInTheDocument();
  expect(screen.getByText(/added: 1/i)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Request Rework/i })).toBeInTheDocument();
});
```

- [ ] **Step 4: Implement Review Workspace page**

Create `apps/web/src/features/review/review-workspace-page.tsx`:

```tsx
export function ReviewWorkspacePage() {
  const { batchId = "" } = useParams();
  const workspaceQuery = useQuery({
    queryKey: ["publish-workspace", batchId],
    queryFn: () => getPublishWorkspace(batchId),
  });

  if (workspaceQuery.isLoading) {
    return (
      <section className="page-stack">
        <h1>Review Workspace</h1>
        <p>Loading review workspace.</p>
      </section>
    );
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <section className="page-stack">
        <h1>Review Workspace</h1>
        <p role="alert">Failed to load review workspace.</p>
      </section>
    );
  }

  const workspace = workspaceQuery.data;

  return (
    <section className="page-stack">
      <header className="page-hero">
        <div>
          <p className="page-kicker">Project 1</p>
          <h1>Review Workspace</h1>
          <p className="page-summary">Inspect overlay, diff context, and structured feedback before final publish decisions.</p>
        </div>
      </header>
      <div className="detail-grid">
        <section className="panel">
          <div className="panel-header">
            <h2>Preview And Overlay</h2>
            <span>{workspace.items.length} items</span>
          </div>
          <div className="stack-list">
            {workspace.items.map((item) => (
              <article className="stack-item" key={item.item_id}>
                <strong>Candidate #{item.candidate_id}</strong>
                <span>Task #{item.task_id}</span>
                <span>{JSON.stringify(item.overlay)}</span>
              </article>
            ))}
          </div>
        </section>

        <section className="panel panel-accent">
          <div className="panel-header">
            <h2>Diff And Feedback</h2>
            <span>{workspace.history.length} history events</span>
          </div>
          <div className="stack-list">
            {workspace.items.map((item) => (
              <article className="stack-item" key={`diff-${item.item_id}`}>
                <strong>Item #{item.item_id}</strong>
                <span>added: {item.diff.added ?? 0}</span>
                <span>updated: {item.diff.updated ?? 0}</span>
                <span>removed: {item.diff.removed ?? 0}</span>
                <button type="button">Request Rework</button>
              </article>
            ))}
          </div>
        </section>
      </div>

      <section className="panel">
        <div className="panel-header">
          <h2>Approval History</h2>
          <span>{workspace.history.length}</span>
        </div>
        <div className="stack-list">
          {workspace.history.map((entry, index) => (
            <article className="stack-item" key={`${entry.stage}-${entry.actor}-${index}`}>
              <strong>{entry.stage}</strong>
              <span>{entry.actor}</span>
              <span>{entry.action}</span>
            </article>
          ))}
        </div>
      </section>
    </section>
  );
}
```

- [ ] **Step 5: Add route and links into workspace**

Modify `apps/web/src/app/router.tsx`:

```tsx
{ path: "review/workspace/:batchId", element: <ReviewWorkspacePage /> },
```

Modify `apps/web/src/features/publish/publish-batch-detail-page.tsx` to link:

```tsx
<Link to={`/review/workspace/${batch.id}`}>Open Review Workspace</Link>
```

- [ ] **Step 6: Run workspace-focused frontend and backend tests**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/publish -count=1 -v
cd apps/web && npm test -- src/features/review/review-workspace.test.tsx src/features/publish/publish-pages.test.tsx
```

Expected:

```text
ok  	yolo-ave-mujica/internal/publish
Test Files  2 passed
```

- [ ] **Step 7: Commit Task 4**

```bash
git add internal/publish apps/web/src/features/review/review-workspace-page.tsx apps/web/src/features/review/review-workspace.test.tsx apps/web/src/app/router.tsx apps/web/src/app/styles.css
git commit -m "feat: add publish review workspace"
```

### Task 5: Finalize Feedback Flows, Owner Rework Paths, And End-to-End Validation

**Files:**
- Modify: `internal/publish/service.go`
- Modify: `internal/publish/service_test.go`
- Modify: `internal/publish/handler.go`
- Modify: `internal/publish/handler_test.go`
- Modify: `apps/web/src/features/publish/api.ts`
- Modify: `apps/web/src/features/publish/publish-batch-detail-page.tsx`
- Modify: `apps/web/src/features/review/review-workspace-page.tsx`
- Modify: `apps/web/src/features/publish/publish-pages.test.tsx`
- Modify: `apps/web/src/features/review/review-workspace.test.tsx`

- [ ] **Step 1: Write the failing tests for batch/item feedback and owner rework**

Add to `internal/publish/service_test.go`:

```go
func TestServiceBatchAndItemFeedbackAreStoredSeparately(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, nil)

	batch, err := svc.CreateBatch(context.Background(), CreateBatchInput{
		ProjectID:  1,
		SnapshotID: 18,
		Source:     SourceSuggested,
		Items: []CreateBatchItemInput{{
			CandidateID: 601,
			TaskID:      71,
			DatasetID:   12,
			SnapshotID:  18,
			ItemPayload: map[string]any{"task": map[string]any{"id": 71}},
		}},
	})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	if _, err := svc.AddBatchFeedback(context.Background(), batch.ID, CreateFeedbackInput{
		Stage:           FeedbackStageReview,
		Action:          FeedbackActionRework,
		Scope:           FeedbackScopeBatch,
		ReasonCode:      "coverage_gap",
		Severity:        "high",
		InfluenceWeight: 1.0,
		Comment:         "Need wider sample coverage",
		Actor:           "reviewer-1",
	}); err != nil {
		t.Fatalf("add batch feedback: %v", err)
	}

	if _, err := svc.AddItemFeedback(context.Background(), batch.ID, batch.Items[0].ID, CreateFeedbackInput{
		Stage:           FeedbackStageOwner,
		Action:          FeedbackActionReject,
		Scope:           FeedbackScopeItem,
		ReasonCode:      "trajectory_break",
		Severity:        "critical",
		InfluenceWeight: 1.0,
		Comment:         "Track continuity is broken",
		Actor:           "owner-1",
	}); err != nil {
		t.Fatalf("add item feedback: %v", err)
	}

	got, err := svc.GetBatch(context.Background(), batch.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if len(got.Feedback) != 2 {
		t.Fatalf("expected 2 feedback rows, got %d", len(got.Feedback))
	}
}
```

- [ ] **Step 2: Implement batch/item feedback handlers and owner rework path**

In `internal/publish/handler.go`, implement JSON handlers for:

```go
type feedbackRequest struct {
	Stage           string  `json:"stage"`
	Action          string  `json:"action"`
	ReasonCode      string  `json:"reason_code"`
	Severity        string  `json:"severity"`
	InfluenceWeight float64 `json:"influence_weight"`
	Comment         string  `json:"comment"`
	Actor           string  `json:"actor"`
}

type approvalRequest struct {
	Actor    string                `json:"actor"`
	Feedback []CreateFeedbackInput `json:"feedback"`
}

func validateFeedbackInput(in CreateFeedbackInput, expectedScope string) error {
	if in.Scope != expectedScope {
		return fmt.Errorf("expected scope %q, got %q", expectedScope, in.Scope)
	}
	if (in.Action == FeedbackActionReject || in.Action == FeedbackActionRework) && strings.TrimSpace(in.ReasonCode) == "" {
		return fmt.Errorf("reason_code is required for %s", in.Action)
	}
	if strings.TrimSpace(in.Severity) == "" {
		return fmt.Errorf("severity is required")
	}
	if in.InfluenceWeight <= 0 {
		return fmt.Errorf("influence_weight must be > 0")
	}
	return nil
}

func (h *Handler) AddBatchFeedback(w http.ResponseWriter, r *http.Request) {
	batchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	in := CreateFeedbackInput{
		Scope:           FeedbackScopeBatch,
		Stage:           req.Stage,
		Action:          req.Action,
		ReasonCode:      req.ReasonCode,
		Severity:        req.Severity,
		InfluenceWeight: req.InfluenceWeight,
		Comment:         req.Comment,
		Actor:           req.Actor,
	}
	if err := validateFeedbackInput(in, FeedbackScopeBatch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	feedback, err := h.svc.AddBatchFeedback(r.Context(), batchID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, feedback)
}

func (h *Handler) AddItemFeedback(w http.ResponseWriter, r *http.Request) {
	batchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	in := CreateFeedbackInput{
		Scope:           FeedbackScopeItem,
		Stage:           req.Stage,
		Action:          req.Action,
		ReasonCode:      req.ReasonCode,
		Severity:        req.Severity,
		InfluenceWeight: req.InfluenceWeight,
		Comment:         req.Comment,
		Actor:           req.Actor,
	}
	if err := validateFeedbackInput(in, FeedbackScopeItem); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	feedback, err := h.svc.AddItemFeedback(r.Context(), batchID, itemID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, feedback)
}

func (h *Handler) OwnerRework(w http.ResponseWriter, r *http.Request) {
	batchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req approvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.svc.OwnerRework(r.Context(), batchID, ApprovalInput{Actor: req.Actor, Feedback: req.Feedback}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) ReviewRework(w http.ResponseWriter, r *http.Request) {
	batchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req approvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.svc.ReviewRework(r.Context(), batchID, ApprovalInput{Actor: req.Actor, Feedback: req.Feedback}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
```

- [ ] **Step 3: Add frontend feedback forms and action wiring**

Modify `apps/web/src/features/publish/publish-batch-detail-page.tsx` and `apps/web/src/features/review/review-workspace-page.tsx` to submit both batch-level and item-level feedback:

```tsx
const [batchFeedback, setBatchFeedback] = useState<CreateFeedbackPayload>({
  stage: "owner",
  action: "rework",
  reason_code: "coverage_gap",
  severity: "high",
  influence_weight: 1,
  comment: "",
  actor: "owner-1",
});

const addBatchFeedbackMutation = useMutation({
  mutationFn: (payload: CreateFeedbackPayload) => addBatchFeedback(batchId, payload),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: ["publish-batch", batchId] });
    await queryClient.invalidateQueries({ queryKey: ["publish-workspace", batchId] });
  },
});

const addItemFeedbackMutation = useMutation({
  mutationFn: ({ itemId, payload }: { itemId: number; payload: CreateFeedbackPayload }) =>
    addItemFeedback(batchId, itemId, payload),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: ["publish-batch", batchId] });
    await queryClient.invalidateQueries({ queryKey: ["publish-workspace", batchId] });
  },
});

<form
  className="task-form"
  onSubmit={(event) => {
    event.preventDefault();
    void addBatchFeedbackMutation.mutateAsync(batchFeedback);
  }}
>
  <label>
    Reason code
    <select
      value={batchFeedback.reason_code}
      onChange={(event) => setBatchFeedback((current) => ({ ...current, reason_code: event.target.value }))}
    >
      <option value="coverage_gap">coverage_gap</option>
      <option value="trajectory_break">trajectory_break</option>
    </select>
  </label>
  <label>
    Severity
    <select
      value={batchFeedback.severity}
      onChange={(event) => setBatchFeedback((current) => ({ ...current, severity: event.target.value }))}
    >
      <option value="high">high</option>
      <option value="critical">critical</option>
    </select>
  </label>
  <label>
    Influence weight
    <input
      type="number"
      min={0.1}
      step={0.1}
      value={batchFeedback.influence_weight}
      onChange={(event) =>
        setBatchFeedback((current) => ({ ...current, influence_weight: Number(event.target.value) }))
      }
    />
  </label>
  <label>
    Comment
    <input
      value={batchFeedback.comment ?? ""}
      onChange={(event) => setBatchFeedback((current) => ({ ...current, comment: event.target.value }))}
    />
  </label>
  <button type="submit">Submit Batch Feedback</button>
</form>
```

- [ ] **Step 4: Add end-to-end page assertions**

Update `apps/web/src/features/publish/publish-pages.test.tsx` and `apps/web/src/features/review/review-workspace.test.tsx` with assertions like:

```tsx
it("posts owner rework feedback and refreshes the batch view", async () => {
  const fetchMock = vi.mocked(global.fetch);
  fetchMock
    .mockResolvedValueOnce(jsonResponse({
      id: 71,
      snapshot_id: 15,
      status: "owner_pending",
      items: [{ id: 801, candidate_id: 401, task_id: 51, dataset_id: 9, snapshot_id: 15, item_payload: {} }],
      feedback: [{ id: 1, scope: "batch", stage: "review", action: "comment", reason_code: "ready", severity: "low", influence_weight: 1, comment: "" }],
    }))
    .mockResolvedValueOnce(jsonResponse({
      id: 2,
      scope: "batch",
      stage: "owner",
      action: "rework",
      reason_code: "coverage_gap",
      severity: "high",
      influence_weight: 1,
      comment: "Need wider sample coverage",
    }))
    .mockResolvedValueOnce(jsonResponse({
      id: 71,
      snapshot_id: 15,
      status: "owner_pending",
      items: [{ id: 801, candidate_id: 401, task_id: 51, dataset_id: 9, snapshot_id: 15, item_payload: {} }],
      feedback: [
        { id: 1, scope: "batch", stage: "review", action: "comment", reason_code: "ready", severity: "low", influence_weight: 1, comment: "" },
        { id: 2, scope: "batch", stage: "owner", action: "rework", reason_code: "coverage_gap", severity: "high", influence_weight: 1, comment: "Need wider sample coverage" },
      ],
    }));

  renderPage("/publish/batches/71");
  await userEvent.type(await screen.findByLabelText(/Comment/i), "Need wider sample coverage");
  await userEvent.click(screen.getByRole("button", { name: /Submit Batch Feedback/i }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/v1/publish/batches/71/feedback", expect.any(Object)));
  expect(await screen.findByText(/Need wider sample coverage/i)).toBeInTheDocument();
});
```

- [ ] **Step 5: Run full slice verification**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/publish ./internal/review ./internal/tasks ./internal/server ./cmd/api-server -count=1 -v
cd apps/web && npm test
cd apps/web && npm run build
```

Expected:

```text
ok  	yolo-ave-mujica/internal/publish
ok  	yolo-ave-mujica/internal/review
ok  	yolo-ave-mujica/internal/tasks
ok  	yolo-ave-mujica/internal/server
ok  	yolo-ave-mujica/cmd/api-server
Test Files  all passed
vite build completed successfully
```

- [ ] **Step 6: Commit Task 5**

```bash
git add internal/publish apps/web/src/features/publish apps/web/src/features/review
git commit -m "feat: finalize publish feedback workflow"
```

## Self-Review

### Spec Coverage

This plan covers every major spec requirement:

1. `PublishBatch` / `PublishBatchItem` / `PublishFeedback` / `PublishRecord`
2. Reviewer + Owner 双审批
3. Owner 修改内容导致 Reviewer 审批失效
4. batch/item 双层结构化反馈
5. Suggested publish candidates
6. `Review Queue`
7. `Publish Candidates`
8. `Publish Batch Detail`
9. `Review Workspace`
10. publish record 后自动创建 downstream task
11. suggestions are rule-grouped by `snapshot + risk + source model + accepted window`

### Placeholder Scan

The plan avoids `TBD` / `TODO` placeholders and includes:

1. exact file paths
2. concrete route names
3. concrete test targets
4. concrete verification commands
5. no remaining file-path ambiguity or placeholder comments

### Type Consistency

The plan uses these stable type families consistently:

1. `Batch`, `BatchItem`, `Feedback`, `Record`
2. `CreateBatchInput`, `CreateBatchItemInput`, `CreateFeedbackInput`
3. `ReviewDecisionApprove|Reject|Rework`
4. `OwnerDecisionApprove|Reject|Rework`
5. `tasks.KindTrainingCandidate`, `tasks.KindPromotionReview`
6. `PublishableCandidate`, `SuggestedPublishGroup`, `Workspace`, `TimelineEntry`
