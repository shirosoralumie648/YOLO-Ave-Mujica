# YOLO Platform Phase 1: Task Kernel And Web Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first real product slice on top of the MVP backend: task resources, overview aggregation, stable HTTP routes, and a usable `apps/web` shell with `Task Overview`, `Task List`, and `Task Detail`.

**Architecture:** Follow the existing backend pattern used by `internal/datahub`, `internal/jobs`, and `internal/review`: small domain package with model, repository, service, handler, and handler tests, then wire it through `internal/server/http_server.go` and `cmd/api-server/main.go`. Add a standalone `apps/web` Vite + React + TypeScript app that consumes the new task and overview APIs, with React Router for URL-based context and TanStack Query for data loading.

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, Vite, React, TypeScript, React Router, TanStack Query, Vitest, Testing Library.

---

## File Structure

**Create**

- `migrations/000003_task_kernel.up.sql`
- `migrations/000003_task_kernel.down.sql`
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
- `apps/web/src/features/tasks/task-detail-page.tsx`
- `apps/web/src/features/tasks/task-pages.test.tsx`

**Modify**

- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `cmd/api-server/main.go`
- `cmd/api-server/main_test.go`
- `internal/review/service.go`
- `internal/review/postgres_repository.go`
- `internal/jobs/postgres_repository.go`
- `Makefile`
- `README.md`
- `README.zh-CN.md`
- `docs/development/local-quickstart.md`
- `docs/development/local-quickstart.zh-CN.md`

## Scope Guardrails

This phase does **not** include:

1. Annotation workspace rendering.
2. Review workspace rendering.
3. Publish gate semantics.
4. Training or evaluation resources.
5. Auth or RBAC.

This phase **does** include:

1. First-class task records for human work.
2. Role-facing overview aggregation.
3. Real frontend app shell.
4. `Task Overview`, `Task List`, and `Task Detail`.
5. Local developer workflow for the frontend.

## Task 1: Add Task Domain Schema, Model, Repository, And Service

**Files:**
- Create: `migrations/000003_task_kernel.up.sql`
- Create: `migrations/000003_task_kernel.down.sql`
- Create: `internal/tasks/model.go`
- Create: `internal/tasks/repository.go`
- Create: `internal/tasks/postgres_repository.go`
- Create: `internal/tasks/service.go`
- Create: `internal/tasks/service_test.go`
- Create: `internal/tasks/postgres_repository_test.go`

- [ ] **Step 1: Write the failing task service and repository tests**

Create `internal/tasks/service_test.go` with:

```go
package tasks

import (
	"testing"
)

func TestServiceCreateListAndGetTask(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)

	task, err := svc.CreateTask(CreateTaskInput{
		ProjectID:  1,
		SnapshotID: 7,
		Title:      "Label parking-lot batch",
		Assignee:   "annotator-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID != 1 {
		t.Fatalf("expected id 1, got %d", task.ID)
	}
	if task.Kind != KindAnnotation || task.Status != StatusQueued || task.Priority != PriorityNormal {
		t.Fatalf("expected defaults to be applied, got %+v", task)
	}

	items, err := svc.ListTasks(1, ListTasksFilter{Assignee: "annotator-1"})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Label parking-lot batch" {
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
```

Append to `internal/tasks/postgres_repository_test.go`:

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

- [ ] **Step 3: Add migration and task model/repository code**

Create `migrations/000003_task_kernel.up.sql`:

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

Create `migrations/000003_task_kernel.down.sql`:

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
	ID             int64      `json:"id"`
	ProjectID      int64      `json:"project_id"`
	SnapshotID     *int64     `json:"snapshot_id,omitempty"`
	Title          string     `json:"title"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	Assignee       string     `json:"assignee"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	BlockerReason  string     `json:"blocker_reason,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateTaskInput struct {
	ProjectID  int64
	SnapshotID int64
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Priority   string `json:"priority"`
	Assignee   string `json:"assignee"`
}

type ListTasksFilter struct {
	Status   string
	Assignee string
}
```

Create `internal/tasks/repository.go` with an in-memory repository following the existing `datahub` and `review` style:

```go
package tasks

import (
	"context"
	"fmt"
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
		Title:          in.Title,
		Kind:           in.Kind,
		Status:         in.Status,
		Priority:       in.Priority,
		Assignee:       in.Assignee,
		LastActivityAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if in.SnapshotID > 0 {
		snapshotID := in.SnapshotID
		task.SnapshotID = &snapshotID
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
		if filter.Assignee != "" && task.Assignee != filter.Assignee {
			continue
		}
		out = append(out, task)
	}
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
		insert into tasks (project_id, snapshot_id, title, kind, status, priority, assignee)
		values ($1, nullif($2, 0), $3, $4, $5, $6, $7)
		returning id, project_id, snapshot_id, title, kind, status, priority, assignee,
		          due_at, blocker_reason, last_activity_at, created_at, updated_at
	`, in.ProjectID, in.SnapshotID, in.Title, in.Kind, in.Status, in.Priority, in.Assignee)
	return scanTask(row)
}

func (r *PostgresRepository) ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error) {
	rows, err := r.pool.Query(ctx, `
		select id, project_id, snapshot_id, title, kind, status, priority, assignee,
		       due_at, blocker_reason, last_activity_at, created_at, updated_at
		from tasks
		where project_id = $1
		  and ($2 = '' or status = $2)
		  and ($3 = '' or assignee = $3)
		order by last_activity_at asc, id asc
	`, projectID, filter.Status, filter.Assignee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetTask(ctx context.Context, taskID int64) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		select id, project_id, snapshot_id, title, kind, status, priority, assignee,
		       due_at, blocker_reason, last_activity_at, created_at, updated_at
		from tasks
		where id = $1
	`, taskID)
	return scanTask(row)
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

var _ Repository = (*PostgresRepository)(nil)
```

- [ ] **Step 4: Run task tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks -count=1 -v
```

Expected:

```text
=== RUN   TestServiceCreateListAndGetTask
=== RUN   TestPostgresRepositoryCreateListAndGetTask
PASS
```

- [ ] **Step 5: Commit**

```bash
git add migrations/000003_task_kernel.up.sql migrations/000003_task_kernel.down.sql internal/tasks/model.go internal/tasks/repository.go internal/tasks/postgres_repository.go internal/tasks/service.go internal/tasks/service_test.go internal/tasks/postgres_repository_test.go
git commit -m "feat: add task kernel backend"
```

## Task 2: Add Task Handler And Project Overview Aggregation

**Files:**
- Create: `internal/tasks/handler.go`
- Create: `internal/tasks/handler_test.go`
- Create: `internal/overview/service.go`
- Create: `internal/overview/handler.go`
- Create: `internal/overview/service_test.go`

- [ ] **Step 1: Write the failing task handler and overview aggregation tests**

Create `internal/tasks/handler_test.go` with:

```go
package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandlerCreateListAndGetTask(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	h := NewHandler(svc)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/projects/1/tasks", strings.NewReader(`{
		"snapshot_id": 7,
		"title": "Label parking-lot batch",
		"kind": "annotation",
		"status": "queued",
		"priority": "high",
		"assignee": "annotator-1"
	}`))
	createRouteCtx := chi.NewRouteContext()
	createRouteCtx.URLParams.Add("id", "1")
	createReq = createReq.WithContext(context.WithValue(createReq.Context(), chi.RouteCtxKey, createRouteCtx))
	createRec := httptest.NewRecorder()
	h.CreateTask(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created Task
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("expected created task id 1, got %d", created.ID)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/projects/1/tasks?assignee=annotator-1", nil)
	listRouteCtx := chi.NewRouteContext()
	listRouteCtx.URLParams.Add("id", "1")
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), chi.RouteCtxKey, listRouteCtx))
	listRec := httptest.NewRecorder()
	h.ListTasks(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), `"Label parking-lot batch"`) {
		t.Fatalf("expected task list to include created task, got %d %s", listRec.Code, listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/tasks/1", nil)
	getRouteCtx := chi.NewRouteContext()
	getRouteCtx.URLParams.Add("id", strconv.FormatInt(created.ID, 10))
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, getRouteCtx))
	getRec := httptest.NewRecorder()
	h.GetTask(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"Label parking-lot batch"`) {
		t.Fatalf("expected get task to return created task, got %d %s", getRec.Code, getRec.Body.String())
	}
}
```

Create `internal/overview/service_test.go`:

```go
package overview

import (
	"testing"
	"time"

	"yolo-ave-mujica/internal/tasks"
)

type fakeTaskSource struct {
	items []tasks.Task
}

func (f fakeTaskSource) ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error) {
	return f.items, nil
}

type fakeReviewSource struct {
	pending int
}

func (f fakeReviewSource) PendingCandidateCount(projectID int64) (int, error) {
	return f.pending, nil
}

type fakeJobSource struct {
	failed int
}

func (f fakeJobSource) FailedRecentJobCount(projectID int64) (int, error) {
	return f.failed, nil
}

func TestServiceBuildOverviewIncludesLongestIdleAndBlockers(t *testing.T) {
	old := time.Now().UTC().Add(-6 * time.Hour)
	recent := time.Now().UTC().Add(-15 * time.Minute)
	service := NewService(
		fakeTaskSource{items: []tasks.Task{
			{ID: 1, ProjectID: 1, Title: "Blocked review batch", Status: tasks.StatusBlocked, BlockerReason: "schema mismatch", LastActivityAt: old},
			{ID: 2, ProjectID: 1, Title: "Queued annotation batch", Status: tasks.StatusQueued, LastActivityAt: recent},
		}},
		fakeReviewSource{pending: 5},
		fakeJobSource{failed: 2},
	)

	out, err := service.BuildOverview(1)
	if err != nil {
		t.Fatalf("build overview: %v", err)
	}
	if out.ReviewBacklogCount != 5 {
		t.Fatalf("expected review backlog 5, got %d", out.ReviewBacklogCount)
	}
	if out.FailedRecentJobs != 2 {
		t.Fatalf("expected failed recent jobs 2, got %d", out.FailedRecentJobs)
	}
	if out.LongestIdleTask == nil || out.LongestIdleTask.ID != 1 {
		t.Fatalf("expected longest idle task id 1, got %+v", out.LongestIdleTask)
	}
	if len(out.Blockers) != 1 || out.Blockers[0].TaskID != 1 {
		t.Fatalf("expected single blocker for task 1, got %+v", out.Blockers)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/overview -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewHandler
FAIL ... undefined: NewService
FAIL ... undefined: Overview
```

- [ ] **Step 3: Add overview service and task handler**

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

	items, err := h.svc.ListTasks(projectID, ListTasksFilter{
		Status:   r.URL.Query().Get("status"),
		Assignee: r.URL.Query().Get("assignee"),
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
	"time"

	"yolo-ave-mujica/internal/tasks"
)

type TaskSource interface {
	ListTasks(projectID int64, filter tasks.ListTasksFilter) ([]tasks.Task, error)
}

type ReviewSource interface {
	PendingCandidateCount(projectID int64) (int, error)
}

type JobSource interface {
	FailedRecentJobCount(projectID int64) (int, error)
}

type BlockerCard struct {
	TaskID  int64  `json:"task_id"`
	Title   string `json:"title"`
	Reason  string `json:"reason"`
	Status  string `json:"status"`
	Minutes int64  `json:"minutes_idle"`
}

type Overview struct {
	OpenTaskCount      int          `json:"open_task_count"`
	BlockedTaskCount   int          `json:"blocked_task_count"`
	ReviewBacklogCount int          `json:"review_backlog_count"`
	FailedRecentJobs   int          `json:"failed_recent_jobs"`
	Blockers           []BlockerCard `json:"blockers"`
	LongestIdleTask    *tasks.Task  `json:"longest_idle_task,omitempty"`
}

type Service struct {
	tasks  TaskSource
	review ReviewSource
	jobs   JobSource
}

func NewService(taskSource TaskSource, reviewSource ReviewSource, jobSource JobSource) *Service {
	return &Service{tasks: taskSource, review: reviewSource, jobs: jobSource}
}

func (s *Service) BuildOverview(projectID int64) (Overview, error) {
	taskItems, err := s.tasks.ListTasks(projectID, tasks.ListTasksFilter{})
	if err != nil {
		return Overview{}, err
	}
	reviewBacklog, err := s.review.PendingCandidateCount(projectID)
	if err != nil {
		return Overview{}, err
	}
	failedRecent, err := s.jobs.FailedRecentJobCount(projectID)
	if err != nil {
		return Overview{}, err
	}

	out := Overview{ReviewBacklogCount: reviewBacklog, FailedRecentJobs: failedRecent, Blockers: []BlockerCard{}}
	var longest *tasks.Task
	now := time.Now().UTC()
	for i := range taskItems {
		task := taskItems[i]
		if task.Status != tasks.StatusDone {
			out.OpenTaskCount++
		}
		if task.Status == tasks.StatusBlocked {
			out.BlockedTaskCount++
			out.Blockers = append(out.Blockers, BlockerCard{
				TaskID:  task.ID,
				Title:   task.Title,
				Reason:  task.BlockerReason,
				Status:  task.Status,
				Minutes: int64(now.Sub(task.LastActivityAt).Minutes()),
			})
		}
		if longest == nil || task.LastActivityAt.Before(longest.LastActivityAt) {
			copyTask := task
			longest = &copyTask
		}
	}
	out.LongestIdleTask = longest
	return out, nil
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

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview -count=1 -v
```

Expected:

```text
=== RUN   TestHandlerCreateListAndGetTask
=== RUN   TestServiceBuildOverviewIncludesLongestIdleAndBlockers
--- PASS: TestServiceBuildOverviewIncludesLongestIdleAndBlockers
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/tasks/handler.go internal/overview internal/tasks/handler_test.go
git commit -m "feat: add overview aggregation and task handlers"
```

## Task 3: Wire Task And Overview Routes Into The API Server

**Files:**
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/main_test.go`
- Modify: `internal/review/service.go`
- Modify: `internal/review/postgres_repository.go`
- Modify: `internal/jobs/postgres_repository.go`

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
func TestBuildModulesWithHandlersCanServeTaskAndOverviewRoutes(t *testing.T) {
	modules := newTestModules()
	srv := server.NewHTTPServerWithModules(modules)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/1/overview", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 before overview module wiring, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server ./cmd/api-server -count=1 -v
```

Expected:

```text
FAIL ... route missing: GET /v1/projects/1/overview
```

- [ ] **Step 3: Add route groups and module wiring**

Update `internal/server/http_server.go` types:

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

Update `cmd/api-server/main.go` module wiring:

```go
	reviewRepo := review.NewPostgresRepository(pool)
	reviewSvc := review.NewServiceWithRepository(reviewRepo)
	reviewHandler := review.NewHandler(reviewSvc)

	taskRepo := tasks.NewPostgresRepository(pool)
	taskSvc := tasks.NewService(taskRepo)
	taskHandler := tasks.NewHandler(taskSvc)

	overviewSvc := overview.NewService(taskSvc, reviewSvc, jobsRepo)
	overviewHandler := overview.NewHandler(overviewSvc)

	modules.Tasks = server.TaskRoutes{
		ListTasks:  taskHandler.ListTasks,
		CreateTask: taskHandler.CreateTask,
		GetTask:    taskHandler.GetTask,
	}
	modules.Overview = server.OverviewRoutes{
		GetProjectOverview: overviewHandler.GetProjectOverview,
	}
```

Add the project-aware review and jobs counters needed by `overview.Service`.

Append to `internal/review/service.go`:

```go
func (s *Service) PendingCandidateCount(projectID int64) (int, error) {
	if counter, ok := s.repo.(interface {
		PendingCandidateCount(projectID int64) (int, error)
	}); ok {
		return counter.PendingCandidateCount(projectID)
	}
	items := s.ListCandidates()
	return len(items), nil
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
func (r *PostgresRepository) FailedRecentJobCount(projectID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(context.Background(), `
		select count(*)
		from jobs
		where project_id = $1
		  and status = 'failed'
		  and coalesce(finished_at, created_at) >= now() - interval '24 hours'
	`, projectID).Scan(&count)
	return count, err
}
```

- [ ] **Step 4: Run server tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server ./cmd/api-server -count=1 -v
```

Expected:

```text
=== RUN   TestMVPRoutesAreRegistered
--- PASS: TestMVPRoutesAreRegistered
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/http_server.go internal/server/http_server_routes_test.go cmd/api-server/main.go cmd/api-server/main_test.go internal/review/service.go internal/review/postgres_repository.go internal/jobs/postgres_repository.go
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
- Create: `apps/web/src/features/tasks/task-pages.test.tsx`

- [ ] **Step 1: Write the failing frontend shell test**

Create `apps/web/src/features/tasks/task-pages.test.tsx`:

```tsx
import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppShell } from "../../app/layout/app-shell";

describe("AppShell", () => {
  it("renders primary navigation links", () => {
    render(
      <MemoryRouter initialEntries={["/projects/1/overview"]}>
        <Routes>
          <Route path="/projects/:projectId" element={<AppShell />}>
            <Route path="overview" element={<div>Overview Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: /overview/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /tasks/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /data/i })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run frontend test to verify it fails**

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

```tsx
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
      <aside className="sidebar">
        <div className="brand">YOLO Platform</div>
        <nav className="nav">
          <NavLink to="/projects/1/overview">Overview</NavLink>
          <NavLink to="/projects/1/tasks">Tasks</NavLink>
          <NavLink to="/projects/1/data">Data</NavLink>
        </nav>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
```

Create `apps/web/src/app/router.tsx`:

```tsx
import { createBrowserRouter, Navigate } from "react-router-dom";
import { AppShell } from "./layout/app-shell";

function PlaceholderPage({ title }: { title: string }) {
  return (
    <section>
      <h1>{title}</h1>
      <p>Page scaffold.</p>
    </section>
  );
}

export const router = createBrowserRouter([
  {
    path: "/",
    element: <Navigate to="/projects/1/overview" replace />,
  },
  {
    path: "/projects/:projectId",
    element: <AppShell />,
    children: [
      { path: "overview", element: <PlaceholderPage title="Overview" /> },
      { path: "tasks", element: <PlaceholderPage title="Tasks" /> },
      { path: "tasks/:taskId", element: <PlaceholderPage title="Task Detail" /> },
    ],
  },
]);
```

Create `apps/web/src/app/styles.css`:

```css
:root {
  color: #111827;
  background: #f4f1e8;
  font-family: "IBM Plex Sans", "Segoe UI", sans-serif;
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
  grid-template-columns: 240px 1fr;
}

.sidebar {
  padding: 24px;
  background: linear-gradient(180deg, #facc15 0%, #f59e0b 100%);
  border-right: 1px solid rgba(17, 24, 39, 0.12);
}

.brand {
  font-size: 1.2rem;
  font-weight: 700;
  margin-bottom: 24px;
}

.nav {
  display: grid;
  gap: 12px;
}

.content {
  padding: 32px;
}

@media (max-width: 800px) {
  .app-shell {
    grid-template-columns: 1fr;
  }

  .sidebar {
    border-right: 0;
    border-bottom: 1px solid rgba(17, 24, 39, 0.12);
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

- [ ] **Step 4: Run frontend tests and build**

Run:

```bash
cd apps/web && npm install
cd apps/web && npm test
cd apps/web && npm run build
```

Expected:

```text
✓ AppShell renders primary navigation links
vite v...
✓ built in ...
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/package.json apps/web/package-lock.json apps/web/tsconfig.json apps/web/tsconfig.node.json apps/web/vite.config.ts apps/web/index.html apps/web/src/main.tsx apps/web/src/app/router.tsx apps/web/src/app/query-client.ts apps/web/src/app/layout/app-shell.tsx apps/web/src/app/styles.css apps/web/src/features/tasks/task-pages.test.tsx
git commit -m "feat: bootstrap web shell"
```

## Task 5: Implement Task Overview Page And API Client

**Files:**
- Create: `apps/web/src/features/shared/http.ts`
- Create: `apps/web/src/features/overview/api.ts`
- Create: `apps/web/src/features/overview/task-overview-page.tsx`
- Create: `apps/web/src/features/overview/task-overview-page.test.tsx`
- Modify: `apps/web/src/app/router.tsx`

- [ ] **Step 1: Write the failing Task Overview page test**

Create `apps/web/src/features/overview/task-overview-page.test.tsx`:

```tsx
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { TaskOverviewPage } from "./task-overview-page";

vi.stubGlobal("fetch", vi.fn(() =>
  Promise.resolve({
    ok: true,
    json: () =>
      Promise.resolve({
        open_task_count: 4,
        blocked_task_count: 1,
        review_backlog_count: 6,
        failed_recent_jobs: 2,
        blockers: [{ task_id: 9, title: "Schema mismatch review", reason: "schema mismatch", status: "blocked", minutes_idle: 180 }],
        longest_idle_task: { id: 9, title: "Schema mismatch review", status: "blocked" }
      }),
  }),
) as any);

describe("TaskOverviewPage", () => {
  it("renders summary cards and blocker list", async () => {
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={["/projects/1/overview"]}>
          <Routes>
            <Route path="/projects/:projectId/overview" element={<TaskOverviewPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Open Tasks")).toBeInTheDocument();
    expect(await screen.findByText("Schema mismatch review")).toBeInTheDocument();
    expect(await screen.findByText(/Longest Idle Task/i)).toBeInTheDocument();
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

- [ ] **Step 3: Add the overview page and API wrapper**

Create `apps/web/src/features/shared/http.ts`:

```tsx
export async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    headers: {
      Accept: "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  return response.json() as Promise<T>;
}
```

Create `apps/web/src/features/overview/api.ts`:

```tsx
import { getJSON } from "../shared/http";

export type BlockerCard = {
  task_id: number;
  title: string;
  reason: string;
  status: string;
  minutes_idle: number;
};

export type OverviewResponse = {
  open_task_count: number;
  blocked_task_count: number;
  review_backlog_count: number;
  failed_recent_jobs: number;
  blockers: BlockerCard[];
  longest_idle_task?: {
    id: number;
    title: string;
    status: string;
  };
};

export function fetchOverview(projectId: string) {
  return getJSON<OverviewResponse>(`/v1/projects/${projectId}/overview`);
}
```

Create `apps/web/src/features/overview/task-overview-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { fetchOverview } from "./api";

export function TaskOverviewPage() {
  const { projectId = "1" } = useParams();
  const overview = useQuery({
    queryKey: ["overview", projectId],
    queryFn: () => fetchOverview(projectId),
  });

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
        <p>Project {projectId} task-first entry.</p>
      </header>

      <div className="summary-grid">
        <article><h2>Open Tasks</h2><p>{data.open_task_count}</p></article>
        <article><h2>Blocked Tasks</h2><p>{data.blocked_task_count}</p></article>
        <article><h2>Review Backlog</h2><p>{data.review_backlog_count}</p></article>
        <article><h2>Failed Jobs</h2><p>{data.failed_recent_jobs}</p></article>
      </div>

      <section>
        <h2>Blockers View</h2>
        <ul>
          {data.blockers.map((blocker) => (
            <li key={blocker.task_id}>
              <Link to={`/projects/${projectId}/tasks/${blocker.task_id}`}>{blocker.title}</Link>
              <span> {blocker.reason}</span>
            </li>
          ))}
        </ul>
      </section>

      <section>
        <h2>Longest Idle Task</h2>
        {data.longest_idle_task ? (
          <Link to={`/projects/${projectId}/tasks/${data.longest_idle_task.id}`}>
            {data.longest_idle_task.title}
          </Link>
        ) : (
          <p>No active tasks.</p>
        )}
      </section>
    </section>
  );
}
```

Update `apps/web/src/app/router.tsx` so the overview route stops using the placeholder page:

```tsx
import { createBrowserRouter, Navigate } from "react-router-dom";
import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";

function PlaceholderPage({ title }: { title: string }) {
  return (
    <section>
      <h1>{title}</h1>
      <p>Page scaffold.</p>
    </section>
  );
}

export const router = createBrowserRouter([
  {
    path: "/",
    element: <Navigate to="/projects/1/overview" replace />,
  },
  {
    path: "/projects/:projectId",
    element: <AppShell />,
    children: [
      { path: "overview", element: <TaskOverviewPage /> },
      { path: "tasks", element: <PlaceholderPage title="Tasks" /> },
      { path: "tasks/:taskId", element: <PlaceholderPage title="Task Detail" /> },
    ],
  },
]);
```

- [ ] **Step 4: Run Task Overview test**

Run:

```bash
cd apps/web && npm test -- task-overview-page.test.tsx
```

Expected:

```text
✓ renders summary cards and blocker list
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/shared/http.ts apps/web/src/features/overview apps/web/src/app/router.tsx
git commit -m "feat: add task overview page"
```

## Task 6: Implement Task List, Task Detail, Make Targets, And Docs

**Files:**
- Create: `apps/web/src/features/tasks/api.ts`
- Create: `apps/web/src/features/tasks/task-list-page.tsx`
- Create: `apps/web/src/features/tasks/task-detail-page.tsx`
- Modify: `apps/web/src/features/tasks/task-pages.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`

- [ ] **Step 1: Write the failing task page tests**

Replace `apps/web/src/features/tasks/task-pages.test.tsx` with:

```tsx
import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { AppShell } from "../../app/layout/app-shell";
import { TaskListPage } from "./task-list-page";
import { TaskDetailPage } from "./task-detail-page";

describe("AppShell", () => {
  it("renders primary navigation links", () => {
    render(
      <MemoryRouter initialEntries={["/projects/1/overview"]}>
        <Routes>
          <Route path="/projects/:projectId" element={<AppShell />}>
            <Route path="overview" element={<div>Overview Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: /overview/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /tasks/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /data/i })).toBeInTheDocument();
  });
});

vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => {
  const url = String(input);
  if (url.endsWith("/v1/projects/1/tasks")) {
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        items: [
          { id: 1, title: "Annotate loading-dock batch", status: "queued", priority: "high", assignee: "annotator-1" }
        ],
      }),
    });
  }
  return Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ id: 1, title: "Annotate loading-dock batch", status: "queued", priority: "high", assignee: "annotator-1" }),
  });
}) as any);

function renderWithProviders(path: string, element: ReactNode, route: string) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path={route} element={element} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

it("renders task list rows", async () => {
  renderWithProviders("/projects/1/tasks", <TaskListPage />, "/projects/:projectId/tasks");
  expect(await screen.findByText("Annotate loading-dock batch")).toBeInTheDocument();
});

it("renders task detail metadata", async () => {
  renderWithProviders("/projects/1/tasks/1", <TaskDetailPage />, "/projects/:projectId/tasks/:taskId");
  expect(await screen.findByText(/annotator-1/i)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd apps/web && npm test -- task-pages.test.tsx
```

Expected:

```text
FAIL ... Cannot find module './task-list-page'
```

- [ ] **Step 3: Add task pages and local-dev commands**

Create `apps/web/src/features/tasks/api.ts`:

```tsx
import { getJSON } from "../shared/http";

export type Task = {
  id: number;
  title: string;
  status: string;
  priority: string;
  assignee: string;
  blocker_reason?: string;
};

export function fetchTasks(projectId: string) {
  return getJSON<{ items: Task[] }>(`/v1/projects/${projectId}/tasks`);
}

export function fetchTask(taskId: string) {
  return getJSON<Task>(`/v1/tasks/${taskId}`);
}
```

Create `apps/web/src/features/tasks/task-list-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { fetchTasks } from "./api";

export function TaskListPage() {
  const { projectId = "1" } = useParams();
  const query = useQuery({
    queryKey: ["tasks", projectId],
    queryFn: () => fetchTasks(projectId),
  });

  if (query.isLoading) {
    return <section><h1>Tasks</h1><p>Loading tasks...</p></section>;
  }
  if (query.isError || !query.data) {
    return <section><h1>Tasks</h1><p>Failed to load tasks.</p></section>;
  }

  return (
    <section>
      <h1>Task List</h1>
      <ul>
        {query.data.items.map((task) => (
          <li key={task.id}>
            <Link to={`/projects/${projectId}/tasks/${task.id}`}>{task.title}</Link>
            <span> {task.status}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
```

Create `apps/web/src/features/tasks/task-detail-page.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { fetchTask } from "./api";

export function TaskDetailPage() {
  const { taskId = "0" } = useParams();
  const query = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => fetchTask(taskId),
  });

  if (query.isLoading) {
    return <section><h1>Task Detail</h1><p>Loading task...</p></section>;
  }
  if (query.isError || !query.data) {
    return <section><h1>Task Detail</h1><p>Failed to load task.</p></section>;
  }

  const task = query.data;
  return (
    <section>
      <h1>{task.title}</h1>
      <dl>
        <dt>Status</dt>
        <dd>{task.status}</dd>
        <dt>Priority</dt>
        <dd>{task.priority}</dd>
        <dt>Assignee</dt>
        <dd>{task.assignee || "unassigned"}</dd>
      </dl>
    </section>
  );
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

Update `apps/web/src/app/router.tsx` so task routes use the real pages:

```tsx
import { createBrowserRouter, Navigate } from "react-router-dom";
import { AppShell } from "./layout/app-shell";
import { TaskOverviewPage } from "../features/overview/task-overview-page";
import { TaskListPage } from "../features/tasks/task-list-page";
import { TaskDetailPage } from "../features/tasks/task-detail-page";

export const router = createBrowserRouter([
  {
    path: "/",
    element: <Navigate to="/projects/1/overview" replace />,
  },
  {
    path: "/projects/:projectId",
    element: <AppShell />,
    children: [
      { path: "overview", element: <TaskOverviewPage /> },
      { path: "tasks", element: <TaskListPage /> },
      { path: "tasks/:taskId", element: <TaskDetailPage /> },
    ],
  },
]);
```

Append this section to `README.md` and `docs/development/local-quickstart.md`:

```md
### Run the web console

1. `make web-install`
2. `make web-dev`
3. Open the Vite URL and keep the API server running on `http://localhost:8080`
```

Append this section to `README.zh-CN.md` and `docs/development/local-quickstart.zh-CN.md`:

```md
### 运行 Web 控制台

1. `make web-install`
2. `make web-dev`
3. 打开 Vite 输出的本地地址，并保持 API 服务运行在 `http://localhost:8080`
```

- [ ] **Step 4: Run the frontend and doc validation commands**

Run:

```bash
cd apps/web && npm test
cd apps/web && npm run build
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview ./internal/server ./cmd/api-server -v
```

Expected:

```text
✓ renders task list rows
✓ renders task detail metadata
PASS
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/tasks/api.ts apps/web/src/features/tasks/task-list-page.tsx apps/web/src/features/tasks/task-detail-page.tsx apps/web/src/features/tasks/task-pages.test.tsx apps/web/src/app/router.tsx Makefile README.md README.zh-CN.md docs/development/local-quickstart.md docs/development/local-quickstart.zh-CN.md
git commit -m "feat: add task pages and frontend dev workflow"
```

## Self-Review

### Spec Coverage

This phase plan covers the Phase 1 goals from the master plan:

1. First-class task resources.
2. Overview aggregation for the Phase 1 slice: blockers, review backlog, open tasks, and failed-job signal.
3. `/v1/projects/{id}/overview`, `/v1/projects/{id}/tasks`, and `/v1/tasks/{id}` routes.
4. `apps/web` bootstrap.
5. `Task Overview`, `Task List`, and `Task Detail`.
6. Frontend dev/test/build commands.

The full V1 `Task Overview` from the specs also calls for data-production, training/evaluation, and artifact-recommendation sections. Those are intentionally deferred to later phases because the supporting domains do not exist yet.

### Placeholder Scan

There are no `TODO`, `TBD`, or "implement later" placeholders. Every task contains concrete files, tests, commands, and code snippets.

### Type Consistency

This plan uses one consistent vocabulary:

1. `Task`
2. `Overview`
3. `BlockerCard`
4. `CreateTaskInput`
5. `ListTasksFilter`
6. `TaskOverviewPage`
7. `TaskListPage`
8. `TaskDetailPage`

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-03-31-yolo-platform-phase-1-task-kernel-and-web-shell.md`.

Subagent-Driven execution was already selected, so the next step is to dispatch Task 1 implementation, then run spec review, then run code-quality review before moving to Task 2.
