# YOLO Platform MVP Remaining Gap Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining gap between the current branch and the original MVP design by turning queued jobs into executable worker flows, implementing real snapshot import behavior, and replacing inline artifact delivery with storage-backed package materialization.

**Architecture:** Keep the current Go modular monolith plus Python worker split. Use the existing PostgreSQL/Redis/S3 seams that are already wired in `cmd/api-server/main.go`, add a narrow worker callback contract in Go, and then implement worker-side queue consumers incrementally so each feature becomes observable and testable end-to-end.

**Tech Stack:** Go 1.24, Chi, pgx/v5, Redis, MinIO/S3, Python 3.11+, unittest, Docker Compose, bash smoke checks.

---

## Current Remaining Assessment

### Already Aligned With The MVP Design

- Data Hub runtime uses PostgreSQL metadata plus S3-backed scan/presign wiring.
- Jobs persist `dataset_id`, `snapshot_id`, capability tags, idempotency keys, and counters.
- Review state is persisted in `annotation_candidates`, `annotations`, and `audit_logs`.
- Snapshot diff is driven by snapshot IDs instead of request-body stand-ins.
- Export route, dataset-aware artifact resolve, and CLI pull now work through the persisted artifact path.
- `POST /v1/snapshots/{id}/import` exists, resolves the target snapshot, and queues a `snapshot-import` job.

### Remaining Drift From The Original MVP Design

1. `snapshot-import` only queues a job; it does not parse COCO/YOLO input or write canonical annotations.
2. Python workers still do not consume Redis lanes or report heartbeat/progress/terminal status back to the control plane.
3. Artifact delivery still relies on inline bundle entries returned by the API instead of a real S3-backed package object with manifest-driven download.
4. Structured logging, operational metrics, and final contract hardening are still below the design target.

## File Structure Map

### Control Plane

- Modify: `internal/jobs/service.go`
  Responsibility: expose mutation paths for worker heartbeat, progress, per-item errors, and terminal completion.
- Modify: `internal/jobs/repository.go`
  Responsibility: define persistence hooks for updating counters, status, lease, and result artifacts.
- Modify: `internal/jobs/postgres_repository.go`
  Responsibility: persist worker callbacks and terminal job counters in PostgreSQL.
- Modify: `internal/jobs/handler.go`
  Responsibility: accept worker callback requests with narrow payload validation.
- Modify: `internal/server/http_server.go`
  Responsibility: register internal worker callback routes without disturbing public MVP routes.
- Modify: `internal/datahub/service.go`
  Responsibility: resolve import sources, map snapshot imports onto canonical annotation writes, and keep snapshot linkage explicit.
- Modify: `internal/datahub/postgres_repository.go`
  Responsibility: look up snapshot/dataset/category/item records and persist imported annotations.
- Modify: `internal/artifacts/service.go`
  Responsibility: stop treating inline bundle entries as the default runtime delivery mechanism once real package objects exist.
- Modify: `internal/artifacts/handler.go`
  Responsibility: preserve current resolve/get behavior while switching download to storage-backed package metadata.
- Modify: `internal/storage/s3.go`
  Responsibility: add package upload/download helpers for artifact objects and manifests.
- Modify: `internal/cli/api_source.go`
  Responsibility: poll for package readiness and download from presigned artifact URLs instead of depending on inline entries.
- Modify: `internal/cli/pull.go`
  Responsibility: keep manifest verification but switch source of package contents to the storage-backed artifact flow.
- Modify: `cmd/api-server/main.go`
  Responsibility: wire new worker callback handlers, import execution services, and artifact storage collaborators.

### Workers

- Modify: `workers/common/job_client.py`
  Responsibility: send heartbeat/progress/item-error/terminal updates to the API, not just build payload dicts.
- Create: `workers/common/queue_runner.py`
  Responsibility: provide shared Redis lane polling, job decode, and callback helpers for all workers.
- Modify: `workers/zero_shot/main.py`
  Responsibility: consume GPU jobs, emit candidates/events, and record partial success counters.
- Modify: `workers/cleaning/main.py`
  Responsibility: consume cleaning jobs and report item-level failures/progress.
- Modify: `workers/video/main.py`
  Responsibility: consume video extraction jobs and emit progress/terminal updates.
- Modify: `workers/packager/main.py`
  Responsibility: build real package trees, upload package/manifests to S3, and mark artifacts ready.
- Create: `workers/importer/main.py`
  Responsibility: parse COCO/YOLO import payloads and write canonical snapshot annotations through the control plane callback contract.

### Tests And Docs

- Modify: `internal/jobs/handler_test.go`
- Modify: `internal/datahub/service_test.go`
- Modify: `internal/datahub/postgres_repository_test.go`
- Modify: `internal/artifacts/handler_test.go`
- Modify: `internal/cli/pull_test.go`
- Modify: `scripts/dev/smoke.sh`
- Modify: `scripts/dev/smoke_test.go`
- Modify: `README.md`
- Modify: `docs/development/local-quickstart.md`
- Modify: `api/openapi/mvp.yaml`

### Scope Note

The remaining work spans three subsystems, but the order below is intentional rather than arbitrary:

1. Worker callback contract first.
2. Snapshot import execution on top of that contract.
3. Real worker consumers and storage-backed artifact materialization after the callback surface exists.

This keeps the work sequence testable and avoids building worker loops against unstable APIs.

### Task 1: Add Worker Callback Contract To The Job Orchestrator

**Files:**
- Modify: `internal/jobs/service.go`
- Modify: `internal/jobs/repository.go`
- Modify: `internal/jobs/postgres_repository.go`
- Modify: `internal/jobs/handler.go`
- Modify: `internal/jobs/handler_test.go`
- Modify: `internal/server/http_server.go`

- [ ] **Step 1: Write the failing tests for heartbeat, progress, and terminal callbacks**

```go
func TestWorkerHeartbeatTouchesLease(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "worker-heartbeat",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := svc.ReportHeartbeat(job.ID, "worker-a", 30); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	got, ok := svc.GetJob(job.ID)
	if !ok || got.LeaseUntil == nil || got.WorkerID != "worker-a" {
		t.Fatalf("expected worker lease to be set, got %+v", got)
	}
}

func TestWorkerTerminalUpdatePersistsCounters(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo)
	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            1,
		SnapshotID:           1,
		JobType:              "snapshot-import",
		RequiredResourceType: "cpu",
		IdempotencyKey:       "worker-terminal",
		Payload:              map[string]any{"format": "yolo"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := svc.ReportTerminal(job.ID, "worker-a", StatusSucceededWithErrors, 10, 8, 2); err != nil {
		t.Fatalf("terminal: %v", err)
	}
	got, ok := svc.GetJob(job.ID)
	if !ok || got.Status != StatusSucceededWithErrors || got.TotalItems != 10 || got.FailedItems != 2 {
		t.Fatalf("unexpected job after terminal update: %+v", got)
	}
}
```

- [ ] **Step 2: Run the jobs tests to verify they fail**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/jobs -run 'TestWorkerHeartbeatTouchesLease|TestWorkerTerminalUpdatePersistsCounters' -count=1 -v`
Expected: FAIL because the callback methods and handler routes do not exist yet.

- [ ] **Step 3: Implement the minimal callback contract in service, repository, and handler**

```go
type WorkerHeartbeatInput struct {
	WorkerID     string `json:"worker_id"`
	LeaseSeconds int    `json:"lease_seconds"`
}

type WorkerTerminalInput struct {
	WorkerID       string `json:"worker_id"`
	Status         string `json:"status"`
	TotalItems     int    `json:"total_items"`
	SucceededItems int    `json:"succeeded_items"`
	FailedItems    int    `json:"failed_items"`
}

func (s *Service) ReportHeartbeat(jobID int64, workerID string, leaseSeconds int) error
func (s *Service) ReportProgress(jobID int64, workerID string, total, ok, failed int) error
func (s *Service) ReportItemError(jobID, itemID int64, message string, detail map[string]any) error
func (s *Service) ReportTerminal(jobID int64, workerID, status string, total, ok, failed int) error
```

```go
r.Post("/internal/jobs/{job_id}/heartbeat", handlerOrNotImplemented(m.Jobs.ReportHeartbeat))
r.Post("/internal/jobs/{job_id}/progress", handlerOrNotImplemented(m.Jobs.ReportProgress))
r.Post("/internal/jobs/{job_id}/events", handlerOrNotImplemented(m.Jobs.ReportItemError))
r.Post("/internal/jobs/{job_id}/complete", handlerOrNotImplemented(m.Jobs.ReportTerminal))
```

- [ ] **Step 4: Re-run the jobs and router tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/jobs ./internal/server -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/service.go internal/jobs/repository.go internal/jobs/postgres_repository.go internal/jobs/handler.go internal/jobs/handler_test.go internal/server/http_server.go
git commit -m "feat: add worker callback contract for job orchestration"
```

### Task 2: Execute Snapshot Import Instead Of Only Queueing It

**Files:**
- Modify: `internal/datahub/service.go`
- Modify: `internal/datahub/postgres_repository.go`
- Modify: `internal/datahub/repository.go`
- Modify: `internal/datahub/service_test.go`
- Modify: `internal/datahub/postgres_repository_test.go`
- Create: `workers/importer/main.py`
- Create: `workers/tests/test_importer.py`

- [ ] **Step 1: Write the failing import execution tests**

```go
func TestImportSnapshotCreatesCategoriesAndAnnotations(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewServiceWithRepository(nil, repo)

	dataset, _ := svc.CreateDataset(CreateDatasetInput{ProjectID: 1, Name: "import", Bucket: "platform-dev", Prefix: "train"})
	snapshot, _ := svc.CreateSnapshot(dataset.ID, CreateSnapshotInput{Note: "baseline"})
	_, _ = svc.ScanDataset(dataset.ID, []string{"train/a.jpg"})

	report, err := svc.ImportSnapshot(snapshot.ID, ImportSnapshotInput{
		Format: "yolo",
		SourceURI: "s3://platform-dev/imports/yolo-demo.zip",
		Entries: []ImportedAnnotation{
			{ObjectKey: "train/a.jpg", CategoryName: "person", BBoxX: 0.1, BBoxY: 0.2, BBoxW: 0.3, BBoxH: 0.4},
		},
	})
	if err != nil {
		t.Fatalf("import snapshot: %v", err)
	}
	if report.ImportedAnnotations != 1 {
		t.Fatalf("expected 1 imported annotation, got %+v", report)
	}
}
```

```python
def test_parse_yolo_entry_returns_boxes():
    payload = {
        "format": "yolo",
        "labels": {"train/a.txt": "0 0.5 0.5 0.2 0.2\n"},
        "names": ["person"],
    }
    boxes = parse_import_payload(payload)
    assert boxes[0]["category_name"] == "person"
```

- [ ] **Step 2: Run the targeted Go and Python tests to verify they fail**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub -run TestImportSnapshotCreatesCategoriesAndAnnotations -count=1 -v`
Expected: FAIL because import execution and import result persistence do not exist.

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_importer -v`
Expected: FAIL because `workers/importer/main.py` is missing.

- [ ] **Step 3: Implement minimal import execution in Go and the importer worker**

```go
type ImportSnapshotInput struct {
	Format    string
	SourceURI string
	Entries   []ImportedAnnotation
}

type ImportedAnnotation struct {
	ObjectKey     string
	CategoryName  string
	BBoxX         float64
	BBoxY         float64
	BBoxW         float64
	BBoxH         float64
}

type ImportSnapshotResult struct {
	DatasetID           int64
	SnapshotID          int64
	ImportedAnnotations int
}

func (s *Service) ImportSnapshot(snapshotID int64, in ImportSnapshotInput) (ImportSnapshotResult, error)
```

```python
def run_import_job(job_payload: dict) -> dict:
    boxes = parse_import_payload(job_payload)
    return {
        "status": "succeeded",
        "total_items": len(boxes),
        "succeeded_items": len(boxes),
        "failed_items": 0,
    }
```

- [ ] **Step 4: Re-run the import execution tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub -count=1 -v`
Expected: PASS.

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_importer -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/datahub/service.go internal/datahub/repository.go internal/datahub/postgres_repository.go internal/datahub/service_test.go internal/datahub/postgres_repository_test.go workers/importer/main.py workers/tests/test_importer.py
git commit -m "feat: execute snapshot import into canonical annotations"
```

### Task 3: Turn Python Workers Into Real Redis Consumers

**Files:**
- Modify: `workers/common/job_client.py`
- Create: `workers/common/queue_runner.py`
- Modify: `workers/zero_shot/main.py`
- Modify: `workers/cleaning/main.py`
- Modify: `workers/video/main.py`
- Modify: `workers/packager/main.py`
- Modify: `workers/tests/test_job_client.py`
- Modify: `workers/tests/test_partial_success.py`
- Create: `workers/tests/test_queue_runner.py`

- [ ] **Step 1: Write the failing worker-consumer tests**

```python
def test_queue_runner_dispatches_matching_job_type():
    runner = QueueRunner(worker_id="packager-a", accepted_job_types={"artifact-package"})
    payload = {"job_id": 1, "job_type": "artifact-package", "payload": {"format": "yolo"}}
    handled = []

    runner.handle_once(payload, lambda job: handled.append(job["job_id"]))

    assert handled == [1]
```

```python
def test_job_client_posts_terminal_update():
    client = JobClient(base_url="http://api.local")
    body = client.build_terminal(job_id=5, worker_id="worker-a", status="succeeded", total=3, ok=3, failed=0)
    assert body["status"] == "succeeded"
    assert body["worker_id"] == "worker-a"
```

- [ ] **Step 2: Run the worker tests to verify they fail**

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_queue_runner workers.tests.test_job_client workers.tests.test_partial_success -v`
Expected: FAIL because queue consumption and HTTP callback helpers are not implemented.

- [ ] **Step 3: Implement shared queue runner and wire worker entrypoints**

```python
class QueueRunner:
    def __init__(self, worker_id: str, accepted_job_types: set[str]):
        self.worker_id = worker_id
        self.accepted_job_types = accepted_job_types

    def handle_once(self, payload: dict, handler):
        if payload.get("job_type") not in self.accepted_job_types:
            return False
        handler(payload)
        return True
```

```python
def main():
    runner = QueueRunner(worker_id=os.getenv("WORKER_ID", "packager-local"), accepted_job_types={"artifact-package"})
    poll_forever(redis_url=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:cpu", runner=runner, handler=run_package_job)
```

- [ ] **Step 4: Re-run the worker tests**

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_queue_runner workers.tests.test_job_client workers.tests.test_partial_success workers.tests.test_cleaning_rules -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add workers/common/job_client.py workers/common/queue_runner.py workers/zero_shot/main.py workers/cleaning/main.py workers/video/main.py workers/packager/main.py workers/tests/test_queue_runner.py workers/tests/test_job_client.py workers/tests/test_partial_success.py
git commit -m "feat: run workers as redis consumers with job callbacks"
```

### Task 4: Replace Inline Artifact Delivery With Storage-Backed Packages

**Files:**
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/artifacts/packager.go`
- Modify: `internal/storage/s3.go`
- Modify: `internal/cli/api_source.go`
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/pull_test.go`
- Modify: `internal/artifacts/handler_test.go`
- Modify: `workers/packager/main.py`
- Modify: `scripts/dev/smoke.sh`
- Modify: `scripts/dev/smoke_test.go`

- [ ] **Step 1: Write the failing package-download tests**

```go
func TestAPIArtifactSourceDownloadsFromPresignedArtifactURL(t *testing.T) {
	source := NewAPIArtifactSource(newArtifactTestServer(t).URL)
	pkg, err := source.FetchArtifact("demo", "yolo", "v1")
	if err != nil {
		t.Fatalf("fetch artifact: %v", err)
	}
	if pkg.ArtifactID == 0 || len(pkg.Entries) == 0 {
		t.Fatalf("expected downloaded package contents, got %+v", pkg)
	}
}
```

```go
func TestGetArtifactReturnsReadyStorageMetadata(t *testing.T) {
	artifact := seedReadyArtifact(t)
	if artifact.URI == "" || artifact.ManifestURI == "" || artifact.Status != "ready" {
		t.Fatalf("expected ready artifact metadata, got %+v", artifact)
	}
}
```

- [ ] **Step 2: Run artifacts and CLI tests to verify they fail**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts ./internal/cli -count=1 -v`
Expected: FAIL because runtime delivery still depends on inline bundle entries.

- [ ] **Step 3: Implement real package upload and download flow**

```go
type Artifact struct {
	ID          int64
	URI         string
	ManifestURI string
	Status      string
}

func (s *Service) MarkArtifactReady(id int64, uri, manifestURI, checksum string, size int64) error
func UploadObject(ctx context.Context, client *minio.Client, bucket, key string, body io.Reader, size int64, contentType string) error
```

```python
def run_package_job(job_payload: dict) -> dict:
    workdir = build_package_tree(...)
    package_uri, manifest_uri = upload_package_tree(...)
    return {"status": "succeeded", "artifact_uri": package_uri, "manifest_uri": manifest_uri}
```

- [ ] **Step 4: Re-run artifacts, CLI, and smoke tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts ./internal/cli ./scripts/dev -count=1 -v`
Expected: PASS.

Run: `bash scripts/dev/smoke.sh`
Expected: PASS with CLI pull downloading the package through the presigned artifact path.

- [ ] **Step 5: Commit**

```bash
git add internal/artifacts/service.go internal/artifacts/handler.go internal/artifacts/packager.go internal/storage/s3.go internal/cli/api_source.go internal/cli/pull.go internal/cli/pull_test.go internal/artifacts/handler_test.go workers/packager/main.py scripts/dev/smoke.sh scripts/dev/smoke_test.go
git commit -m "feat: deliver storage-backed artifact packages"
```

### Task 5: Add Observability And Final Acceptance Hardening

**Files:**
- Modify: `cmd/api-server/main.go`
- Modify: `internal/jobs/handler.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `api/openapi/mvp.yaml`
- Modify: `README.md`
- Modify: `docs/development/local-quickstart.md`

- [ ] **Step 1: Write the failing contract and readiness assertions**

```go
func TestReadyzReturnsDependencyFailureName(t *testing.T) {
	srv := NewHTTPServerWithModules(Modules{
		ReadyChecks: []ReadyCheck{
			func(context.Context) error { return fmt.Errorf("redis not ready: dial tcp 127.0.0.1:6379: connect refused") },
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the contract tests to verify they fail**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./cmd/api-server ./internal/server -count=1 -v`
Expected: FAIL until the final contract/logging changes are in place.

- [ ] **Step 3: Implement minimal structured logs, docs parity, and final route documentation**

```go
log.Printf("job callback handled route=%s job_id=%d worker_id=%s status=%s", routeName, jobID, workerID, status)
```

```yaml
  /internal/jobs/{job_id}/complete:
    post:
      summary: Internal worker terminal callback
```

- [ ] **Step 4: Run the full acceptance suite**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...`
Expected: PASS.

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success workers.tests.test_job_client workers.tests.test_cleaning_rules workers.tests.test_queue_runner workers.tests.test_importer -v`
Expected: PASS.

Run: `bash scripts/dev/smoke.sh`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/api-server/main.go internal/jobs/handler.go internal/datahub/handler.go internal/artifacts/handler.go api/openapi/mvp.yaml README.md docs/development/local-quickstart.md
git commit -m "docs: harden final mvp contracts and observability"
```

## Exit Criteria For This Plan

1. Snapshot import does more than queue a job; it can materialize imported annotations into the target snapshot flow.
2. Python workers consume Redis job lanes and report heartbeat/progress/item failures/terminal status back to the API.
3. Artifact packages are uploaded to storage and downloaded by CLI through package metadata plus presigned URLs rather than API-inline entries.
4. Smoke covers import, export, artifact resolve, and CLI verification against the Docker-backed local stack.
5. OpenAPI, README, local quickstart, and worker/runtime contracts all describe the same behavior.
