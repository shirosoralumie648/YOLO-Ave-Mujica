# Task Overview And Web Shell Design

- Date: 2026-04-01
- Scope: Deliver the first real V1 product entry with a task kernel, overview aggregation, and a usable web shell
- Status: Drafted from approved design discussion
- Owner: Platform team
- Parent Specs:
  - [2026-03-30-yolo-platform-product-framework-design.md](./2026-03-30-yolo-platform-product-framework-design.md)
  - [2026-03-30-yolo-platform-user-journey-and-interaction-design.md](./2026-03-30-yolo-platform-user-journey-and-interaction-design.md)

## 1. Objective

This design defines the first product-facing slice that moves the repository beyond a backend-only control-plane MVP.

The goal is to deliver four things together:

1. A first-class `Task` resource for human production work.
2. A stable `Overview` aggregation contract for the default product home page.
3. A real `apps/web` frontend shell based on `Vite + React + TypeScript`.
4. A usable `Task Overview` page plus a minimal `Task List` page so overview cards have real destinations.

This is intentionally not a full V1 implementation. It establishes the product entry, task kernel, and navigation spine that later annotation, review, training, and evaluation work will build on.

## 2. Scope And Constraints

### 2.1 In Scope

1. Add a new `tasks` table through a forward-only migration.
2. Add `internal/tasks` and `internal/overview` backend modules.
3. Add task create, list, and get APIs.
4. Add one overview API for project-scoped aggregation.
5. Add a standalone `apps/web` application shell.
6. Make `Task Overview` the default frontend route.
7. Add a minimal `Create Task` interaction from the overview page.
8. Add a minimal `Task List` page with URL-based filters.
9. Use live backend data for overview cards, blockers, review backlog, recent failed jobs, and longest-idle task.

### 2.2 Out Of Scope

1. `Task Detail` UI.
2. Annotation workspace rendering.
3. Review workspace rendering.
4. Formal publish-gate semantics.
5. Training and evaluation resources.
6. Auth, RBAC, and role-aware access control.
7. New AI-assist behaviors beyond consuming current job and review state.

### 2.3 Working Constraints

1. Existing MVP APIs must remain stable.
2. Existing Go control-plane runtime and Python worker flows must keep working without the frontend.
3. New task and overview APIs must follow the same handler and service structure already used by `internal/datahub`, `internal/jobs`, and `internal/review`.
4. The overview page must be live-data driven, not a static mock or a hard-coded product demo.
5. The slice must stop short of workspace scope creep. The default entry should become real without claiming that annotation and review workstations already exist.

## 3. Chosen Approach

### 3.1 Recommended Approach

Use a balanced Phase 1 slice with two backend modules and two frontend features:

1. `internal/tasks` owns the durable task resource.
2. `internal/overview` aggregates task, review, and job state into one page-oriented response.
3. `apps/web` provides the product shell and routing.
4. Frontend features are limited to `Task Overview` and `Task List`.

The `Task Overview` page supports a minimal action surface:

1. Read overview cards and blockers.
2. Create a task.
3. Navigate into a filtered task list using URL search params.

### 3.2 Why This Approach

1. It directly satisfies the design requirement that `Task Overview` be the default product entry.
2. It introduces the missing task kernel without prematurely committing to workspace-level UX.
3. It gives later phases stable backend contracts for task-first navigation.
4. It keeps scope below the larger Phase 1 plan that also includes `Task Detail`, which is useful but not required for this slice.

### 3.3 Alternatives Rejected

1. Overview-only with no task list was rejected because overview cards would dead-end into placeholders.
2. Full `Task Overview + Task List + Task Detail` was rejected for this iteration because it expands scope without unblocking the next major product slice enough to justify the cost.

## 4. Backend Design

### 4.1 Task Resource Model

Add a `tasks` table with the following fields:

1. `id`
2. `project_id`
3. `snapshot_id`
4. `title`
5. `kind`
6. `status`
7. `priority`
8. `assignee`
9. `due_at`
10. `blocker_reason`
11. `last_activity_at`
12. `created_at`
13. `updated_at`

Enum boundaries for this slice:

1. `kind`: `annotation`, `review`, `qa`, `ops`
2. `status`: `queued`, `ready`, `in_progress`, `blocked`, `done`
3. `priority`: `low`, `normal`, `high`, `critical`

Defaults:

1. `kind = annotation`
2. `status = queued`
3. `priority = normal`
4. `assignee = ''`
5. `blocker_reason = ''`
6. `last_activity_at = now()`

Validation rules:

1. `title` is required.
2. `project_id` comes from the route and must exist.
3. `snapshot_id` is optional, but if present it must resolve to a valid snapshot.
4. If `status = blocked`, `blocker_reason` is required.

### 4.2 Task Service And Repository Boundaries

`internal/tasks` should follow the current repository pattern:

1. `model.go` defines task enums and DTOs.
2. `repository.go` defines the repository contract and an in-memory implementation for focused tests.
3. `postgres_repository.go` implements durable storage.
4. `service.go` owns defaults, validation, and filter normalization.
5. `handler.go` performs HTTP translation only.

The backend should expose task rows with lightweight context fields derived by join:

1. `snapshot_version`
2. `dataset_id`
3. `dataset_name`

These are read-only response fields so the frontend does not need to reconstruct task context client-side.

### 4.3 Overview Aggregation Model

`internal/overview` should return one project-scoped payload optimized for `Task Overview`.

The response should include:

1. `summary_cards`
2. `blockers`
3. `longest_idle_task`
4. `recent_failed_jobs`

`summary_cards` should at minimum cover:

1. open tasks
2. blocked tasks
3. review backlog
4. failed recent jobs

Blocker derivation rules for this slice:

1. All tasks with `status = blocked` produce blocker entries.
2. Review backlog produces a blocker entry when pending candidate count is non-zero.
3. Recently failed jobs produce blocker entries from the jobs table.

`longest_idle_task` should be the non-done task with the oldest `last_activity_at`.

`recent_failed_jobs` should surface a small ordered set of recent terminal failures or retry-waiting jobs. The exact count can stay small and fixed for the first version.

### 4.4 Overview Data Sources

Overview aggregation should consume live state from:

1. the new `tasks` table
2. `annotation_candidates` pending rows through the existing review repository path
3. `jobs` state through the existing jobs repository path

This slice should not introduce a separate analytics store, background cache, or denormalized read model.

### 4.5 HTTP API Surface

Add the following routes:

1. `GET /v1/projects/{id}/overview`
2. `GET /v1/projects/{id}/tasks`
3. `POST /v1/projects/{id}/tasks`
4. `GET /v1/tasks/{id}`

`GET /v1/projects/{id}/tasks` supports these first-pass filters:

1. `status`
2. `kind`
3. `assignee`
4. `priority`
5. `snapshot_id`

`GET /v1/projects/{id}/overview` may also accept lightweight query shaping such as `assignee`, but it should remain a single stable page payload rather than a generic reporting endpoint.

### 4.6 Wiring And Composition

Runtime composition remains in [main.go](/home/shirosora/YOLO-Ave-Mujica/cmd/api-server/main.go).

Required composition changes:

1. Build and wire a PostgreSQL-backed `tasks` repository and service.
2. Build an `overview` service using task, review, and job repositories.
3. Extend [http_server.go](/home/shirosora/YOLO-Ave-Mujica/internal/server/http_server.go) with task and overview route groups.
4. Keep all existing MVP routes unchanged.

## 5. Frontend Design

### 5.1 App Shell

Create `apps/web` as a standalone Vite application using:

1. React
2. TypeScript
3. React Router
4. TanStack Query
5. Vitest and Testing Library

The shell should feel like a production tool rather than a marketing page:

1. dense information layout
2. stable left navigation
3. persistent top context area
4. clear content sections

This is not the moment for a visual system detour. The UI should optimize for legibility, scanning, and future extension.

### 5.2 Routing

The first route set should be:

1. `/` -> `Task Overview`
2. `/tasks` -> `Task List`

`Task Overview` is the default entry because the approved specs define it as the product home and operational surface.

### 5.3 Task Overview Page

The page should have four stable sections:

1. summary cards
2. blockers
3. longest-idle task
4. create-task form

Page requirements:

1. Load live overview data on first render.
2. Show loading, empty, and error states clearly.
3. Allow overview cards to navigate into `/tasks` with URL filters.
4. Refresh overview data after successful task creation.

The create-task form should stay intentionally minimal:

1. `title`
2. optional `snapshot_id`
3. optional `kind`
4. optional `assignee`
5. optional `priority`

### 5.4 Task List Page

The first version of `Task List` should support:

1. project-scoped task listing
2. filters from URL search params
3. loading, empty, and error states
4. simple tabular or list presentation

This page is primarily a real navigation target for overview cards, not a full task-management console.

### 5.5 Context Inheritance

To match the user-journey spec, the frontend should preserve context in URL search params when moving from overview to task list.

Examples:

1. blocked card -> `/tasks?status=blocked`
2. my tasks card -> `/tasks?assignee=current-user`
3. review backlog card -> `/tasks?kind=review&status=ready`

This keeps the first frontend slice compatible with later deeper navigation without rebuilding route semantics.

## 6. Data Flow

### 6.1 Create Task Flow

1. User opens `Task Overview`.
2. Frontend calls `GET /v1/projects/{id}/overview`.
3. User submits the minimal create-task form.
4. Frontend calls `POST /v1/projects/{id}/tasks`.
5. On success, the form clears.
6. Frontend invalidates both overview and task-list queries.
7. Updated summary cards and idle-task state appear from live backend data.

### 6.2 Overview To Task List Flow

1. User clicks a summary card or blocker action.
2. Frontend navigates to `/tasks` with matching search params.
3. `Task List` reads filters from the URL.
4. Frontend calls `GET /v1/projects/{id}/tasks` with those filters.
5. The list reflects the same context the user selected from overview.

## 7. Error Handling And Empty States

### 7.1 Backend

1. Validation errors return `400`.
2. Missing project or task returns `404`.
3. Invalid enum values return `400`.
4. If a blocked task is created without `blocker_reason`, return `400`.

### 7.2 Frontend

1. `Task Overview` must render a clear failed-load state instead of a blank shell.
2. `Task List` must render a no-results empty state when filters match nothing.
3. Create-task errors should remain inline and recoverable without leaving the page.

## 8. Testing And Acceptance

### 8.1 Backend Tests

Add or update focused tests for:

1. `internal/tasks/service_test.go`
2. `internal/tasks/handler_test.go`
3. `internal/overview/service_test.go`
4. `internal/server/http_server_routes_test.go`

If feasible in the same slice, add PostgreSQL integration coverage for the task repository using the existing integration-test style already present in the repo.

### 8.2 Frontend Tests

Add tests for:

1. `Task Overview` loading, success, and failure states
2. create-task success path
3. overview card navigation to filtered task-list URLs
4. `Task List` filter parsing, empty state, and failure state

### 8.3 Validation Commands

The slice should validate with:

1. `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview ./internal/server -v`
2. `cd apps/web && npm test`
3. `cd apps/web && npm run build`

### 8.4 Exit Criteria

This slice is complete only if:

1. `apps/web` runs locally and renders a real `Task Overview`.
2. `Task Overview` is the default frontend route.
3. Users can create tasks from the overview page.
4. Overview cards deep-link into a working filtered `Task List`.
5. Overview values come from live backend state rather than hard-coded fixtures.
6. The repository still does not claim `Task Detail`, workspaces, publish-gate semantics, training, or evaluation as complete.

## 9. Follow-On Work Intentionally Deferred

The next phases should build on this slice rather than reopen it:

1. `Task Detail`
2. `Review Queue` and `Review Workspace`
3. `Publish Candidates`
4. `Annotation Workspace`
5. training and evaluation resources
6. auth and RBAC

This design is successful if it creates a trustworthy first product entry and a stable task-first backend contract, not if it tries to impersonate the whole V1 roadmap.
