# YOLO Platform Phase 1 Web Shell And Task Overview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the first usable V1 product slice by adding project tasks, overview aggregation APIs, and a real `apps/web` shell with `Task Overview`, `Task List`, and `Task Detail`.

**Architecture:** Extend the Go control plane with new `tasks` and `overview` domains backed by PostgreSQL, then add a separate Vite + React + TypeScript app under `apps/web` that consumes those APIs. Keep the backend minimal but real: task lifecycle, blocker aggregation, and idle-task detection are computed server-side; the frontend focuses on task-first navigation and URL-driven context.

**Tech Stack:** Go 1.20, Chi, pgx/v5, PostgreSQL, Vite, React, TypeScript, React Router, TanStack Query, Vitest, Testing Library.

---

## File Structure

**Create**

- `migrations/000003_tasks_and_overview.up.sql`
- `migrations/000003_tasks_and_overview.down.sql`
- `internal/tasks/model.go`
- `internal/tasks/repository.go`
- `internal/tasks/postgres_repository.go`
- `internal/tasks/service.go`
- `internal/tasks/handler.go`
- `internal/tasks/handler_test.go`
- `internal/overview/model.go`
- `internal/overview/service.go`
- `internal/overview/handler.go`
- `internal/overview/service_test.go`
- `apps/web/package.json`
- `apps/web/tsconfig.json`
- `apps/web/tsconfig.node.json`
- `apps/web/vite.config.ts`
- `apps/web/index.html`
- `apps/web/src/main.tsx`
- `apps/web/src/app/router.tsx`
- `apps/web/src/app/query-client.ts`
- `apps/web/src/app/layout/app-shell.tsx`
- `apps/web/src/app/styles.css`
- `apps/web/src/lib/api.ts`
- `apps/web/src/features/overview/api.ts`
- `apps/web/src/features/overview/task-overview-page.tsx`
- `apps/web/src/features/overview/task-overview-page.test.tsx`
- `apps/web/src/features/tasks/api.ts`
- `apps/web/src/features/tasks/task-list-page.tsx`
- `apps/web/src/features/tasks/task-detail-page.tsx`

**Modify**

- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `cmd/api-server/main.go`
- `Makefile`
- `README.md`
- `README.zh-CN.md`
- `docs/development/local-quickstart.md`
- `docs/development/local-quickstart.zh-CN.md`

## Task 1: Add Task Persistence And HTTP APIs

**Files:**

- Create: `migrations/000003_tasks_and_overview.up.sql`
- Create: `migrations/000003_tasks_and_overview.down.sql`
- Create: `internal/tasks/model.go`
- Create: `internal/tasks/repository.go`
- Create: `internal/tasks/postgres_repository.go`
- Create: `internal/tasks/service.go`
- Create: `internal/tasks/handler.go`
- Create: `internal/tasks/handler_test.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`

- [ ] **Step 1: Write failing route-shape and handler tests**
- [ ] **Step 2: Run focused Go tests and confirm missing task routes fail**
- [ ] **Step 3: Add migration for `tasks` table**
- [ ] **Step 4: Implement task model, repository, service, and handlers**
- [ ] **Step 5: Wire `/v1/projects/{id}/tasks` and `/v1/tasks/{id}` into the HTTP server**
- [ ] **Step 6: Re-run focused Go tests until green**

## Task 2: Add Overview Aggregation Service And API

**Files:**

- Create: `internal/overview/model.go`
- Create: `internal/overview/service.go`
- Create: `internal/overview/handler.go`
- Create: `internal/overview/service_test.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`

- [ ] **Step 1: Write failing service tests for blocker cards, review backlog, failed jobs, and longest idle task**
- [ ] **Step 2: Run focused Go tests and confirm overview service is missing**
- [ ] **Step 3: Implement overview aggregation using tasks, jobs, and review repositories**
- [ ] **Step 4: Add `/v1/projects/{id}/overview` handler and server wiring**
- [ ] **Step 5: Re-run focused Go tests until green**

## Task 3: Bootstrap `apps/web`

**Files:**

- Create: `apps/web/package.json`
- Create: `apps/web/tsconfig.json`
- Create: `apps/web/tsconfig.node.json`
- Create: `apps/web/vite.config.ts`
- Create: `apps/web/index.html`
- Create: `apps/web/src/main.tsx`
- Create: `apps/web/src/app/router.tsx`
- Create: `apps/web/src/app/query-client.ts`
- Create: `apps/web/src/app/layout/app-shell.tsx`
- Create: `apps/web/src/app/styles.css`
- Create: `apps/web/src/lib/api.ts`
- Modify: `Makefile`

- [ ] **Step 1: Add a failing frontend smoke test for app-shell rendering**
- [ ] **Step 2: Initialize Vite + React + Vitest scaffolding**
- [ ] **Step 3: Add app shell, router, shared API client, and global styles**
- [ ] **Step 4: Add Makefile targets for frontend dev, test, and build**
- [ ] **Step 5: Run frontend tests and build**

## Task 4: Implement Task Overview, Task List, And Task Detail Pages

**Files:**

- Create: `apps/web/src/features/overview/api.ts`
- Create: `apps/web/src/features/overview/task-overview-page.tsx`
- Create: `apps/web/src/features/overview/task-overview-page.test.tsx`
- Create: `apps/web/src/features/tasks/api.ts`
- Create: `apps/web/src/features/tasks/task-list-page.tsx`
- Create: `apps/web/src/features/tasks/task-detail-page.tsx`
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/app/layout/app-shell.tsx`
- Modify: `apps/web/src/app/styles.css`

- [ ] **Step 1: Write failing page tests for overview and task routes**
- [ ] **Step 2: Implement API hooks and pages with URL-driven project/task context**
- [ ] **Step 3: Show `Longest Idle Task`, blocker cards, task list, and task detail context**
- [ ] **Step 4: Re-run frontend tests and build**

## Task 5: Update Docs And Verify Full Slice

**Files:**

- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`

- [ ] **Step 1: Document how to run backend + frontend together**
- [ ] **Step 2: Document new task and overview endpoints**
- [ ] **Step 3: Run final backend tests**
- [ ] **Step 4: Run final frontend tests and build**
- [ ] **Step 5: Record any verification gap before moving to Phase 2**

## Validation Commands

- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview ./internal/server -v`
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...`
- `cd apps/web && npm test`
- `cd apps/web && npm run build`

## Exit Criteria

- The backend exposes real task and overview endpoints.
- `Task Overview` is usable as the default product entry for project `1`.
- Users can navigate from overview to task list and task detail with URL-preserved context.
- `Longest Idle Task` and blocker cards are computed from live API data.
- Local docs explain how to run both backend and frontend.
