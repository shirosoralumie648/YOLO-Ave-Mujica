# Task Detail And Status Transition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the next task-first slice by adding a minimal task status transition API, a live `Task Detail` page, and deep links from overview/list views into task detail.

**Architecture:** Extend `internal/tasks` with one constrained transition path instead of a generic workflow engine. Keep the existing Go module wiring pattern in `internal/server` and `cmd/api-server`, then add a focused `Task Detail` React page in `apps/web` that fetches one task, triggers transitions with TanStack Query mutations, and reuses the existing shell and route style.

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, Vite, React, TypeScript, React Router, TanStack Query, Vitest, Testing Library.

---

## File Structure

**Create**

- `apps/web/src/features/tasks/task-detail-page.tsx`
- `apps/web/src/features/tasks/task-detail-page.test.tsx`

**Modify**

- `internal/tasks/model.go`
- `internal/tasks/repository.go`
- `internal/tasks/postgres_repository.go`
- `internal/tasks/postgres_repository_test.go`
- `internal/tasks/service.go`
- `internal/tasks/service_test.go`
- `internal/tasks/handler.go`
- `internal/tasks/handler_test.go`
- `internal/overview/service.go`
- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `cmd/api-server/main.go`
- `cmd/api-server/main_test.go`
- `apps/web/src/app/router.tsx`
- `apps/web/src/app/styles.css`
- `apps/web/src/features/tasks/api.ts`
- `apps/web/src/features/tasks/task-list-page.tsx`
- `apps/web/src/features/tasks/task-list-page.test.tsx`
- `apps/web/src/features/overview/task-overview-page.tsx`
- `apps/web/src/features/overview/task-overview-page.test.tsx`

## Task 1: Add Task Transition Domain Behavior

**Files:**

- Modify: `internal/tasks/model.go`
- Modify: `internal/tasks/repository.go`
- Modify: `internal/tasks/postgres_repository.go`
- Modify: `internal/tasks/postgres_repository_test.go`
- Modify: `internal/tasks/service.go`
- Modify: `internal/tasks/service_test.go`

- [ ] **Step 1: Write the failing task transition tests**

Append these tests to `internal/tasks/service_test.go`:

```go
func TestServiceTransitionTaskFollowsAllowedStatusPath(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	svc := NewService(repo)

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Review lane 4",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	ready, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusReady})
	if err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if ready.Status != StatusReady {
		t.Fatalf("expected ready, got %q", ready.Status)
	}

	inProgress, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusInProgress})
	if err != nil {
		t.Fatalf("transition to in_progress: %v", err)
	}
	if inProgress.Status != StatusInProgress {
		t.Fatalf("expected in_progress, got %q", inProgress.Status)
	}

	blocked, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{
		Status:        StatusBlocked,
		BlockerReason: "waiting for schema update",
	})
	if err != nil {
		t.Fatalf("transition to blocked: %v", err)
	}
	if blocked.Status != StatusBlocked || blocked.BlockerReason != "waiting for schema update" {
		t.Fatalf("unexpected blocked task: %+v", blocked)
	}

	resumed, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusInProgress})
	if err != nil {
		t.Fatalf("resume task: %v", err)
	}
	if resumed.Status != StatusInProgress {
		t.Fatalf("expected resumed in_progress, got %q", resumed.Status)
	}
	if resumed.BlockerReason != "" {
		t.Fatalf("expected blocker_reason to clear on resume, got %q", resumed.BlockerReason)
	}

	done, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusDone})
	if err != nil {
		t.Fatalf("transition to done: %v", err)
	}
	if done.Status != StatusDone {
		t.Fatalf("expected done, got %q", done.Status)
	}
}

func TestServiceTransitionTaskRejectsInvalidTransitionsAndBlockedWithoutReason(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository())

	created, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID: 1,
		Title:     "Annotate night dock",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusDone}); err == nil {
		t.Fatal("expected queued -> done transition to fail")
	}

	if _, err := svc.TransitionTask(ctx, created.ID, TransitionTaskInput{Status: StatusBlocked}); err == nil {
		t.Fatal("expected blocked transition without blocker_reason to fail")
	}
}
```

Append this integration test to `internal/tasks/postgres_repository_test.go`:

```go
func TestPostgresRepositoryTransitionTaskRoundTrip(t *testing.T) {
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

	projectName := "tasks-transition-project-" + time.Now().UTC().Format("20060102150405.000000000")
	var projectID int64
	if err := pool.QueryRow(ctx, `
		insert into projects (name, owner)
		values ($1, 'test-owner')
		returning id
	`, projectName).Scan(&projectID); err != nil {
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks -count=1 -v
```

Expected: FAIL with undefined transition types or methods such as `TransitionTaskInput`, `TransitionTask`, or missing repository support.

- [ ] **Step 3: Add the transition types and repository methods**

Update `internal/tasks/model.go` to add:

```go
type TransitionTaskInput struct {
	Status        string    `json:"status"`
	BlockerReason string    `json:"blocker_reason"`
	LastActivityAt time.Time `json:"last_activity_at"`
}
```

Update `internal/tasks/repository.go`:

```go
type Repository interface {
	CreateTask(ctx context.Context, in CreateTaskInput) (Task, error)
	ListTasks(ctx context.Context, projectID int64, filter ListTasksFilter) ([]Task, error)
	GetTask(ctx context.Context, taskID int64) (Task, error)
	TransitionTask(ctx context.Context, taskID int64, in TransitionTaskInput) (Task, error)
}
```

Implement `TransitionTask` in the in-memory repository so it updates `status`, `blocker_reason`, `last_activity_at`, and `updated_at`.

- [ ] **Step 4: Add service-level transition validation**

Update `internal/tasks/service.go` to add:

```go
func (s *Service) TransitionTask(ctx context.Context, taskID int64, in TransitionTaskInput) (Task, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}

	in.Status = normalizeStatus(in.Status)
	in.BlockerReason = strings.TrimSpace(in.BlockerReason)
	if in.LastActivityAt.IsZero() {
		in.LastActivityAt = time.Now().UTC()
	}

	if !isValidStatus(in.Status) {
		return Task{}, fmt.Errorf("invalid status %q", in.Status)
	}
	if !isAllowedTransition(current.Status, in.Status) {
		return Task{}, fmt.Errorf("invalid status transition %q -> %q", current.Status, in.Status)
	}
	if in.Status == StatusBlocked && in.BlockerReason == "" {
		return Task{}, fmt.Errorf("blocker_reason is required when status is blocked")
	}
	if in.Status != StatusBlocked {
		in.BlockerReason = ""
	}

	return s.repo.TransitionTask(ctx, taskID, in)
}
```

Add:

```go
var allowedTransitions = map[string]map[string]bool{
	StatusQueued: {
		StatusReady: true,
	},
	StatusReady: {
		StatusInProgress: true,
		StatusBlocked:    true,
	},
	StatusInProgress: {
		StatusBlocked: true,
		StatusDone:    true,
	},
	StatusBlocked: {
		StatusReady:      true,
		StatusInProgress: true,
	},
}

func isAllowedTransition(from, to string) bool {
	return allowedTransitions[from][to]
}
```

- [ ] **Step 5: Add PostgreSQL transition persistence**

Update `internal/tasks/postgres_repository.go` with:

```go
func (r *PostgresRepository) TransitionTask(ctx context.Context, taskID int64, in TransitionTaskInput) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		with updated as (
			update tasks
			set status = $2,
			    blocker_reason = $3,
			    last_activity_at = $4,
			    updated_at = now()
			where id = $1
			returning id, project_id, snapshot_id, title, kind, status, priority,
			          assignee, due_at, blocker_reason, last_activity_at, created_at, updated_at
		)
		select `+taskSelectColumns+`
		from updated tasks
		`+taskSelectJoins+`
	`, taskID, in.Status, in.BlockerReason, in.LastActivityAt)

	return scanTask(row)
}
```

- [ ] **Step 6: Run task package tests to verify they pass**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks -count=1 -v
```

Expected: PASS, with the integration transition test skipped unless `INTEGRATION_DATABASE_URL` is set.

## Task 2: Wire The Transition Endpoint Through HTTP

**Files:**

- Modify: `internal/tasks/handler.go`
- Modify: `internal/tasks/handler_test.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/main_test.go`

- [ ] **Step 1: Write the failing HTTP tests**

Append this test to `internal/tasks/handler_test.go`:

```go
func TestHandlerTransitionsTask(t *testing.T) {
	svc := NewService(NewInMemoryRepository())
	h := NewHandler(svc)

	created, err := svc.CreateTask(context.Background(), CreateTaskInput{
		ProjectID: 1,
		Title:     "Transition me",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/1/transition", strings.NewReader(`{"status":"ready"}`))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", strconv.FormatInt(created.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	rec := httptest.NewRecorder()
	h.TransitionTask(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected transition response, got %d %s", rec.Code, rec.Body.String())
	}
}
```

Add this route assertion to `internal/server/http_server_routes_test.go`:

```go
{http.MethodPost, "/v1/tasks/1/transition"},
```

Update `cmd/api-server/main_test.go` to assert the transition route is unwired in the test-only module setup:

```go
req := httptest.NewRequest(http.MethodPost, "/v1/tasks/1/transition", strings.NewReader(`{"status":"ready"}`))
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/server ./cmd/api-server -count=1 -v
```

Expected: FAIL with missing `TransitionTask` handler or route wiring.

- [ ] **Step 3: Add the handler and route wiring**

Update `internal/tasks/handler.go` with:

```go
func (h *Handler) TransitionTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var in TransitionTaskInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	task, err := h.svc.TransitionTask(r.Context(), taskID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}
```

Update `internal/server/http_server.go`:

```go
type TaskRoutes struct {
	ListTasks      http.HandlerFunc
	CreateTask     http.HandlerFunc
	GetTask        http.HandlerFunc
	TransitionTask http.HandlerFunc
}
```

and register:

```go
r.Post("/tasks/{id}/transition", handlerOrNotImplemented(m.Tasks.TransitionTask))
```

Update `cmd/api-server/main.go`:

```go
modules.Tasks = server.TaskRoutes{
	ListTasks:      taskHandler.ListTasks,
	CreateTask:     taskHandler.CreateTask,
	GetTask:        taskHandler.GetTask,
	TransitionTask: taskHandler.TransitionTask,
}
```

- [ ] **Step 4: Re-run the server tests to verify they pass**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/server ./cmd/api-server -count=1 -v
```

Expected: PASS.

## Task 3: Add Task Detail UI And Deep Links

**Files:**

- Create: `apps/web/src/features/tasks/task-detail-page.tsx`
- Create: `apps/web/src/features/tasks/task-detail-page.test.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/styles.css`
- Modify: `apps/web/src/features/tasks/api.ts`
- Modify: `apps/web/src/features/tasks/task-list-page.tsx`
- Modify: `apps/web/src/features/tasks/task-list-page.test.tsx`
- Modify: `apps/web/src/features/overview/task-overview-page.tsx`
- Modify: `apps/web/src/features/overview/task-overview-page.test.tsx`
- Modify: `internal/overview/service.go`

- [ ] **Step 1: Write the failing Task Detail tests**

Create `apps/web/src/features/tasks/task-detail-page.test.tsx`:

```tsx
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TaskDetailPage } from "./task-detail-page";

const originalFetch = global.fetch;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderPage(initialEntry = "/tasks/4") {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/tasks/:taskId" element={<TaskDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("TaskDetailPage", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    global.fetch = originalFetch;
  });

  it("loads task metadata and snapshot context", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock.mockResolvedValue(
      jsonResponse({
        id: 4,
        project_id: 1,
        snapshot_id: 12,
        snapshot_version: "v7",
        dataset_id: 5,
        dataset_name: "dock-night",
        title: "Review lane 4",
        kind: "review",
        status: "blocked",
        priority: "high",
        assignee: "reviewer-2",
        blocker_reason: "waiting for schema update",
        last_activity_at: "2026-04-01T08:00:00Z",
        created_at: "2026-04-01T08:00:00Z",
        updated_at: "2026-04-01T08:00:00Z",
      }),
    );

    renderPage();

    expect(await screen.findByRole("heading", { name: "Review lane 4" })).toBeInTheDocument();
    expect(screen.getByText(/dock-night/i)).toBeInTheDocument();
    expect(screen.getByText(/v7/i)).toBeInTheDocument();
    expect(screen.getByText(/waiting for schema update/i)).toBeInTheDocument();
  });

  it("transitions a blocked task back to in progress", async () => {
    const fetchMock = vi.mocked(global.fetch);
    fetchMock
      .mockResolvedValueOnce(
        jsonResponse({
          id: 4,
          project_id: 1,
          title: "Review lane 4",
          kind: "review",
          status: "blocked",
          priority: "high",
          assignee: "reviewer-2",
          blocker_reason: "waiting for schema update",
          last_activity_at: "2026-04-01T08:00:00Z",
          created_at: "2026-04-01T08:00:00Z",
          updated_at: "2026-04-01T08:00:00Z",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          id: 4,
          project_id: 1,
          title: "Review lane 4",
          kind: "review",
          status: "in_progress",
          priority: "high",
          assignee: "reviewer-2",
          blocker_reason: "",
          last_activity_at: "2026-04-02T08:00:00Z",
          created_at: "2026-04-01T08:00:00Z",
          updated_at: "2026-04-02T08:00:00Z",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          id: 4,
          project_id: 1,
          title: "Review lane 4",
          kind: "review",
          status: "in_progress",
          priority: "high",
          assignee: "reviewer-2",
          blocker_reason: "",
          last_activity_at: "2026-04-02T08:00:00Z",
          created_at: "2026-04-01T08:00:00Z",
          updated_at: "2026-04-02T08:00:00Z",
        }),
      );

    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Resume Task" }));

    expect(await screen.findByText(/in progress/i)).toBeInTheDocument();
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/v1/tasks/4/transition");
  });
});
```

Append this assertion to `apps/web/src/features/tasks/task-list-page.test.tsx`:

```tsx
expect(screen.getByRole("link", { name: "Blocked review batch" })).toHaveAttribute("href", "/tasks/1");
```

Append this assertion to `apps/web/src/features/overview/task-overview-page.test.tsx` after data loads:

```tsx
expect(screen.getByRole("link", { name: /blocked review batch/i })).toHaveAttribute("href", "/tasks/1");
```

- [ ] **Step 2: Run the frontend tests to verify they fail**

Run:

```bash
cd apps/web && npm test -- src/features/tasks/task-detail-page.test.tsx src/features/tasks/task-list-page.test.tsx src/features/overview/task-overview-page.test.tsx
```

Expected: FAIL because `TaskDetailPage` and transition APIs do not exist yet.

- [ ] **Step 3: Add task detail data access and transitions**

Update `apps/web/src/features/tasks/api.ts`:

```tsx
export function getTask(taskId: number | string) {
  return getJSON<TaskItem>(`/v1/tasks/${taskId}`);
}

export function transitionTask(taskId: number | string, payload: { status: string; blocker_reason?: string }) {
  return postJSON<TaskItem>(`/v1/tasks/${taskId}/transition`, payload);
}
```

Create `apps/web/src/features/tasks/task-detail-page.tsx` with:

```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { transitionTask, getTask } from "./api";
import { useParams } from "react-router-dom";

// render heading, metadata cards, blocker reason, and status action buttons
```

Required behavior:

1. Fetch `/v1/tasks/:taskId`.
2. Render dataset/snapshot context when available.
3. Show one action row based on current status:
   - `queued` -> `Mark Ready`
   - `ready` -> `Start Task`, `Mark Blocked`
   - `in_progress` -> `Mark Blocked`, `Mark Done`
   - `blocked` -> `Resume Task`, `Move To Ready`
4. When action is clicked, post to `/v1/tasks/:taskId/transition`, then invalidate/refetch `["task", taskId]` and `["tasks"]`.

- [ ] **Step 4: Add route and deep-link integration**

Update `apps/web/src/app/router.tsx`:

```tsx
import { TaskDetailPage } from "../features/tasks/task-detail-page";

// add child route
{ path: "tasks/:taskId", element: <TaskDetailPage /> },
```

Update `apps/web/src/features/tasks/task-list-page.tsx` so task titles are links:

```tsx
import { Link } from "react-router-dom";
...
<strong><Link to={`/tasks/${task.id}`}>{task.title}</Link></strong>
```

Update `internal/overview/service.go` so task blockers deep-link to detail:

```go
Href: fmt.Sprintf("/tasks/%d", task.ID),
```

Update `apps/web/src/features/overview/task-overview-page.tsx` so the longest idle task title links to `/tasks/:id`.

- [ ] **Step 5: Add minimal task detail styles**

Extend `apps/web/src/app/styles.css` with focused styles for:

1. `.detail-grid`
2. `.detail-meta`
3. `.detail-actions`
4. `.action-row`
5. `.button-secondary`

Keep the same visual language as the current shell.

- [ ] **Step 6: Run frontend verification**

Run:

```bash
cd apps/web && npm test
cd apps/web && npm run build
```

Expected: PASS.

## Task 4: Full Slice Verification

**Files:**

- Modify: none

- [ ] **Step 1: Run backend verification**

Run:

```bash
env GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/server ./cmd/api-server -count=1 -v
```

Expected: PASS.

- [ ] **Step 2: Run frontend verification**

Run:

```bash
cd apps/web && npm test
cd apps/web && npm run build
```

Expected: PASS.

- [ ] **Step 3: Re-check scope against the approved slice**

Confirm the completed work includes:

1. A minimal `POST /v1/tasks/{id}/transition` API.
2. Allowed status transitions only.
3. `Task Detail` page at `/tasks/:taskId`.
4. Deep links from task list and overview task entries into task detail.

Confirm the completed work still excludes:

1. Annotation workspace.
2. Review workspace.
3. Publish gate semantics.
4. Training and evaluation flows.
