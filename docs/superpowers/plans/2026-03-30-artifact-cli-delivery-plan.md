# Artifact CLI Delivery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver real YOLO artifact builds from canonical annotations, persist artifact lifecycle state, expose immediate-return artifact build APIs plus direct package download, and make `platform-cli pull` download, extract, verify, and clean up real archives.

**Architecture:** The implementation adds a PostgreSQL-backed artifact repository, a canonical export query, a YOLO package builder, and a filesystem-backed artifact storage adapter with atomic promotion. Package creation returns `202 Accepted`, queues work into a bounded in-process build runner, updates artifact status (`pending -> building -> ready|failed`), and exposes a direct archive download endpoint that the CLI consumes using a temp-archive pull pipeline.

**Tech Stack:** Go 1.20, chi, pgx/pgxpool, MinIO SDK, archive/tar + compress/gzip from stdlib, PostgreSQL, MinIO, bash smoke script.

---

## File Structure

**Create**

- `migrations/000002_artifact_build_runtime.up.sql`
- `migrations/000002_artifact_build_runtime.down.sql`
- `internal/artifacts/postgres_repository.go`
- `internal/artifacts/export_query.go`
- `internal/artifacts/builder.go`
- `internal/artifacts/storage.go`
- `internal/artifacts/filesystem_storage.go`
- `internal/artifacts/build_runner.go`
- `internal/artifacts/postgres_repository_test.go`
- `internal/artifacts/export_query_test.go`
- `internal/artifacts/builder_test.go`
- `internal/artifacts/filesystem_storage_test.go`
- `internal/artifacts/build_runner_test.go`
- `internal/cli/archive.go`
- `internal/cli/archive_test.go`
- `cmd/dev-seed-artifact-smoke/main.go`

**Modify**

- `internal/artifacts/service.go`
- `internal/artifacts/handler.go`
- `internal/artifacts/repository.go`
- `internal/artifacts/handler_test.go`
- `internal/artifacts/packager.go`
- `internal/artifacts/packager_test.go`
- `internal/server/http_server.go`
- `internal/server/http_server_routes_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `cmd/api-server/main.go`
- `cmd/api-server/main_test.go`
- `internal/cli/api_source.go`
- `internal/cli/pull.go`
- `internal/cli/pull_test.go`
- `internal/cli/verify.go`
- `api/openapi/mvp.yaml`
- `scripts/dev/smoke.sh`
- `scripts/dev/smoke_test.go`
- `docs/development/local-quickstart.md`
- `README.md`

## Task 1: Persist Artifact Lifecycle In PostgreSQL

**Files:**
- Create: `migrations/000002_artifact_build_runtime.up.sql`
- Create: `migrations/000002_artifact_build_runtime.down.sql`
- Create: `internal/artifacts/postgres_repository.go`
- Create: `internal/artifacts/postgres_repository_test.go`
- Modify: `internal/artifacts/repository.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing repository tests**

```go
func TestPostgresRepositoryCreateUpdateAndResolveReadyArtifact(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)
	artifact, err := repo.Create(context.Background(), Artifact{
		ProjectID:   1,
		DatasetID:   1,
		SnapshotID:  1,
		ArtifactType:"dataset-export",
		Format:      "yolo",
		Version:     "v-artifact-repo",
		URI:         "",
		ManifestURI: "",
		Checksum:    "",
		Size:        0,
		Status:      StatusPending,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	if _, ok, err := repo.FindReadyByFormatVersion(ctx, "yolo", "v-artifact-repo"); err != nil || ok {
		t.Fatalf("expected pending artifact to be hidden from resolve, ok=%v err=%v", ok, err)
	}

	updated, err := repo.UpdateBuildResult(ctx, artifact.ID, BuildResult{
		Status:      StatusReady,
		URI:         "artifact://ready/package.yolo.tar.gz",
		ManifestURI: "artifact://ready/manifest.json",
		Checksum:    "sha256:abc123",
		Size:        123,
	})
	if err != nil {
		t.Fatalf("update build result: %v", err)
	}
	if updated.Status != StatusReady {
		t.Fatalf("expected ready status, got %s", updated.Status)
	}

	resolved, ok, err := repo.FindReadyByFormatVersion(ctx, "yolo", "v-artifact-repo")
	if err != nil || !ok || resolved.ID != artifact.ID {
		t.Fatalf("expected resolved ready artifact, got ok=%v err=%v artifact=%+v", ok, err, resolved)
	}
}
```

```go
func TestInMemoryRepositoryMarksQueuedArtifactsFailedOnStartupRecovery(t *testing.T) {
	repo := NewInMemoryRepository()
	artifact, err := repo.Create(context.Background(), Artifact{
		ProjectID: 1, DatasetID: 1, SnapshotID: 1, Format: "yolo", Version: "v-stale", Status: StatusBuilding,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	affected, err := repo.MarkStaleBuildsFailed(context.Background(), "startup_recovery")
	if err != nil {
		t.Fatalf("mark stale failed: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected one stale artifact to be marked failed, got %d", affected)
	}

	got, ok, err := repo.Get(context.Background(), artifact.ID)
	if err != nil || !ok || got.Status != StatusFailed {
		t.Fatalf("expected failed artifact after recovery, got ok=%v err=%v artifact=%+v", ok, err, got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts -run 'Test(PostgresRepositoryCreateUpdateAndResolveReadyArtifact|InMemoryRepositoryMarksQueuedArtifactsFailedOnStartupRecovery)' -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewPostgresRepository
FAIL ... undefined: StatusPending
```

- [ ] **Step 3: Add the migration and repository implementation**

Create `migrations/000002_artifact_build_runtime.up.sql`:

```sql
alter table artifacts
  add column if not exists error_msg text not null default '';

create index if not exists idx_artifacts_format_version_ready
  on artifacts(format, version)
  where status = 'ready';
```

Create `migrations/000002_artifact_build_runtime.down.sql`:

```sql
drop index if exists idx_artifacts_format_version_ready;

alter table artifacts
  drop column if exists error_msg;
```

Update `internal/artifacts/repository.go`:

```go
type Repository interface {
	Create(ctx context.Context, a Artifact) (Artifact, error)
	Get(ctx context.Context, id int64) (Artifact, bool, error)
	FindReadyByFormatVersion(ctx context.Context, format, version string) (Artifact, bool, error)
	UpdateStatus(ctx context.Context, id int64, status string, errorMsg string) (Artifact, error)
	UpdateBuildResult(ctx context.Context, id int64, result BuildResult) (Artifact, error)
	MarkStaleBuildsFailed(ctx context.Context, reason string) (int64, error)
}

const (
	StatusPending  = "pending"
	StatusBuilding = "building"
	StatusReady    = "ready"
	StatusFailed   = "failed"
)

type BuildResult struct {
	Status      string
	URI         string
	ManifestURI string
	Checksum    string
	Size        int64
	ErrorMsg    string
}
```

Create `internal/artifacts/postgres_repository.go` with `insert`, `select`, `update`, and `stale build` methods mirroring the style used in `internal/datahub/postgres_repository.go`.

Update `internal/artifacts/service.go` model fields:

```go
type Artifact struct {
	ID           int64             `json:"id"`
	ProjectID    int64             `json:"project_id"`
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	ArtifactType string            `json:"artifact_type"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	URI          string            `json:"uri"`
	ManifestURI  string            `json:"manifest_uri"`
	Checksum     string            `json:"checksum"`
	Size         int64             `json:"size"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
	Status       string            `json:"status"`
	ErrorMsg     string            `json:"error_msg,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}
```

Update `internal/config/config.go`:

```go
type Config struct {
	// existing fields...
	ArtifactStorageDir      string
	ArtifactBuildConcurrency int
}

cfg := Config{
	// existing fields...
	ArtifactStorageDir:       envOrDefault("ARTIFACT_STORAGE_DIR", filepath.Join(os.TempDir(), "platform-artifacts")),
	ArtifactBuildConcurrency: intOrDefault("ARTIFACT_BUILD_CONCURRENCY", 2),
}
```

- [ ] **Step 4: Run the artifact repository tests again**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts -run 'Test(PostgresRepositoryCreateUpdateAndResolveReadyArtifact|InMemoryRepositoryMarksQueuedArtifactsFailedOnStartupRecovery)' -count=1 -v
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/artifacts
```

- [ ] **Step 5: Commit**

```bash
git add migrations/000002_artifact_build_runtime.up.sql migrations/000002_artifact_build_runtime.down.sql internal/artifacts/repository.go internal/artifacts/postgres_repository.go internal/artifacts/postgres_repository_test.go internal/artifacts/service.go internal/config/config.go internal/config/config_test.go
git commit -m "feat: persist artifact lifecycle state"
```

## Task 2: Query Canonical Snapshot Data And Build YOLO Packages

**Files:**
- Create: `internal/artifacts/export_query.go`
- Create: `internal/artifacts/builder.go`
- Create: `internal/artifacts/export_query_test.go`
- Create: `internal/artifacts/builder_test.go`
- Modify: `internal/artifacts/packager.go`
- Modify: `internal/artifacts/packager_test.go`

- [ ] **Step 1: Write the failing export query test**

```go
func TestExportQueryBuildsSnapshotBundleFromCanonicalAnnotations(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, config.Config{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	projectID := seedProject(t, ctx, pool)
	datasetID, snapshot1, snapshot2, itemID := seedDatasetSnapshotAndItem(t, ctx, pool, projectID)
	categoryID := seedCategory(t, ctx, pool, projectID, "person")
	seedAnnotation(t, ctx, pool, datasetID, itemID, categoryID, snapshot1, nil)

	query := NewExportQuery(pool)
	bundle, err := query.LoadSnapshotBundle(ctx, datasetID, snapshot2, "v2")
	if err != nil {
		t.Fatalf("load snapshot bundle: %v", err)
	}
	if len(bundle.Categories) != 1 || bundle.Categories[0] != "person" {
		t.Fatalf("unexpected categories: %+v", bundle.Categories)
	}
	if len(bundle.Items) != 1 || len(bundle.Items[0].Boxes) != 1 {
		t.Fatalf("unexpected bundle items: %+v", bundle.Items)
	}
}
```

- [ ] **Step 2: Write the failing package builder test**

```go
func TestBuilderWritesTrainLayoutAndManifestStats(t *testing.T) {
	workdir := t.TempDir()
	builder := NewBuilder(fakeObjectSource{
		"train/a.jpg": []byte("fake-image-a"),
	})

	bundle := ExportBundle{
		Version:    "v1",
		Categories: []string{"person"},
		Items: []ExportItem{
			{
				ObjectKey:     "train/a.jpg",
				OutputName:    "a.jpg",
				LabelFileName: "a.txt",
				Boxes: []YOLOBox{
					{ClassIndex: 0, XCenter: 0.5, YCenter: 0.5, Width: 0.2, Height: 0.2},
				},
			},
		},
	}

	out, err := builder.Build(context.Background(), workdir, bundle)
	if err != nil {
		t.Fatalf("build package: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.RootDir, "train", "images", "a.jpg")); err != nil {
		t.Fatalf("missing train image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.RootDir, "train", "labels", "a.txt")); err != nil {
		t.Fatalf("missing train label: %v", err)
	}
	manifestBody, err := os.ReadFile(filepath.Join(out.RootDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !bytes.Contains(manifestBody, []byte(`"category_map"`)) || !bytes.Contains(manifestBody, []byte(`"sha256:`)) {
		t.Fatalf("manifest missing category_map or sha256 checksums: %s", manifestBody)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts -run 'Test(ExportQueryBuildsSnapshotBundleFromCanonicalAnnotations|BuilderWritesTrainLayoutAndManifestStats)' -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewExportQuery
FAIL ... undefined: NewBuilder
```

- [ ] **Step 4: Implement the export query and builder**

Create `internal/artifacts/export_query.go`:

```go
type ExportQuery struct {
	pool *pgxpool.Pool
}

type ExportBundle struct {
	ProjectID    int64
	DatasetID    int64
	SnapshotID   int64
	Version      string
	Categories   []string
	CategoryMap  map[int64]int
	Items        []ExportItem
	TotalBoxes   int
}

type ExportItem struct {
	ItemID         int64
	ObjectKey      string
	OutputName     string
	LabelFileName  string
	Boxes          []YOLOBox
}

type YOLOBox struct {
	ClassIndex int
	XCenter    float64
	YCenter    float64
	Width      float64
	Height     float64
}
```

Use one query for dataset/snapshot validation plus category lookup, one query for dataset items, and one interval-filtered query:

```sql
select a.item_id, a.category_id, a.bbox_x, a.bbox_y, a.bbox_w, a.bbox_h
from annotations a
where a.dataset_id = $1
  and a.created_at_snapshot_id <= $2
  and (a.deleted_at_snapshot_id is null or a.deleted_at_snapshot_id > $2)
order by a.item_id asc, a.category_id asc, a.id asc
```

Create `internal/artifacts/builder.go`:

```go
type ObjectSource interface {
	ReadObject(ctx context.Context, objectKey string) ([]byte, error)
}

type BuildOutput struct {
	RootDir       string
	ManifestPath  string
	ArchivePath   string
	ArchiveSHA256 string
	ArchiveSize   int64
}

type Builder struct {
	source ObjectSource
}
```

Builder responsibilities:

```go
func (b *Builder) Build(ctx context.Context, workdir string, bundle ExportBundle) (BuildOutput, error) {
	rootDir := filepath.Join(workdir, "package")
	imagesDir := filepath.Join(rootDir, "train", "images")
	labelsDir := filepath.Join(rootDir, "train", "labels")
	// mkdirs
	// copy image bytes from ObjectSource
	// write YOLO label files
	// write data.yaml with train: ./train/images and val: ./train/images
	// write manifest.json with category_map and stats
	// tar.gz the rootDir
	// return archive checksum as sha256:<hex>
}
```

Update `internal/artifacts/packager.go` so manifest entries and helpers use the new labeled checksum format:

```go
type Manifest struct {
	Version      string            `json:"version"`
	GeneratedAt  time.Time         `json:"generated_at"`
	CategoryMap  map[string]string `json:"category_map"`
	Stats        ManifestStats     `json:"stats"`
	Entries      []ManifestEntry   `json:"entries"`
}
```

- [ ] **Step 5: Run the artifact package tests again**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts -run 'Test(ExportQueryBuildsSnapshotBundleFromCanonicalAnnotations|BuilderWritesTrainLayoutAndManifestStats|BuildManifestIncludesChecksums)' -count=1 -v
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/artifacts
```

- [ ] **Step 6: Commit**

```bash
git add internal/artifacts/export_query.go internal/artifacts/builder.go internal/artifacts/export_query_test.go internal/artifacts/builder_test.go internal/artifacts/packager.go internal/artifacts/packager_test.go
git commit -m "feat: build yolo artifact packages from canonical annotations"
```

## Task 3: Add Filesystem Artifact Storage And Async Build Runner

**Files:**
- Create: `internal/artifacts/storage.go`
- Create: `internal/artifacts/filesystem_storage.go`
- Create: `internal/artifacts/filesystem_storage_test.go`
- Create: `internal/artifacts/build_runner.go`
- Create: `internal/artifacts/build_runner_test.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/artifacts/handler_test.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`
- Modify: `cmd/api-server/main.go`
- Modify: `cmd/api-server/main_test.go`
- Modify: `api/openapi/mvp.yaml`

- [ ] **Step 1: Write the failing storage and runner tests**

```go
func TestFilesystemStoragePromotesArchiveAtomically(t *testing.T) {
	root := t.TempDir()
	storage := NewFilesystemStorage(root)
	srcDir := t.TempDir()

	archivePath := filepath.Join(srcDir, "package.yolo.tar.gz")
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	loc, err := storage.StoreBuild(context.Background(), StoreRequest{
		Version:      "v1",
		ArchivePath:  archivePath,
		ManifestPath: filepath.Join(srcDir, "manifest.json"),
	})
	if err != nil {
		t.Fatalf("store build: %v", err)
	}
	if strings.Contains(loc.ArchivePath, ".tmp") {
		t.Fatalf("expected final archive path, got %s", loc.ArchivePath)
	}
}
```

```go
func TestBuildRunnerLimitsConcurrentBuilds(t *testing.T) {
	repo := NewInMemoryRepository()
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	runner := NewBuildRunner(1, func(ctx context.Context, artifactID int64) error {
		started <- struct{}{}
		<-release
		return nil
	}, repo)

	first, _ := repo.Create(context.Background(), Artifact{ProjectID: 1, DatasetID: 1, SnapshotID: 1, Format: "yolo", Version: "v1", Status: StatusPending})
	second, _ := repo.Create(context.Background(), Artifact{ProjectID: 1, DatasetID: 1, SnapshotID: 2, Format: "yolo", Version: "v2", Status: StatusPending})

	runner.Start(context.Background())
	runner.Enqueue(first.ID)
	runner.Enqueue(second.ID)

	<-started
	select {
	case <-started:
		t.Fatal("expected only one build to start while concurrency=1")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-started
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts -run 'Test(FilesystemStoragePromotesArchiveAtomically|BuildRunnerLimitsConcurrentBuilds)' -count=1 -v
```

Expected:

```text
FAIL ... undefined: NewFilesystemStorage
FAIL ... undefined: NewBuildRunner
```

- [ ] **Step 3: Implement the storage adapter**

Create `internal/artifacts/storage.go`:

```go
type StoreRequest struct {
	Version      string
	ArchivePath  string
	ManifestPath string
	PackageDir   string
}

type StoredArtifact struct {
	ArchivePath  string
	ManifestPath string
	ArchiveURI   string
	ManifestURI  string
	ArchiveSize  int64
}

type ArtifactStorage interface {
	StoreBuild(ctx context.Context, req StoreRequest) (StoredArtifact, error)
	OpenArchive(ctx context.Context, uri string) (io.ReadSeekCloser, int64, error)
}
```

Create `internal/artifacts/filesystem_storage.go`:

```go
type FilesystemStorage struct {
	root string
}

func (s *FilesystemStorage) StoreBuild(ctx context.Context, req StoreRequest) (StoredArtifact, error) {
	finalDir := filepath.Join(s.root, req.Version)
	tempDir := finalDir + ".tmp"
	// mkdir tempDir
	// copy manifest + archive into tempDir
	// rename tempDir -> finalDir
	// return artifact:// URIs plus archive size
}
```

- [ ] **Step 4: Implement the build runner and service wiring**

Create `internal/artifacts/build_runner.go`:

```go
type BuildRunner struct {
	queue chan int64
	run   func(ctx context.Context, artifactID int64) error
}

func NewBuildRunner(concurrency int, run func(context.Context, int64) error) *BuildRunner {
	return &BuildRunner{
		queue: make(chan int64, 128),
		run:   run,
	}
}

func (r *BuildRunner) Start(ctx context.Context, concurrency int) {
	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case artifactID := <-r.queue:
					_ = r.run(ctx, artifactID)
				}
			}
		}()
	}
}
```

Update `internal/artifacts/service.go` to compose:

```go
type Service struct {
	repo    Repository
	query   *ExportQuery
	builder *Builder
	storage ArtifactStorage
	runner  *BuildRunner
}

func (s *Service) CreatePackageJob(ctx context.Context, in PackageRequest) (int64, Artifact, error) {
	artifact, err := s.repo.Create(ctx, Artifact{
		ProjectID:    1,
		DatasetID:    in.DatasetID,
		SnapshotID:   in.SnapshotID,
		ArtifactType: "dataset-export",
		Format:       in.Format,
		Version:      defaultVersion(in),
		Status:       StatusPending,
	})
	if err != nil {
		return 0, Artifact{}, err
	}
	s.runner.Enqueue(artifact.ID)
	return artifact.ID, artifact, nil
}
```

Update `internal/artifacts/handler.go` to return `202 Accepted`:

```go
writeJSON(w, http.StatusAccepted, map[string]any{
	"job_id":      artifact.ID,
	"artifact_id": artifact.ID,
	"status":      artifact.Status,
})
```

Update `internal/server/http_server.go` to add:

```go
type ArtifactRoutes struct {
	CreatePackage   http.HandlerFunc
	GetArtifact     http.HandlerFunc
	PresignArtifact http.HandlerFunc
	ResolveArtifact http.HandlerFunc
	DownloadArtifact http.HandlerFunc
}

r.Get("/artifacts/{id}/download", handlerOrNotImplemented(m.Artifacts.DownloadArtifact))
```

Update `cmd/api-server/main.go` to wire the concrete runtime path:

```go
artifactRepo := artifacts.NewPostgresRepository(pool)
artifactQuery := artifacts.NewExportQuery(pool)
artifactStorage := artifacts.NewFilesystemStorage(cfg.ArtifactStorageDir)
artifactSource := artifacts.NewS3ObjectSource(s3Client, cfg.S3Bucket)
artifactBuilder := artifacts.NewBuilder(artifactSource)
artifactService := artifacts.NewService(artifactRepo, artifactQuery, artifactBuilder, artifactStorage)
artifactService.MarkStaleBuildsFailed(ctx, "startup_recovery")
artifactService.StartBuildRunner(ctx, cfg.ArtifactBuildConcurrency)
artifactHandler := artifacts.NewHandler(artifactService)
```

Update `api/openapi/mvp.yaml`:

```yaml
  /v1/artifacts/{id}/download:
    get:
      summary: Download artifact package archive
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: Artifact archive stream
```

- [ ] **Step 5: Run the artifact handler tests again**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts ./internal/server ./cmd/api-server -run 'Test(CreatePackageReturnsJobID|FilesystemStoragePromotesArchiveAtomically|BuildRunnerLimitsConcurrentBuilds|MVPRoutesAreRegistered|StartBackgroundLoopInvokesTick)' -count=1 -v
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/artifacts
ok  	yolo-ave-mujica/internal/server
ok  	yolo-ave-mujica/cmd/api-server
```

- [ ] **Step 6: Commit**

```bash
git add internal/artifacts/storage.go internal/artifacts/filesystem_storage.go internal/artifacts/filesystem_storage_test.go internal/artifacts/build_runner.go internal/artifacts/build_runner_test.go internal/artifacts/service.go internal/artifacts/handler.go internal/artifacts/handler_test.go internal/server/http_server.go internal/server/http_server_routes_test.go cmd/api-server/main.go cmd/api-server/main_test.go
git commit -m "feat: add async artifact builds with bounded concurrency"
```

## Task 4: Expose Direct Archive Download And Update CLI Pull

**Files:**
- Create: `internal/cli/archive.go`
- Create: `internal/cli/archive_test.go`
- Modify: `internal/cli/api_source.go`
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/verify.go`
- Modify: `internal/cli/pull_test.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/artifacts/handler_test.go`

- [ ] **Step 1: Write the failing CLI pull test**

```go
func TestPullDownloadsArchiveExtractsManifestAndCleansTempArchive(t *testing.T) {
	dir := t.TempDir()
	server := newArtifactDownloadServer(t)
	client := NewPullClientWithSource(dir, NewAPIArtifactSource(server.URL))

	err := client.Pull(PullOptions{Format: "yolo", Version: "v1", OutputDir: dir})
	if err != nil {
		t.Fatalf("pull returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "pulled-v1", "train", "images", "a.jpg")); err != nil {
		t.Fatalf("missing extracted image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pull-v1.tar.gz.part")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temporary archive cleanup, got err=%v", err)
	}
}
```

```go
func TestVerifyFileRejectsNonSHA256Checksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	if err := VerifyFile(path, "md5:deadbeef"); err == nil {
		t.Fatal("expected non-sha256 checksum to be rejected")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/cli -run 'Test(PullDownloadsArchiveExtractsManifestAndCleansTempArchive|VerifyFileRejectsNonSHA256Checksum)' -count=1 -v
```

Expected:

```text
FAIL ... missing extracted image
FAIL ... expected non-sha256 checksum to be rejected
```

- [ ] **Step 3: Implement archive download/extract helpers**

Create `internal/cli/archive.go`:

```go
func downloadArchiveToTemp(ctx context.Context, client *http.Client, targetURL, tempPath string) error {
	// open tempPath with append mode
	// if file has bytes already, send Range header
	// stream response body into file
}

func extractTarGz(archivePath, outDir string) error {
	// open archivePath
	// gzip.NewReader
	// tar.NewReader
	// create directories and files under outDir
}
```

Update `internal/cli/verify.go`:

```go
func VerifyFile(path string, expected string) error {
	parts := strings.SplitN(expected, ":", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return fmt.Errorf("unsupported checksum algorithm %q", expected)
	}
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if got != parts[1] {
		return fmt.Errorf("checksum mismatch: got sha256:%s expected %s", got, expected)
	}
	return nil
}
```

Update `internal/cli/api_source.go` interface to return resolved metadata with a download URL:

```go
type ResolvedArtifact struct {
	ArtifactID   int64
	Version      string
	DownloadURL  string
}

type ArtifactSource interface {
	ResolveArtifact(format, version string) (ResolvedArtifact, error)
	DownloadArchive(ctx context.Context, artifact ResolvedArtifact, tempPath string) error
}
```

- [ ] **Step 4: Update `Pull` to use temp archive + extract + cleanup**

Modify `internal/cli/pull.go`:

```go
tempArchivePath := filepath.Join(outDir, fmt.Sprintf("pull-%s.tar.gz.part", resolved.Version))
cleanup := func() { _ = os.Remove(tempArchivePath) }
defer cleanup()

if err := c.source.DownloadArchive(context.Background(), resolved, tempArchivePath); err != nil {
	return err
}
if err := extractTarGz(tempArchivePath, artifactDir); err != nil {
	return err
}
manifest, err := loadManifest(filepath.Join(artifactDir, "manifest.json"))
if err != nil {
	return err
}
for _, entry := range manifest.Entries {
	if err := VerifyFile(filepath.Join(artifactDir, entry.Path), entry.Checksum); err != nil {
		// existing failedFiles logic
	}
}
```

Add direct download handler in `internal/artifacts/handler.go`:

```go
func (h *Handler) DownloadArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	reader, size, filename, err := h.svc.OpenArchive(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer reader.Close()
	http.ServeContent(w, r, filename, time.Now().UTC(), io.NewSectionReader(reader, 0, size))
}
```

- [ ] **Step 5: Run CLI and artifact tests again**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/cli ./internal/artifacts -run 'Test(PullDownloadsArchiveExtractsManifestAndCleansTempArchive|VerifyFileRejectsNonSHA256Checksum|ResolveArtifactByFormatAndVersion)' -count=1 -v
```

Expected:

```text
PASS
ok  	yolo-ave-mujica/internal/cli
ok  	yolo-ave-mujica/internal/artifacts
```

- [ ] **Step 6: Commit**

```bash
git add internal/cli/archive.go internal/cli/archive_test.go internal/cli/api_source.go internal/cli/pull.go internal/cli/verify.go internal/cli/pull_test.go internal/artifacts/handler.go internal/artifacts/handler_test.go
git commit -m "feat: pull and verify real artifact archives"
```

## Task 5: Extend Local Smoke To Cover Artifact Delivery

**Files:**
- Create: `cmd/dev-seed-artifact-smoke/main.go`
- Modify: `scripts/dev/smoke.sh`
- Modify: `scripts/dev/smoke_test.go`
- Modify: `docs/development/local-quickstart.md`
- Modify: `README.md`

- [ ] **Step 1: Write the failing smoke-script unit test**

```go
func TestSmokeExercisesArtifactBuildAndPull(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash smoke test is not supported on windows")
	}

	fakeBin := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
url=""
for arg in "$@"; do
  if [[ "$arg" == http://* || "$arg" == https://* ]]; then
    url="$arg"
  fi
done
case "$url" in
  */v1/artifacts/packages)
    printf '{"artifact_id":7,"status":"pending"}\n'
    ;;
  */v1/artifacts/7)
    printf '{"id":7,"status":"ready","format":"yolo","version":"v1"}\n'
    ;;
  */v1/artifacts/resolve*)
    printf '{"id":7,"version":"v1","download_url":"http://127.0.0.1:8080/v1/artifacts/7/download"}\n'
    ;;
  *)
    exit 0
    ;;
esac
`)

	cmd := exec.Command("bash", "scripts/dev/smoke.sh")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke script failed: %v\n%s", err, out)
	}
}
```

- [ ] **Step 2: Run the smoke unit test to verify it fails**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./scripts/dev -run TestSmokeExercisesArtifactBuildAndPull -count=1 -v
```

Expected:

```text
FAIL ... artifact package request failed
```

- [ ] **Step 3: Add a helper command to seed smoke fixtures**

Create `cmd/dev-seed-artifact-smoke/main.go`:

```go
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	s3Client, err := storage.NewS3Client(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := seedArtifactSmokeData(ctx, pool, s3Client, cfg.S3Bucket); err != nil {
		log.Fatal(err)
	}
}
```

Seed:

1. dataset if missing
2. dataset items `train/a.jpg`, `train/b.jpg`
3. snapshot `v1`
4. one category `person`
5. one canonical annotation row
6. upload two small fixture images to MinIO

- [ ] **Step 4: Extend `scripts/dev/smoke.sh`**

Add the artifact path after the existing zero-shot job check:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/dev-seed-artifact-smoke >/dev/null || fail "artifact smoke seed failed"

artifact_response="$(curl -fsS -X POST "${api_base}/v1/artifacts/packages" \
  -H 'Content-Type: application/json' \
  -d "{\"dataset_id\":${dataset_id},\"snapshot_id\":1,\"format\":\"yolo\",\"version\":\"v-smoke\"}")" || fail "artifact package request failed"

artifact_id="$(printf '%s' "$artifact_response" | tr -d '\n' | sed -n 's/.*"artifact_id":[[:space:]]*\([0-9][0-9]*\).*/\1/p')"

for _ in $(seq 1 60); do
  artifact_status="$(curl -fsS "${api_base}/v1/artifacts/${artifact_id}")" || fail "artifact status request failed"
  if [[ "$artifact_status" == *'"status":"ready"'* ]]; then
    break
  fi
  sleep 0.5
done

GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/platform-cli pull --format yolo --version v-smoke >/dev/null || fail "artifact pull failed"
[[ -f "pulled-v-smoke/data.yaml" ]] || fail "missing pulled data.yaml"
[[ -f "pulled-v-smoke/train/images/a.jpg" ]] || fail "missing pulled train image"
[[ -f "pulled-v-smoke/train/labels/a.txt" ]] || fail "missing pulled train label"
```

- [ ] **Step 5: Run the smoke tests and update docs**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./scripts/dev -run 'Test(SmokeSkipsUpDevWhenDependenciesAreAlreadyReachable|SmokeExercisesArtifactBuildAndPull)' -count=1 -v
```

Then update `docs/development/local-quickstart.md` and `README.md` to document:

```text
ARTIFACT_STORAGE_DIR=/tmp/platform-artifacts
ARTIFACT_BUILD_CONCURRENCY=2
scripts/dev/smoke.sh now covers artifact build + pull
```

- [ ] **Step 6: Run the full verification pass**

Run:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
```

Run:

```bash
PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success workers.tests.test_job_client workers.tests.test_cleaning_rules -v
```

Run:

```bash
bash scripts/dev/smoke.sh
```

Expected:

```text
all Go tests pass
all Python worker tests pass
smoke.sh exits 0
```

- [ ] **Step 7: Commit**

```bash
git add cmd/dev-seed-artifact-smoke/main.go scripts/dev/smoke.sh scripts/dev/smoke_test.go docs/development/local-quickstart.md README.md
git commit -m "test: cover artifact delivery in local smoke"
```

## Plan Self-Review

### Spec coverage

- Artifact persistence and lifecycle states: covered in Task 1.
- Canonical annotation export and YOLO package generation: covered in Task 2.
- Atomic storage writes plus filesystem adapter: covered in Task 3.
- Immediate-return async build with bounded concurrency: covered in Task 3.
- Direct archive download and CLI pull with temp archive cleanup: covered in Task 4.
- Smoke and docs updates: covered in Task 5.

### Placeholder scan

- No `TODO`, `TBD`, or deferred implementation markers remain in task steps.
- All code-changing steps include concrete file paths and representative code blocks.
- Every verification step includes an explicit command.

### Type consistency

- Artifact lifecycle constants are introduced once in Task 1 and reused in later tasks.
- Export bundle types are introduced in Task 2 and consumed by Task 3.
- CLI checksum format is defined once as `sha256:<hex>` and reused in Task 4 and Task 5.
