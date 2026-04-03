# YOLO Platform Completion Roadmap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On top of the already working MVP control plane, close the remaining product and engineering gaps so the repository moves from "core flow is runnable" to "feature-complete, observable, and extensible dataset platform MVP".

**Architecture:** Keep the existing Go modular monolith plus Python worker split. Preserve the current PostgreSQL, Redis, MinIO, filesystem artifact storage, and CLI contracts where they already work, then replace stubs and thin adapters with real worker execution, stronger scheduling, richer artifact delivery, and operational controls in small, testable phases.

**Tech Stack:** Go 1.24, Chi, pgx/v5, Redis, MinIO/S3, Python 3.11+, unittest, Docker Compose, bash smoke checks.

---

## Scope Note

This is intentionally a decomposition roadmap, not a single batch implementation plan. The repository already spans multiple independent subsystems:

1. Control-plane APIs and persistence
2. Worker execution and queue semantics
3. Dataset import/export formats
4. Artifact delivery and CLI behavior
5. Security and access control
6. Observability and operational hardening

Do not execute all of this in one branch. Each phase below should become its own short execution plan before coding starts.

## Current State Summary

### Already Complete Enough To Treat As The Baseline

- Data Hub control-plane flow is working: dataset create, scan, item listing, snapshot create/list, and object presign.
- Jobs support async creation, idempotency, queue-lane dispatch, event listing, worker heartbeat/progress/terminal callbacks, and lease sweeper recovery.
- Review and Versioning have real handlers, service logic, PostgreSQL-backed runtime paths, and tests.
- Artifact export is not a placeholder: snapshot export, build runner, package materialization, download, resolve, and CLI verification all pass.
- The repository has both unit tests and a real smoke path. `go test ./...`, Python worker tests, and `bash scripts/dev/smoke.sh` all pass as of 2026-03-30.

### Still Incomplete Relative To The MVP Design

- Zero-shot and video workers are still stubs; they do not perform real model inference or frame extraction.
- Capability-aware scheduling is only partially implemented; jobs record capabilities but workers do not enforce them.
- Snapshot import/export only covers YOLO-oriented flows; COCO support is still absent.
- Artifact delivery still terminates through API/filesystem storage instead of direct object-storage delivery.
- Authentication, authorization, throttling, and broader mutation audit coverage are absent.
- Structured logs, metrics, trace context, and queue/job operational dashboards are absent.

## Phase 1: Replace Worker Stubs With Real Execution Paths

**Outcome:** Zero-shot, video, cleaning, importer, and packager workers stop behaving like contract demos and start producing domain outputs that the control plane can persist and inspect.

**Files:**
- Modify: `workers/zero_shot/main.py`
- Modify: `workers/video/main.py`
- Modify: `workers/cleaning/main.py`
- Modify: `workers/importer/main.py`
- Modify: `workers/packager/main.py`
- Modify: `workers/common/job_client.py`
- Modify: `workers/common/queue_runner.py`
- Modify: `internal/jobs/handler.go`
- Modify: `internal/jobs/service.go`
- Modify: `internal/review/postgres_repository.go`
- Create: `workers/tests/test_zero_shot_worker.py`
- Create: `workers/tests/test_video_worker.py`

**Work Items:**
- [ ] Add explicit worker-side lifecycle around `claimed -> heartbeat -> progress -> item_error -> terminal`, including periodic heartbeats for long jobs instead of one-shot callback usage.
- [ ] Make zero-shot jobs produce review candidates through a narrow internal callback or repository-backed API contract instead of only returning counters.
- [ ] Make video-extract jobs emit a deterministic list of extracted frame objects and persist job results that downstream flows can consume.
- [ ] Make cleaning jobs persist machine-readable reports and removal candidates so the API can expose or download them later.
- [ ] Keep importer and packager workers on the same callback contract, but move their payload parsing and error reporting onto the same progress model as other workers.
- [ ] Add job-event assertions for `heartbeat`, `progress`, `item_failed`, `lease_recovered`, and terminal states across worker tests.

**Validation:**
- `PYTHONPATH=. python3 -m unittest workers.tests.test_job_client workers.tests.test_queue_runner workers.tests.test_importer workers.tests.test_packager -v`
- Add and run targeted tests for zero-shot/video workers
- Add one smoke scenario that proves a non-stub worker produces durable output

**Exit Criteria:**
- Zero-shot no longer behaves as a counter-only stub.
- Video-extract no longer behaves as a counter-only stub.
- Cleaning results can be retrieved after execution.
- Every worker job can emit progress and at least one domain output or report.

## Phase 2: Finish Capability-Aware Scheduling And Job Contracts

**Outcome:** The job orchestrator matches workers by resource lane and declared capabilities instead of relying only on `job_type`.

**Files:**
- Modify: `internal/jobs/dispatcher.go`
- Modify: `internal/jobs/service.go`
- Modify: `internal/jobs/repository.go`
- Modify: `internal/jobs/postgres_repository.go`
- Modify: `workers/common/queue_runner.py`
- Modify: `workers/common/job_client.py`
- Modify: `internal/jobs/model.go`
- Modify: `internal/jobs/handler_test.go`
- Modify: `internal/jobs/postgres_repository_test.go`
- Modify: `workers/tests/test_queue_runner.py`

**Work Items:**
- [ ] Introduce explicit worker capability declarations and make queue consumers reject mismatched capability sets instead of only checking `job_type`.
- [ ] Add worker registration or startup metadata so the control plane can see what each worker instance claims to support.
- [ ] Implement transient-vs-fatal retry classification and configurable retry backoff instead of immediate requeue on lease recovery.
- [ ] Persist `result_artifact_ids_json` or an equivalent result reference for jobs that create artifacts or reports.
- [ ] Add job query fields for `worker_id`, `retry_count`, `lease_until`, and terminal error context so clients can debug stuck jobs without reading raw events.

**Validation:**
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/jobs -v`
- Worker contract tests proving mismatched capabilities are requeued or rejected predictably

**Exit Criteria:**
- Required capabilities affect execution, not just persistence.
- Retries are bounded, classified, and observable.
- Job detail responses expose enough state to debug worker churn.

## Phase 3: Complete Dataset Format Support And Versioning Semantics

**Outcome:** The repository supports the format and delta behaviors promised by the design, rather than only the currently working YOLO-first path.

**Files:**
- Modify: `internal/datahub/service.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/datahub/postgres_repository.go`
- Modify: `internal/versioning/service.go`
- Modify: `internal/versioning/repository.go`
- Modify: `internal/artifacts/export_query.go`
- Modify: `workers/importer/main.py`
- Create: `workers/common/coco.py`
- Create: `internal/datahub/import_formats.go`
- Create: `internal/datahub/import_formats_test.go`
- Create: `workers/tests/test_coco_importer.py`

**Work Items:**
- [ ] Add COCO import support alongside the existing YOLO importer flow.
- [ ] Define whether COCO export is part of this repository or explicitly defer it in docs and API contract.
- [ ] Start using `annotation_changes` or remove it from the design/runtime contract; the current schema should not promise change history that the code never writes.
- [ ] Add dataset-level import validation for unknown categories, unknown object keys, duplicate boxes, and invalid geometry.
- [ ] Extend diff responses with per-class count deltas if the team still wants the output promised in the original design.

**Validation:**
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub ./internal/versioning ./internal/artifacts -v`
- New importer tests for COCO payload parsing and canonical annotation writes

**Exit Criteria:**
- Import format support matches the documented MVP promise, or the docs are narrowed explicitly.
- Delta history semantics are either implemented or removed from the contract.
- Diff output is aligned with the design and tests.

## Phase 4: Harden Artifact Delivery And CLI Behavior

**Outcome:** Artifact storage and CLI pull move closer to the intended production flow, with stronger integrity semantics and less API coupling.

**Files:**
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/artifacts/storage.go`
- Modify: `internal/artifacts/filesystem_storage.go`
- Modify: `internal/cli/api_source.go`
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/verify.go`
- Modify: `cmd/api-server/main.go`
- Modify: `workers/packager/main.py`
- Modify: `workers/tests/test_packager.py`
- Modify: `internal/cli/pull_test.go`
- Modify: `internal/artifacts/handler_test.go`

**Work Items:**
- [ ] Decide whether runtime artifact storage should stay filesystem-backed for MVP or move to S3/MinIO-backed package objects. Implement one path cleanly instead of keeping hybrid semantics.
- [ ] If artifact readiness can be delayed, teach `platform-cli pull` to poll for readiness instead of assuming resolve succeeds immediately.
- [ ] Extend `manifest.json` with file size metadata and tighten CLI verification against both checksum and size.
- [ ] Add artifact-job linking so exported artifacts are traceable from the originating job.
- [ ] Make presign/download behavior consistent across filesystem and object-storage backends.

**Validation:**
- `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts ./internal/cli -v`
- `bash scripts/dev/smoke.sh`

**Exit Criteria:**
- CLI pull supports realistic "artifact not ready yet" behavior.
- Artifact metadata and manifest fields are sufficient for reproducibility checks.
- Storage backend semantics are explicit and not half-filesystem, half-S3.

## Phase 5: Add Security, Access Control, And Mutation Auditing

**Outcome:** The platform stops behaving like an unauthenticated local control plane and gains the minimum operational security promised by the design.

**Files:**
- Modify: `cmd/api-server/main.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/jobs/handler.go`
- Modify: `internal/review/handler.go`
- Modify: `internal/artifacts/handler.go`
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/context.go`
- Create: `internal/auth/middleware_test.go`
- Create: `internal/audit/service.go`
- Modify: `internal/review/postgres_repository.go`
- Modify: `api/openapi/mvp.yaml`

**Work Items:**
- [ ] Add a minimal auth layer for local/dev environments, even if it starts with static bearer tokens or reverse-proxy identity headers.
- [ ] Enforce authz on dataset, job, review, and artifact mutations instead of returning every object to every caller.
- [ ] Add mutation audit logging for dataset create, snapshot create/import/export, job creation, and artifact package creation.
- [ ] Add request validation for prompt length, import payload size, and rate-limited endpoints.
- [ ] Decide whether public/internal routes need separate auth treatment and document it.

**Validation:**
- Targeted auth middleware tests
- Updated handler tests for unauthorized and forbidden responses
- OpenAPI contract update for auth requirements

**Exit Criteria:**
- Mutating endpoints are not anonymous.
- Audit coverage extends beyond review accept/reject.
- API docs reflect actual auth behavior.

## Phase 6: Add Observability And Operational Hardening

**Outcome:** Operators can understand queue pressure, job health, worker failures, and artifact throughput without reading raw ad hoc logs.

**Files:**
- Modify: `cmd/api-server/main.go`
- Modify: `internal/jobs/service.go`
- Modify: `internal/jobs/sweeper.go`
- Modify: `internal/artifacts/build_runner.go`
- Modify: `workers/common/queue_runner.py`
- Modify: `workers/common/job_client.py`
- Modify: `scripts/dev/smoke.sh`
- Modify: `docs/development/local-quickstart.md`
- Modify: `docs/development/local-quickstart.zh-CN.md`
- Create: `internal/observability/logging.go`
- Create: `internal/observability/metrics.go`
- Create: `docs/development/operations.zh-CN.md`

**Work Items:**
- [ ] Standardize structured logging fields across Go and Python components.
- [ ] Add metrics for queue depth, job durations, lease recoveries, artifact build outcomes, and review backlog.
- [ ] Add trace/request correlation fields so worker callbacks can be tied back to the originating API request and job.
- [ ] Expand smoke or integration coverage to exercise retry recovery and partial-success terminal states.
- [ ] Document common failure modes and local debugging commands.

**Validation:**
- Logging and metrics unit tests where practical
- Smoke additions covering retry or partial-success paths
- Documentation review against actual runtime commands

**Exit Criteria:**
- Operators can answer "what is stuck, why, and where" from logs and metrics.
- Local runbooks match the current system behavior.

## Recommended Execution Order

1. Phase 1: Real worker execution
2. Phase 2: Capability-aware scheduling
3. Phase 3: Format and versioning semantics
4. Phase 4: Artifact and CLI hardening
5. Phase 5: Security and audit
6. Phase 6: Observability and operations

## Immediate Recommendation

If only one follow-up branch is started now, start with **Phase 1**. The current repository already proves the control-plane backbone works, but the biggest remaining product gap is that several worker flows are still stubbed. Finishing security or observability first would harden a system that still lacks its core AI and media behaviors.
