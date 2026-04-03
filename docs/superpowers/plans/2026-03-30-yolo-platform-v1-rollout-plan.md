# YOLO Platform V1 Rollout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the current control-plane MVP into the first real V1 product slice sequence: a usable web console with Task Overview and task resources first, then the annotation/review workspaces, then the training/evaluation/artifact product loop, and finally the IAM/deployment hardening needed to treat the system as a real production platform.

**Architecture:** Keep the current Go modular monolith plus Python worker split as the backend foundation. Add a separate `apps/web` Vite + React + TypeScript frontend that consumes stable project/task/overview APIs. Deliver V1 as four short, testable slices that each leave the repository in a working state rather than attempting one oversized branch.

**Tech Stack:** Go 1.20, Chi, pgx/v5, PostgreSQL, Redis, MinIO/S3, Python 3.11+, Vite, React, TypeScript, React Router, TanStack Query, Testing Library/Vitest, bash smoke checks.

---

## Scope Note

I'm using the writing-plans skill to create the implementation plan.

This product scope spans multiple independent subsystems:

1. Web frontend bootstrap and routing
2. Project / task / overview control-plane resources
3. Annotation and review workstation UX
4. Training, evaluation, and artifact user-facing workflows
5. IAM, publish gates, deployment adaptation, and observability

Do not implement all of this in a single branch. Treat this document as a rollout plan. Each phase below should become its own execution plan before coding starts.

## Current Baseline

### Already Working

- Data Hub control-plane flow is live: dataset create, scan, item listing, snapshot create/list, object presign.
- Jobs support create, idempotency, queue lanes, event listing, worker heartbeat/progress callbacks, and sweeper recovery.
- Review and diff handlers exist and are backed by PostgreSQL.
- Artifact export, resolve, archive download, and CLI pull verification already work.
- Local PostgreSQL, Redis, and MinIO development flow plus smoke validation already exist.

### Missing Relative To The V1 Product Specs

- No web frontend exists yet.
- No task domain exists even though the product entry is supposed to be task-first.
- No Task Overview or Blockers View exists.
- No online annotation workspace exists.
- No review workspace UX exists.
- No user-facing training / evaluation / artifact console exists.
- No real IAM or role-aware UI flow exists.

## Phase 1: Web Console Shell, Project Tasks, And Task Overview

**Outcome:** The repository gains the first user-facing product slice: `apps/web`, a stable application shell, project/task resources, and a real `Task Overview` page backed by the control plane instead of static mocks.

**Files:**
- Create: `migrations/000003_tasks_and_overview.up.sql`
- Create: `migrations/000003_tasks_and_overview.down.sql`
- Create: `internal/tasks/model.go`
- Create: `internal/tasks/repository.go`
- Create: `internal/tasks/postgres_repository.go`
- Create: `internal/tasks/service.go`
- Create: `internal/tasks/handler.go`
- Create: `internal/tasks/handler_test.go`
- Create: `internal/overview/service.go`
- Create: `internal/overview/handler.go`
- Create: `internal/overview/service_test.go`
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
- Create: `apps/web/src/features/overview/api.ts`
- Create: `apps/web/src/features/overview/task-overview-page.tsx`
- Create: `apps/web/src/features/overview/task-overview-page.test.tsx`
- Create: `apps/web/src/features/tasks/task-list-page.tsx`
- Create: `apps/web/src/features/tasks/task-detail-page.tsx`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`

**Work Items:**
- [ ] Add a `tasks` table with lifecycle state, assignee, priority, due date, and `last_activity_at` so the product can represent real task ownership and idle-task detection.
- [ ] Add a lightweight overview aggregation service that computes role-facing cards such as open tasks, review backlog, failed recent jobs, blocker cards, and `Longest Idle Task`.
- [ ] Add `/v1/projects/{id}/overview`, `/v1/projects/{id}/tasks`, `POST /v1/projects/{id}/tasks`, and `GET /v1/tasks/{id}` routes without disturbing the current MVP routes.
- [ ] Bootstrap `apps/web` with a stable app shell, routing, query client, and production-oriented layout rather than a throwaway demo page.
- [ ] Implement `Task Overview` first, because it is the product's default entry and the root of every downstream workflow in the approved specs.
- [ ] Implement a minimal `Task List` and `Task Detail` so overview cards can deep-link into actionable pages with URL-preserved context.
- [ ] Add make targets for frontend install, dev, test, and build so local development is not hidden behind ad hoc shell history.

**Validation:**
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/tasks ./internal/overview ./internal/server -v`
- `cd apps/web && npm test`
- `cd apps/web && npm run build`
- `bash scripts/dev/smoke.sh`

**Exit Criteria:**
- Developers can run a real frontend locally from `apps/web`.
- The API exposes real project task and overview endpoints.
- `Task Overview` is the default usable product entry, not a stub page.
- `Longest Idle Task` and blocker cards are computed from live backend state, not hard-coded fixtures.

## Phase 2: Annotation Workspace And Review Workspace

**Outcome:** The first production workstations become real: Annotator and Reviewer can work online through dedicated pages instead of only interacting with control-plane APIs.

**Files:**
- Create: `internal/annotations/model.go`
- Create: `internal/annotations/repository.go`
- Create: `internal/annotations/postgres_repository.go`
- Create: `internal/annotations/service.go`
- Create: `internal/annotations/handler.go`
- Create: `internal/annotations/handler_test.go`
- Create: `internal/feedback/model.go`
- Create: `internal/feedback/repository.go`
- Create: `internal/feedback/postgres_repository.go`
- Create: `internal/feedback/service.go`
- Create: `internal/feedback/service_test.go`
- Create: `apps/web/src/features/workspace/annotation-workspace-page.tsx`
- Create: `apps/web/src/features/workspace/review-workspace-page.tsx`
- Create: `apps/web/src/features/workspace/canvas-runtime.ts`
- Create: `apps/web/src/features/workspace/object-list.tsx`
- Create: `apps/web/src/features/workspace/timeline-strip.tsx`
- Create: `apps/web/src/features/workspace/ai-candidate-panel.tsx`
- Create: `apps/web/src/features/workspace/object-persistence-checker.ts`
- Create: `apps/web/src/features/review/review-queue-page.tsx`
- Create: `apps/web/src/features/review/publish-candidates-page.tsx`
- Create: `apps/web/src/features/review/reason-code-form.tsx`
- Modify: `internal/review/service.go`
- Modify: `internal/review/handler.go`
- Modify: `internal/review/postgres_repository.go`
- Modify: `internal/server/http_server.go`
- Modify: `cmd/api-server/main.go`
- Modify: `workers/zero_shot/main.py`
- Modify: `workers/video/main.py`
- Modify: `workers/tests/test_queue_runner.py`

**Work Items:**
- [ ] Add working annotation persistence that separates draft, submitted, accepted, rejected, and published states instead of treating review output as the only writable data model.
- [ ] Add feedback persistence so Reject and Rework actions create structured `FeedbackRecord` rows with `reason_code`, `severity`, and `influence_weight`.
- [ ] Implement a Canvas-based annotation workspace for image and video tasks, keeping frame list, object list, and timeline virtualized from the start.
- [ ] Add an `Object Persistence Checker` that flags missing object IDs or broken trajectories across frames in video tasks.
- [ ] Implement a `Review Queue` and `Review Workspace` that reuses the rendering runtime but changes behavior toward Accept, Reject, Rework, and Escalate.
- [ ] Make Reject require a structured reason code path rather than a free-text-only comment.
- [ ] Add a `Publish Candidates` page that lets reviewers and owners see what is ready for publication and why.

**Validation:**
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/annotations ./internal/feedback ./internal/review -v`
- `cd apps/web && npm test`
- Focused browser/dev validation for long frame lists and object-list virtualization

**Exit Criteria:**
- Annotators can complete tasks online through a real workspace.
- Reviewers can process review queues through a real review workstation.
- Reject and Rework generate structured feedback.
- Video tasks remain operable under large frame counts.

## Phase 3: Training, Evaluation, Artifact Promotion, And CLI Upstream Flows

**Outcome:** The user-facing product loop becomes complete: published snapshots can lead to visible training runs, comparable evaluations, and recommendation/promotion decisions, with CLI/SDK feeding outputs back into the platform.

**Files:**
- Create: `internal/training/model.go`
- Create: `internal/training/repository.go`
- Create: `internal/training/postgres_repository.go`
- Create: `internal/training/service.go`
- Create: `internal/training/handler.go`
- Create: `internal/training/handler_test.go`
- Create: `internal/evaluation/model.go`
- Create: `internal/evaluation/repository.go`
- Create: `internal/evaluation/postgres_repository.go`
- Create: `internal/evaluation/service.go`
- Create: `internal/evaluation/handler.go`
- Create: `internal/evaluation/handler_test.go`
- Create: `apps/web/src/features/training/training-run-list-page.tsx`
- Create: `apps/web/src/features/training/training-run-detail-page.tsx`
- Create: `apps/web/src/features/evaluation/evaluation-compare-page.tsx`
- Create: `apps/web/src/features/evaluation/recommended-model-promotion-page.tsx`
- Create: `apps/web/src/features/artifacts/artifact-registry-page.tsx`
- Create: `apps/web/src/features/artifacts/artifact-detail-page.tsx`
- Create: `apps/web/src/features/artifacts/cli-sdk-access-page.tsx`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/verify.go`
- Modify: `cmd/platform-cli/main.go`
- Modify: `cmd/api-server/main.go`
- Modify: `internal/server/http_server.go`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Work Items:**
- [ ] Add `TrainingRun` resources and APIs that bind a run to one published snapshot and expose status, logs, curves, checkpoints, and environment context.
- [ ] Add `EvaluationReport` resources that bind a report to a `benchmark_snapshot_id` and reject direct horizontal comparison when benchmark context differs.
- [ ] Build `Training Run Detail` and `Evaluation Compare` pages that put comparability and provenance ahead of ranking.
- [ ] Add a real `Recommended Model Promotion` flow where ML Engineer nominates and Project Owner approves.
- [ ] Extend CLI/SDK contracts so they can upload logs, curves, checkpoints, and evaluation outputs, not only pull artifacts.
- [ ] Surface artifact lineage and promotion state in user-facing pages rather than only in raw API output.

**Validation:**
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/training ./internal/evaluation ./internal/artifacts ./internal/cli -v`
- `cd apps/web && npm test`
- Focused CLI verification against a seeded training/evaluation scenario

**Exit Criteria:**
- A published snapshot can produce a visible training run, evaluation report, and promoted artifact path through the product UI.
- Users cannot accidentally compare checkpoints under incompatible benchmark snapshots.
- CLI and backend agree on upstream reporting semantics.

## Phase 4: IAM, Role-Aware UX, Deployment Profiles, And Operational Hardening

**Outcome:** The product stops behaving like a trusted local demo and starts behaving like a real multi-role system that can survive self-hosted and hosted deployment shapes.

**Files:**
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/context.go`
- Create: `internal/auth/middleware_test.go`
- Create: `internal/audit/service.go`
- Create: `internal/audit/service_test.go`
- Create: `internal/projects/policy.go`
- Create: `apps/web/src/features/auth/login-page.tsx`
- Create: `apps/web/src/features/settings/members-and-roles-page.tsx`
- Create: `apps/web/src/features/settings/service-accounts-page.tsx`
- Create: `apps/web/src/features/settings/audit-log-page.tsx`
- Modify: `cmd/api-server/main.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/review/handler.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/tasks/handler.go`
- Modify: `internal/overview/handler.go`
- Modify: `internal/config/config.go`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Work Items:**
- [ ] Add a minimal but real auth layer for local and private deployments, starting with local account or static bearer support rather than leaving every mutation anonymous.
- [ ] Add project-scoped authorization checks over task creation, review decisions, artifact promotion, and sensitive data mutations.
- [ ] Add request-context propagation so user-facing pages can render role-aware task and blocker views.
- [ ] Add mutation audit coverage for snapshot creation, reject/rework, publish, training-run creation, and artifact promotion.
- [ ] Add deployment-profile documentation for local, hosted-like, and private-network flows, including how frontend and API are expected to run together.
- [ ] Harden operational visibility around review backlog, task idle time, training failure rate, and invalid evaluation counts.

**Validation:**
- Targeted auth and audit middleware tests
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/auth ./internal/audit ./internal/server -v`
- Manual role-aware UI validation through `apps/web`

**Exit Criteria:**
- Mutating flows are not anonymous by default.
- Role-aware UI logic exists for the main V1 surfaces.
- Audit coverage extends across the high-risk product actions defined in the specs.
- Local and self-hosted bring-up instructions cover both backend and frontend runtimes.

## Phase Ordering Rules

The phases above should execute in this order:

1. Phase 1 establishes the product shell and task-first entry point.
2. Phase 2 makes the main human production workstations real.
3. Phase 3 closes the training/evaluation/artifact product loop.
4. Phase 4 hardens the product into a role-aware deployable system.

Do not start Phase 2 before Phase 1 produces a stable web shell and task APIs.
Do not start Phase 3 before published snapshots, review workflows, and task contexts are stable enough to anchor training.
Do not leave Phase 4 as an indefinite afterthought if any environment beyond a single local developer machine is planned.

## Deliverable Boundaries

Each phase should end with:

1. Passing targeted tests.
2. Updated local-development docs.
3. A demoable vertical slice.
4. No stub routes or dead pages for the capabilities claimed by that slice.

## Suggested Next Detailed Plans

The next concrete execution plans should be written in this order:

1. `2026-03-30-yolo-platform-phase-1-web-shell-and-task-overview.md`
2. `2026-03-30-yolo-platform-phase-2-annotation-and-review-workspaces.md`
3. `2026-03-30-yolo-platform-phase-3-training-evaluation-and-artifacts.md`
4. `2026-03-30-yolo-platform-phase-4-iam-and-deployment-hardening.md`

## Self-Review

### Spec Coverage

This rollout plan maps the approved specs into four execution slices:

1. Task-first homepage and task domain from the user-journey spec are covered by Phase 1.
2. Annotation/review workstations and structured feedback are covered by Phase 2.
3. Training/evaluation/artifact promotion and CLI upstream reporting are covered by Phase 3.
4. IAM, publish hardening, deployment profiles, and audit requirements are covered by Phase 4.

No major V1 product area from the specs is intentionally omitted.

### Placeholder Scan

This plan intentionally avoids unresolved filler items inside each phase. Where details remain too large for one document, they are explicitly split into subsequent concrete phase plans.

### Type And Boundary Consistency

The plan uses the same resource vocabulary as the approved specs:

1. `Snapshot`
2. `Task`
3. `FeedbackRecord`
4. `TrainingRun`
5. `EvaluationReport`
6. `Artifact`
7. `benchmark_snapshot_id`
8. `Task Overview`
9. `Blockers View`
10. `Longest Idle Task`

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-03-30-yolo-platform-v1-rollout-plan.md`.

Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per phase, review between phases, fast iteration

**2. Inline Execution** - Execute phases in this session using executing-plans, batch execution with checkpoints

Which approach?
