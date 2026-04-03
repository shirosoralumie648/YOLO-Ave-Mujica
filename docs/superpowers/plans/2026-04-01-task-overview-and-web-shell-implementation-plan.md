# Task Overview And Web Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first real V1 product entry by adding a task kernel, project overview aggregation, a Vite web shell, a live `Task Overview` page, and a filtered `Task List` page.

**Architecture:** Keep the existing Go modular monolith and add two focused backend modules: `internal/tasks` for durable task resources and `internal/overview` for page-oriented aggregation over tasks, review backlog, and failed jobs. Add a separate `apps/web` Vite + React + TypeScript frontend that consumes the new project-scoped APIs, keeps context in URL search params, and stops short of workspace or `Task Detail` scope.

**Tech Stack:** Go 1.20, Chi, pgx/v5, PostgreSQL, Vite, React, TypeScript, React Router, TanStack Query, Vitest, Testing Library, npm.

---

## File Structure

### Backend

**Create**

- `migrations/000003_task_overview_kernel.up.sql`
- `migrations/000003_task_overview_kernel.down.sql`
- `internal/tasks/model.go`
- `internal/tasks/repository.go`
- `internal/tasks/postgres_repository.go`
- `internal/tasks/service.go`
- `internal/tasks/service_test.go`
- `internal/tasks/handler.go`
- `internal/tasks/handler_test.go`
- `internal/tasks/postgres_repository_test.go`
- `internal/overview/service.go`
- `internal/overview/handler.go`
- `internal/overview/service_test.go`

**Modify**

- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `cmd/api-server/main.go`
- `cmd/api-server/main_test.go`
- `internal/review/postgres_repository.go`
- `internal/jobs/postgres_repository.go`

### Frontend

**Create**

- `apps/web/package.json`
- `apps/web/package-lock.json`
- `apps/web/tsconfig.json`
- `apps/web/tsconfig.node.json`
- `apps/web/vite.config.ts`
- `apps/web/index.html`
- `apps/web/src/main.tsx`
- `apps/web/src/app/router.tsx`
- `apps/web/src/app/query-client.ts`
- `apps/web/src/app/layout/app-shell.tsx`
- `apps/web/src/app/styles.css`
- `apps/web/src/features/shared/http.ts`
- `apps/web/src/features/overview/api.ts`
- `apps/web/src/features/overview/task-overview-page.tsx`
- `apps/web/src/features/overview/task-overview-page.test.tsx`
- `apps/web/src/features/tasks/api.ts`
- `apps/web/src/features/tasks/task-list-page.tsx`
- `apps/web/src/features/tasks/task-list-page.test.tsx`

### Docs And Local Dev

**Modify**

- `Makefile`
- `README.md`
- `README.zh-CN.md`
- `docs/development/local-quickstart.md`
- `docs/development/local-quickstart.zh-CN.md`

## Scope Guardrails

This plan does **not** include:

1. `Task Detail` UI
2. annotation workspace
3. review workspace
4. publish gate semantics
5. training and evaluation resources
6. auth or RBAC

This plan **does** include:

1. first-class `Task` persistence
2. overview aggregation for a task-first home page
3. live `Task Overview` as the default frontend route
4. a minimal create-task flow
5. a minimal `Task List` page with URL filters

## Frontend Route Rule

For this slice, the browser routes stay product-first and project-light:

1. `/` renders `Task Overview`
2. `/tasks` renders `Task List`
3. backend API calls still use `project_id = 1` until a real project switcher exists

## Task 1: Add Task Schema, Model, Repository, And Service

**Files:**

- Create: `migrations/000003_task_overview_kernel.up.sql`
- Create: `migrations/000003_task_overview_kernel.down.sql`
- Create: `internal/tasks/model.go`
- Create: `internal/tasks/repository.go`
- Create: `internal/tasks/postgres_repository.go`
- Create: `internal/tasks/service.go`
- Create: `internal/tasks/service_test.go`
- Create: `internal/tasks/postgres_repository_test.go`

- [ ] **Step 1: Write the failing service tests**

Create `internal/tasks/service_test.go`:

```go
package tasks

import "testing"

func TestServiceCreateListAndGetTask(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)

	task, err := svc.CreateTask(CreateTaskInput{
		ProjectID: 1,
		Title:     "Label loading-dock night batch",
		Assignee:  "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID != 1 {
		t.Fatalf("expected task id 1, got %d", task.ID)
	}
	if task.Kind != KindAnnotation || task.Status != StatusQueued || task.Priority != PriorityNormal {
		t.Fatalf("expected defaults to be applied, got %+v", task)
	}

	items, err := svc.ListTasks(1, ListTasksFilter{Assignee: "annotator-1"})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Label loading-dock night batch" {
		t.Fatalf("unexpected task list: %+v", items)
	}

	got, err := svc.GetTask(task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ID != task.ID || got.Assignee != "annotator-1" {
		t.Fatalf("unexpected task: %+v", got)
	}
}

func TestServiceRequiresBlockerReasonForBlockedTasks(t *testing.T) {
	svc := NewService(NewInMemoryRepository())

	_, err := svc.CreateTask(CreateTaskInput{
		ProjectID: 1,
		Title:     "Blocked review batch",
		Status:    StatusBlocked,
	})
	if err == nil {
		t.Fatal("expected blocked task creation to fail without blocker reason")
	}
}
```

Create `internal/tasks/postgres_repository_test.go`:

```go
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
		SnapshotID: &snapshotID,
		Title:      "Review imported loading-dock batch",
		Kind:       KindReview,
		Status:     StatusReady,
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
	if got.SnapshotVersion != "v1" {
		t.Fatalf("expected snapshot version context, got %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewInMemoryRepository
FAIL ... undefined: NewService
FAIL ... undefined: NewPostgresRepository
```

- [ ] **Step 3: Add the migration, model, repository, and service**

Create `migrations/000003_task_overview_kernel.up.sql`:

```sql
create table tasks (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  snapshot_id bigint references dataset_snapshots(id),
  title text not null,
  kind text not null,
  status text not null,
  priority text not null default 'normal',
  assignee text not null default '',
  due_at timestamptz,
  blocker_reason text not null default '',
  last_activity_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint tasks_kind_check check (kind in ('annotation','review','qa','ops')),
  constraint tasks_status_check check (status in ('queued','ready','in_progress','blocked','done')),
  constraint tasks_priority_check check (priority in ('low','normal','high','critical'))
);

create index idx_tasks_project_status on tasks(project_id, status);
create index idx_tasks_project_activity on tasks(project_id, last_activity_at);
```

Create `migrations/000003_task_overview_kernel.down.sql`:

```sql
drop index if exists idx_tasks_project_activity;
drop index if exists idx_tasks_project_status;
drop table if exists tasks;
```

Create `internal/tasks/model.go`:

```go
package tasks

import "time"

const (
	KindAnnotation = "annotation"
	KindReview     = "review"
	KindQA         = "qa"
	KindOps        = "ops"

	StatusQueued     = "queued"
	StatusReady      = "ready"
	StatusInProgress = "in_progress"
	StatusBlocked    = "blocked"
	StatusDone       = "done"

	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

type Task struct {
	ID              int64      `json:"id"`
	ProjectID       int64      `json:"project_id"`
	SnapshotID      *int64     `json:"snapshot_id,omitempty"`
	SnapshotVersion string     `json:"snapshot_version,omitempty"`
	DatasetID       int64      `json:"dataset_id,omitempty"`
	DatasetName     string     `json:"dataset_name,omitempty"`
	Title           string     `json:"title"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	Assignee        string     `json:"assignee"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	BlockerReason   string     `json:"blocker_reason,omitempty"`
	LastActivityAt  time.Time  `json:"last_activity_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type CreateTaskInput struct {
	ProjectID     int64      `json:"-"`
	SnapshotID    *int64     `json:"snapshot_id,omitempty"`
	Title         string     `json:"title"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Priority      string     `json:"priority"`
	Assignee      string     `json:"assignee"`
	DueAt         *time.Time `json:"due_at,omitempty"`
	BlockerReason string     `json:"blocker_reason,omitempty"`
}

type ListTasksFilter struct {
	Status     string
	Kind       string
	Assignee   string
	Priority   string
	SnapshotID *int64
}
```

Create `internal/tasks/repository.go`:

```go
package tasks

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Repository interface {
	CreateTask(ctx context.Context, in CreateTaskInput) (Task, error)
	ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error)
	GetTask(ctx context.Context, taskID int64) (Task, error)
}

type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]Task
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		nextID: 1,
		items:  map[int64]Task{},
	}
}

func (r *InMemoryRepository) CreateTask(_ context.Context, in CreateTaskInput) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	task := Task{
		ID:             r.nextID,
		ProjectID:      in.ProjectID,
		SnapshotID:     in.SnapshotID,
		Title:          in.Title,
		Kind:           in.Kind,
		Status:         in.Status,
		Priority:       in.Priority,
		Assignee:       in.Assignee,
		DueAt:          in.DueAt,
		BlockerReason:  in.BlockerReason,
		LastActivityAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	r.items[task.ID] = task
	r.nextID++
	return task, nil
}

func (r *InMemoryRepository) ListTasks(_ context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := []Task{}
	for _, task := range r.items {
		if task.ProjectID != projectID {
			continue
		}
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if filter.Kind != "" && task.Kind != filter.Kind {
			continue
		}
		if filter.Assignee != "" && task.Assignee != filter.Assignee {
			continue
		}
		if filter.Priority != "" && task.Priority != filter.Priority {
			continue
		}
		if filter.SnapshotID != nil {
			if task.SnapshotID == nil || *task.SnapshotID != *filter.SnapshotID {
				continue
			}
		}
		out = append(out, task)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastActivityAt.Equal(out[j].LastActivityAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].LastActivityAt.Before(out[j].LastActivityAt)
	})
	return out, nil
}

func (r *InMemoryRepository) GetTask(_ context.Context, taskID int64) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.items[taskID]
	if !ok {
		return Task{}, fmt.Errorf("task %d not found", taskID)
	}
	return task, nil
}
```

Create `internal/tasks/service.go`:

```go
package tasks

import (
	"context"
	"errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	if repo == nil {
		repo = NewInMemoryRepository()
	}
	return &Service{repo: repo}
}

func (s *Service) CreateTask(in CreateTaskInput) (Task, error) {
	if in.ProjectID <= 0 {
		return Task{}, errors.New("project_id must be > 0")
	}
	if in.Title == "" {
		return Task{}, errors.New("title is required")
	}
	if in.Kind == "" {
		in.Kind = KindAnnotation
	}
	if in.Status == "" {
		in.Status = StatusQueued
	}
	if in.Priority == "" {
		in.Priority = PriorityNormal
	}
	if in.Status == StatusBlocked && in.BlockerReason == "" {
		return Task{}, errors.New("blocker_reason is required when status=blocked")
	}
	return s.repo.CreateTask(context.Background(), in)
}

func (s *Service) ListTasks(projectID int64, filter ListTasksFilter) ([]Task, error) {
	if projectID <= 0 {
		return nil, errors.New("project_id must be > 0")
	}
	return s.repo.ListTasks(context.Background(), projectID, filter)
}

func (s *Service) GetTask(taskID int64) (Task, error) {
	if taskID <= 0 {
		return Task{}, errors.New("task id must be > 0")
	}
	return s.repo.GetTask(context.Background(), taskID)
}
```

Create `internal/tasks/postgres_repository.go`:

```go
package tasks

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateTask(ctx context.Context, in CreateTaskInput) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		insert into tasks (
			project_id, snapshot_id, title, kind, status, priority, assignee, due_at, blocker_reason
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning id, project_id, snapshot_id, title, kind, status, priority, assignee,
		          due_at, blocker_reason, last_activity_at, created_at, updated_at
	`, in.ProjectID, in.SnapshotID, in.Title, in.Kind, in.Status, in.Priority, in.Assignee, in.DueAt, in.BlockerReason)
	task, err := scanTask(row)
	if err != nil {
		return Task{}, err
	}
	return r.loadTask(ctx, task.ID)
}

func (r *PostgresRepository) ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	rows, err := r.pool.Query(ctx, `
		select
			t.id, t.project_id, t.snapshot_id, t.title, t.kind, t.status, t.priority, t.assignee,
			t.due_at, t.blocker_reason, t.last_activity_at, t.created_at, t.updated_at,
			coalesce(s.version, ''), coalesce(d.id, 0), coalesce(d.name, '')
		from tasks t
		left join dataset_snapshots s on s.id = t.snapshot_id
		left join datasets d on d.id = s.dataset_id
		where t.project_id = $1
		  and ($2 = '' or t.status = $2)
		  and ($3 = '' or t.kind = $3)
		  and ($4 = '' or t.assignee = $4)
		  and ($5 = '' or t.priority = $5)
		  and ($6::bigint is null or t.snapshot_id = $6)
		order by t.last_activity_at asc, t.id asc
	`, projectID, filter.Status, filter.Kind, filter.Assignee, filter.Priority, filter.SnapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Task{}
	for rows.Next() {
		task, err := scanTaskWithContext(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetTask(ctx context.Context, taskID int64) (Task, error) {
	return r.loadTask(ctx, taskID)
}

func (r *PostgresRepository) loadTask(ctx context.Context, taskID int64) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		select
			t.id, t.project_id, t.snapshot_id, t.title, t.kind, t.status, t.priority, t.assignee,
			t.due_at, t.blocker_reason, t.last_activity_at, t.created_at, t.updated_at,
			coalesce(s.version, ''), coalesce(d.id, 0), coalesce(d.name, '')
		from tasks t
		left join dataset_snapshots s on s.id = t.snapshot_id
		left join datasets d on d.id = s.dataset_id
		where t.id = $1
	`, taskID)
	return scanTaskWithContext(row)
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (Task, error) {
	var out Task
	var snapshotID sql.NullInt64
	var dueAt sql.NullTime
	if err := scanner.Scan(
		&out.ID,
		&out.ProjectID,
		&snapshotID,
		&out.Title,
		&out.Kind,
		&out.Status,
		&out.Priority,
		&out.Assignee,
		&dueAt,
		&out.BlockerReason,
		&out.LastActivityAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return Task{}, err
	}
	if snapshotID.Valid {
		value := snapshotID.Int64
		out.SnapshotID = &value
	}
	if dueAt.Valid {
		value := dueAt.Time
		out.DueAt = &value
	}
	return out, nil
}

func scanTaskWithContext(scanner taskScanner) (Task, error) {
	var out Task
	var snapshotID sql.NullInt64
	var dueAt sql.NullTime
	if err := scanner.Scan(
		&out.ID,
		&out.ProjectID,
		&snapshotID,
		&out.Title,
		&out.Kind,
		&out.Status,
		&out.Priority,
		&out.Assignee,
		&dueAt,
		&out.BlockerReason,
		&out.LastActivityAt,
		&out.CreatedAt,
		&out.UpdatedAt,
		&out.SnapshotVersion,
		&out.DatasetID,
		&out.DatasetName,
	); err != nil {
		return Task{}, err
	}
	if snapshotID.Valid {
		value := snapshotID.Int64
		out.SnapshotID = &value
	}
	if dueAt.Valid {
		value := dueAt.Time
		out.DueAt = &value
	}
	return out, nil
}
```

- [ ] **Step 4: Run the task tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks -count=1 -v
```

Expected:

```text
=== RUN   TestServiceCreateListAndGetTask
=== RUN   TestServiceRequiresBlockerReasonForBlockedTasks
PASS
```

- [ ] **Step 5: Commit**

```bash
git add migrations/000003_task_overview_kernel.up.sql migrations/000003_task_overview_kernel.down.sql internal/tasks/model.go internal/tasks/repository.go internal/tasks/postgres_repository.go internal/tasks/service.go internal/tasks/service_test.go internal/tasks/postgres_repository_test.go
git commit -m "feat: add task kernel backend"
```

## Task 2: Add Task HTTP Handlers And Overview Aggregation

**Files:**

- Create: `internal/tasks/handler.go`
- Create: `internal/tasks/handler_test.go`
- Create: `internal/overview/service.go`
- Create: `internal/overview/handler.go`
- Create: `internal/overview/service_test.go`
- Modify: `internal/review/postgres_repository.go`
- Modify: `internal/jobs/postgres_repository.go`

- [ ] **Step 1: Write the failing task handler and overview tests**

Create `internal/tasks/handler_test.go`:

```go
package tasks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandlerCreateListAndGetTask(t *testing.T) {
	svc := NewService(NewInMemoryRepository())
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/projects/1/tasks", strings.NewReader(`{
		"title":"Label loading-dock batch",
		"kind":"annotation",
		"priority":"high",
		"assignee":"annotator-1"
	}`))
	createCtx := chi.NewRouteContext()
	createCtx.URLParams.Add("id", "1")
	createReq = createReq.WithContext(context.WithValue(createReq.Context(), chi.RouteCtxKey, createCtx))
	createRec := httptest.NewRecorder()
	h.CreateTask(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/projects/1/tasks?assignee=annotator-1", nil)
	listCtx := chi.NewRouteContext()
	listCtx.URLParams.Add("id", "1")
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), chi.RouteCtxKey, listCtx))
	listRec := httptest.NewRecorder()
	h.ListTasks(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), `"Label loading-dock batch"`) {
		t.Fatalf("expected created task in list response, got %d %s", listRec.Code, listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/1", nil)
	getCtx := chi.NewRouteContext()
	getCtx.URLParams.Add("id", "1")
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, getCtx))
	getRec := httptest.NewRecorder()
	h.GetTask(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"annotator-1"`) {
		t.Fatalf("expected created task in get response, got %d %s", getRec.Code, getRec.Body.String())
	}
}
```

Create `internal/overview/service_test.go`:

```go
package overview

import (
	"testing"
	"time"

	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/tasks"
)

type fakeTaskSource struct {
	items []tasks.Task
}

func (f fakeTaskSource) ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error) {
	return f.items, nil
}

type fakeReviewSource struct {
	count int
}

func (f fakeReviewSource) PendingCandidateCount(projectID int64) (int, error) {
	return f.count, nil
}

type fakeJobSource struct {
	items []jobs.Job
}

func (f fakeJobSource) ListRecentFailedJobs(projectID int64, limit int) ([]jobs.Job, error) {
	return f.items, nil
}

func TestServiceBuildOverviewAggregatesCardsAndBlockers(t *testing.T) {
	old := time.Now().UTC().Add(-4 * time.Hour)
	service := NewService(
		fakeTaskSource{items: []tasks.Task{
			{ID: 1, ProjectID: 1, Title: "Blocked review batch", Kind: tasks.KindReview, Status: tasks.StatusBlocked, BlockerReason: "schema mismatch", LastActivityAt: old},
			{ID: 2, ProjectID: 1, Title: "Queued annotation batch", Kind: tasks.KindAnnotation, Status: tasks.StatusQueued, LastActivityAt: time.Now().UTC()},
		}},
		fakeReviewSource{count: 3},
		fakeJobSource{items: []jobs.Job{
			{ID: 9, ProjectID: 1, JobType: "zero-shot", Status: jobs.StatusFailed, ErrorMsg: "provider unavailable"},
		}},
	)

	out, err := service.BuildOverview(1)
	if err != nil {
		t.Fatalf("build overview: %v", err)
	}
	if len(out.SummaryCards) != 4 {
		t.Fatalf("expected 4 summary cards, got %+v", out.SummaryCards)
	}
	if out.LongestIdleTask == nil || out.LongestIdleTask.ID != 1 {
		t.Fatalf("expected longest idle task id 1, got %+v", out.LongestIdleTask)
	}
	if len(out.Blockers) < 2 {
		t.Fatalf("expected blocked-task and review backlog blockers, got %+v", out.Blockers)
	}
	if len(out.RecentFailedJobs) != 1 || out.RecentFailedJobs[0].ID != 9 {
		t.Fatalf("expected one recent failed job, got %+v", out.RecentFailedJobs)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewHandler
FAIL ... undefined: NewService
```

- [ ] **Step 3: Add the handlers, overview service, and repo query helpers**

Create `internal/tasks/handler.go`:

```go
package tasks

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in CreateTaskInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	in.ProjectID = projectID

	task, err := h.svc.CreateTask(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var snapshotID *int64
	if raw := r.URL.Query().Get("snapshot_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		snapshotID = &value
	}

	items, err := h.svc.ListTasks(projectID, ListTasksFilter{
		Status:     r.URL.Query().Get("status"),
		Kind:       r.URL.Query().Get("kind"),
		Assignee:   r.URL.Query().Get("assignee"),
		Priority:   r.URL.Query().Get("priority"),
		SnapshotID: snapshotID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	task, err := h.svc.GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
```

Create `internal/overview/service.go`:

```go
package overview

import (
	"fmt"

	"yolo-ave-mujica/internal/jobs"
	"yolo-ave-mujica/internal/tasks"
)

type TaskSource interface {
	ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error)
}

type ReviewSource interface {
	PendingCandidateCount(projectID int64) (int, error)
}

type JobSource interface {
	ListRecentFailedJobs(projectID int64, limit int) ([]jobs.Job, error)
}

type SummaryCard struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
	Href  string `json:"href"`
}

type BlockerCard struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Href   string `json:"href"`
}

type FailedJobItem struct {
	ID       int64  `json:"id"`
	JobType  string `json:"job_type"`
	Status   string `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

type Response struct {
	SummaryCards    []SummaryCard   `json:"summary_cards"`
	Blockers        []BlockerCard   `json:"blockers"`
	LongestIdleTask *tasks.Task     `json:"longest_idle_task,omitempty"`
	RecentFailedJobs []FailedJobItem `json:"recent_failed_jobs"`
}

type Service struct {
	tasks  TaskSource
	review ReviewSource
	jobs   JobSource
}

func NewService(taskSource TaskSource, reviewSource ReviewSource, jobSource JobSource) *Service {
	return &Service{tasks: taskSource, review: reviewSource, jobs: jobSource}
}

func (s *Service) BuildOverview(projectID int64) (Response, error) {
	taskItems, err := s.tasks.ListTasks(projectID, tasks.ListTasksFilter{})
	if err != nil {
		return Response{}, err
	}
	reviewBacklog, err := s.review.PendingCandidateCount(projectID)
	if err != nil {
		return Response{}, err
	}
	failedJobs, err := s.jobs.ListRecentFailedJobs(projectID, 5)
	if err != nil {
		return Response{}, err
	}

	var (
		openCount    int
		blockedCount int
		longestIdle  *tasks.Task
		blockers     []BlockerCard
	)

	for i := range taskItems {
		task := taskItems[i]
		if task.Status != tasks.StatusDone {
			openCount++
		}
		if task.Status == tasks.StatusBlocked {
			blockedCount++
			blockers = append(blockers, BlockerCard{
				ID:     fmt.Sprintf("task-%d", task.ID),
				Title:  task.Title,
				Reason: task.BlockerReason,
				Href:   "/tasks?status=blocked",
			})
		}
		if task.Status != tasks.StatusDone && (longestIdle == nil || task.LastActivityAt.Before(longestIdle.LastActivityAt)) {
			copyTask := task
			longestIdle = &copyTask
		}
	}

	if reviewBacklog > 0 {
		blockers = append(blockers, BlockerCard{
			ID:     "review-backlog",
			Title:  "Review backlog requires action",
			Reason: fmt.Sprintf("%d pending candidates remain in review", reviewBacklog),
			Href:   "/tasks?kind=review&status=ready",
		})
	}

	summaryCards := []SummaryCard{
		{ID: "open-tasks", Title: "Open Tasks", Count: openCount, Href: "/tasks"},
		{ID: "blocked-tasks", Title: "Blocked Tasks", Count: blockedCount, Href: "/tasks?status=blocked"},
		{ID: "review-backlog", Title: "Review Backlog", Count: reviewBacklog, Href: "/tasks?kind=review&status=ready"},
		{ID: "failed-jobs", Title: "Failed Jobs", Count: len(failedJobs), Href: "/tasks?status=blocked"},
	}

	recent := make([]FailedJobItem, 0, len(failedJobs))
	for _, job := range failedJobs {
		blockers = append(blockers, BlockerCard{
			ID:     fmt.Sprintf("job-%d", job.ID),
			Title:  fmt.Sprintf("Failed job: %s", job.JobType),
			Reason: job.ErrorMsg,
			Href:   "/tasks?status=blocked",
		})
		recent = append(recent, FailedJobItem{
			ID:       job.ID,
			JobType:  job.JobType,
			Status:   job.Status,
			ErrorMsg: job.ErrorMsg,
		})
	}

	return Response{
		SummaryCards:    summaryCards,
		Blockers:        blockers,
		LongestIdleTask: longestIdle,
		RecentFailedJobs: recent,
	}, nil
}
```

Create `internal/overview/handler.go`:

```go
package overview

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetProjectOverview(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	out, err := h.svc.BuildOverview(projectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
```

Append to `internal/review/postgres_repository.go`:

```go
func (r *PostgresRepository) PendingCandidateCount(projectID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(context.Background(), `
		select count(*)
		from annotation_candidates c
		join datasets d on d.id = c.dataset_id
		where d.project_id = $1 and c.review_status = 'pending'
	`, projectID).Scan(&count)
	return count, err
}
```

Append to `internal/jobs/postgres_repository.go`:

```go
func (r *PostgresRepository) ListRecentFailedJobs(projectID int64, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := r.pool.Query(context.Background(), `
		select id, project_id, dataset_id, snapshot_id, job_type, status, required_resource_type,
		       required_capabilities_json, idempotency_key, worker_id, payload_json, total_items,
		       succeeded_items, failed_items, created_at, started_at, finished_at, lease_until,
		       retry_count, error_code, error_msg
		from jobs
		where project_id = $1 and status in ('failed', 'retry_waiting')
		order by coalesce(finished_at, created_at) desc
		limit $2
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *job)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview -count=1 -v
```

Expected:

```text
=== RUN   TestHandlerCreateListAndGetTask
=== RUN   TestServiceBuildOverviewAggregatesCardsAndBlockers
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/tasks/handler.go internal/tasks/handler_test.go internal/overview/service.go internal/overview/handler.go internal/overview/service_test.go internal/review/postgres_repository.go internal/jobs/postgres_repository.go
git commit -m "feat: add overview aggregation and task handlers"
```

## Task 3: Wire Task And Overview Routes Into The API Server

**Files:**

- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/main_test.go`

- [ ] **Step 1: Write the failing route-registration tests**

Update `internal/server/http_server_routes_test.go` route list:

```go
		{http.MethodGet, "/v1/projects/1/overview"},
		{http.MethodGet, "/v1/projects/1/tasks"},
		{http.MethodPost, "/v1/projects/1/tasks"},
		{http.MethodGet, "/v1/tasks/1"},
```

Append to `cmd/api-server/main_test.go`:

```go
func TestNewTestModulesLeavesTaskAndOverviewRoutesUnwired(t *testing.T) {
	srv := server.NewHTTPServerWithModules(newTestModules())

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/1/overview", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 before task and overview wiring, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server ./cmd/api-server -count=1 -v
```

Expected:

```text
FAIL ... route missing: GET /v1/projects/1/overview
```

- [ ] **Step 3: Add route groups and runtime wiring**

Update `internal/server/http_server.go`:

```go
type TaskRoutes struct {
	ListTasks  http.HandlerFunc
	CreateTask http.HandlerFunc
	GetTask    http.HandlerFunc
}

type OverviewRoutes struct {
	GetProjectOverview http.HandlerFunc
}

type Modules struct {
	DataHub     DataHubRoutes
	Jobs        JobRoutes
	Versioning  VersioningRoutes
	Review      ReviewRoutes
	Artifacts   ArtifactRoutes
	Tasks       TaskRoutes
	Overview    OverviewRoutes
	ReadyChecks []ReadyCheck
}
```

Add routes inside `/v1`:

```go
		r.Get("/projects/{id}/overview", handlerOrNotImplemented(m.Overview.GetProjectOverview))
		r.Get("/projects/{id}/tasks", handlerOrNotImplemented(m.Tasks.ListTasks))
		r.Post("/projects/{id}/tasks", handlerOrNotImplemented(m.Tasks.CreateTask))
		r.Get("/tasks/{id}", handlerOrNotImplemented(m.Tasks.GetTask))
```

Update `cmd/api-server/main.go` imports and wiring:

```go
import (
	// existing imports
	"yolo-ave-mujica/internal/overview"
	"yolo-ave-mujica/internal/tasks"
)
```

Add module construction:

```go
	taskRepo := tasks.NewPostgresRepository(pool)
	taskSvc := tasks.NewService(taskRepo)
	taskHandler := tasks.NewHandler(taskSvc)

	overviewSvc := overview.NewService(taskSvc, review.NewPostgresRepository(pool), jobsRepo)
	overviewHandler := overview.NewHandler(overviewSvc)
```

Append route wiring:

```go
	modules.Tasks = server.TaskRoutes{
		ListTasks:  taskHandler.ListTasks,
		CreateTask: taskHandler.CreateTask,
		GetTask:    taskHandler.GetTask,
	}
	modules.Overview = server.OverviewRoutes{
		GetProjectOverview: overviewHandler.GetProjectOverview,
	}
```

- [ ] **Step 4: Run the server tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server ./cmd/api-server -count=1 -v
```

Expected:

```text
=== RUN   TestMVPRoutesAreRegistered
=== RUN   TestNewTestModulesLeavesTaskAndOverviewRoutesUnwired
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/http_server.go internal/server/http_server_routes_test.go cmd/api-server/main.go cmd/api-server/main_test.go
git commit -m "feat: expose task and overview api routes"
```

## Task 4: Bootstrap `apps/web` And The Application Shell

**Files:**

- Create: `apps/web/package.json`
- Create: `apps/web/package-lock.json`
- Create: `apps/web/tsconfig.json`
- Create: `apps/web/tsconfig.node.json`
- Create: `apps/web/vite.config.ts`
- Create: `apps/web/index.html`
- Create: `apps/web/src/main.tsx`
- Create: `apps/web/src/app/router.tsx`
- Create: `apps/web/src/app/query-client.ts`
- Create: `apps/web/src/app/layout/app-shell.tsx`
- Create: `apps/web/src/app/styles.css`
- Create: `apps/web/src/features/tasks/task-list-page.test.tsx`

- [ ] **Step 1: Write the failing app-shell test**

Create `apps/web/src/features/tasks/task-list-page.test.tsx`:

```tsx
import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppShell } from "../../app/layout/app-shell";

describe("AppShell", () => {
  it("renders navigation links for overview and tasks", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<AppShell />}>
            <Route index element={<div>Overview Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: /overview/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /tasks/i })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the frontend test to verify it fails**

Run:

```bash
cd apps/web && npm test
```

Expected:

```text
npm ERR! missing script: test
```

- [ ] **Step 3: Add the web toolchain and shell**

Create `apps/web/package.json`:

```json
{
  "name": "@yolo/web",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "vitest run"
  },
  "dependencies": {
    "@tanstack/react-query": "^5.66.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "react-router-dom": "^7.3.0"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.6.3",
    "@testing-library/react": "^16.2.0",
    "@testing-library/user-event": "^14.6.1",
    "@types/react": "^19.0.10",
    "@types/react-dom": "^19.0.4",
    "@vitejs/plugin-react": "^4.4.1",
    "jsdom": "^26.0.0",
    "typescript": "^5.7.3",
    "vite": "^6.2.0",
    "vitest": "^3.0.8"
  }
}
```

Create `apps/web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx"
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

Create `apps/web/tsconfig.node.json`:

```json
{
  "compilerOptions": {
    "composite": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]
}
```

Create `apps/web/vite.config.ts`:

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
  },
});
```

Create `apps/web/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>YOLO Platform</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Create `apps/web/src/app/query-client.ts`:

```ts
import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      staleTime: 15_000,
    },
  },
});
```

Create `apps/web/src/app/layout/app-shell.tsx`:

```tsx
import { NavLink, Outlet } from "react-router-dom";

export function AppShell() {
  return (
    <div className="app-shell">
      <aside className="shell-sidebar">
        <div className="shell-brand">
          <span className="shell-eyebrow">YOLO Platform</span>
          <strong>Operations Shell</strong>
        </div>
        <nav className="shell-nav">
          <NavLink to="/">Overview</NavLink>
          <NavLink to="/tasks">Tasks</NavLink>
        </nav>
      </aside>
      <main className="shell-content">
        <header className="shell-context">
          <span>Project Context</span>
          <strong>Project 1</strong>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
```

Create `apps/web/src/app/router.tsx`:

```tsx
import { createBrowserRouter } from "react-router-dom";
import { AppShell } from "./layout/app-shell";

function PageStub({ title }: { title: string }) {
  return (
    <section>
      <h1>{title}</h1>
      <p>Loading page.</p>
    </section>
  );
}

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <PageStub title="Task Overview" /> },
      { path: "tasks", element: <PageStub title="Task List" /> }
    ],
  },
]);
```

Create `apps/web/src/app/styles.css`:

```css
:root {
  color: #0f172a;
  background: #f4efe4;
  font-family: "Avenir Next", "Segoe UI", sans-serif;
}

body {
  margin: 0;
}

a {
  color: inherit;
  text-decoration: none;
}

.app-shell {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 248px 1fr;
}

.shell-sidebar {
  padding: 24px;
  background: linear-gradient(180deg, #facc15 0%, #f59e0b 100%);
  border-right: 1px solid rgba(15, 23, 42, 0.16);
}

.shell-brand {
  display: grid;
  gap: 6px;
  margin-bottom: 24px;
}

.shell-eyebrow {
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.shell-nav {
  display: grid;
  gap: 12px;
}

.shell-content {
  padding: 32px;
}

.shell-context {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

@media (max-width: 840px) {
  .app-shell {
    grid-template-columns: 1fr;
  }

  .shell-sidebar {
    border-right: 0;
    border-bottom: 1px solid rgba(15, 23, 42, 0.16);
  }
}
```

Create `apps/web/src/main.tsx`:

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "react-router-dom";
import { queryClient } from "./app/query-client";
import { router } from "./app/router";
import "./app/styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </React.StrictMode>,
);
```

- [ ] **Step 4: Install packages and verify the shell**

Run:

```bash
cd apps/web && npm install
cd apps/web && npm test
cd apps/web && npm run build
```

Expected:

```text
✓ renders navigation links for overview and tasks
vite v...
✓ built in ...
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/package.json apps/web/package-lock.json apps/web/tsconfig.json apps/web/tsconfig.node.json apps/web/vite.config.ts apps/web/index.html apps/web/src/main.tsx apps/web/src/app/router.tsx apps/web/src/app/query-client.ts apps/web/src/app/layout/app-shell.tsx apps/web/src/app/styles.css apps/web/src/features/tasks/task-list-page.test.tsx
git commit -m "feat: bootstrap web shell"
```

## Task 5: Implement The Task Overview Page And Create-Task Flow

**Files:**

- Create: `apps/web/src/features/shared/http.ts`
- Create: `apps/web/src/features/overview/api.ts`
- Create: `apps/web/src/features/overview/task-overview-page.tsx`
- Create: `apps/web/src/features/overview/task-overview-page.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/styles.css`

- [ ] **Step 1: Write the failing overview-page test**

Create `apps/web/src/features/overview/task-overview-page.test.tsx`:

```tsx
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TaskOverviewPage } from "./task-overview-page";

describe("TaskOverviewPage", () => {
  it("renders summary cards, blockers, and submits create task", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          summary_cards: [
            { id: "open-tasks", title: "Open Tasks", count: 4, href: "/tasks" },
            { id: "blocked-tasks", title: "Blocked Tasks", count: 1, href: "/tasks?status=blocked" },
            { id: "review-backlog", title: "Review Backlog", count: 3, href: "/tasks?kind=review&status=ready" },
            { id: "failed-jobs", title: "Failed Jobs", count: 1, href: "/tasks?status=blocked" }
          ],
          blockers: [
            { id: "task-1", title: "Blocked review batch", reason: "schema mismatch", href: "/tasks?status=blocked" },
            { id: "job-9", title: "Failed job: zero-shot", reason: "provider unavailable", href: "/tasks?status=blocked" }
          ],
          longest_idle_task: { id: 1, title: "Blocked review batch", status: "blocked" },
          recent_failed_jobs: [{ id: 9, job_type: "zero-shot", status: "failed", error_msg: "provider unavailable" }]
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ id: 11, title: "Create ontology review task" }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          summary_cards: [
            { id: "open-tasks", title: "Open Tasks", count: 5, href: "/tasks" },
            { id: "blocked-tasks", title: "Blocked Tasks", count: 1, href: "/tasks?status=blocked" },
            { id: "review-backlog", title: "Review Backlog", count: 3, href: "/tasks?kind=review&status=ready" },
            { id: "failed-jobs", title: "Failed Jobs", count: 1, href: "/tasks?status=blocked" }
          ],
          blockers: [],
          longest_idle_task: { id: 1, title: "Blocked review batch", status: "blocked" },
          recent_failed_jobs: []
        }),
      });

    vi.stubGlobal("fetch", fetchMock as unknown as typeof fetch);

    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={["/"]}>
          <Routes>
            <Route path="/" element={<TaskOverviewPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Open Tasks")).toBeInTheDocument();
    expect(screen.getByText("Blocked review batch")).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText(/title/i), "Create ontology review task");
    await userEvent.click(screen.getByRole("button", { name: /create task/i }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/v1/projects/1/tasks",
        expect.objectContaining({ method: "POST" }),
      );
    });
    expect(await screen.findByText("5")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
cd apps/web && npm test -- task-overview-page.test.tsx
```

Expected:

```text
FAIL ... Cannot find module './task-overview-page'
```

- [ ] **Step 3: Add the overview API and page**

Create `apps/web/src/features/shared/http.ts`:

```ts
export async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}
```

Create `apps/web/src/features/overview/api.ts`:

```ts
import { getJSON, postJSON } from "../shared/http";

export type SummaryCard = {
  id: string;
  title: string;
  count: number;
  href: string;
};

export type BlockerCard = {
  id: string;
  title: string;
  reason: string;
  href: string;
};

export type OverviewResponse = {
  summary_cards: SummaryCard[];
  blockers: BlockerCard[];
  longest_idle_task?: {
    id: number;
    title: string;
    status: string;
  };
  recent_failed_jobs: {
    id: number;
    job_type: string;
    status: string;
    error_msg: string;
  }[];
};

export type CreateTaskPayload = {
  title: string;
  kind?: string;
  assignee?: string;
  priority?: string;
  snapshot_id?: number;
};

export function fetchOverview(projectId: string) {
  return getJSON<OverviewResponse>(`/v1/projects/${projectId}/overview`);
}

export function createTask(projectId: string, payload: CreateTaskPayload) {
  return postJSON(`/v1/projects/${projectId}/tasks`, payload);
}
```

Create `apps/web/src/features/overview/task-overview-page.tsx`:

```tsx
import { FormEvent, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { createTask, fetchOverview } from "./api";

export function TaskOverviewPage() {
  const projectId = "1";
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  const [assignee, setAssignee] = useState("");
  const [kind, setKind] = useState("annotation");
  const [priority, setPriority] = useState("normal");

  const overview = useQuery({
    queryKey: ["overview", projectId],
    queryFn: () => fetchOverview(projectId),
  });

  const createTaskMutation = useMutation({
    mutationFn: () =>
      createTask(projectId, {
        title,
        assignee: assignee || undefined,
        kind,
        priority,
      }),
    onSuccess: async () => {
      setTitle("");
      setAssignee("");
      await queryClient.invalidateQueries({ queryKey: ["overview", projectId] });
      await queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!title.trim()) {
      return;
    }
    createTaskMutation.mutate();
  }

  if (overview.isLoading) {
    return <section><h1>Task Overview</h1><p>Loading overview...</p></section>;
  }

  if (overview.isError || !overview.data) {
    return <section><h1>Task Overview</h1><p>Failed to load overview.</p></section>;
  }

  const data = overview.data;
  return (
    <section className="overview-page">
      <header className="page-header">
        <h1>Task Overview</h1>
        <p>Project 1 default entry.</p>
      </header>

      <div className="summary-grid">
        {data.summary_cards.map((card) => (
          <Link className="summary-card" key={card.id} to={card.href}>
            <span>{card.title}</span>
            <strong>{card.count}</strong>
          </Link>
        ))}
      </div>

      <div className="overview-grid">
        <section className="panel">
          <h2>Blockers View</h2>
          {data.blockers.length === 0 ? (
            <p>No active blockers.</p>
          ) : (
            <ul className="blocker-list">
              {data.blockers.map((blocker) => (
                <li key={blocker.id}>
                  <Link to={blocker.href}>{blocker.title}</Link>
                  <p>{blocker.reason}</p>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel">
          <h2>Longest Idle Task</h2>
          {data.longest_idle_task ? (
            <Link to={`/tasks?status=${data.longest_idle_task.status}`}>
              {data.longest_idle_task.title}
            </Link>
          ) : (
            <p>No active tasks.</p>
          )}
        </section>

        <section className="panel">
          <h2>Recent Failed Jobs</h2>
          {data.recent_failed_jobs.length === 0 ? (
            <p>No recent failed jobs.</p>
          ) : (
            <ul className="job-list">
              {data.recent_failed_jobs.map((job) => (
                <li key={job.id}>
                  <strong>{job.job_type}</strong>
                  <span>{job.error_msg || job.status}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel">
          <h2>Create Task</h2>
          <form className="task-form" onSubmit={handleSubmit}>
            <label>
              Title
              <input value={title} onChange={(event) => setTitle(event.target.value)} />
            </label>
            <label>
              Assignee
              <input value={assignee} onChange={(event) => setAssignee(event.target.value)} />
            </label>
            <label>
              Kind
              <select value={kind} onChange={(event) => setKind(event.target.value)}>
                <option value="annotation">Annotation</option>
                <option value="review">Review</option>
                <option value="qa">QA</option>
                <option value="ops">Ops</option>
              </select>
            </label>
            <label>
              Priority
              <select value={priority} onChange={(event) => setPriority(event.target.value)}>
                <option value="low">Low</option>
                <option value="normal">Normal</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </label>
            <button type="submit" disabled={createTaskMutation.isPending}>
              Create Task
            </button>
          </form>
          {createTaskMutation.isError ? <p>Failed to create task.</p> : null}
        </section>
      </div>
    </section>
  );
}
```

Update `apps/web/src/app/router.tsx`:

```tsx
import { createBrowserRouter } from "react-router-dom";
import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";
function PageStub() {
  return <section><h1>Task List</h1><p>Loading page.</p></section>;
}

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <TaskOverviewPage /> },
      { path: "tasks", element: <PageStub /> }
    ],
  },
]);
```

Append to `apps/web/src/app/styles.css`:

```css
.page-header {
  margin-bottom: 24px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.summary-card,
.panel {
  border: 1px solid rgba(15, 23, 42, 0.12);
  background: rgba(255, 255, 255, 0.72);
  backdrop-filter: blur(6px);
  border-radius: 18px;
  padding: 18px;
}

.summary-card {
  display: grid;
  gap: 8px;
}

.summary-card strong {
  font-size: 1.8rem;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 16px;
}

.task-form {
  display: grid;
  gap: 12px;
}

.task-form input,
.task-form select,
.task-form button {
  font: inherit;
  padding: 10px 12px;
}

.blocker-list,
.job-list {
  display: grid;
  gap: 12px;
  padding-left: 18px;
}
```

- [ ] **Step 4: Run the overview-page test**

Run:

```bash
cd apps/web && npm test -- task-overview-page.test.tsx
```

Expected:

```text
✓ renders summary cards, blockers, and submits create task
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/shared/http.ts apps/web/src/features/overview/api.ts apps/web/src/features/overview/task-overview-page.tsx apps/web/src/features/overview/task-overview-page.test.tsx apps/web/src/app/router.tsx apps/web/src/app/styles.css
git commit -m "feat: add task overview page and create flow"
```

## Task 6: Implement The Filtered Task List, Make Targets, And Docs

**Files:**

- Create: `apps/web/src/features/tasks/api.ts`
- Create: `apps/web/src/features/tasks/task-list-page.tsx`
- Modify: `apps/web/src/features/tasks/task-list-page.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`

- [ ] **Step 1: Write the failing task-list page test**

Replace `apps/web/src/features/tasks/task-list-page.test.tsx`:

```tsx
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TaskListPage } from "./task-list-page";

describe("TaskListPage", () => {
  it("reads filters from the URL and renders matching tasks", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        items: [
          {
            id: 1,
            title: "Blocked review batch",
            status: "blocked",
            kind: "review",
            priority: "high",
            assignee: "reviewer-1"
          }
        ],
      }),
    });
    vi.stubGlobal("fetch", fetchMock as unknown as typeof fetch);

    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={["/tasks?status=blocked&kind=review"]}>
          <Routes>
            <Route path="/tasks" element={<TaskListPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/v1/projects/1/tasks?status=blocked&kind=review",
        expect.anything(),
      );
    });
    expect(await screen.findByText("Blocked review batch")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
cd apps/web && npm test -- task-list-page.test.tsx
```

Expected:

```text
FAIL ... Cannot find module './task-list-page'
```

- [ ] **Step 3: Add the task-list page, route wiring, and local-dev commands**

Create `apps/web/src/features/tasks/api.ts`:

```ts
import { getJSON } from "../shared/http";

export type Task = {
  id: number;
  title: string;
  kind: string;
  status: string;
  priority: string;
  assignee: string;
  snapshot_version?: string;
  dataset_name?: string;
  blocker_reason?: string;
};

export async function fetchTasks(projectId: string, queryString: string) {
  const suffix = queryString ? `?${queryString}` : "";
  return getJSON<{ items: Task[] }>(`/v1/projects/${projectId}/tasks${suffix}`);
}
```

Create `apps/web/src/features/tasks/task-list-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { fetchTasks } from "./api";

export function TaskListPage() {
  const projectId = "1";
  const [searchParams] = useSearchParams();
  const queryString = searchParams.toString();

  const query = useQuery({
    queryKey: ["tasks", projectId, queryString],
    queryFn: () => fetchTasks(projectId, queryString),
  });

  if (query.isLoading) {
    return <section><h1>Task List</h1><p>Loading tasks...</p></section>;
  }

  if (query.isError || !query.data) {
    return <section><h1>Task List</h1><p>Failed to load tasks.</p></section>;
  }

  if (query.data.items.length === 0) {
    return <section><h1>Task List</h1><p>No tasks match the current filters.</p></section>;
  }

  return (
    <section>
      <header className="page-header">
        <h1>Task List</h1>
        <p>{queryString ? `Filters: ${queryString}` : "All active task records."}</p>
      </header>

      <div className="panel">
        <table className="task-table">
          <thead>
            <tr>
              <th>Title</th>
              <th>Kind</th>
              <th>Status</th>
              <th>Priority</th>
              <th>Assignee</th>
            </tr>
          </thead>
          <tbody>
            {query.data.items.map((task) => (
              <tr key={task.id}>
                <td>{task.title}</td>
                <td>{task.kind}</td>
                <td>{task.status}</td>
                <td>{task.priority}</td>
                <td>{task.assignee || "unassigned"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
```

Update `apps/web/src/app/router.tsx`:

```tsx
import { createBrowserRouter } from "react-router-dom";
import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";
import { TaskListPage } from "../features/tasks/task-list-page";

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <TaskOverviewPage /> },
      { path: "tasks", element: <TaskListPage /> }
    ],
  },
]);
```

Append to `apps/web/src/app/styles.css`:

```css
.task-table {
  width: 100%;
  border-collapse: collapse;
}

.task-table th,
.task-table td {
  text-align: left;
  padding: 10px 12px;
  border-bottom: 1px solid rgba(15, 23, 42, 0.1);
}
```

Update `Makefile`:

```make
.PHONY: up-dev down-dev test migrate-up migrate-down migrate-version web-install web-dev web-test web-build

web-install:
	cd apps/web && npm install

web-dev:
	cd apps/web && npm run dev

web-test:
	cd apps/web && npm test

web-build:
	cd apps/web && npm run build
```

Append to `README.md` and `docs/development/local-quickstart.md`:

```md
## Run the web console

```bash
make web-install
make web-dev
```

Keep the API server running on `http://localhost:8080` while the Vite app is open.
```

Append to `README.zh-CN.md` and `docs/development/local-quickstart.zh-CN.md`:

```md
## 运行 Web 控制台

```bash
make web-install
make web-dev
```

使用前请保持 API 服务运行在 `http://localhost:8080`。
```

- [ ] **Step 4: Run the validation commands**

Run:

```bash
cd apps/web && npm test
cd apps/web && npm run build
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview ./internal/server ./cmd/api-server -v
```

Expected:

```text
✓ reads filters from the URL and renders matching tasks
PASS
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/tasks/api.ts apps/web/src/features/tasks/task-list-page.tsx apps/web/src/features/tasks/task-list-page.test.tsx apps/web/src/app/router.tsx apps/web/src/app/styles.css Makefile README.md README.zh-CN.md docs/development/local-quickstart.md docs/development/local-quickstart.zh-CN.md
git commit -m "feat: add task list and frontend dev workflow"
```

## Self-Review

### Spec Coverage

This plan was corrected against [2026-04-01-task-overview-and-web-shell-design.md](/home/shirosora/YOLO-Ave-Mujica/docs/superpowers/specs/2026-04-01-task-overview-and-web-shell-design.md) before handoff. The main fixes were:

1. frontend routes now match the approved `/` and `/tasks` shape
2. overview aggregation now emits blocker entries for failed jobs
3. the duplicate `scanTaskWithContext` snippet was collapsed into one implementation

After those corrections, the plan covers:

1. task schema and validation
2. task create/list/get APIs
3. overview aggregation with summary cards, blockers, longest-idle task, and recent failed jobs
4. runtime wiring into the existing API server
5. `apps/web` shell bootstrap
6. default `Task Overview` route
7. minimal create-task interaction
8. filtered `Task List`
9. local dev commands and docs

No task in this plan claims:

1. `Task Detail` UI
2. workspaces
3. publish gate
4. training or evaluation
5. auth or RBAC

### Placeholder Scan

This plan contains:

1. exact file paths
2. concrete test code
3. concrete implementation snippets
4. exact commands with expected outcomes

There are no `TODO`, `TBD`, or deferred placeholders.

### Type Consistency

The plan uses one consistent vocabulary across all tasks:

1. `Task`
2. `CreateTaskInput`
3. `ListTasksFilter`
4. `SummaryCard`
5. `BlockerCard`
6. `TaskOverviewPage`
7. `TaskListPage`

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-01-task-overview-and-web-shell-implementation-plan.md`. Two execution options:

1. Subagent-Driven (recommended) - I dispatch a fresh subagent per task, review between tasks, fast iteration

2. Inline Execution - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
