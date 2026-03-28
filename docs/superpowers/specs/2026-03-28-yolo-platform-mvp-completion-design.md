# YOLO Platform MVP Completion Design

- Date: 2026-03-28
- Scope: Complete the MVP on top of the current in-flight codebase
- Status: Drafted from approved completion discussion
- Owner: Platform team

## 1. Objective

This document defines how to complete the current MVP branch without discarding the in-progress implementation already present in the repository. The goal is to turn the existing foundation and partial completion work into a coherent, testable control-plane and CLI flow that satisfies the current branch definition of done:

1. Complete the remaining MVP feature path described by the existing completion plan.
2. Treat the current uncommitted changes as the implementation baseline rather than redoing the branch from the last clean commit.
3. Finish with both automated tests and local smoke checks passing.

This is a completion-focused design. It does not replace the broader architecture in `docs/superpowers/specs/2026-03-28-yolo-platform-mvp-design.md`; it narrows that architecture into an execution-safe implementation target for the current repository state.

## 2. Scope And Constraints

### 2.1 In Scope

1. Server route completion and module wiring for Data Hub, Jobs, Review, Versioning, and Artifacts.
2. Data Hub create, scan, item listing, snapshot creation/listing, and object presign flow.
3. Job creation, queue-lane dispatch, job event visibility, and lease recovery primitives.
4. Review candidate listing plus accept/reject trust-chain handling.
5. Snapshot diff API behavior with deterministic aggregate metrics.
6. Artifact package metadata flow, artifact presign, CLI pull verification, and local verification report output.
7. Local smoke coverage and development documentation updates required to validate the above behavior.

### 2.2 Out Of Scope

1. Re-architecting the MVP into a different service topology.
2. Full production-grade persistence for every module before any functional closure.
3. Broad refactors unrelated to the MVP main path.
4. New product scope beyond the current MVP and completion plan.

### 2.3 Working Constraints

1. The repository is already in a dirty state with in-progress completion work; those changes are the baseline for this iteration.
2. Existing tests must remain easy to run in isolation, so test-oriented in-memory implementations should be preserved where they improve determinism.
3. Runtime wiring should move toward PostgreSQL, Redis, and S3-backed behavior without forcing handlers to know which backing implementation they are using.
4. From the start of completion execution, schema changes must go through versioned migrations only; manual drift between code and database shape is not allowed.

## 3. Chosen Completion Strategy

### 3.1 Strategy

Use a layered completion approach with dual implementations:

1. Preserve in-memory repositories and pure-compute services as the default test path.
2. Add or complete PostgreSQL, Redis, and S3-backed runtime paths at the composition layer.
3. Finish the main user-visible MVP flow first, then extend supporting coverage around it.

### 3.2 Why This Strategy

This approach fits the current repository better than either a full rewrite or an API-only mock completion.

1. A full backend replacement would create the largest conflict surface with the current uncommitted work and is unlikely to be the fastest path to a passing, verifiable MVP.
2. An API-only completion on top of in-memory behavior would leave the branch short of the intended runtime design and would likely force immediate follow-up work.
3. A layered completion preserves existing test stability while making the actual `api-server` runtime behave more like the target architecture.

## 4. Module Boundaries

### 4.1 Server Composition

`cmd/api-server/main.go` is the sole runtime composition root.

It is responsible for:

1. Loading configuration.
2. Constructing concrete module dependencies.
3. Wiring handlers into `internal/server/http_server.go`.
4. Failing fast when required runtime dependencies are unavailable.

Handlers must not contain branching logic that switches between test and runtime implementations.

### 4.2 Data Hub

`internal/datahub/service.go` remains the orchestration layer and depends on:

1. `Repository` for dataset, item, and snapshot persistence.
2. `PresignFunc` for object access URLs.

Two repository implementations are allowed:

1. `InMemoryRepository` for focused tests.
2. A PostgreSQL-backed runtime implementation for local development execution.

Required stable behaviors:

1. Create dataset.
2. Scan object keys into indexed items.
3. List indexed items.
4. Create snapshots.
5. List snapshots.
6. Issue short-lived object presign URLs.

### 4.3 Jobs

`internal/jobs/service.go` is the job orchestration surface and should depend on:

1. Repository behavior for create, get, events, counters, and lease updates.
2. Publisher behavior for Redis-backed queue-lane dispatch.
3. Time control seams for lease and retry logic.

The Jobs module must expose:

1. Immediate async job creation with `job_id`.
2. Lane routing to `cpu`, `gpu`, or `mixed`.
3. Event visibility for external polling.
4. Lease timeout recovery via a sweeper path.

The MVP lease model is explicitly `heartbeat + lease_until timeout`:

1. A worker claims a job and records a stable `worker_id` for that execution attempt.
2. The worker refreshes `lease_until` on a fixed heartbeat cadence while work is active.
3. The sweeper treats an expired `lease_until` on a `running` job as abandoned work and decides whether to requeue or fail it.
4. Job events related to claim, heartbeat, timeout, recovery, and terminal failure should carry `worker_id` explicitly so lease churn can be traced across worker instances.

### 4.4 Review

`internal/review/service.go` owns trust-chain transitions.

It must keep the following behavior inside the service boundary:

1. List pending candidates.
2. Accept candidate and promote it to canonical annotation state.
3. Reject candidate without deleting review history.
4. Record audit-friendly reviewer metadata.

Handlers should only translate HTTP input and output.

### 4.5 Versioning

`internal/versioning/service.go` remains a pure calculation service.

Its responsibilities are:

1. Compare two annotation sets.
2. Produce `add`, `remove`, and `update` changes.
3. Compute stable aggregate metrics such as count deltas and average IOU drift.
4. Return a `compatibility_score` in the diff response so downstream review automation has a stable similarity signal to consume.

This module should stay detached from storage concerns for the current MVP completion pass.

`compatibility_score` must be deterministic and bounded to `[0,1]`. For the MVP completion pass it should be computed from the diff result as:

1. `baseline = max(len(before_annotations), len(after_annotations), 1)`.
2. `exact_matches = baseline - added_count - removed_count - updated_count`.
3. `weighted_similarity = exact_matches + sum(update.IOU for each updated annotation)`.
4. `compatibility_score = max(0, min(1, weighted_similarity / baseline))`.

This keeps the field simple enough for the MVP while avoiding an empty placeholder with no defined semantics.

### 4.6 Artifacts

The Artifacts module is split into:

1. Service: request package creation, read artifact metadata, presign artifact access.
2. Repository: persist artifact metadata and lifecycle state.
3. Packager helpers: generate `manifest.json`, `data.yaml`, and label-map aware export output.

CLI code must depend only on a source abstraction that returns standard artifact contents, not on internal repository details.

## 5. Main MVP Flow

The implementation must reliably support the following path:

1. Create dataset through `/v1/datasets`.
2. Index object keys through `/v1/datasets/{id}/scan`.
3. Validate indexed inventory through `/v1/datasets/{id}/items`.
4. Create a snapshot through `/v1/datasets/{id}/snapshots`.
5. Create async jobs through `/v1/jobs/*` and return `job_id` immediately.
6. Surface job status and events through `/v1/jobs/{job_id}` and `/v1/jobs/{job_id}/events`.
7. Route model-generated review candidates through `/v1/review/candidates` and accept/reject actions.
8. Produce snapshot diff output through `/v1/snapshots/diff`.
9. Create artifact package requests through `/v1/artifacts/packages`.
10. Query and presign artifacts through `/v1/artifacts/{id}` and `/v1/artifacts/{id}/presign`.
11. Pull artifact contents with `platform-cli pull`, verify checksums, and write `verify-report.json`.

This flow is the definition of functional completeness for the current iteration.

## 6. Error Handling And Operational Behavior

### 6.1 HTTP Behavior

1. Validation errors return `400`.
2. Missing resources return `404`.
3. Routes that are intentionally unwired during incremental construction may return `501`, but the completion target is to eliminate these for the MVP route surface.

### 6.2 Jobs And Retries

1. Job creation must remain idempotent on `idempotency_key`.
2. Dispatch failure should not silently lose a created job.
3. Lease-based recovery must be testable without real background infrastructure.
4. Event emission should record enough structured detail to explain partial success and retry behavior.
5. Lease and recovery events should include explicit `worker_id` attribution.

### 6.3 CLI Verification

1. CLI pull must fail on checksum mismatch by default.
2. `--allow-partial` may relax terminal behavior, but verification results must still be recorded.
3. A local verification report is required for reproducibility.
4. `verify-report.json` must include an `environment_context` object containing at least `os`, `arch`, `cli_version`, and `storage_driver`.

## 7. Testing And Acceptance

### 7.1 Automated Tests

The completion work must preserve or add focused tests in these categories:

1. Server route registration and base health/readiness behavior.
2. Migration application and schema compatibility checks against the current repository schema.
3. Data Hub handler and service behavior for scan, items, snapshots, and presign.
4. Job creation, idempotency, lane dispatch, event listing, and lease recovery behavior.
5. Review accept/reject handling and trust-chain state changes.
6. Versioning diff correctness, aggregate statistics, and `compatibility_score` output.
7. Artifact service and packager outputs.
8. CLI pull, verification behavior, and `environment_context` report output.
9. Worker-side client and rule-processing unit tests already present in the branch.

### 7.2 Local Smoke

`scripts/dev/smoke.sh` should validate more than liveness. At minimum it should cover:

1. Health and readiness endpoints.
2. Dataset creation.
3. Dataset scan.
4. Item listing.
5. Object presign response shape.
6. At least one async job creation path.

Local runtime smoke should use the repository's MinIO-backed development stack rather than a mock-only object store path. Presign host handling is part of the integration surface, so the smoke path should validate against the same MinIO endpoint family configured in local development.

If the local API is not already running, the smoke script may start a temporary process. Any required runtime assumptions should be documented in `docs/development/local-quickstart.md`.

### 7.3 Definition Of Done

This completion pass is done when:

1. The MVP route surface is wired and behaves consistently.
2. The main flow described in Section 5 is supported by implementation, not placeholder handlers.
3. Relevant unit tests pass.
4. Local smoke passes against the resulting runtime.
5. Documentation matches actual local development behavior.

## 8. Implementation Order

Recommended implementation order:

0. Lock schema management with `golang-migrate`, standardize on versioned migrations, and stop manual schema edits outside the migration flow.
1. Finish server composition and route wiring.
2. Stabilize Data Hub runtime and tests.
3. Complete Jobs runtime path, dispatch, and sweeper logic.
4. Finish Review and Versioning handler/service behavior.
5. Finish Artifacts plus CLI pull verification.
6. Update smoke coverage and local quickstart.

This order minimizes dead ends because each step increases the amount of the main path that can be validated end-to-end.
