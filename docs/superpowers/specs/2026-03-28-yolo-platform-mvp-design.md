# YOLO Platform MVP Design (Data Hub + Job Orchestrator)

- Date: 2026-03-28
- Scope: 6-8 week MVP
- Status: Draft validated in brainstorming session
- Owner: Platform team

## 1. Goals And Non-Goals

### 1.1 Goals

1. Build a production-oriented MVP without Kubernetes for now, while preserving future migration paths.
2. Use Go as the backend control plane and Python containerized workers for model/compute workloads.
3. Deliver Data Hub capabilities:
   - S3 mount-like indexing (no data copy)
   - immutable dataset snapshots
   - import/export for COCO and YOLO
4. Deliver Job Orchestrator capabilities:
   - async jobs with queueing
   - idempotency and retries
   - heartbeat and timeout recovery
5. Deliver first AI productivity features:
   - zero-shot labeling pipeline (Grounded-DINO + SAM2)
   - snapshot diff foundation
   - basic cleaning service rules

### 1.2 Non-Goals (MVP)

1. No Kubernetes deployment in MVP stage.
2. No plugin marketplace in MVP stage.
3. No full multi-tenant billing or quota system in MVP stage.
4. No full hardware benchmark matrix (TensorRT/OpenVINO/RKNN) in MVP stage.

## 2. Architecture Decision

### 2.1 Chosen Architecture

Modular monolith in Go for control plane + containerized Python microservices for compute plane.

1. Go services (non-container, managed by systemd):
   - API server
   - Data Hub module
   - Versioning module
   - Job Orchestrator module
2. Python workers (containerized):
   - zero-shot-worker
   - video-worker
   - cleaning-worker
3. Shared infra:
   - PostgreSQL for metadata/state
   - Redis for queueing and short-lived runtime signals
   - S3/MinIO for raw data and generated artifacts

### 2.2 Why This Option

1. Meets 6-8 week delivery speed target.
2. Keeps compute dependencies isolated in Python containers.
3. Preserves future Kubernetes migration by keeping services stateless and contract-driven.
4. Avoids early distributed-system overhead while retaining clear module boundaries.

## 3. Component Responsibilities

### 3.1 API Server (Go)

1. Exposes REST APIs.
2. Handles authentication/authorization.
3. Validates payloads and writes command intent into orchestrator/data modules.
4. Returns immediately for long tasks with `job_id`.

### 3.2 Data Hub Module (Go)

1. Registers dataset to `bucket + prefix`.
2. Scans object metadata from S3 (without file migration).
3. Maintains dataset item index.
4. Creates immutable snapshots.
5. Runs import/export jobs through orchestrator.

### 3.3 Versioning Module (Go)

1. Manages snapshot lineage.
2. Stores annotation records linked to snapshot IDs.
3. Produces diff data (`add/remove/update`) between snapshots.

### 3.4 Job Orchestrator Module (Go)

1. Persists jobs with status machine.
2. Enqueues tasks into Redis by job type.
3. Handles idempotency key deduplication.
4. Tracks heartbeats and lease timeout.
5. Performs retry with backoff for transient failures.

### 3.5 Python Workers (Containerized)

1. Consume typed queue topics.
2. Execute compute tasks:
   - zero-shot model inference
   - video frame extraction
   - data quality checks
3. Write outputs to S3 first, then update job result metadata.
4. Emit heartbeat and structured logs.

## 4. Data Model

## 4.1 Core Tables

1. `projects`
   - `id, name, owner, created_at`
2. `datasets`
   - `id, project_id, name, storage_type, bucket, prefix, created_at`
3. `dataset_items`
   - `id, dataset_id, object_key, etag, size, width, height, mime, discovered_at`
4. `dataset_snapshots`
   - `id, dataset_id, version, based_on_snapshot_id, created_by, created_at, note`
5. `categories`
   - `id, project_id, name, alias_group, color`
6. `annotations`
   - `id, snapshot_id, item_id, category_id, bbox_x, bbox_y, bbox_w, bbox_h, polygon_json, score, source, model_name, created_at`
7. `annotation_changes`
   - `id, from_snapshot_id, to_snapshot_id, item_id, change_type, before_json, after_json, created_at`
8. `jobs`
   - `id, project_id, dataset_id, snapshot_id, job_type, status, priority, idempotency_key, payload_json, result_json, error_code, error_msg, retry_count, lease_until, created_at, started_at, finished_at`
9. `job_events`
   - `id, job_id, event_type, message, ts`
10. `audit_logs`
    - `id, actor, action, resource_type, resource_id, detail_json, ts`

### 4.2 Key Constraints

1. Unique index on `(project_id, job_type, idempotency_key)`.
2. Snapshot rows are immutable once created.
3. Annotation rows are snapshot-bound, never overwritten in place.

## 5. API Contract (MVP)

1. `POST /v1/datasets`
2. `POST /v1/datasets/{id}/scan`
3. `POST /v1/datasets/{id}/snapshots`
4. `GET /v1/datasets/{id}/snapshots`
5. `POST /v1/snapshots/{id}/import`
6. `POST /v1/snapshots/{id}/export`
7. `POST /v1/jobs/zero-shot`
8. `POST /v1/jobs/video-extract`
9. `POST /v1/jobs/cleaning`
10. `GET /v1/jobs/{job_id}`
11. `POST /v1/snapshots/diff`
12. `GET /v1/datasets/{id}/items`

### 5.1 API Conventions

1. Long-running operations are always async and return `job_id`.
2. Error classes:
   - `4xx` for validation/auth/business constraints
   - `429` for throttling
   - `5xx` for internal/system failures
3. All async responses include at minimum:
   - `job_id`
   - `status` (`queued` initially)

## 6. Job Orchestration Design

### 6.1 Status Machine

`queued -> running -> succeeded | failed | canceled`

Optional intermediate state:
`retry_waiting`

### 6.2 Idempotency

1. API requires `idempotency_key` for job-creation endpoints.
2. Duplicate request returns existing `job_id`.
3. Worker handlers must be side-effect safe for re-delivery.

### 6.3 Heartbeat And Timeout Recovery

1. Worker acquires lease and updates `lease_until` every `N` seconds.
2. Orchestrator sweeper marks stale `running` job as retryable if lease expired.
3. Retry path re-enqueues payload for transient error classes only.

### 6.4 Retry Policy

1. Transient errors: exponential backoff (`10s -> 30s -> 90s`, max configurable attempts).
2. Fatal errors: direct `failed` terminal state.
3. All failures write `error_code`, `error_msg`, and job event timeline.

### 6.5 Write Ordering

1. Worker writes artifact to S3 first.
2. Worker persists `result_json` and emits success event second.
3. Job is marked `succeeded` last.

This ordering prevents a success state with missing artifacts.

## 7. Diff And Cleaning (MVP Rules)

### 7.1 Snapshot Diff

1. Match annotations by `item_id + category_id`.
2. Use IoU threshold to classify updates.
3. Emit:
   - add
   - remove
   - update
4. Aggregate stats:
   - per-class count delta
   - total box delta
   - average IoU drift for updated boxes

### 7.2 Cleaning Rules (First Batch)

1. Bounding box with zero/negative area.
2. Category label mismatch against project taxonomy.
3. Extremely dark image score below configurable threshold.

Outputs:
1. machine-readable report (JSON)
2. optional candidate-removal list for user review

## 8. Observability And Operations

### 8.1 Logging

1. Structured JSON logs for Go and Python.
2. Mandatory context fields:
   - `trace_id`
   - `job_id` (if available)
   - `project_id` (if available)

### 8.2 Metrics

1. Queue depth by job type.
2. Job latency (P50/P95).
3. Success/failure/retry rates.
4. Lease-timeout recovery count.

### 8.3 Health Endpoints

1. `/healthz` for liveness.
2. `/readyz` for dependency readiness.

## 9. Security And Access Baseline

1. S3 credentials managed via environment variables or secret manager.
2. Principle of least privilege for S3 bucket access.
3. Input validation for all import payloads and prompt fields.
4. Audit log for dataset/snapshot/job mutations.

## 10. Testing Strategy (MVP)

1. Unit tests:
   - job state machine transitions
   - idempotency behavior
   - diff classification logic
2. Integration tests:
   - dataset scan to snapshot creation
   - job enqueue to worker completion
   - retry and timeout recovery path
3. Contract tests:
   - OpenAPI schema checks
   - worker payload schema validation
4. Smoke tests:
   - zero-shot sample run on a small dataset
   - export artifact readability

## 11. Implementation Plan (6-8 Weeks)

### Week 1-2

1. Data Hub object indexing from S3.
2. Core tables and migrations for datasets/snapshots/items.
3. API endpoints for dataset create, scan, snapshot create/list.

### Week 3-4

1. Jobs tables and orchestrator core.
2. Redis queue integration.
3. Worker heartbeat, lease, retry, timeout sweeper.

### Week 5-6

1. zero-shot-worker integration (Grounded-DINO + SAM2).
2. artifact writeback to S3 + metadata result link.
3. API endpoint and result retrieval flow.

### Week 7-8

1. snapshot diff API and aggregation output.
2. cleaning-worker with first batch rules.
3. reliability hardening and end-to-end MVP runbook.

## 12. Repository Skeleton

```text
YOLO-Ave-Mujica/
  cmd/
    api-server/
  internal/
    datahub/
    versioning/
    jobs/
    audit/
    storage/
    queue/
  api/
    openapi/
    proto/
  workers/
    zero-shot/
    video/
    cleaning/
  deploy/
    systemd/
    docker/
    env/
  migrations/
  docs/
    superpowers/
      specs/
```

## 13. Production Migration Path

1. Keep services stateless and config-driven.
2. Keep job contracts and storage contracts stable.
3. Move deployment layer from `systemd + docker` to Kubernetes later.
4. Reuse same APIs, schemas, and queue semantics during migration.

## 14. Risks And Mitigations

1. Risk: Python dependency drift across workers.
   - Mitigation: per-worker pinned image and lockfile.
2. Risk: Long-running job stuck in `running`.
   - Mitigation: lease-based recovery sweeper.
3. Risk: S3 scan cost growth on massive bucket prefixes.
   - Mitigation: incremental scan marker and prefix partitioning.
4. Risk: Pseudo-label quality variance.
   - Mitigation: confidence thresholds + manual review workflow.

## 15. Exit Criteria For MVP

1. Data Hub supports stable S3 indexing and immutable snapshots.
2. Job Orchestrator supports async execution with idempotency and retry recovery.
3. Zero-shot pipeline can produce pseudo labels and persist outputs.
4. Snapshot diff and base cleaning rules are operational.
5. One-click local deployment runbook (Go processes + Python containers) works in development.
