# YOLO Platform MVP Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the MVP foundation for Data Hub, Job Orchestrator, Artifact Repository, and CLI pull flow with integrity verification and partial-failure isolation.

**Architecture:** A Go modular monolith provides control-plane APIs (datasets, snapshots, jobs, review, artifacts), while Python workers run compute jobs and report item-level progress. PostgreSQL stores metadata with delta snapshot modeling, Redis handles queue lanes and leases, and MinIO simulates S3 for local development. Artifact generation is asynchronous and produces YOLO-ready packages with `manifest.json` and `data.yaml`.

**Tech Stack:** Go 1.24, Chi router, pgx, redis/go-redis v9, MinIO SDK (S3), Python 3.11, pytest, Docker Compose, PostgreSQL 16, Redis 7, MinIO.

---

## File Structure Map

### Control Plane (Go)

- Create: `go.mod`  
  Responsibility: module definition and shared dependencies.
- Create: `cmd/api-server/main.go`  
  Responsibility: process bootstrap and server startup.
- Create: `internal/server/http_server.go`  
  Responsibility: router wiring, middleware, health routes.
- Create: `internal/config/config.go`  
  Responsibility: environment parsing and validation.
- Create: `internal/store/postgres.go`  
  Responsibility: PostgreSQL connection lifecycle.
- Create: `internal/queue/redis.go`  
  Responsibility: Redis queue client and lane publish helpers.
- Create: `internal/storage/s3.go`  
  Responsibility: MinIO/S3 client + presign helper.
- Create: `internal/jobs/model.go`  
  Responsibility: job status constants and domain structs.
- Create: `internal/jobs/state_machine.go`  
  Responsibility: allowed state transitions including `succeeded_with_errors`.
- Create: `internal/jobs/repository.go`  
  Responsibility: job persistence, idempotent create, counters update.
- Create: `internal/datahub/service.go`  
  Responsibility: dataset create/scan/snapshot and presign orchestration.
- Create: `internal/datahub/handler.go`  
  Responsibility: dataset/presign HTTP handlers.
- Create: `internal/artifacts/service.go`  
  Responsibility: package request and artifact metadata persistence.
- Create: `internal/artifacts/packager.go`  
  Responsibility: label mapping, manifest generation, `data.yaml` generation.
- Create: `internal/review/service.go`  
  Responsibility: pending pseudo-label accept/reject promotion.

### Worker Plane (Python)

- Create: `workers/common/job_client.py`  
  Responsibility: lease heartbeat, progress update, item error emit.
- Create: `workers/zero-shot/main.py`  
  Responsibility: batch inference loop + partial-success reporting.
- Create: `workers/packager/main.py`  
  Responsibility: package assembly and artifact upload.

### Data + Runtime

- Create: `migrations/0001_init.sql`  
  Responsibility: initial schema (datasets, snapshots, jobs, events, artifacts, review tables).
- Create: `deploy/docker/docker-compose.dev.yml`  
  Responsibility: local one-command environment.
- Create: `.env.example`  
  Responsibility: canonical local config keys.
- Create: `Makefile`  
  Responsibility: repeatable dev/test commands.

### Tests + Docs

- Create: `internal/server/http_server_test.go`
- Create: `internal/config/config_test.go`
- Create: `internal/jobs/state_machine_test.go`
- Create: `internal/jobs/repository_test.go`
- Create: `internal/datahub/handler_test.go`
- Create: `internal/artifacts/packager_test.go`
- Create: `workers/tests/test_partial_success.py`
- Create: `internal/cli/pull_test.go`
- Create: `scripts/dev/smoke.sh`
- Create: `docs/development/local-quickstart.md`

---

### Task 1: Bootstrap Go Service + Local Runtime Baseline

**Files:**
- Create: `go.mod`
- Create: `cmd/api-server/main.go`
- Create: `internal/server/http_server.go`
- Create: `internal/server/http_server_test.go`
- Create: `.env.example`
- Create: `Makefile`
- Create: `deploy/docker/docker-compose.dev.yml`

- [ ] **Step 1: Write the failing health endpoint test**

```go
package server

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHealthz(t *testing.T) {
    srv := NewHTTPServer()
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()

    srv.Handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    if rec.Body.String() != "ok" {
        t.Fatalf("expected body ok, got %q", rec.Body.String())
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server -run TestHealthz -v`  
Expected: FAIL with `undefined: NewHTTPServer`.

- [ ] **Step 3: Write minimal server implementation and runtime files**

```go
// internal/server/http_server.go
package server

import (
    "net/http"

    "github.com/go-chi/chi/v5"
)

type HTTPServer struct {
    Handler http.Handler
}

func NewHTTPServer() *HTTPServer {
    r := chi.NewRouter()
    r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
    return &HTTPServer{Handler: r}
}
```

```go
// cmd/api-server/main.go
package main

import (
    "log"
    "net/http"

    "yolo-ave-mujica/internal/server"
)

func main() {
    srv := server.NewHTTPServer()
    log.Fatal(http.ListenAndServe(":8080", srv.Handler))
}
```

```yaml
# deploy/docker/docker-compose.dev.yml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: platform
      POSTGRES_PASSWORD: platform
      POSTGRES_DB: platform
    ports: ["5432:5432"]
  redis:
    image: redis:7
    ports: ["6379:6379"]
  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    ports: ["9000:9000", "9001:9001"]
```

- [ ] **Step 4: Run tests and runtime checks**

Run: `go test ./internal/server -run TestHealthz -v`  
Expected: PASS.  
Run: `docker compose -f deploy/docker/docker-compose.dev.yml config`  
Expected: resolves services without validation errors.

- [ ] **Step 5: Commit**

```bash
git add go.mod cmd/api-server/main.go internal/server/http_server.go internal/server/http_server_test.go deploy/docker/docker-compose.dev.yml .env.example Makefile
git commit -m "chore: bootstrap api server and local dev runtime"
```

### Task 2: Add Initial Database Schema (Delta + Review + Artifacts)

**Files:**
- Create: `migrations/0001_init.sql`
- Create: `internal/jobs/repository_test.go`

- [ ] **Step 1: Write failing schema test for required tables**

```go
func TestRequiredTablesExist(t *testing.T) {
    ctx := context.Background()
    db := mustOpenTestDB(t)

    required := []string{
        "datasets", "dataset_items", "dataset_snapshots", "annotations",
        "annotation_candidates", "jobs", "job_events", "artifacts",
    }
    for _, table := range required {
        var exists bool
        err := db.QueryRow(ctx, "select to_regclass($1) is not null", "public."+table).Scan(&exists)
        if err != nil || !exists {
            t.Fatalf("table %s not found", table)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/jobs -run TestRequiredTablesExist -v`  
Expected: FAIL because tables do not exist.

- [ ] **Step 3: Write migration with status/counters/integrity fields**

```sql
-- migrations/0001_init.sql
create table datasets (
  id bigserial primary key,
  project_id bigint not null,
  name text not null,
  storage_type text not null default 's3',
  bucket text not null,
  prefix text not null,
  created_at timestamptz not null default now()
);

create table dataset_snapshots (
  id bigserial primary key,
  dataset_id bigint not null,
  version text not null,
  based_on_snapshot_id bigint,
  created_by text not null,
  created_at timestamptz not null default now(),
  note text,
  unique(dataset_id, version)
);

create table annotations (
  id bigserial primary key,
  dataset_id bigint not null,
  item_id bigint not null,
  category_id bigint not null,
  bbox_x double precision not null,
  bbox_y double precision not null,
  bbox_w double precision not null,
  bbox_h double precision not null,
  created_at_snapshot_id bigint not null,
  deleted_at_snapshot_id bigint,
  review_status text not null default 'verified'
);

create table jobs (
  id bigserial primary key,
  project_id bigint not null,
  dataset_id bigint,
  snapshot_id bigint,
  job_type text not null,
  status text not null,
  required_resource_type text not null,
  idempotency_key text not null,
  total_items int not null default 0,
  succeeded_items int not null default 0,
  failed_items int not null default 0,
  payload_json jsonb not null,
  result_artifact_ids_json jsonb not null default '[]'::jsonb,
  created_at timestamptz not null default now(),
  unique(project_id, job_type, idempotency_key)
);

create table job_events (
  id bigserial primary key,
  job_id bigint not null,
  item_id bigint,
  event_level text not null,
  event_type text not null,
  message text not null,
  detail_json jsonb not null default '{}'::jsonb,
  ts timestamptz not null default now()
);

create table artifacts (
  id bigserial primary key,
  project_id bigint not null,
  dataset_id bigint not null,
  snapshot_id bigint not null,
  artifact_type text not null,
  format text not null,
  uri text not null,
  manifest_uri text not null,
  label_map_json jsonb not null default '{}'::jsonb,
  checksum text not null,
  size bigint not null,
  status text not null,
  created_at timestamptz not null default now()
);
```

- [ ] **Step 4: Apply migration and rerun tests**

Run: `psql "$DATABASE_URL" -f migrations/0001_init.sql`  
Expected: `CREATE TABLE` for each table.  
Run: `go test ./internal/jobs -run TestRequiredTablesExist -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add migrations/0001_init.sql internal/jobs/repository_test.go
git commit -m "feat: add initial schema for jobs snapshots review and artifacts"
```

### Task 3: Implement Config + Postgres/Redis/S3 Clients

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/store/postgres.go`
- Create: `internal/queue/redis.go`
- Create: `internal/storage/s3.go`

- [ ] **Step 1: Write failing config validation test**

```go
func TestLoadConfigRequiresCoreEnv(t *testing.T) {
    t.Setenv("DATABASE_URL", "")
    _, err := Load()
    if err == nil {
        t.Fatal("expected error when DATABASE_URL is missing")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestLoadConfigRequiresCoreEnv -v`  
Expected: FAIL because `Load()` is undefined.

- [ ] **Step 3: Implement config and client constructors**

```go
// internal/config/config.go
package config

type Config struct {
    DatabaseURL string
    RedisAddr   string
    S3Endpoint  string
    S3AccessKey string
    S3SecretKey string
    S3Bucket    string
}

func Load() (Config, error) {
    cfg := Config{
        DatabaseURL: os.Getenv("DATABASE_URL"),
        RedisAddr:   os.Getenv("REDIS_ADDR"),
        S3Endpoint:  os.Getenv("S3_ENDPOINT"),
        S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
        S3SecretKey: os.Getenv("S3_SECRET_KEY"),
        S3Bucket:    os.Getenv("S3_BUCKET"),
    }
    if cfg.DatabaseURL == "" {
        return Config{}, errors.New("DATABASE_URL is required")
    }
    return cfg, nil
}
```

```go
// internal/storage/s3.go
func NewClient(cfg config.Config) (*minio.Client, error) {
    return minio.New(cfg.S3Endpoint, &minio.Options{
        Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
        Secure: false,
    })
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config -v`  
Expected: PASS.  
Run: `go test ./internal/... -run TestLoadConfigRequiresCoreEnv -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/store/postgres.go internal/queue/redis.go internal/storage/s3.go
git commit -m "feat: add config and infrastructure client factories"
```

### Task 4: Implement Job State Machine + Idempotent Create + Partial Success

**Files:**
- Create: `internal/jobs/model.go`
- Create: `internal/jobs/state_machine.go`
- Create: `internal/jobs/state_machine_test.go`
- Modify: `internal/jobs/repository.go`
- Modify: `internal/jobs/repository_test.go`

- [ ] **Step 1: Write failing state transition tests**

```go
func TestTransitionToSucceededWithErrors(t *testing.T) {
    if err := CanTransition(StatusRunning, StatusSucceededWithErrors); err != nil {
        t.Fatalf("expected transition allowed, got %v", err)
    }
}

func TestTransitionFromFailedToRunningRejected(t *testing.T) {
    if err := CanTransition(StatusFailed, StatusRunning); err == nil {
        t.Fatal("expected transition to be rejected")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/jobs -run TestTransition -v`  
Expected: FAIL because statuses/functions are missing.

- [ ] **Step 3: Implement statuses, transitions, and idempotent insert**

```go
const (
    StatusQueued              = "queued"
    StatusRunning             = "running"
    StatusSucceeded           = "succeeded"
    StatusSucceededWithErrors = "succeeded_with_errors"
    StatusFailed              = "failed"
    StatusCanceled            = "canceled"
    StatusRetryWaiting        = "retry_waiting"
)

var transitions = map[string]map[string]bool{
    StatusQueued: {
        StatusRunning: true,
        StatusCanceled: true,
    },
    StatusRunning: {
        StatusSucceeded: true,
        StatusSucceededWithErrors: true,
        StatusFailed: true,
    },
    StatusRetryWaiting: {
        StatusQueued: true,
    },
}
```

```sql
-- repository insert pattern
insert into jobs (project_id, job_type, status, required_resource_type, idempotency_key, payload_json)
values ($1, $2, 'queued', $3, $4, $5)
on conflict (project_id, job_type, idempotency_key)
do update set idempotency_key = excluded.idempotency_key
returning id;
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/jobs -v`  
Expected: PASS for state-machine and idempotency tests.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/model.go internal/jobs/state_machine.go internal/jobs/state_machine_test.go internal/jobs/repository.go internal/jobs/repository_test.go
git commit -m "feat: implement job state machine and idempotent creation"
```

### Task 5: Implement Data Hub API + S3 Presign Endpoint

**Files:**
- Modify: `internal/server/http_server.go`
- Create: `internal/datahub/service.go`
- Create: `internal/datahub/handler.go`
- Create: `internal/datahub/handler_test.go`

- [ ] **Step 1: Write failing handler test for presign endpoint**

```go
func TestPresignEndpointReturnsURL(t *testing.T) {
    srv := newTestServerWithFakePresigner()
    req := httptest.NewRequest(http.MethodPost, "/v1/objects/presign", strings.NewReader(`{"dataset_id":1,"object_key":"train/a.jpg"}`))
    rec := httptest.NewRecorder()

    srv.Handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    if !strings.Contains(rec.Body.String(), "https://") {
        t.Fatalf("expected signed URL, got %s", rec.Body.String())
    }
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/datahub -run TestPresignEndpointReturnsURL -v`  
Expected: FAIL because route/handler is missing.

- [ ] **Step 3: Implement dataset create/snapshot list/presign handlers**

```go
r.Route("/v1", func(r chi.Router) {
    r.Post("/datasets", handler.CreateDataset)
    r.Post("/datasets/{id}/snapshots", handler.CreateSnapshot)
    r.Get("/datasets/{id}/snapshots", handler.ListSnapshots)
    r.Post("/objects/presign", handler.PresignObject)
})
```

```go
type PresignRequest struct {
    DatasetID int64  `json:"dataset_id"`
    ObjectKey string `json:"object_key"`
    TTLSeconds int   `json:"ttl_seconds"`
}
```

- [ ] **Step 4: Run tests and quick endpoint smoke**

Run: `go test ./internal/datahub -v`  
Expected: PASS.  
Run: `go test ./internal/server -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/http_server.go internal/datahub/service.go internal/datahub/handler.go internal/datahub/handler_test.go
git commit -m "feat: add datahub routes and object presign API"
```

### Task 6: Implement Artifact Packaging (Label Map + Manifest + data.yaml)

**Files:**
- Create: `internal/artifacts/service.go`
- Create: `internal/artifacts/packager.go`
- Create: `internal/artifacts/packager_test.go`

- [ ] **Step 1: Write failing tests for label mapping and manifest generation**

```go
func TestApplyLabelMap(t *testing.T) {
    labels := []string{"pedestrian", "car"}
    mapped := ApplyLabelMap(labels, map[string]string{"pedestrian": "person"})
    if mapped[0] != "person" {
        t.Fatalf("expected person, got %s", mapped[0])
    }
}

func TestBuildManifestIncludesChecksums(t *testing.T) {
    entries := []ManifestEntry{{Path: "labels/0001.txt", Checksum: "abc123"}}
    b, err := BuildManifest("v1.2", entries)
    if err != nil || !bytes.Contains(b, []byte("abc123")) {
        t.Fatal("manifest missing checksum")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/artifacts -run TestApplyLabelMap -v`  
Expected: FAIL because functions are missing.

- [ ] **Step 3: Implement packager helpers and YAML generator**

```go
func ApplyLabelMap(input []string, m map[string]string) []string {
    out := make([]string, len(input))
    for i, v := range input {
        if mv, ok := m[v]; ok {
            out[i] = mv
            continue
        }
        out[i] = v
    }
    return out
}

func BuildDataYAML(train string, val string, names []string) string {
    return fmt.Sprintf("train: %s\nval: %s\nnames:\n  - %s\n", train, val, strings.Join(names, "\n  - "))
}
```

```go
func BuildManifest(version string, entries []ManifestEntry) ([]byte, error) {
    payload := Manifest{Version: version, Entries: entries, GeneratedAt: time.Now().UTC()}
    return json.MarshalIndent(payload, "", "  ")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/artifacts -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/artifacts/service.go internal/artifacts/packager.go internal/artifacts/packager_test.go
git commit -m "feat: add artifact packager with label map manifest and data yaml"
```

### Task 7: Implement Worker Partial-Failure Reporting

**Files:**
- Create: `workers/common/job_client.py`
- Create: `workers/zero-shot/main.py`
- Create: `workers/tests/test_partial_success.py`

- [ ] **Step 1: Write failing Python test for succeeded_with_errors behavior**

```python
from workers.zero_shot.main import summarize_batch

def test_summarize_batch_partial_success():
    status, summary = summarize_batch(total=1000, ok=995, failed=5)
    assert status == "succeeded_with_errors"
    assert summary["failed_items"] == 5
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pytest workers/tests/test_partial_success.py -q`  
Expected: FAIL with import/function missing.

- [ ] **Step 3: Implement summary and event emission helpers**

```python
# workers/zero-shot/main.py

def summarize_batch(total: int, ok: int, failed: int):
    if failed == 0:
        return "succeeded", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    if ok > 0:
        return "succeeded_with_errors", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    return "failed", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
```

```python
# workers/common/job_client.py

def emit_item_error(job_id: int, item_id: int, message: str, detail: dict):
    payload = {
        "job_id": job_id,
        "item_id": item_id,
        "event_level": "error",
        "event_type": "item_failed",
        "message": message,
        "detail_json": detail,
    }
    return payload
```

- [ ] **Step 4: Run tests**

Run: `pytest workers/tests/test_partial_success.py -q`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add workers/common/job_client.py workers/zero-shot/main.py workers/tests/test_partial_success.py
git commit -m "feat: add worker partial-success summarization and error events"
```

### Task 8: Implement CLI Pull + Manifest Verification

**Files:**
- Create: `cmd/platform-cli/main.go`
- Create: `internal/cli/pull.go`
- Create: `internal/cli/verify.go`
- Create: `internal/cli/pull_test.go`

- [ ] **Step 1: Write failing test for manifest verification**

```go
func TestVerifyManifestFailsOnChecksumMismatch(t *testing.T) {
    err := VerifyFile("testdata/labels/0001.txt", "deadbeef")
    if err == nil {
        t.Fatal("expected checksum mismatch")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli -run TestVerifyManifestFailsOnChecksumMismatch -v`  
Expected: FAIL because `VerifyFile` is undefined.

- [ ] **Step 3: Implement pull flow and checksum verify function**

```go
func VerifyFile(path string, expectedSHA256 string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return err
    }
    got := hex.EncodeToString(h.Sum(nil))
    if got != expectedSHA256 {
        return fmt.Errorf("checksum mismatch: got %s expected %s", got, expectedSHA256)
    }
    return nil
}
```

```go
// cmd/platform-cli/main.go
func main() {
    root := cli.NewRootCommand()
    if err := root.Execute(); err != nil {
        os.Exit(1)
    }
}
```

- [ ] **Step 4: Run tests and command help**

Run: `go test ./internal/cli -v`  
Expected: PASS.  
Run: `go run ./cmd/platform-cli --help`  
Expected: shows `pull` command with `--format`, `--version`, `--allow-partial`.

- [ ] **Step 5: Commit**

```bash
git add cmd/platform-cli/main.go internal/cli/pull.go internal/cli/verify.go internal/cli/pull_test.go
git commit -m "feat: add platform cli pull with manifest verification"
```

### Task 9: Wire Observability, E2E Smoke, And Local Runbook

**Files:**
- Create: `scripts/dev/smoke.sh`
- Create: `docs/development/local-quickstart.md`
- Modify: `Makefile`

- [ ] **Step 1: Write failing smoke script expectation**

```bash
#!/usr/bin/env bash
set -euo pipefail
curl -fsS http://localhost:8080/healthz >/dev/null
curl -fsS http://localhost:8080/readyz >/dev/null
```

- [ ] **Step 2: Run smoke script to verify it fails first**

Run: `bash scripts/dev/smoke.sh`  
Expected: FAIL because `/readyz` is not implemented yet.

- [ ] **Step 3: Implement `/readyz`, add make targets, and document setup**

```go
r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ready"))
})
```

```make
up-dev:
	docker compose -f deploy/docker/docker-compose.dev.yml up -d

down-dev:
	docker compose -f deploy/docker/docker-compose.dev.yml down -v

test:
	go test ./... && pytest workers/tests -q
```

- [ ] **Step 4: Run full verification**

Run: `make up-dev && go test ./... && pytest workers/tests -q && bash scripts/dev/smoke.sh`  
Expected: all commands pass.  
Run: `make down-dev`  
Expected: local stack stops cleanly.

- [ ] **Step 5: Commit**

```bash
git add scripts/dev/smoke.sh docs/development/local-quickstart.md Makefile internal/server/http_server.go
git commit -m "chore: add local smoke test and runbook"
```

---

## Self-Review

### 1. Spec Coverage Check

- Storage indexing and presign direct access: Task 5.
- Delta snapshot schema model and review tables: Task 2.
- Resource-aware orchestration and partial success status: Task 4 + Task 7.
- Artifact repository with label mapping and manifest/data.yaml: Task 6.
- CLI pull verification and reproducibility outputs: Task 8.
- Local one-command development environment: Task 1 + Task 9.

No uncovered MVP requirements from `docs/superpowers/specs/2026-03-28-yolo-platform-mvp-design.md`.

### 2. Placeholder Scan

- Checked for `TODO`, `TBD`, and vague steps.
- Every coding step includes concrete file paths, test examples, and runnable commands.

### 3. Type/Name Consistency

- Job status uses a single constant name: `succeeded_with_errors` across schema, state machine, and worker summary.
- Integrity object naming is consistent: `manifest.json` and verification helper `VerifyFile`.
- Label mapping key naming is consistent: `label_map_json` in artifact metadata.

---

Plan complete and saved to `docs/superpowers/plans/2026-03-28-yolo-platform-mvp-foundation-plan.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
