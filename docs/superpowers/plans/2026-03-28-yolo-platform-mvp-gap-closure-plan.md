# YOLO Platform MVP Gap Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the gap between the current completion branch and the original MVP design so the repository supports a real Data Hub -> Jobs -> Review -> Artifacts -> CLI flow instead of a mixed foundation-plus-placeholder implementation.

**Architecture:** Keep the current Go modular monolith plus Python worker split, but move runtime behavior onto PostgreSQL, Redis, and S3-backed paths wherever the code still uses process-local state or request-body stand-ins. Preserve deterministic in-memory and pure-function seams for tests, while making the default `api-server` and `platform-cli` path operate on persisted datasets, snapshots, candidates, jobs, and artifacts.

**Tech Stack:** Go 1.24, Chi, pgx/v5, go-redis/v9, MinIO SDK, Python 3.11+, unittest, Docker Compose, golang-migrate, OpenAPI 3.1.

---

## Current Assessment

### Completed Foundation

- Route wiring, liveness/readiness, graceful shutdown, and migration baseline are present and covered by Go tests.
- Data Hub supports dataset create, manual item ingest, snapshot create/list, and object presign.
- Jobs support idempotent create, lane dispatch, event listing, and lease sweeper primitives.
- Versioning diff logic returns add/remove/update aggregates and `compatibility_score`.
- CLI verification writes `verify-report.json` with `environment_context`.
- Local docs, compose bootstrap, smoke script, Go tests, and Python worker unit tests are in place.

### Partial Or Drifted From Original Design

- Data Hub scan accepts caller-supplied `object_keys`; it does not enumerate S3 objects from `bucket + prefix` or persist rich object metadata.
- Review and Artifacts runtime services are still in-memory when `cmd/api-server/main.go` builds the process.
- `platform-cli pull` resolves artifact metadata but does not download real package contents in the default API-backed path.
- Snapshot diff is request-body driven (`before` / `after` arrays) instead of snapshot-storage driven.
- Jobs do not persist `dataset_id`, `snapshot_id`, `required_capabilities_json`, or artifact result linkage through the public create path.

### Missing From Original MVP Design

- Snapshot import/export APIs for COCO and YOLO.
- Real worker queue consumption for zero-shot, video extraction, cleaning, and package assembly.
- Persistent review trust chain backed by `annotation_candidates`, `annotations`, and `audit_logs`.
- Artifact package lifecycle backed by S3 objects plus manifest-driven CLI download.
- Structured logging, operational metrics, stronger OpenAPI parity, and end-to-end smoke beyond request-shape checks.

## File Structure Map

### Control Plane

- Modify: `cmd/api-server/main.go`
  Responsibility: wire PostgreSQL-backed Review and Artifacts services, plus any worker-facing runtime dependencies.
- Modify: `internal/server/http_server.go`
  Responsibility: register newly completed routes without leaving spec-only behavior outside the router.
- Modify: `internal/datahub/service.go`
  Responsibility: shift scan behavior from caller-fed keys to S3-backed prefix enumeration.
- Modify: `internal/datahub/handler.go`
  Responsibility: keep dataset APIs stable while accepting runtime scan inputs.
- Modify: `internal/datahub/postgres_repository.go`
  Responsibility: persist scanned object metadata and snapshot linkage.
- Modify: `internal/jobs/model.go`
  Responsibility: keep job persistence aligned with schema fields already present in migrations.
- Modify: `internal/jobs/repository.go`
  Responsibility: accept a richer job create input and counters/result updates.
- Modify: `internal/jobs/postgres_repository.go`
  Responsibility: persist full job state, capability data, and artifact result linkage.
- Modify: `internal/jobs/service.go`
  Responsibility: expose a real create path for dataset/snapshot-aware jobs.
- Modify: `internal/jobs/handler.go`
  Responsibility: validate and pass through dataset/snapshot/capability fields.
- Create: `internal/review/postgres_repository.go`
  Responsibility: persist pending candidates, accepted annotations, and audit rows.
- Modify: `internal/review/service.go`
  Responsibility: move trust-chain transitions onto repository-backed state.
- Modify: `internal/review/handler.go`
  Responsibility: preserve the HTTP contract while surfacing storage-backed review errors.
- Modify: `internal/versioning/service.go`
  Responsibility: support snapshot-linked diff execution in addition to pure comparison.
- Modify: `internal/versioning/handler.go`
  Responsibility: accept snapshot identifiers and load effective annotations.
- Create: `internal/artifacts/postgres_repository.go`
  Responsibility: persist artifact metadata, readiness, and lookup by dataset/format/version.
- Modify: `internal/artifacts/repository.go`
  Responsibility: define runtime-safe create/get/update/resolve behavior.
- Modify: `internal/artifacts/service.go`
  Responsibility: integrate package jobs with persisted artifacts and ready-state transitions.
- Modify: `internal/artifacts/handler.go`
  Responsibility: expose package creation, metadata lookup, resolve, and download/presign flow consistently.
- Modify: `internal/storage/s3.go`
  Responsibility: add dataset scan listing helpers and package-object retrieval/presign helpers.

### CLI + Worker Plane

- Modify: `internal/cli/api_source.go`
  Responsibility: fetch real artifact bundles instead of metadata-only stubs.
- Modify: `internal/cli/pull.go`
  Responsibility: download, unpack, and verify actual package contents.
- Modify: `internal/cli/verify.go`
  Responsibility: keep manifest verification and environment report generation aligned with the new bundle flow.
- Modify: `cmd/platform-cli/main.go`
  Responsibility: bring CLI arguments closer to the original MVP UX.
- Modify: `workers/common/job_client.py`
  Responsibility: emit heartbeat/progress/terminal payloads plus candidate/result payload helpers.
- Modify: `workers/zero_shot/main.py`
  Responsibility: consume jobs and write review-candidate outputs.
- Modify: `workers/video/main.py`
  Responsibility: consume frame-extraction jobs and emit counters.
- Modify: `workers/cleaning/main.py`
  Responsibility: consume cleaning jobs and write JSON reports plus removal candidates.
- Modify: `workers/packager/main.py`
  Responsibility: assemble YOLO-ready package trees and manifest payloads from persisted artifact jobs.

### Tests + Contract + Docs

- Modify: `internal/datahub/handler_test.go`
- Modify: `internal/datahub/postgres_repository_test.go`
- Modify: `internal/jobs/handler_test.go`
- Modify: `internal/jobs/postgres_repository_test.go`
- Modify: `internal/jobs/sweeper_test.go`
- Modify: `internal/review/handler_test.go`
- Create: `internal/review/postgres_repository_test.go`
- Modify: `internal/versioning/handler_test.go`
- Modify: `internal/artifacts/handler_test.go`
- Create: `internal/artifacts/postgres_repository_test.go`
- Modify: `internal/cli/pull_test.go`
- Modify: `workers/tests/test_job_client.py`
- Modify: `workers/tests/test_cleaning_rules.py`
- Modify: `api/openapi/mvp.yaml`
- Modify: `scripts/dev/smoke.sh`
- Modify: `docs/development/local-quickstart.md`
- Modify: `README.md`

### Task 1: Repair Runtime Persistence Boundaries

**Files:**
- Modify: `internal/jobs/model.go`
- Modify: `internal/jobs/repository.go`
- Modify: `internal/jobs/postgres_repository.go`
- Create: `internal/review/postgres_repository.go`
- Modify: `internal/review/service.go`
- Create: `internal/artifacts/postgres_repository.go`
- Modify: `internal/artifacts/repository.go`
- Modify: `internal/artifacts/service.go`
- Modify: `cmd/api-server/main.go`
- Test: `internal/jobs/postgres_repository_test.go`
- Test: `internal/review/postgres_repository_test.go`
- Test: `internal/artifacts/postgres_repository_test.go`

- [ ] **Step 1: Add failing tests for persisted job fields, review rows, and artifact rows**

```go
func TestCreateJobPersistsDatasetAndSnapshot(t *testing.T) {
	repo := newTestJobsRepo(t)
	svc := NewServiceWithPublisher(repo, nil)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            10,
		SnapshotID:           20,
		JobType:              "zero-shot",
		RequiredResourceType: "gpu",
		IdempotencyKey:       "job-1",
		Payload:              map[string]any{"prompt": "person"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.DatasetID != 10 || job.SnapshotID != 20 {
		t.Fatalf("expected dataset/snapshot linkage, got %+v", job)
	}
}

func TestAcceptCandidateWritesAnnotationAndAudit(t *testing.T) {
	repo := newTestReviewRepo(t)
	svc := NewServiceWithRepository(repo)
	candidateID := seedPendingCandidate(t, repo)

	if err := svc.AcceptCandidate(candidateID, "reviewer-1"); err != nil {
		t.Fatalf("accept candidate: %v", err)
	}
	assertAnnotationAndAuditExist(t, repo, candidateID, "reviewer-1")
}

func TestCreatePackagePersistsArtifactRow(t *testing.T) {
	repo := newTestArtifactsRepo(t)
	svc := NewServiceWithRepository(repo)

	jobID, artifactID, err := svc.CreatePackageJob(PackageRequest{
		DatasetID:  1,
		SnapshotID: 2,
		Format:     "yolo",
		Version:    "v2",
	})
	if err != nil {
		t.Fatalf("create package job: %v", err)
	}
	if jobID == 0 || artifactID == 0 {
		t.Fatalf("expected non-zero ids, got job=%d artifact=%d", jobID, artifactID)
	}
}
```

- [ ] **Step 2: Run repository-focused tests to verify they fail**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/jobs ./internal/review ./internal/artifacts -run 'TestCreateJobPersistsDatasetAndSnapshot|TestAcceptCandidateWritesAnnotationAndAudit|TestCreatePackagePersistsArtifactRow' -v`
Expected: FAIL because runtime repositories and services do not yet persist these paths.

- [ ] **Step 3: Introduce richer repository contracts and wire PostgreSQL-backed Review/Artifacts at runtime**

```go
type CreateJobInput struct {
	ProjectID            int64
	DatasetID            int64
	SnapshotID           int64
	JobType              string
	RequiredResourceType string
	RequiredCapabilities []string
	IdempotencyKey       string
	Payload              map[string]any
}

type ReviewRepository interface {
	ListPending(ctx context.Context) ([]Candidate, error)
	Accept(ctx context.Context, candidateID int64, reviewer string) error
	Reject(ctx context.Context, candidateID int64, reviewer string) error
}

type ArtifactRepository interface {
	Create(ctx context.Context, a Artifact) (Artifact, error)
	Get(ctx context.Context, id int64) (Artifact, bool, error)
	UpdateReady(ctx context.Context, id int64, uri, manifestURI, checksum string, size int64) error
	FindByDatasetFormatVersion(ctx context.Context, datasetName, format, version string) (Artifact, bool, error)
}
```

```go
reviewRepo := review.NewPostgresRepository(pool)
reviewSvc := review.NewServiceWithRepository(reviewRepo)
reviewHandler := review.NewHandler(reviewSvc)

artifactRepo := artifacts.NewPostgresRepository(pool)
artifactSvc := artifacts.NewServiceWithRepository(artifactRepo)
artifactHandler := artifacts.NewHandler(artifactSvc)
```

- [ ] **Step 4: Re-run repository and API bootstrap tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./cmd/api-server ./internal/jobs ./internal/review ./internal/artifacts -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/api-server/main.go internal/jobs internal/review internal/artifacts
git commit -m "feat: persist runtime review artifacts and full job identifiers"
```

### Task 2: Replace Manual Scan With S3-Native Data Hub Indexing

**Files:**
- Modify: `internal/datahub/service.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/datahub/postgres_repository.go`
- Modify: `internal/storage/s3.go`
- Modify: `internal/datahub/postgres_repository_test.go`
- Modify: `internal/datahub/handler_test.go`
- Modify: `scripts/dev/smoke.sh`

- [ ] **Step 1: Add failing tests for prefix-based S3 scans and metadata persistence**

```go
func TestScanDatasetListsObjectsFromBucketPrefix(t *testing.T) {
	repo := newTestDataHubRepo(t)
	lister := fakeObjectLister{
		items: []ScannedObject{
			{Key: "train/a.jpg", ETag: "etag-a", Size: 12, Mime: "image/jpeg"},
			{Key: "train/b.jpg", ETag: "etag-b", Size: 14, Mime: "image/jpeg"},
		},
	}
	svc := NewServiceWithRepositoryAndScanner(presignStub, repo, lister)

	dataset, err := svc.CreateDataset(CreateDatasetInput{ProjectID: 1, Name: "set-1", Bucket: "platform-dev", Prefix: "train"})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	added, err := svc.ScanDataset(dataset.ID)
	if err != nil {
		t.Fatalf("scan dataset: %v", err)
	}
	if added != 2 {
		t.Fatalf("expected 2 indexed objects, got %d", added)
	}
}
```

- [ ] **Step 2: Run Data Hub tests to verify the old manual path fails**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub -run TestScanDatasetListsObjectsFromBucketPrefix -v`
Expected: FAIL because `ScanDataset` still depends on request-supplied `object_keys`.

- [ ] **Step 3: Introduce an S3 scanner seam and persist rich object metadata**

```go
type ScannedObject struct {
	Key    string
	ETag   string
	Size   int64
	Mime   string
	Width  int
	Height int
}

type ObjectScanner interface {
	ListObjects(ctx context.Context, bucket, prefix string) ([]ScannedObject, error)
}

func (s *Service) ScanDataset(datasetID int64) (int, error) {
	dataset, err := s.repo.GetDataset(context.Background(), datasetID)
	if err != nil {
		return 0, err
	}
	objects, err := s.scanner.ListObjects(context.Background(), dataset.Bucket, dataset.Prefix)
	if err != nil {
		return 0, err
	}
	return s.repo.UpsertScannedItems(context.Background(), datasetID, objects)
}
```

- [ ] **Step 4: Re-run tests and update smoke input to use the dataset record instead of raw `object_keys`**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub ./scripts/dev -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/datahub internal/storage/s3.go scripts/dev/smoke.sh
git commit -m "feat: index datasets from s3 prefix metadata"
```

### Task 3: Complete Job Runtime Contract And Worker Consumption

**Files:**
- Modify: `internal/jobs/service.go`
- Modify: `internal/jobs/handler.go`
- Modify: `internal/jobs/postgres_repository.go`
- Modify: `internal/jobs/sweeper.go`
- Modify: `workers/common/job_client.py`
- Modify: `workers/zero_shot/main.py`
- Modify: `workers/video/main.py`
- Modify: `workers/cleaning/main.py`
- Modify: `workers/tests/test_job_client.py`
- Modify: `workers/tests/test_cleaning_rules.py`

- [ ] **Step 1: Add failing tests for worker-facing counters, capabilities, and terminal status updates**

```go
func TestCreateJobStoresCapabilitiesAndCounters(t *testing.T) {
	repo := newTestJobsRepo(t)
	svc := NewServiceWithPublisher(repo, nil)

	job, err := svc.CreateJob(CreateJobInput{
		ProjectID:            1,
		DatasetID:            3,
		SnapshotID:           4,
		JobType:              "cleaning",
		RequiredResourceType: "cpu",
		RequiredCapabilities: []string{"rules_engine"},
		IdempotencyKey:       "clean-1",
		Payload:              map[string]any{"rules": map[string]any{"dark_threshold": 0.2}},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.Payload["rules"] == nil {
		t.Fatalf("expected rules payload, got %+v", job.Payload)
	}
}
```

```python
def test_emit_progress_payload_includes_worker_id_and_counters(self):
    payload = emit_progress(job_id=7, worker_id="worker-a", total=10, ok=8, failed=2)
    self.assertEqual(payload["detail_json"]["worker_id"], "worker-a")
    self.assertEqual(payload["detail_json"]["failed_items"], 2)
```

- [ ] **Step 2: Run Go and Python worker tests to verify missing contract details**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/jobs -v`
Expected: FAIL once the new persistence/counter assertions are added.
Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_job_client workers.tests.test_cleaning_rules -v`
Expected: FAIL once the new worker contract checks are added.

- [ ] **Step 3: Add a single create/update contract for runtime jobs and use it in worker code**

```go
type JobUpdate struct {
	JobID           int64
	WorkerID        string
	Status          string
	TotalItems      int
	SucceededItems  int
	FailedItems     int
	ResultArtifactIDs []int64
	ErrorCode       string
	ErrorMsg        string
}

func (s *Service) ReportProgress(in JobUpdate) error {
	return s.repo.ApplyUpdate(context.Background(), in)
}
```

```python
def emit_terminal(job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int):
    return {
        "job_id": job_id,
        "worker_id": worker_id,
        "status": status,
        "total_items": total,
        "succeeded_items": ok,
        "failed_items": failed,
    }
```

- [ ] **Step 4: Re-run jobs and worker tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/jobs -v`
Expected: PASS.
Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_job_client workers.tests.test_cleaning_rules workers.tests.test_partial_success -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs workers/common/job_client.py workers/zero_shot/main.py workers/video/main.py workers/cleaning/main.py workers/tests
git commit -m "feat: complete job worker runtime contract"
```

### Task 4: Make Review Trust Chain And Snapshot Diff Storage-Backed

**Files:**
- Modify: `internal/review/service.go`
- Modify: `internal/review/handler.go`
- Modify: `internal/versioning/service.go`
- Modify: `internal/versioning/handler.go`
- Modify: `internal/review/handler_test.go`
- Modify: `internal/versioning/handler_test.go`

- [ ] **Step 1: Add failing tests for snapshot-linked diff and persistent review reads**

```go
func TestDiffSnapshotsBySnapshotID(t *testing.T) {
	svc := NewServiceWithRepository(seedVersioningRepo(t))
	result, err := svc.DiffBySnapshotIDs(1, 2, 0.5)
	if err != nil {
		t.Fatalf("diff snapshots: %v", err)
	}
	if result.CompatibilityScore <= 0 {
		t.Fatalf("expected positive compatibility score, got %+v", result)
	}
}

func TestListCandidatesReadsPendingRows(t *testing.T) {
	repo := newTestReviewRepo(t)
	seedPendingCandidate(t, repo)
	svc := NewServiceWithRepository(repo)

	items, err := svc.ListCandidates()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pending candidate, got %d", len(items))
	}
}
```

- [ ] **Step 2: Run review/versioning tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/review ./internal/versioning -v`
Expected: FAIL because review and diff still rely on in-memory state or request-body arrays.

- [ ] **Step 3: Add repository-backed review reads and snapshot-ID diff entrypoints**

```go
type AnnotationRepository interface {
	ListEffectiveAnnotations(ctx context.Context, snapshotID int64) ([]Annotation, error)
}

func (s *Service) DiffBySnapshotIDs(beforeSnapshotID, afterSnapshotID int64, iouThreshold float64) (DiffResult, error) {
	before, err := s.repo.ListEffectiveAnnotations(context.Background(), beforeSnapshotID)
	if err != nil {
		return DiffResult{}, err
	}
	after, err := s.repo.ListEffectiveAnnotations(context.Background(), afterSnapshotID)
	if err != nil {
		return DiffResult{}, err
	}
	return s.DiffSnapshots(before, after, iouThreshold), nil
}
```

```go
type DiffRequest struct {
	BeforeSnapshotID int64   `json:"before_snapshot_id"`
	AfterSnapshotID  int64   `json:"after_snapshot_id"`
	IOUThreshold     float64 `json:"iou_threshold"`
}
```

- [ ] **Step 4: Re-run review/versioning tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/review ./internal/versioning -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/review internal/versioning
git commit -m "feat: back review and diff with persisted snapshot state"
```

### Task 5: Add Import/Export And End-To-End Artifact Packaging

**Files:**
- Modify: `internal/server/http_server.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/artifacts/packager.go`
- Modify: `workers/packager/main.py`
- Modify: `internal/cli/api_source.go`
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/pull_test.go`
- Modify: `internal/artifacts/handler_test.go`
- Modify: `api/openapi/mvp.yaml`

- [ ] **Step 1: Add failing tests for real bundle download and import/export route presence**

```go
func TestAPIArtifactSourceFetchesBundleEntries(t *testing.T) {
	source := NewAPIArtifactSource(newArtifactTestServer(t).URL)
	pkg, err := source.FetchArtifact("demo", "yolo", "v1")
	if err != nil {
		t.Fatalf("fetch artifact: %v", err)
	}
	if len(pkg.Entries) == 0 {
		t.Fatalf("expected bundle entries, got %+v", pkg)
	}
}

func TestImportAndExportRoutesAreRegistered(t *testing.T) {
	srv := NewHTTPServerWithModules(newFakeModules())
	for _, path := range []string{"/v1/snapshots/1/import", "/v1/snapshots/1/export"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		srv.Handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("missing route %s", path)
		}
	}
}
```

- [ ] **Step 2: Run artifacts, CLI, and server tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts ./internal/cli ./internal/server -v`
Expected: FAIL because the default API source still returns metadata-only artifacts and the import/export routes do not exist.

- [ ] **Step 3: Add storage-backed package resolution and snapshot import/export handlers**

```go
type PulledArtifact struct {
	ArtifactID int64
	Version    string
	Entries    []ArtifactEntry
}

func (s *APIArtifactSource) FetchArtifact(dataset, format, version string) (PulledArtifact, error) {
	artifact, err := s.resolveArtifact(dataset, format, version)
	if err != nil {
		return PulledArtifact{}, err
	}
	return s.downloadBundle(artifact)
}
```

```go
r.Post("/snapshots/{id}/import", handlerOrNotImplemented(m.DataHub.ImportSnapshot))
r.Post("/snapshots/{id}/export", handlerOrNotImplemented(m.Artifacts.ExportSnapshot))
```

- [ ] **Step 4: Re-run artifacts, CLI, and route tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts ./internal/cli ./internal/server -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/http_server.go internal/artifacts internal/cli api/openapi/mvp.yaml workers/packager/main.py
git commit -m "feat: deliver real artifact bundles and snapshot import export routes"
```

### Task 6: Harden Contracts, Smoke, Docs, And Acceptance

**Files:**
- Modify: `api/openapi/mvp.yaml`
- Modify: `scripts/dev/smoke.sh`
- Modify: `docs/development/local-quickstart.md`
- Modify: `README.md`
- Modify: `cmd/platform-cli/main.go`
- Modify: `internal/cli/pull.go`

- [ ] **Step 1: Add failing contract and smoke assertions for the final MVP path**

```go
func TestPullHelpShowsDatasetVersionAndFormat(t *testing.T) {
	text := helpText()
	for _, token := range []string{"pull", "--dataset", "--version", "--format", "--allow-partial"} {
		if !strings.Contains(text, token) {
			t.Fatalf("expected help text to contain %s", token)
		}
	}
}
```

```bash
bash scripts/dev/smoke.sh
```

Expected: FAIL until the smoke path covers review candidate creation, diff, package build, artifact resolve, and CLI pull verification.

- [ ] **Step 2: Expand OpenAPI, README, CLI help text, and smoke steps to match actual behavior**

```go
func helpText() string {
	return `platform-cli

Commands:
  pull

Flags for pull:
  --dataset
  --format
  --version
  --allow-partial
`
}
```

- [ ] **Step 3: Run the full acceptance suite**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...`
Expected: PASS.
Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success workers.tests.test_job_client workers.tests.test_cleaning_rules -v`
Expected: PASS.
Run: `bash scripts/dev/smoke.sh`
Expected: PASS against the Docker-backed PostgreSQL, Redis, and MinIO stack.

- [ ] **Step 4: Update docs with the exact local runtime sequence**

```markdown
1. `make up-dev`
2. `make migrate-up`
3. `go run ./cmd/api-server`
4. `platform-cli pull --dataset smoke-dataset --format yolo --version v1`
5. `bash scripts/dev/smoke.sh`
```

- [ ] **Step 5: Commit**

```bash
git add api/openapi/mvp.yaml scripts/dev/smoke.sh docs/development/local-quickstart.md README.md cmd/platform-cli/main.go internal/cli/pull.go
git commit -m "docs: align contracts smoke and cli with final mvp flow"
```

## Exit Criteria For This Plan

1. `cmd/api-server` no longer wires Review and Artifacts through process-local state.
2. Data Hub scan enumerates S3 objects from stored dataset coordinates instead of trusting client-fed keys.
3. Jobs persist dataset/snapshot/capability state and workers can report counters, heartbeat, and terminal status.
4. Review uses `annotation_candidates`, `annotations`, and `audit_logs` instead of in-memory slices.
5. Snapshot diff is callable by snapshot IDs, and import/export routes exist in router and OpenAPI.
6. Artifact package requests produce a persisted, downloadable package that `platform-cli pull` can actually verify.
7. OpenAPI, docs, smoke, and tests all match the runtime behavior.

