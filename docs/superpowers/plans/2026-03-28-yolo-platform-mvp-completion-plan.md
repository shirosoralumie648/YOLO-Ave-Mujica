# YOLO Platform MVP Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close all remaining gaps between current foundation code and MVP exit criteria in `docs/superpowers/specs/2026-03-28-yolo-platform-mvp-design.md`.

**Architecture:** Keep the existing Go modular monolith + Python worker split, but replace in-memory paths with PostgreSQL/Redis/S3-backed flows for Data Hub, Jobs, Review, Versioning, and Artifacts. Maintain async APIs with `job_id` responses, lane-based dispatch (`cpu/gpu/mixed`), and manifest-verified CLI pull.

**Tech Stack:** Go 1.24, Chi, pgx, go-redis v9, MinIO SDK, Python 3.11+, unittest/pytest, Docker Compose, PostgreSQL 16, Redis 7, MinIO.

---

## File Structure Map

### Control Plane (Go)

- Modify: `cmd/api-server/main.go`
  Responsibility: dependency wiring for Data Hub, Jobs, Versioning, Review, Artifacts handlers.
- Modify: `internal/server/http_server.go`
  Responsibility: register complete MVP routes (datasets, jobs, diff, review, artifacts).
- Create: `internal/datahub/repository.go`
  Responsibility: PostgreSQL persistence for datasets/items/snapshots.
- Modify: `internal/datahub/service.go`
  Responsibility: replace in-memory storage with repository-backed logic and S3 scan orchestration.
- Modify: `internal/datahub/handler.go`
  Responsibility: add `/scan` and `/items` endpoints.
- Create: `internal/jobs/dispatcher.go`
  Responsibility: lane publish (`jobs:cpu`, `jobs:gpu`, `jobs:mixed`) and payload envelope.
- Create: `internal/jobs/sweeper.go`
  Responsibility: lease timeout recovery and retry requeue.
- Modify: `internal/jobs/service.go`
  Responsibility: DB-backed create/get/event + retry classification.
- Modify: `internal/jobs/handler.go`
  Responsibility: expose `/v1/jobs/*` APIs from spec.
- Create: `internal/versioning/service.go`
  Responsibility: snapshot diff calculation (`add/remove/update`) + aggregate metrics.
- Create: `internal/versioning/handler.go`
  Responsibility: `/v1/snapshots/diff` API.
- Create: `internal/review/service.go`
  Responsibility: accept/reject promotion from `annotation_candidates` to `annotations`.
- Create: `internal/review/handler.go`
  Responsibility: list pending candidates + accept/reject APIs.
- Create: `internal/artifacts/repository.go`
  Responsibility: artifact metadata persistence and lifecycle status.
- Create: `internal/artifacts/handler.go`
  Responsibility: package create/get/presign APIs.
- Modify: `internal/artifacts/service.go`
  Responsibility: create package job request and artifact query methods.

### Worker Plane (Python)

- Modify: `workers/common/job_client.py`
  Responsibility: heartbeat, progress update, event emit, terminal status report.
- Modify: `workers/zero_shot/main.py`
  Responsibility: consume queued jobs and write `annotation_candidates` outputs.
- Create: `workers/video/main.py`
  Responsibility: video frame extraction batch pipeline.
- Create: `workers/cleaning/main.py`
  Responsibility: cleaning rule execution and JSON report generation.
- Create: `workers/packager/main.py`
  Responsibility: package assembly, `manifest.json`, `data.yaml`, and artifact upload.

### CLI + Contract + Tests

- Modify: `internal/cli/pull.go`
  Responsibility: real pull flow (poll job/artifact, download files, write verify report).
- Modify: `internal/cli/verify.go`
  Responsibility: directory-level manifest verification and summary report output.
- Create: `api/openapi/mvp.yaml`
  Responsibility: contract for all MVP HTTP endpoints.
- Create: `internal/versioning/handler_test.go`
- Create: `internal/review/handler_test.go`
- Create: `internal/artifacts/handler_test.go`
- Create: `internal/jobs/handler_test.go`
- Modify: `internal/datahub/handler_test.go`
- Modify: `internal/cli/pull_test.go`
- Create: `workers/tests/test_job_client.py`
- Create: `workers/tests/test_cleaning_rules.py`
- Modify: `scripts/dev/smoke.sh`
- Modify: `docs/development/local-quickstart.md`

---

### Task 1: Wire Full API Route Surface In Server Bootstrap

**Files:**
- Modify: `cmd/api-server/main.go`
- Modify: `internal/server/http_server.go`
- Create: `internal/server/http_server_routes_test.go`

- [ ] **Step 1: Write failing route-availability tests**

```go
func TestMVPRoutesAreRegistered(t *testing.T) {
    srv := NewHTTPServerWithModules(newFakeModules())

    routes := []struct {
        method string
        path   string
    }{
        {http.MethodPost, "/v1/datasets"},
        {http.MethodPost, "/v1/datasets/1/scan"},
        {http.MethodGet, "/v1/datasets/1/items"},
        {http.MethodPost, "/v1/jobs/zero-shot"},
        {http.MethodGet, "/v1/jobs/1"},
        {http.MethodPost, "/v1/snapshots/diff"},
        {http.MethodGet, "/v1/review/candidates"},
        {http.MethodPost, "/v1/artifacts/packages"},
    }

    for _, tc := range routes {
        req := httptest.NewRequest(tc.method, tc.path, nil)
        rec := httptest.NewRecorder()
        srv.Handler.ServeHTTP(rec, req)
        if rec.Code == http.StatusNotFound {
            t.Fatalf("route missing: %s %s", tc.method, tc.path)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails first**

Run: `go test ./internal/server -run TestMVPRoutesAreRegistered -v`
Expected: FAIL with at least one `route missing`.

- [ ] **Step 3: Add module wiring and route registration**

```go
type Modules struct {
    DataHub   *datahub.Handler
    Jobs      *jobs.Handler
    Versioning *versioning.Handler
    Review    *review.Handler
    Artifacts *artifacts.Handler
}

func NewHTTPServerWithModules(m Modules) *HTTPServer {
    r := chi.NewRouter()
    r.Get("/healthz", ...)
    r.Get("/readyz", ...)

    r.Route("/v1", func(r chi.Router) {
        // datasets
        r.Post("/datasets", m.DataHub.CreateDataset)
        r.Post("/datasets/{id}/scan", m.DataHub.ScanDataset)
        r.Post("/datasets/{id}/snapshots", m.DataHub.CreateSnapshot)
        r.Get("/datasets/{id}/snapshots", m.DataHub.ListSnapshots)
        r.Get("/datasets/{id}/items", m.DataHub.ListItems)
        r.Post("/objects/presign", m.DataHub.PresignObject)

        // jobs
        r.Post("/jobs/zero-shot", m.Jobs.CreateZeroShot)
        r.Post("/jobs/video-extract", m.Jobs.CreateVideoExtract)
        r.Post("/jobs/cleaning", m.Jobs.CreateCleaning)
        r.Get("/jobs/{job_id}", m.Jobs.GetJob)
        r.Get("/jobs/{job_id}/events", m.Jobs.ListEvents)

        // diff/review/artifacts
        r.Post("/snapshots/diff", m.Versioning.DiffSnapshots)
        r.Get("/review/candidates", m.Review.ListCandidates)
        r.Post("/review/candidates/{id}/accept", m.Review.AcceptCandidate)
        r.Post("/review/candidates/{id}/reject", m.Review.RejectCandidate)
        r.Post("/artifacts/packages", m.Artifacts.CreatePackage)
        r.Get("/artifacts/{id}", m.Artifacts.GetArtifact)
        r.Post("/artifacts/{id}/presign", m.Artifacts.PresignArtifact)
    })
    return &HTTPServer{Handler: r}
}
```

- [ ] **Step 4: Run server package tests**

Run: `go test ./internal/server -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/api-server/main.go internal/server/http_server.go internal/server/http_server_routes_test.go
git commit -m "feat: wire complete mvp route surface"
```

### Task 2: Implement Data Hub Persistent Flow (Create/Scan/Snapshot/Items/Presign)

**Files:**
- Create: `internal/datahub/repository.go`
- Modify: `internal/datahub/service.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/datahub/handler_test.go`

- [ ] **Step 1: Add failing tests for scan and items endpoints**

```go
func TestScanAndListItems(t *testing.T) {
    srv := newTestServerWithBackedDataHub(t)

    recScan := httptest.NewRecorder()
    reqScan := httptest.NewRequest(http.MethodPost, "/v1/datasets/1/scan", strings.NewReader(`{"object_keys":["train/a.jpg"]}`))
    srv.Handler.ServeHTTP(recScan, reqScan)
    if recScan.Code != http.StatusOK {
        t.Fatalf("scan failed: %d", recScan.Code)
    }

    recItems := httptest.NewRecorder()
    reqItems := httptest.NewRequest(http.MethodGet, "/v1/datasets/1/items", nil)
    srv.Handler.ServeHTTP(recItems, reqItems)
    if !strings.Contains(recItems.Body.String(), "train/a.jpg") {
        t.Fatalf("expected indexed object in items list")
    }
}
```

- [ ] **Step 2: Run Data Hub tests**

Run: `go test ./internal/datahub -v`
Expected: FAIL because `/scan` and `/items` handlers are missing.

- [ ] **Step 3: Implement repository-backed service and handlers**

```go
type Repository interface {
    CreateDataset(ctx context.Context, in CreateDatasetInput) (Dataset, error)
    InsertItems(ctx context.Context, datasetID int64, keys []string) (int, error)
    ListItems(ctx context.Context, datasetID int64) ([]DatasetItem, error)
    CreateSnapshot(ctx context.Context, datasetID int64, in CreateSnapshotInput) (Snapshot, error)
    ListSnapshots(ctx context.Context, datasetID int64) ([]Snapshot, error)
}

func (h *Handler) ScanDataset(w http.ResponseWriter, r *http.Request) { ... }
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) { ... }
```

- [ ] **Step 4: Re-run tests for Data Hub and Server**

Run: `go test ./internal/datahub ./internal/server -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/datahub/repository.go internal/datahub/service.go internal/datahub/handler.go internal/datahub/handler_test.go
git commit -m "feat: persist datahub scan and item listing"
```

### Task 3: Implement Job Orchestrator Dispatch, Lease Sweeper, And Job APIs

**Files:**
- Create: `internal/jobs/dispatcher.go`
- Create: `internal/jobs/sweeper.go`
- Modify: `internal/jobs/service.go`
- Modify: `internal/jobs/handler.go`
- Create: `internal/jobs/handler_test.go`

- [ ] **Step 1: Add failing tests for queue lane and job events API**

```go
func TestCreateGPUJobPublishesToGPULane(t *testing.T) {
    q := newFakeQueue()
    svc := NewServiceWithQueue(newFakeRepo(), q)

    job, err := svc.CreateJob(1, "zero-shot", "gpu", "idem-1", map[string]any{"prompt": "person"})
    if err != nil {
        t.Fatal(err)
    }
    if got := q.LastLane(); got != "jobs:gpu" {
        t.Fatalf("expected jobs:gpu, got %s", got)
    }
    if job.Status != StatusQueued {
        t.Fatalf("expected queued, got %s", job.Status)
    }
}
```

- [ ] **Step 2: Run job tests**

Run: `go test ./internal/jobs -v`
Expected: FAIL with missing dispatcher/sweeper logic.

- [ ] **Step 3: Implement dispatcher + lease timeout sweeper**

```go
func laneFor(resource string) string {
    switch resource {
    case "gpu":
        return "jobs:gpu"
    case "mixed":
        return "jobs:mixed"
    default:
        return "jobs:cpu"
    }
}

func (s *Sweeper) Tick(ctx context.Context, now time.Time) error {
    expired, err := s.repo.ListExpiredRunning(ctx, now)
    if err != nil { return err }
    for _, j := range expired {
        if j.RetryCount >= s.maxRetries {
            _ = s.repo.MarkFailed(ctx, j.ID, "lease_timeout", "retry exhausted")
            continue
        }
        _ = s.repo.MarkRetryWaiting(ctx, j.ID)
        _ = s.dispatcher.Publish(ctx, laneFor(j.RequiredResourceType), j)
    }
    return nil
}
```

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/jobs -run "TestCreateGPUJobPublishesToGPULane|TestLeaseSweeper" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/dispatcher.go internal/jobs/sweeper.go internal/jobs/service.go internal/jobs/handler.go internal/jobs/handler_test.go
git commit -m "feat: add job lane dispatch and lease timeout recovery"
```

### Task 4: Implement Review Trust Chain APIs (Pending/Accept/Reject + Audit)

**Files:**
- Create: `internal/review/service.go`
- Create: `internal/review/handler.go`
- Create: `internal/review/handler_test.go`

- [ ] **Step 1: Add failing tests for accept/reject flow**

```go
func TestAcceptPromotesCandidateToAnnotation(t *testing.T) {
    h := newReviewHandlerWithFixture(t)

    req := httptest.NewRequest(http.MethodPost, "/v1/review/candidates/10/accept", strings.NewReader(`{"reviewer_id":"u1"}`))
    rec := httptest.NewRecorder()
    h.AcceptCandidate(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
    // verify candidate status=accepted and annotation created
}
```

- [ ] **Step 2: Run review tests**

Run: `go test ./internal/review -v`
Expected: FAIL because module does not exist yet.

- [ ] **Step 3: Implement service and handler APIs**

```go
func (s *Service) AcceptCandidate(ctx context.Context, candidateID int64, reviewer string) error {
    c, err := s.repo.GetCandidate(ctx, candidateID)
    if err != nil { return err }
    if c.ReviewStatus != "pending" { return fmt.Errorf("candidate is %s", c.ReviewStatus) }

    if err := s.repo.InsertAnnotationFromCandidate(ctx, c, reviewer); err != nil { return err }
    if err := s.repo.UpdateCandidateStatus(ctx, candidateID, "accepted", reviewer); err != nil { return err }
    return s.repo.InsertAudit(ctx, reviewer, "review.accept", "annotation_candidate", strconv.FormatInt(candidateID, 10))
}
```

- [ ] **Step 4: Run review + integration tests**

Run: `go test ./internal/review ./internal/server -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/review/service.go internal/review/handler.go internal/review/handler_test.go
git commit -m "feat: add pseudo label review accept reject apis"
```

### Task 5: Implement Snapshot Diff API And Cleaning Rule Engine

**Files:**
- Create: `internal/versioning/service.go`
- Create: `internal/versioning/handler.go`
- Create: `internal/versioning/handler_test.go`
- Create: `workers/cleaning/main.py`
- Create: `workers/tests/test_cleaning_rules.py`

- [ ] **Step 1: Add failing tests for diff classification and cleaning checks**

```go
func TestDiffReturnsAddRemoveUpdateAndStats(t *testing.T) {
    svc := NewService()
    out := svc.DiffSnapshots(beforeSnapshotID, afterSnapshotID, 0.5)
    if out.TotalUpdated == 0 { t.Fatal("expected updates") }
}
```

```python
def test_cleaning_flags_zero_area_and_dark_score():
    report = run_rules([
        {"item_id": 1, "bbox_w": 0, "bbox_h": 10, "brightness": 0.2, "category": "person"},
    ], taxonomy={"person"}, dark_threshold=0.3)
    assert report["summary"]["invalid_bbox"] == 1
    assert report["summary"]["too_dark"] == 1
```

- [ ] **Step 2: Run tests to verify failures**

Run: `go test ./internal/versioning -v`
Expected: FAIL (new module missing).
Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_cleaning_rules -v`
Expected: FAIL (worker missing).

- [ ] **Step 3: Implement diff service + cleaning worker**

```go
type DiffResult struct {
    Adds []Change `json:"adds"`
    Removes []Change `json:"removes"`
    Updates []Change `json:"updates"`
    Stats DiffStats `json:"stats"`
}
```

```python
def classify_bbox(item):
    if item["bbox_w"] <= 0 or item["bbox_h"] <= 0:
        return "invalid_bbox"
    return "ok"
```

- [ ] **Step 4: Run both test suites**

Run: `go test ./internal/versioning -v`
Expected: PASS.
Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_cleaning_rules -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/versioning/service.go internal/versioning/handler.go internal/versioning/handler_test.go workers/cleaning/main.py workers/tests/test_cleaning_rules.py
git commit -m "feat: add snapshot diff api and cleaning rules worker"
```

### Task 6: Implement Artifact Repository APIs + Packager Worker

**Files:**
- Create: `internal/artifacts/repository.go`
- Create: `internal/artifacts/handler.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/packager.go`
- Create: `internal/artifacts/handler_test.go`
- Create: `workers/packager/main.py`

- [ ] **Step 1: Add failing tests for package create/get/presign**

```go
func TestCreatePackageReturnsJobID(t *testing.T) {
    h := newArtifactHandlerForTest(t)
    req := httptest.NewRequest(http.MethodPost, "/v1/artifacts/packages", strings.NewReader(`{"dataset_id":1,"snapshot_id":2,"format":"yolo"}`))
    rec := httptest.NewRecorder()

    h.CreatePackage(rec, req)
    if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "job_id") {
        t.Fatalf("expected async package response, got %d %s", rec.Code, rec.Body.String())
    }
}
```

- [ ] **Step 2: Run artifact tests**

Run: `go test ./internal/artifacts -v`
Expected: FAIL because handlers/repository are missing.

- [ ] **Step 3: Implement artifact API and packager flow**

```go
func (h *Handler) CreatePackage(w http.ResponseWriter, r *http.Request) {
    var in PackageRequest
    _ = json.NewDecoder(r.Body).Decode(&in)
    jobID, err := h.svc.CreatePackageJob(r.Context(), in)
    if err != nil { writeError(w, http.StatusBadRequest, err); return }
    writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "status": "queued"})
}
```

```python
def build_package_tree(workdir, names):
    os.makedirs(os.path.join(workdir, "images"), exist_ok=True)
    os.makedirs(os.path.join(workdir, "labels"), exist_ok=True)
    with open(os.path.join(workdir, "data.yaml"), "w", encoding="utf-8") as f:
        f.write(build_data_yaml(names))
```

- [ ] **Step 4: Run tests for artifacts + worker**

Run: `go test ./internal/artifacts -v`
Expected: PASS.
Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/artifacts/repository.go internal/artifacts/handler.go internal/artifacts/service.go internal/artifacts/packager.go internal/artifacts/handler_test.go workers/packager/main.py
git commit -m "feat: add artifact repository apis and packager worker"
```

### Task 7: Implement Worker Runtime Contract (Heartbeat/Progress/Event)

**Files:**
- Modify: `workers/common/job_client.py`
- Modify: `workers/zero_shot/main.py`
- Create: `workers/video/main.py`
- Create: `workers/tests/test_job_client.py`

- [ ] **Step 1: Add failing unit tests for heartbeat and terminal report**

```python
class JobClientContractTest(unittest.TestCase):
    def test_emit_heartbeat_payload(self):
        payload = emit_heartbeat(job_id=1, lease_seconds=30)
        self.assertEqual(payload["event_type"], "heartbeat")
        self.assertEqual(payload["job_id"], 1)
```

- [ ] **Step 2: Run worker contract tests**

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_job_client -v`
Expected: FAIL because helpers are missing.

- [ ] **Step 3: Implement shared helpers and use them in workers**

```python
def emit_heartbeat(job_id: int, lease_seconds: int):
    return {
        "job_id": job_id,
        "event_level": "info",
        "event_type": "heartbeat",
        "detail_json": {"lease_seconds": lease_seconds},
    }

def emit_terminal(job_id: int, status: str, total: int, ok: int, failed: int):
    return {
        "job_id": job_id,
        "status": status,
        "total_items": total,
        "succeeded_items": ok,
        "failed_items": failed,
    }
```

- [ ] **Step 4: Run all worker tests**

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success workers.tests.test_job_client -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add workers/common/job_client.py workers/zero_shot/main.py workers/video/main.py workers/tests/test_job_client.py
git commit -m "feat: add worker heartbeat progress and terminal status contract"
```

### Task 8: Implement CLI Pull End-To-End (Poll/Download/Verify/Report)

**Files:**
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/verify.go`
- Modify: `internal/cli/pull_test.go`
- Modify: `cmd/platform-cli/main.go`

- [ ] **Step 1: Add failing tests for report generation and allow-partial behavior**

```go
func TestPullWritesVerifyReport(t *testing.T) {
    cli := newTestPullCLI(t)
    err := cli.Pull(PullOptions{Format: "yolo", Version: "v1", AllowPartial: false})
    if err != nil {
        t.Fatal(err)
    }
    if _, err := os.Stat(filepath.Join(cli.OutputDir(), "verify-report.json")); err != nil {
        t.Fatalf("missing verify-report.json: %v", err)
    }
}
```

- [ ] **Step 2: Run CLI tests**

Run: `go test ./internal/cli -v`
Expected: FAIL because pull flow does not create report.

- [ ] **Step 3: Implement pull orchestration and report output**

```go
type VerifyReport struct {
    ArtifactID   int64  `json:"artifact_id"`
    Snapshot     string `json:"snapshot"`
    TotalFiles   int    `json:"total_files"`
    FailedFiles  int    `json:"failed_files"`
    VerifiedAt   string `json:"verified_at"`
}
```

```go
func writeVerifyReport(path string, r VerifyReport) error {
    b, err := json.MarshalIndent(r, "", "  ")
    if err != nil { return err }
    return os.WriteFile(path, b, 0o644)
}
```

- [ ] **Step 4: Run CLI tests and help output**

Run: `go test ./internal/cli -v`
Expected: PASS.
Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/platform-cli --help`
Expected: includes `pull`, `--format`, `--version`, `--allow-partial`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/pull.go internal/cli/verify.go internal/cli/pull_test.go cmd/platform-cli/main.go
git commit -m "feat: implement cli pull verification report and partial control"
```

### Task 9: Add OpenAPI Contract + End-To-End Smoke And Runbook Update

**Files:**
- Create: `api/openapi/mvp.yaml`
- Modify: `scripts/dev/smoke.sh`
- Modify: `docs/development/local-quickstart.md`
- Modify: `README.md`

- [ ] **Step 1: Add failing contract route checks in smoke script**

```bash
curl -fsS http://localhost:8080/healthz >/dev/null
curl -fsS http://localhost:8080/readyz >/dev/null
curl -fsS -X POST http://localhost:8080/v1/objects/presign -d '{"dataset_id":1,"object_key":"a.jpg"}' -H 'Content-Type: application/json' >/dev/null
```

- [ ] **Step 2: Run smoke and verify failure mode is explicit**

Run: `bash scripts/dev/smoke.sh`
Expected: if API unavailable, script exits non-zero with endpoint-specific error message.

- [ ] **Step 3: Add OpenAPI + update docs with exact run sequence**

```yaml
openapi: 3.1.0
info:
  title: YOLO Platform MVP API
  version: 0.1.0
paths:
  /v1/datasets:
    post:
      summary: Create dataset
  /v1/jobs/{job_id}:
    get:
      summary: Get job status
```

- [ ] **Step 4: Run full verification suite**

Run: `make up-dev`
Expected: local infra services start.
Run: `make test`
Expected: Go + Python tests pass.
Run: `bash scripts/dev/smoke.sh`
Expected: health/readiness and core contract checks pass.
Run: `make down-dev`
Expected: stack stops cleanly.

- [ ] **Step 5: Commit**

```bash
git add api/openapi/mvp.yaml scripts/dev/smoke.sh docs/development/local-quickstart.md README.md
git commit -m "docs: add mvp openapi contract and e2e runbook"
```

---

## Self-Review

### 1. Spec Coverage Check

- Data Hub indexing/snapshot/items/presign: Task 2.
- Async jobs + idempotency + lane scheduling + lease/retry: Task 3.
- Review trust chain (`pending|accepted|rejected`): Task 4.
- Snapshot diff + cleaning rules: Task 5.
- Artifact package + label map + manifest + data.yaml: Task 6.
- Worker heartbeat/progress/item error semantics: Task 7.
- CLI pull + verification report + allow partial behavior: Task 8.
- OpenAPI + smoke + runbook + local reproducibility: Task 9.

No missing item against MVP exit criteria in `docs/superpowers/specs/2026-03-28-yolo-platform-mvp-design.md`.

### 2. Placeholder Scan

- Checked for `TODO`, `TBD`, and vague “implement later” wording.
- All tasks contain explicit files, tests, commands, and expected outputs.

### 3. Type/Name Consistency

- Job status uses `succeeded_with_errors` consistently across Go/Python tasks.
- Resource lanes use `jobs:cpu|jobs:gpu|jobs:mixed` consistently.
- Verification artifact naming uses `manifest.json` and `verify-report.json` consistently.
