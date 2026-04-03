# YOLO Platform Full V1 Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the repository from a working control-plane MVP to a complete V1 product that satisfies the approved product framework and user-journey specs.

**Architecture:** Keep the existing Go modular monolith plus Python worker split as the backend base. Add a dedicated `apps/web` Vite + React + TypeScript frontend, expand the kernel resource model around `Project`, `Task`, `Annotation`, `FeedbackRecord`, `TrainingRun`, and `EvaluationReport`, then close the human production loop in vertical slices instead of one oversized branch.

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, Redis, MinIO/S3, Python 3.11+, Vite, React, TypeScript, React Router, TanStack Query, Vitest, Testing Library, bash smoke checks.

---

## Scope Note

I'm using the writing-plans skill to create the implementation plan.

This project is too large for one execution branch. Treat this document as the master completion roadmap for V1. Each phase below should become its own short execution plan before coding starts.

This document supersedes the current planning split across:

1. `docs/superpowers/plans/2026-03-30-yolo-platform-completion-roadmap.md`
2. `docs/superpowers/plans/2026-03-30-yolo-platform-v1-rollout-plan.md`

Keep those files as historical references, but use this file as the current top-level completion plan.

## Current Audited Baseline

### Already Working Enough To Treat As Stable MVP Foundation

- Data Hub control-plane flow exists: dataset create, scan, item list, snapshot create/list, object presign, and snapshot import callback path.
- Jobs support create, idempotency, lane dispatch, heartbeat, progress, terminal callbacks, event listing, and lease sweeper recovery.
- Review supports pending-candidate list plus accept/reject transitions backed by PostgreSQL.
- Versioning supports snapshot diff against stored annotations.
- Artifact export supports queue/build/resolve/download/presign plus `platform-cli pull` verification.
- Python worker primitives for queue polling, callbacks, importer, packager, and cleaning are implemented and tested.

### Verified Current Reality

- `go test` passes for the main Go packages under `cmd/api-server`, `internal/datahub`, `internal/jobs`, `internal/review`, `internal/versioning`, `internal/artifacts`, and `internal/cli`.
- Worker-side Python tests pass for job client, queue runner, importer, packager, cleaning rules, and partial-success behavior.
- `scripts/dev` smoke tests are not currently trustworthy as the final source of truth because the fake `go` command used in `scripts/dev/smoke_test.go` does not allow the `go run ./cmd/api-server` call that `scripts/dev/smoke.sh` now performs.

### Missing Relative To The Approved V1 Product

- No web frontend exists.
- No real task domain exists for annotation work; only async jobs exist.
- No `Task Overview`, `Blockers View`, `Task List`, or `Task Detail` pages exist.
- No annotation workspace or review workspace exists.
- No publish gate exists; imported annotations can bypass formal review/publish semantics.
- No training, evaluation, benchmark comparison, or recommendation/promotion domain exists.
- CLI only supports pull; it does not support upstream upload for training outputs.
- No auth, RBAC, scope, or role-aware UI flow exists.
- No plugin runtime skeleton exists in runtime code.

## Program Rules

Every phase must end with:

1. Passing targeted tests.
2. Updated docs for local development and runtime behavior.
3. A demoable vertical slice.
4. No claimed feature area that still depends on dead routes or placeholder UI.

Do not start the next phase until the current phase has:

1. Stable resource contracts.
2. Verified migrations.
3. Explicit rollback plan.
4. Clear owner-facing acceptance criteria.

## Phase 0: Baseline Stabilization And Plan Decomposition

**Outcome:** The current MVP stops drifting under outdated assumptions, and the project gains reliable execution starting points for later product work.

**Files:**
- Modify: `scripts/dev/smoke.sh`
- Modify: `scripts/dev/smoke_test.go`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`
- Create: `docs/superpowers/plans/2026-03-31-yolo-platform-phase-1-task-kernel-and-web-shell.md`
- Create: `docs/superpowers/plans/2026-03-31-yolo-platform-phase-2-data-review-and-publish.md`
- Create: `docs/superpowers/plans/2026-03-31-yolo-platform-phase-3-workspace-and-ai-assist.md`
- Create: `docs/superpowers/plans/2026-03-31-yolo-platform-phase-4-training-evaluation-and-artifacts.md`
- Create: `docs/superpowers/plans/2026-03-31-yolo-platform-phase-5-iam-ops-and-deployment.md`

**Work Items:**
- [ ] Reconcile `scripts/dev/smoke.sh` and `scripts/dev/smoke_test.go` so the smoke path reflects the actual local bring-up flow.
- [ ] Update repository docs to describe the real current state: control-plane MVP complete, product V1 incomplete.
- [ ] Freeze the execution order for the next five phase plans so later work does not reopen scope debates.
- [ ] Define branch boundaries so frontend, resource-model, and training/evaluation work do not collide in one long-lived branch.

**Validation:**
- `go test ./scripts/dev -v`
- `bash scripts/dev/smoke.sh`

**Exit Criteria:**
- Smoke verification is reliable again.
- The repository docs match actual implementation status.
- The remaining work is decomposed into execution-ready phase plans.

## Phase 1: Product Kernel, Tasks, Feedback, And Web Shell

**Outcome:** The product gains the minimum kernel required for task-first V1 behavior: task resources, overview aggregation, publish-related metadata, and a real frontend shell.

**Files:**
- Create: `migrations/000003_task_kernel.up.sql`
- Create: `migrations/000003_task_kernel.down.sql`
- Create: `internal/tasks/model.go`
- Create: `internal/tasks/repository.go`
- Create: `internal/tasks/postgres_repository.go`
- Create: `internal/tasks/service.go`
- Create: `internal/tasks/handler.go`
- Create: `internal/tasks/handler_test.go`
- Create: `internal/overview/service.go`
- Create: `internal/overview/handler.go`
- Create: `internal/overview/service_test.go`
- Create: `internal/feedback/model.go`
- Create: `internal/feedback/repository.go`
- Create: `internal/feedback/postgres_repository.go`
- Create: `internal/feedback/service.go`
- Create: `apps/web/package.json`
- Create: `apps/web/vite.config.ts`
- Create: `apps/web/src/main.tsx`
- Create: `apps/web/src/app/router.tsx`
- Create: `apps/web/src/app/layout/app-shell.tsx`
- Create: `apps/web/src/features/overview/task-overview-page.tsx`
- Create: `apps/web/src/features/tasks/task-list-page.tsx`
- Create: `apps/web/src/features/tasks/task-detail-page.tsx`
- Modify: `internal/server/http_server.go`
- Modify: `cmd/api-server/main.go`
- Modify: `Makefile`

**Work Items:**
- [ ] Add first-class `Task` resources for annotation work instead of overloading async jobs.
- [ ] Add overview aggregation for role todo, review backlog, blocker cards, failed runs, and longest-idle task.
- [ ] Add `FeedbackRecord` persistence so reject/rework can become structured system state later.
- [ ] Bootstrap `apps/web` and make `Task Overview` the real default entry.
- [ ] Add `Task List` and `Task Detail` so overview cards have real deep links.

**Validation:**
- `go test ./internal/tasks ./internal/overview ./internal/feedback ./internal/server -v`
- `cd apps/web && npm test`
- `cd apps/web && npm run build`

**Exit Criteria:**
- The repository has a real frontend shell.
- The system can represent human work as tasks, not only background jobs.
- `Task Overview` exists as a live product page.

## Phase 2: Data Domain Pages, Snapshot Trust, And Publish Gate

**Outcome:** Data Manager and Reviewer workflows become trustworthy. Snapshots gain explicit publish state, import/export semantics are tightened, and publish is no longer implied by raw annotation writes.

**Files:**
- Modify: `migrations/000001_init.up.sql`
- Create: `migrations/000004_snapshot_publish_and_import_semantics.up.sql`
- Create: `migrations/000004_snapshot_publish_and_import_semantics.down.sql`
- Modify: `internal/datahub/model.go`
- Modify: `internal/datahub/repository.go`
- Modify: `internal/datahub/postgres_repository.go`
- Modify: `internal/datahub/service.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/versioning/service.go`
- Modify: `internal/versioning/repository.go`
- Modify: `internal/review/service.go`
- Modify: `internal/review/handler.go`
- Modify: `internal/review/postgres_repository.go`
- Create: `apps/web/src/features/data/dataset-list-page.tsx`
- Create: `apps/web/src/features/data/dataset-detail-page.tsx`
- Create: `apps/web/src/features/data/snapshot-detail-page.tsx`
- Create: `apps/web/src/features/data/snapshot-diff-page.tsx`
- Create: `apps/web/src/features/review/review-queue-page.tsx`
- Create: `apps/web/src/features/review/publish-candidates-page.tsx`
- Modify: `internal/server/http_server.go`
- Modify: `cmd/api-server/main.go`

**Work Items:**
- [ ] Add explicit snapshot trust and publish state instead of treating imported annotations as immediately formal.
- [ ] Make reject and rework collect structured feedback fields such as `reason_code`, `severity`, and `influence_weight`.
- [ ] Implement `Dataset List`, `Dataset Detail`, `Snapshot Detail`, and `Snapshot Diff` pages from the specs.
- [ ] Add `Publish Candidates` and the backend state needed to support formal publish gating.
- [ ] Decide and document whether COCO export is in V1 or explicitly deferred.

**Validation:**
- `go test ./internal/datahub ./internal/review ./internal/versioning -v`
- `cd apps/web && npm test`

**Exit Criteria:**
- Snapshots have an explicit publish story.
- Review no longer means only accept/reject without structured reasons.
- Data pages expose enough context for Data Manager and Reviewer workflows.

## Phase 3: Annotation Workspace, Review Workspace, And AI Assist MVP

**Outcome:** Annotators and Reviewers can do real online production work through dedicated workspaces, and AI assist becomes more than a stub counter flow.

**Files:**
- Create: `internal/annotations/model.go`
- Create: `internal/annotations/repository.go`
- Create: `internal/annotations/postgres_repository.go`
- Create: `internal/annotations/service.go`
- Create: `internal/annotations/handler.go`
- Modify: `workers/zero_shot/main.py`
- Modify: `workers/video/main.py`
- Modify: `workers/common/queue_runner.py`
- Modify: `workers/common/job_client.py`
- Create: `workers/tests/test_zero_shot_worker.py`
- Create: `workers/tests/test_video_worker.py`
- Create: `apps/web/src/features/workspace/annotation-workspace-page.tsx`
- Create: `apps/web/src/features/workspace/review-workspace-page.tsx`
- Create: `apps/web/src/features/workspace/canvas-runtime.ts`
- Create: `apps/web/src/features/workspace/object-list.tsx`
- Create: `apps/web/src/features/workspace/timeline-strip.tsx`
- Create: `apps/web/src/features/workspace/ai-candidate-panel.tsx`
- Create: `apps/web/src/features/workspace/object-persistence-checker.ts`
- Modify: `internal/server/http_server.go`
- Modify: `cmd/api-server/main.go`

**Work Items:**
- [ ] Add draft/submitted/reviewed/published annotation states separate from canonical review output.
- [ ] Build a usable image/video annotation workspace with task context and version context visible.
- [ ] Build a review workspace that exposes confidence, source model, previous modifier, and structured reject/rework controls.
- [ ] Make zero-shot jobs create review candidates instead of only returning terminal counters.
- [ ] Make video-extract jobs produce durable frame outputs that downstream task flows can consume.

**Validation:**
- `go test ./internal/annotations ./internal/review -v`
- `PYTHONPATH=. python3 -m unittest workers.tests.test_job_client workers.tests.test_queue_runner workers.tests.test_zero_shot_worker workers.tests.test_video_worker -v`
- `cd apps/web && npm test`

**Exit Criteria:**
- Annotators can complete online tasks.
- Reviewers can process online review queues.
- AI candidate lifecycle is visible and persists real candidate data.

## Phase 4: Training, Evaluation, CLI Upstream, And Artifact Product Loop

**Outcome:** The product closes the ML Engineer loop: published snapshots lead to training runs, evaluations, benchmark-aware comparison, promoted artifacts, and CLI upload flows.

**Files:**
- Create: `migrations/000005_training_and_evaluation.up.sql`
- Create: `migrations/000005_training_and_evaluation.down.sql`
- Create: `internal/training/model.go`
- Create: `internal/training/repository.go`
- Create: `internal/training/postgres_repository.go`
- Create: `internal/training/service.go`
- Create: `internal/training/handler.go`
- Create: `internal/evaluation/model.go`
- Create: `internal/evaluation/repository.go`
- Create: `internal/evaluation/postgres_repository.go`
- Create: `internal/evaluation/service.go`
- Create: `internal/evaluation/handler.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/verify.go`
- Modify: `cmd/platform-cli/main.go`
- Create: `apps/web/src/features/training/training-run-list-page.tsx`
- Create: `apps/web/src/features/training/training-run-detail-page.tsx`
- Create: `apps/web/src/features/evaluation/evaluation-compare-page.tsx`
- Create: `apps/web/src/features/evaluation/recommended-model-promotion-page.tsx`
- Create: `apps/web/src/features/artifacts/artifact-registry-page.tsx`
- Create: `apps/web/src/features/artifacts/artifact-detail-page.tsx`
- Create: `apps/web/src/features/artifacts/cli-sdk-access-page.tsx`
- Modify: `internal/server/http_server.go`
- Modify: `cmd/api-server/main.go`

**Work Items:**
- [ ] Add `TrainingRun` resources with status, logs, curves, checkpoints, and environment context.
- [ ] Add `EvaluationReport` resources with required `benchmark_snapshot_id`.
- [ ] Prevent direct comparison when evaluation benchmark context differs.
- [ ] Add recommended-model nomination and approval flow.
- [ ] Extend CLI from pull-only to pull-plus-upload for logs, curves, checkpoints, and evaluation results.
- [ ] Surface artifact lineage in web pages instead of only API JSON.

**Validation:**
- `go test ./internal/training ./internal/evaluation ./internal/artifacts ./internal/cli -v`
- `cd apps/web && npm test`

**Exit Criteria:**
- ML Engineer can complete the training-to-evaluation-to-artifact loop inside the product.
- CLI and backend share an explicit upstream reporting contract.
- Artifact pages reflect evaluation and promotion lineage.

## Phase 5: IAM, Audit, Observability, Deployment Profiles, And Plugin Skeleton

**Outcome:** The system becomes deployable as a real multi-role product instead of a trusted local MVP.

**Files:**
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/context.go`
- Create: `internal/auth/middleware_test.go`
- Create: `internal/audit/service.go`
- Create: `internal/audit/service_test.go`
- Create: `internal/observability/logging.go`
- Create: `internal/observability/metrics.go`
- Create: `internal/plugins/registry.go`
- Create: `internal/plugins/manifest.go`
- Create: `apps/web/src/features/auth/login-page.tsx`
- Create: `apps/web/src/features/settings/members-and-roles-page.tsx`
- Create: `apps/web/src/features/settings/service-accounts-page.tsx`
- Create: `apps/web/src/features/settings/audit-log-page.tsx`
- Modify: `cmd/api-server/main.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/config/config.go`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`
- Create: `docs/development/operations.md`
- Create: `docs/development/operations.zh-CN.md`

**Work Items:**
- [ ] Add minimal real authentication for local and private deployments.
- [ ] Add project-scoped authorization over task, review, publish, training, and artifact actions.
- [ ] Extend audit coverage beyond review accept/reject to snapshot import/export, publish, training creation, and artifact promotion.
- [ ] Add structured logging, metrics, and operational debugging guidance.
- [ ] Add plugin runtime skeleton boundaries so V1 can honestly claim built-in kernel plus future extension points.

**Validation:**
- `go test ./internal/auth ./internal/audit ./internal/observability ./internal/server -v`
- Manual role-aware web validation through `apps/web`

**Exit Criteria:**
- Mutating endpoints are not anonymous.
- High-risk actions have audit coverage.
- Operators can diagnose queue, job, and artifact failures without raw ad hoc inspection.
- The repo has a real plugin skeleton, even if not a marketplace.

## Mandatory Phase Order

1. Phase 0: Baseline stabilization and decomposition
2. Phase 1: Product kernel and web shell
3. Phase 2: Data domain, review, and publish gate
4. Phase 3: Annotation/review workspaces and AI assist MVP
5. Phase 4: Training/evaluation/artifact product loop
6. Phase 5: IAM, ops, deployment, and plugin skeleton

Do not start workspace implementation before task and publish semantics exist.
Do not start training/evaluation UI before publish, lineage, and feedback semantics are stable enough to anchor it.
Do not leave IAM and observability as an afterthought once the frontend is user-facing.

## Suggested Next Detailed Plans

Write and execute these plans in order:

1. `2026-03-31-yolo-platform-phase-1-task-kernel-and-web-shell.md`
2. `2026-03-31-yolo-platform-phase-2-data-review-and-publish.md`
3. `2026-03-31-yolo-platform-phase-3-workspace-and-ai-assist.md`
4. `2026-03-31-yolo-platform-phase-4-training-evaluation-and-artifacts.md`
5. `2026-03-31-yolo-platform-phase-5-iam-ops-and-deployment.md`

## Self-Review

### Spec Coverage

This plan covers the approved framework and journey specs in the following way:

1. Task-first homepage and blockers are covered by Phase 1.
2. Dataset/snapshot/review/publish trust flow is covered by Phase 2.
3. Annotation and review workstations plus AI candidate handling are covered by Phase 3.
4. Training/evaluation/artifact comparison and promotion are covered by Phase 4.
5. IAM, audit, observability, deployment, and plugin skeleton are covered by Phase 5.

No major V1 product area from the approved specs is intentionally omitted.

### Placeholder Scan

This document is intentionally a master roadmap, not a single branch execution checklist. The unresolved implementation detail is handled by the explicit next-phase plan files listed above rather than by hidden placeholders.

### Type And Boundary Consistency

The plan uses the same resource names and page vocabulary as the approved specs and current repository language:

1. `Task`
2. `Task Overview`
3. `Blockers View`
4. `Snapshot`
5. `FeedbackRecord`
6. `TrainingRun`
7. `EvaluationReport`
8. `benchmark_snapshot_id`
9. `Artifact`
10. `Publish Candidates`

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-03-31-yolo-platform-full-v1-completion-plan.md`.

Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per phase, review between phases, and keep each branch small

**2. Inline Execution** - Execute the phases in this session using executing-plans, with checkpoints between phases

Which approach?
