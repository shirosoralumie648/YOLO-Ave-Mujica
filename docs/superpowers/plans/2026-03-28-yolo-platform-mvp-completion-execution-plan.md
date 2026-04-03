# YOLO Platform MVP Completion Execution Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the current in-flight MVP branch so the full route surface, runtime wiring, lease-aware job flow, verification-aware CLI flow, and local smoke path all pass against the approved completion design.

**Architecture:** Keep in-memory implementations for focused tests, but add PostgreSQL, Redis, and MinIO-backed runtime paths from the composition layer. Put schema control under `golang-migrate`, add bounded graceful shutdown to `api-server`, finish the Data Hub -> Jobs -> Review/Versioning -> Artifacts -> CLI -> smoke path, and add one minimal artifact lookup surface so `platform-cli pull --format --version` has a real runtime resolver.

**Tech Stack:** Go 1.24, Chi, pgx/v5, go-redis/v9, MinIO SDK, golang-migrate, Python 3.11+, unittest, Docker Compose, PostgreSQL 16, Redis 7, MinIO.

---

## File Structure Map

### Schema + Runtime Bootstrap

- Create: `cmd/migrate/main.go`  
  Responsibility: run `golang-migrate` commands (`up`, `down`, `version`, `force`) against `DATABASE_URL`.
- Create: `internal/store/migrate.go`  
  Responsibility: shared migration runner helper used by `cmd/migrate`.
- Create: `internal/store/migrate_test.go`  
  Responsibility: verify the repository uses `golang-migrate` file naming and a canonical baseline.
- Create: `migrations/000001_init.up.sql`  
  Responsibility: canonical baseline schema for all existing tables.
- Create: `migrations/000001_init.down.sql`  
  Responsibility: reverse DDL for the canonical baseline.
- Delete: `migrations/0001_init.sql`  
  Responsibility: remove the pre-migration-tool schema file after the baseline is split into `up/down`.
- Modify: `Makefile`  
  Responsibility: add `migrate-up`, `migrate-down`, `migrate-version`, and wire smoke prerequisites.

### API Process Lifecycle

- Modify: `cmd/api-server/main.go`  
  Responsibility: runtime composition root, signal handling, graceful shutdown, dependency wiring.
- Create: `cmd/api-server/main_test.go`  
  Responsibility: verify bounded shutdown and cancellation-driven server exit.
- Modify: `internal/config/config.go`  
  Responsibility: parse runtime knobs (`HTTP_ADDR`, `SHUTDOWN_TIMEOUT`, `SWEEPER_INTERVAL`, `LEASE_SECONDS`, `API_BASE_URL`).
- Modify: `internal/config/config_test.go`  
  Responsibility: verify required env and default runtime values.
- Modify: `internal/store/postgres.go`  
  Responsibility: create a tuned `pgxpool` using config.
- Modify: `internal/queue/redis.go`  
  Responsibility: create and verify Redis runtime client.
- Modify: `internal/storage/s3.go`  
  Responsibility: MinIO client + presign helpers with runtime-safe host handling.

### Data Hub

- Modify: `internal/datahub/repository.go`  
  Responsibility: keep the repository interface stable while preserving the in-memory implementation.
- Create: `internal/datahub/postgres_repository.go`  
  Responsibility: PostgreSQL-backed dataset, item, and snapshot persistence.
- Create: `internal/datahub/postgres_repository_test.go`  
  Responsibility: integration coverage for dataset create, scan, list, and snapshot round-trips.
- Modify: `internal/datahub/service.go`  
  Responsibility: orchestrate repository-backed runtime behavior plus MinIO presign.
- Modify: `internal/datahub/handler.go`  
  Responsibility: keep HTTP shapes stable while surfacing repository errors correctly.
- Modify: `internal/datahub/handler_test.go`  
  Responsibility: preserve route-level behavior and add error-path expectations.

### Jobs

- Modify: `internal/jobs/model.go`  
  Responsibility: add `WorkerID` and keep lease/job fields aligned with schema.
- Modify: `internal/jobs/repository.go`  
  Responsibility: define repository interfaces and keep the in-memory implementation test-friendly.
- Create: `internal/jobs/postgres_repository.go`  
  Responsibility: PostgreSQL-backed job, event, and lease persistence.
- Create: `internal/jobs/postgres_repository_test.go`  
  Responsibility: integration coverage for idempotent create, events, and lease updates.
- Modify: `internal/jobs/service.go`  
  Responsibility: depend on interfaces, append events through the repository, and preserve idempotent creation.
- Modify: `internal/jobs/dispatcher.go`  
  Responsibility: include runtime dispatch payload fields needed by workers.
- Modify: `internal/jobs/handler.go`  
  Responsibility: surface persisted job/event state and reject invalid create requests.
- Modify: `internal/jobs/handler_test.go`  
  Responsibility: keep create/get/event HTTP behavior stable.
- Modify: `internal/jobs/sweeper.go`  
  Responsibility: requeue expired leases, mark exhausted retries failed, append recovery events.
- Create: `internal/jobs/sweeper_test.go`  
  Responsibility: verify retry, recovery event, and worker attribution behavior.

### Worker Contract

- Modify: `workers/common/job_client.py`  
  Responsibility: emit `worker_id` in heartbeat/progress/terminal payloads.
- Modify: `workers/tests/test_job_client.py`  
  Responsibility: assert the heartbeat and terminal payload contract includes worker attribution.
- Modify: `workers/zero_shot/main.py`  
  Responsibility: thread `worker_id` through heartbeat/progress/terminal emission.

### Review + Versioning

- Modify: `internal/review/service.go`  
  Responsibility: preserve pending review semantics, canonical promotion, and audit metadata.
- Modify: `internal/review/handler.go`  
  Responsibility: keep accept/reject request handling minimal and consistent.
- Modify: `internal/review/handler_test.go`  
  Responsibility: verify accept/reject and audit-friendly reviewer propagation.
- Modify: `internal/versioning/service.go`  
  Responsibility: compute `compatibility_score` in addition to existing diff stats.
- Modify: `internal/versioning/handler.go`  
  Responsibility: return the extended diff contract.
- Modify: `internal/versioning/handler_test.go`  
  Responsibility: verify `compatibility_score`, including the empty-to-empty case.

### Artifacts + CLI

- Modify: `internal/artifacts/repository.go`  
  Responsibility: support artifact lookup by `format + version` in addition to `id`.
- Modify: `internal/artifacts/service.go`  
  Responsibility: persist artifact version metadata and resolve artifacts for CLI pull.
- Modify: `internal/artifacts/handler.go`  
  Responsibility: add a minimal artifact resolve endpoint for CLI runtime use.
- Modify: `internal/artifacts/handler_test.go`  
  Responsibility: verify create/get/presign plus `resolve?format=&version=`.
- Modify: `internal/artifacts/packager.go`  
  Responsibility: keep manifest/data.yaml helpers deterministic for CLI verification.
- Create: `internal/cli/api_source.go`  
  Responsibility: HTTP-backed artifact source for `platform-cli pull`.
- Modify: `internal/cli/pull.go`  
  Responsibility: use a real source by default, write `verify-report.json`, and preserve `--allow-partial`.
- Modify: `internal/cli/verify.go`  
  Responsibility: add `environment_context` to verification output.
- Modify: `internal/cli/pull_test.go`  
  Responsibility: verify `environment_context`, partial-failure behavior, and report output.
- Modify: `cmd/platform-cli/main.go`  
  Responsibility: keep the CLI entrypoint stable while using the real source by default.
- Modify: `internal/server/http_server.go`  
  Responsibility: register the minimal artifact resolve route.
- Modify: `internal/server/http_server_routes_test.go`  
  Responsibility: verify the new resolve route is wired.

### Ops + Docs

- Modify: `scripts/dev/smoke.sh`  
  Responsibility: run migration prerequisites and extend smoke checks to scan/items.
- Modify: `docs/development/local-quickstart.md`  
  Responsibility: document migration baseline, MinIO, and smoke behavior.
- Modify: `README.md`  
  Responsibility: keep quick-start and implemented feature descriptions aligned with the final MVP behavior.

---

### Task 1: Lock Schema Management With A Canonical Baseline

**Files:**
- Create: `cmd/migrate/main.go`
- Create: `internal/store/migrate.go`
- Create: `internal/store/migrate_test.go`
- Create: `migrations/000001_init.up.sql`
- Create: `migrations/000001_init.down.sql`
- Delete: `migrations/0001_init.sql`
- Modify: `Makefile`

- [ ] **Step 1: Write the failing migration baseline test**

```go
package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineMigrationFilesExist(t *testing.T) {
	root := filepath.Join("..", "..", "migrations")
	up := filepath.Join(root, "000001_init.up.sql")
	down := filepath.Join(root, "000001_init.down.sql")

	for _, p := range []string{up, down} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected migration file %s: %v", p, err)
		}
	}

	body, err := os.ReadFile(up)
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	if !strings.Contains(string(body), "create table jobs") {
		t.Fatalf("expected jobs table in baseline migration")
	}
}
```

- [ ] **Step 2: Run the store test to verify it fails**

Run: `go test ./internal/store -run TestBaselineMigrationFilesExist -v`  
Expected: FAIL because `000001_init.up.sql` and `000001_init.down.sql` do not exist.

- [ ] **Step 3: Add the `golang-migrate` baseline and runner**

```go
// internal/store/migrate.go
package store

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(databaseURL, sourceURL, command string, forceVersion int) error {
	if databaseURL == "" {
		return errors.New("database url is required")
	}
	if sourceURL == "" {
		sourceURL = "file://migrations"
	}

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	switch command {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
	case "down":
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
	case "version":
		_, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			return nil
		}
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("migration state is dirty")
		}
	case "force":
		return m.Force(forceVersion)
	default:
		return fmt.Errorf("unsupported migration command %q", command)
	}
	return nil
}
```

```go
// cmd/migrate/main.go
package main

import (
	"flag"
	"log"
	"os"

	"yolo-ave-mujica/internal/store"
)

func main() {
	command := flag.String("command", "up", "migration command: up|down|version|force")
	source := flag.String("source", "file://migrations", "migration source url")
	forceVersion := flag.Int("force-version", 0, "version used by force")
	flag.Parse()

	if err := store.RunMigrations(os.Getenv("DATABASE_URL"), *source, *command, *forceVersion); err != nil {
		log.Fatal(err)
	}
}
```

```sql
-- migrations/000001_init.up.sql
create table projects (
  id bigserial primary key,
  name text not null,
  owner text not null,
  created_at timestamptz not null default now()
);

create table datasets (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  name text not null,
  storage_type text not null default 's3',
  bucket text not null,
  prefix text not null,
  created_at timestamptz not null default now()
);

create table dataset_items (
  id bigserial primary key,
  dataset_id bigint not null references datasets(id),
  object_key text not null,
  etag text,
  size bigint,
  width int,
  height int,
  mime text,
  discovered_at timestamptz not null default now(),
  unique(dataset_id, object_key)
);

create table dataset_snapshots (
  id bigserial primary key,
  dataset_id bigint not null references datasets(id),
  version text not null,
  based_on_snapshot_id bigint references dataset_snapshots(id),
  created_by text not null,
  created_at timestamptz not null default now(),
  note text,
  unique(dataset_id, version)
);

create table categories (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  name text not null,
  alias_group text,
  color text,
  unique(project_id, name)
);

create table annotations (
  id bigserial primary key,
  dataset_id bigint not null references datasets(id),
  item_id bigint not null references dataset_items(id),
  category_id bigint not null references categories(id),
  bbox_x double precision not null,
  bbox_y double precision not null,
  bbox_w double precision not null,
  bbox_h double precision not null,
  polygon_json jsonb,
  source text not null default 'manual',
  model_name text,
  created_at_snapshot_id bigint not null references dataset_snapshots(id),
  deleted_at_snapshot_id bigint references dataset_snapshots(id),
  review_status text not null default 'verified',
  is_pseudo boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table annotation_candidates (
  id bigserial primary key,
  job_id bigint,
  dataset_id bigint not null references datasets(id),
  snapshot_id bigint not null references dataset_snapshots(id),
  item_id bigint not null references dataset_items(id),
  category_id bigint not null references categories(id),
  bbox_x double precision not null,
  bbox_y double precision not null,
  bbox_w double precision not null,
  bbox_h double precision not null,
  polygon_json jsonb,
  confidence double precision,
  model_name text,
  is_pseudo boolean not null default true,
  review_status text not null default 'pending',
  reviewer_id text,
  reviewed_at timestamptz,
  created_at timestamptz not null default now()
);

create table annotation_changes (
  id bigserial primary key,
  from_snapshot_id bigint not null references dataset_snapshots(id),
  to_snapshot_id bigint not null references dataset_snapshots(id),
  item_id bigint not null references dataset_items(id),
  change_type text not null,
  before_json jsonb,
  after_json jsonb,
  created_at timestamptz not null default now()
);

create table jobs (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  dataset_id bigint references datasets(id),
  snapshot_id bigint references dataset_snapshots(id),
  job_type text not null,
  status text not null,
  priority text not null default 'normal',
  required_resource_type text not null,
  required_capabilities_json jsonb not null default '[]'::jsonb,
  idempotency_key text not null,
  worker_id text,
  payload_json jsonb not null,
  result_artifact_ids_json jsonb not null default '[]'::jsonb,
  total_items int not null default 0,
  succeeded_items int not null default 0,
  failed_items int not null default 0,
  error_code text,
  error_msg text,
  retry_count int not null default 0,
  lease_until timestamptz,
  created_at timestamptz not null default now(),
  started_at timestamptz,
  finished_at timestamptz,
  constraint jobs_status_check check (status in (
    'queued','running','succeeded','succeeded_with_errors','failed','canceled','retry_waiting'
  )),
  constraint jobs_resource_check check (required_resource_type in ('cpu','gpu','mixed')),
  unique(project_id, job_type, idempotency_key)
);

create table job_events (
  id bigserial primary key,
  job_id bigint not null references jobs(id),
  item_id bigint references dataset_items(id),
  event_level text not null,
  event_type text not null,
  message text not null,
  detail_json jsonb not null default '{}'::jsonb,
  ts timestamptz not null default now()
);

create table artifacts (
  id bigserial primary key,
  project_id bigint not null references projects(id),
  dataset_id bigint not null references datasets(id),
  snapshot_id bigint not null references dataset_snapshots(id),
  artifact_type text not null,
  format text not null,
  version text not null,
  uri text not null,
  checksum text not null,
  size bigint not null,
  manifest_uri text not null,
  label_map_json jsonb not null default '{}'::jsonb,
  status text not null,
  ttl_expire_at timestamptz,
  created_by_job_id bigint references jobs(id),
  created_at timestamptz not null default now()
);

create table audit_logs (
  id bigserial primary key,
  actor text not null,
  action text not null,
  resource_type text not null,
  resource_id text not null,
  detail_json jsonb not null default '{}'::jsonb,
  ts timestamptz not null default now()
);

create index idx_dataset_items_dataset on dataset_items(dataset_id);
create index idx_annotations_interval on annotations(dataset_id, created_at_snapshot_id, deleted_at_snapshot_id);
create index idx_jobs_status_resource on jobs(status, required_resource_type);
create index idx_job_events_job on job_events(job_id);
```

```sql
-- migrations/000001_init.down.sql
drop table if exists audit_logs;
drop table if exists artifacts;
drop table if exists job_events;
drop table if exists jobs;
drop table if exists annotation_changes;
drop table if exists annotation_candidates;
drop table if exists annotations;
drop table if exists categories;
drop table if exists dataset_snapshots;
drop table if exists dataset_items;
drop table if exists datasets;
drop table if exists projects;
```

```make
.PHONY: up-dev down-dev test migrate-up migrate-down migrate-version

migrate-up:
	DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} \
		GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command up

migrate-down:
	DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} \
		GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command down

migrate-version:
	DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} \
		GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command version
```

- [ ] **Step 4: Run the migration and tooling checks**

Run: `go test ./internal/store -v`  
Expected: PASS.

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command version`  
Expected: exits cleanly with either no version yet or the current version, depending on the database state.

Run: `make migrate-up`  
Expected: exits 0 and applies the canonical baseline or reports `no change`.

- [ ] **Step 5: Commit**

```bash
git add cmd/migrate/main.go internal/store/migrate.go internal/store/migrate_test.go migrations/000001_init.up.sql migrations/000001_init.down.sql Makefile
git rm -f migrations/0001_init.sql
git commit -m "chore: baseline schema with golang-migrate"
```

### Task 2: Wire Runtime Bootstrap And Graceful Shutdown

**Files:**
- Modify: `cmd/api-server/main.go`
- Create: `cmd/api-server/main_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/store/postgres.go`
- Modify: `internal/queue/redis.go`
- Modify: `internal/storage/s3.go`

- [ ] **Step 1: Write the failing config and shutdown tests**

```go
package main

import (
	"context"
	"testing"
	"time"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/server"
)

func testConfig() config.Config {
	return config.Config{
		HTTPAddr:        "127.0.0.1:0",
		ShutdownTimeout: 100 * time.Millisecond,
	}
}

func newTestModules() server.Modules {
	return server.Modules{}
}

func TestRunStopsAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := run(ctx, testConfig(), newTestModules())
	if err != nil {
		t.Fatalf("expected canceled startup to shut down cleanly, got %v", err)
	}
}

func TestLoadConfigProvidesRuntimeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://platform:platform@localhost:5432/platform?sslmode=disable")
	t.Setenv("S3_ENDPOINT", "localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "minioadmin")
	t.Setenv("S3_SECRET_KEY", "minioadmin")
	t.Setenv("S3_BUCKET", "platform-dev")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("expected default :8080, got %s", cfg.HTTPAddr)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("expected 10s shutdown timeout, got %s", cfg.ShutdownTimeout)
	}
}
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./cmd/api-server ./internal/config -run 'TestRunStopsAfterContextCancellation|TestLoadConfigProvidesRuntimeDefaults' -v`  
Expected: FAIL because `run`, `testConfig`, and the new config fields do not exist yet.

- [ ] **Step 3: Implement runtime config parsing, dependency bootstrapping, and bounded shutdown**

```go
// internal/config/config.go
type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	RedisAddr       string
	S3Endpoint      string
	S3AccessKey     string
	S3SecretKey     string
	S3Bucket        string
	APIBaseURL      string
	ShutdownTimeout time.Duration
	SweeperInterval time.Duration
	LeaseSeconds    int
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        envOrDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisAddr:       envOrDefault("REDIS_ADDR", "localhost:6379"),
		S3Endpoint:      os.Getenv("S3_ENDPOINT"),
		S3AccessKey:     os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:     os.Getenv("S3_SECRET_KEY"),
		S3Bucket:        os.Getenv("S3_BUCKET"),
		APIBaseURL:      envOrDefault("API_BASE_URL", "http://localhost:8080"),
		ShutdownTimeout: durationOrDefault("SHUTDOWN_TIMEOUT", 10*time.Second),
		SweeperInterval: durationOrDefault("SWEEPER_INTERVAL", 5*time.Second),
		LeaseSeconds:    intOrDefault("LEASE_SECONDS", 30),
	}
	// validate required fields here
	return cfg, nil
}
```

```go
// cmd/api-server/main.go
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	modules, cleanup, err := buildModules(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	if err := run(ctx, cfg, modules); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg config.Config, modules server.Modules) error {
	httpServer := server.NewHTTPServerWithModules(modules)
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: httpServer.Handler}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
```

```go
// internal/store/postgres.go
func NewPostgresPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	poolCfg.MaxConns = 4
	return pgxpool.NewWithConfig(ctx, poolCfg)
}
```

```go
// internal/storage/s3.go
func PresignURLString(client *minio.Client, bucket, objectKey string, ttl time.Duration) (string, error) {
	u, err := PresignGetObject(client, bucket, objectKey, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
```

- [ ] **Step 4: Run the bootstrap tests**

Run: `go test ./cmd/api-server ./internal/config -v`  
Expected: PASS.

Run: `go test ./internal/store ./internal/queue ./internal/storage -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/api-server/main.go cmd/api-server/main_test.go internal/config/config.go internal/config/config_test.go internal/store/postgres.go internal/queue/redis.go internal/storage/s3.go
git commit -m "feat: add runtime bootstrap and graceful shutdown"
```

### Task 3: Add PostgreSQL-Backed Data Hub Runtime Behavior

**Files:**
- Modify: `internal/datahub/repository.go`
- Create: `internal/datahub/postgres_repository.go`
- Create: `internal/datahub/postgres_repository_test.go`
- Modify: `internal/datahub/service.go`
- Modify: `internal/datahub/handler.go`
- Modify: `internal/datahub/handler_test.go`
- Modify: `cmd/api-server/main.go`

- [ ] **Step 1: Write the failing repository round-trip test**

```go
package datahub

import (
	"context"
	"os"
	"testing"

	"yolo-ave-mujica/internal/config"
	"yolo-ave-mujica/internal/store"
)

func TestPostgresRepositoryRoundTripDatasetScanAndSnapshots(t *testing.T) {
	databaseURL := os.Getenv("INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is required")
	}

	cfg := config.Config{DatabaseURL: databaseURL}
	ctx := context.Background()
	pool, err := store.NewPostgresPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)
	ds, err := repo.CreateDataset(ctx, CreateDatasetInput{
		ProjectID: 1,
		Name:      "round-trip",
		Bucket:    "platform-dev",
		Prefix:    "train",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	if _, err := repo.InsertItems(ctx, ds.ID, []string{"train/a.jpg"}); err != nil {
		t.Fatalf("insert items: %v", err)
	}
	items, err := repo.ListItems(ctx, ds.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 || items[0].ObjectKey != "train/a.jpg" {
		t.Fatalf("unexpected items: %+v", items)
	}
}
```

- [ ] **Step 2: Run the Data Hub test to verify it fails**

Run: `go test ./internal/datahub -run TestPostgresRepositoryRoundTripDatasetScanAndSnapshots -v`  
Expected: FAIL because `NewPostgresRepository` does not exist yet.

- [ ] **Step 3: Implement the PostgreSQL repository and MinIO presign wiring**

```go
// internal/datahub/postgres_repository.go
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateDataset(ctx context.Context, in CreateDatasetInput) (Dataset, error) {
	var out Dataset
	err := r.pool.QueryRow(ctx, `
		insert into datasets (project_id, name, bucket, prefix)
		values ($1, $2, $3, $4)
		returning id, project_id, name, bucket, prefix
	`, in.ProjectID, in.Name, in.Bucket, in.Prefix).
		Scan(&out.ID, &out.ProjectID, &out.Name, &out.Bucket, &out.Prefix)
	return out, err
}

func (r *PostgresRepository) InsertItems(ctx context.Context, datasetID int64, objectKeys []string) (int, error) {
	batch := &pgx.Batch{}
	for _, key := range objectKeys {
		batch.Queue(`
			insert into dataset_items (dataset_id, object_key, etag)
			values ($1, $2, md5($2))
			on conflict (dataset_id, object_key) do nothing
		`, datasetID, key)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	added := 0
	for range objectKeys {
		tag, err := results.Exec()
		if err != nil {
			return 0, err
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}
```

```go
// cmd/api-server/main.go (inside buildModules)
pool, err := store.NewPostgresPool(ctx, cfg)
if err != nil {
	return server.Modules{}, nil, err
}
s3Client, err := storage.NewS3Client(cfg)
if err != nil {
	return server.Modules{}, nil, err
}
dataHubRepo := datahub.NewPostgresRepository(pool)
dataHubSvc := datahub.NewServiceWithRepository(func(datasetID int64, objectKey string, ttlSeconds int) (string, error) {
	return storage.PresignURLString(s3Client, cfg.S3Bucket, objectKey, time.Duration(ttlSeconds)*time.Second)
}, dataHubRepo)
dataHubHandler := datahub.NewHandler(dataHubSvc)
```

- [ ] **Step 4: Run the Data Hub verification commands**

Run: `go test ./internal/datahub ./internal/server -v`  
Expected: PASS for unit tests.

Run: `INTEGRATION_DATABASE_URL=postgres://platform:platform@localhost:5432/platform?sslmode=disable go test ./internal/datahub -run TestPostgresRepositoryRoundTripDatasetScanAndSnapshots -v`  
Expected: PASS when local PostgreSQL is up and migrated.

- [ ] **Step 5: Commit**

```bash
git add internal/datahub/repository.go internal/datahub/postgres_repository.go internal/datahub/postgres_repository_test.go internal/datahub/service.go internal/datahub/handler.go internal/datahub/handler_test.go cmd/api-server/main.go
git commit -m "feat: add postgres-backed datahub runtime"
```

### Task 4: Refactor Jobs To Repository Interfaces And Persistent Events

**Files:**
- Modify: `internal/jobs/model.go`
- Modify: `internal/jobs/repository.go`
- Create: `internal/jobs/postgres_repository.go`
- Create: `internal/jobs/postgres_repository_test.go`
- Modify: `internal/jobs/service.go`
- Modify: `internal/jobs/dispatcher.go`
- Modify: `internal/jobs/handler.go`
- Modify: `internal/jobs/handler_test.go`

- [ ] **Step 1: Write the failing jobs persistence tests**

```go
func TestCreateJobAppendsQueuedEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(1, "zero-shot", "gpu", "idem-queued-event", map[string]any{"prompt": "person"})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	events, err := repo.ListEvents(job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 || events[0].EventType != "queued" {
		t.Fatalf("expected queued event, got %+v", events)
	}
}

func TestRepositoryClaimSetsWorkerIDAndLease(t *testing.T) {
	repo := NewInMemoryRepository()
	job, _, err := repo.CreateOrGet(1, "cleaning", "cpu", "idem-claim", map[string]any{"dataset_id": 1})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	claimed, err := repo.Claim(job.ID, "worker-a", time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if claimed.WorkerID != "worker-a" {
		t.Fatalf("expected worker-a, got %s", claimed.WorkerID)
	}
}
```

- [ ] **Step 2: Run the jobs tests to verify they fail**

Run: `go test ./internal/jobs -run 'TestCreateJobAppendsQueuedEvent|TestRepositoryClaimSetsWorkerIDAndLease' -v`  
Expected: FAIL because `ListEvents`, `Claim`, and repository-backed event storage are incomplete.

- [ ] **Step 3: Introduce repository interfaces, persistent events, and `worker_id` on jobs**

```go
// internal/jobs/model.go
type Job struct {
	ID                   int64
	ProjectID            int64
	DatasetID            int64
	SnapshotID           int64
	JobType              string
	Status               string
	RequiredResourceType string
	IdempotencyKey       string
	WorkerID             string
	Payload              map[string]any
	TotalItems           int
	SucceededItems       int
	FailedItems          int
	CreatedAt            time.Time
	StartedAt            *time.Time
	FinishedAt           *time.Time
	LeaseUntil           *time.Time
	RetryCount           int
	ErrorCode            string
	ErrorMsg             string
}
```

```go
// internal/jobs/repository.go
type Repository interface {
	CreateOrGet(projectID int64, jobType, requiredResourceType, key string, payload map[string]any) (*Job, bool, error)
	Get(id int64) (*Job, bool)
	UpdateStatus(id int64, to string) error
	Claim(id int64, workerID string, leaseUntil time.Time) (*Job, error)
	TouchLease(id int64, workerID string, leaseUntil time.Time) error
	ListExpiredRunning(now time.Time) []*Job
	IncrementRetryCount(id int64) error
	MarkRetryWaiting(id int64) error
	MarkFailed(id int64, code, msg string) error
	AppendEvent(jobID int64, itemID *int64, level, typ, message string, detail map[string]any) (Event, error)
	ListEvents(jobID int64) ([]Event, error)
}
```

```go
// internal/jobs/service.go
type Service struct {
	repo       Repository
	dispatcher Publisher
}

func (s *Service) CreateJob(projectID int64, jobType, requiredResourceType, idempotencyKey string, payload map[string]any) (*Job, error) {
	job, created, err := s.repo.CreateOrGet(projectID, jobType, requiredResourceType, idempotencyKey, payload)
	if err != nil {
		return nil, err
	}
	if created {
		if _, err := s.repo.AppendEvent(job.ID, nil, "info", "queued", "job queued", map[string]any{"job_type": jobType}); err != nil {
			return nil, err
		}
		if s.dispatcher != nil {
			if err := s.dispatcher.Publish(context.Background(), laneFor(job.RequiredResourceType), buildDispatchPayload(job)); err != nil {
				return nil, err
			}
		}
	}
	return job, nil
}
```

- [ ] **Step 4: Run the Jobs package tests**

Run: `go test ./internal/jobs -v`  
Expected: PASS.

Run: `INTEGRATION_DATABASE_URL=postgres://platform:platform@localhost:5432/platform?sslmode=disable go test ./internal/jobs -run TestPostgresRepositoryRoundTrip -v`  
Expected: PASS after the PostgreSQL jobs repository is added.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/model.go internal/jobs/repository.go internal/jobs/postgres_repository.go internal/jobs/postgres_repository_test.go internal/jobs/service.go internal/jobs/dispatcher.go internal/jobs/handler.go internal/jobs/handler_test.go
git commit -m "feat: persist jobs events and worker attribution"
```

### Task 5: Complete Lease Heartbeats, Recovery Events, And Worker Contract

**Files:**
- Modify: `internal/jobs/sweeper.go`
- Create: `internal/jobs/sweeper_test.go`
- Modify: `workers/common/job_client.py`
- Modify: `workers/tests/test_job_client.py`
- Modify: `workers/zero_shot/main.py`

- [ ] **Step 1: Write the failing lease recovery and worker payload tests**

```go
func TestLeaseSweeperRequeuesExpiredJobAndAppendsRecoveryEvent(t *testing.T) {
	repo := NewInMemoryRepository()
	pub := NewInMemoryPublisher()
	svc := NewServiceWithPublisher(repo, pub)

	job, err := svc.CreateJob(1, "zero-shot", "gpu", "idem-requeue", map[string]any{"prompt": "person"})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := repo.Claim(job.ID, "worker-a", time.Now().Add(-1*time.Second)); err != nil {
		t.Fatalf("claim: %v", err)
	}

	sw := NewSweeper(repo, pub, 3)
	if err := sw.Tick(time.Now()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	events, err := repo.ListEvents(job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 2 || events[len(events)-1].EventType != "lease_recovered" {
		t.Fatalf("expected lease_recovered event, got %+v", events)
	}
}
```

```python
def test_emit_heartbeat_payload(self):
    payload = emit_heartbeat(job_id=1, worker_id="worker-a", lease_seconds=30)
    self.assertEqual(payload["event_type"], "heartbeat")
    self.assertEqual(payload["detail_json"]["worker_id"], "worker-a")
    self.assertEqual(payload["detail_json"]["lease_seconds"], 30)
```

- [ ] **Step 2: Run the failing tests**

Run: `go test ./internal/jobs -run TestLeaseSweeperRequeuesExpiredJobAndAppendsRecoveryEvent -v`  
Expected: FAIL because sweeper does not append a recovery event with `worker_id`.

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_job_client -v`  
Expected: FAIL because `emit_heartbeat` does not accept `worker_id`.

- [ ] **Step 3: Implement lease recovery events and `worker_id` propagation**

```go
// internal/jobs/sweeper.go
func (s *Sweeper) Tick(now time.Time) error {
	expired := s.repo.ListExpiredRunning(now)
	for _, job := range expired {
		workerID := job.WorkerID
		if job.RetryCount >= s.maxRetries {
			if err := s.repo.MarkFailed(job.ID, "lease_timeout", "retry exhausted"); err != nil {
				return err
			}
			_, _ = s.repo.AppendEvent(job.ID, nil, "error", "lease_timeout", "job failed after lease expiry", map[string]any{"worker_id": workerID})
			continue
		}
		if err := s.repo.MarkRetryWaiting(job.ID); err != nil {
			return err
		}
		if err := s.repo.IncrementRetryCount(job.ID); err != nil {
			return err
		}
		if err := s.repo.UpdateStatus(job.ID, StatusQueued); err != nil {
			return err
		}
		_, _ = s.repo.AppendEvent(job.ID, nil, "warn", "lease_recovered", "lease expired; job requeued", map[string]any{"worker_id": workerID})
		if s.dispatcher != nil {
			if err := s.dispatcher.Publish(context.Background(), laneFor(job.RequiredResourceType), buildDispatchPayload(job)); err != nil {
				return err
			}
		}
	}
	return nil
}
```

```python
# workers/common/job_client.py
def emit_heartbeat(job_id: int, worker_id: str, lease_seconds: int):
    return {
        "job_id": job_id,
        "event_level": "info",
        "event_type": "heartbeat",
        "detail_json": {
            "worker_id": worker_id,
            "lease_seconds": lease_seconds,
        },
    }

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

```python
# workers/zero_shot/main.py
worker_id = os.getenv("WORKER_ID", "zero-shot-local")
heartbeat = emit_heartbeat(job_id=job_id, worker_id=worker_id, lease_seconds=lease_seconds)
terminal = emit_terminal(job_id=job_id, worker_id=worker_id, status=status, total=total, ok=ok, failed=failed)
```

- [ ] **Step 4: Run the Go and Python contract tests**

Run: `go test ./internal/jobs -v`  
Expected: PASS.

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success workers.tests.test_job_client -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/sweeper.go internal/jobs/sweeper_test.go workers/common/job_client.py workers/tests/test_job_client.py workers/zero_shot/main.py
git commit -m "feat: add lease recovery events and worker heartbeat contract"
```

### Task 6: Finish Review Trust Chain And Versioning Compatibility Score

**Files:**
- Modify: `internal/review/service.go`
- Modify: `internal/review/handler.go`
- Modify: `internal/review/handler_test.go`
- Modify: `internal/versioning/service.go`
- Modify: `internal/versioning/handler.go`
- Modify: `internal/versioning/handler_test.go`

- [ ] **Step 1: Write the failing review and versioning tests**

```go
func TestRejectCandidatePreservesReviewMetadata(t *testing.T) {
	svc := NewService()
	svc.SeedCandidate(Candidate{ID: 11, DatasetID: 1, SnapshotID: 1, ItemID: 1, CategoryID: 1, ReviewStatus: "pending"})

	if err := svc.RejectCandidate(11, "reviewer-1"); err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	c, ok := svc.GetCandidate(11)
	if !ok || c.ReviewerID != "reviewer-1" || c.ReviewStatus != "rejected" {
		t.Fatalf("unexpected candidate state: %+v", c)
	}
}

func TestDiffReturnsCompatibilityScore(t *testing.T) {
	out := NewService().DiffSnapshots(nil, nil, 0.5)
	if out.CompatibilityScore != 1 {
		t.Fatalf("expected empty snapshots to be fully compatible, got %f", out.CompatibilityScore)
	}
}
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./internal/review ./internal/versioning -run 'TestRejectCandidatePreservesReviewMetadata|TestDiffReturnsCompatibilityScore' -v`  
Expected: FAIL because `CompatibilityScore` does not exist yet.

- [ ] **Step 3: Add compatibility scoring and tighten review metadata behavior**

```go
// internal/versioning/service.go
type DiffResult struct {
	Adds               []Change  `json:"adds"`
	Removes            []Change  `json:"removes"`
	Updates            []Change  `json:"updates"`
	Stats              DiffStats `json:"stats"`
	CompatibilityScore float64   `json:"compatibility_score"`
}

func compatibilityScore(beforeCount, afterCount, addedCount, removedCount int, updates []Change) float64 {
	baseline := maxInt(beforeCount, afterCount, 1)
	exactMatches := baseline - addedCount - removedCount - len(updates)
	weightedSimilarity := float64(exactMatches)
	for _, update := range updates {
		weightedSimilarity += update.IOU
	}
	return clamp01(weightedSimilarity / float64(baseline))
}

func maxInt(values ...int) int {
	max := values[0]
	for _, value := range values[1:] {
		if value > max {
			max = value
		}
	}
	return max
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// inside the existing DiffSnapshots function, right before `return out`
out.CompatibilityScore = compatibilityScore(len(before), len(after), len(out.Adds), len(out.Removes), out.Updates)
```

```go
// internal/review/service.go
func (s *Service) RejectCandidate(candidateID int64, reviewer string) error {
	// preserve reviewed metadata even when no canonical annotation is created
	now := time.Now().UTC()
	c.ReviewStatus = "rejected"
	c.ReviewerID = reviewer
	c.ReviewedAt = now
	s.candidates[candidateID] = c
	s.audits = append(s.audits, AuditEvent{
		Actor:        reviewer,
		Action:       "review.reject",
		ResourceType: "annotation_candidate",
		ResourceID:   fmt.Sprintf("%d", candidateID),
		TS:           now,
	})
	return nil
}
```

- [ ] **Step 4: Run the review and versioning tests**

Run: `go test ./internal/review ./internal/versioning -v`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/review/service.go internal/review/handler.go internal/review/handler_test.go internal/versioning/service.go internal/versioning/handler.go internal/versioning/handler_test.go
git commit -m "feat: finalize review metadata and diff compatibility scoring"
```

### Task 7: Add Artifact Resolution And A Real CLI Source

**Files:**
- Modify: `internal/artifacts/repository.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/artifacts/handler.go`
- Modify: `internal/artifacts/handler_test.go`
- Modify: `internal/artifacts/packager.go`
- Create: `internal/cli/api_source.go`
- Modify: `internal/cli/pull.go`
- Modify: `internal/cli/verify.go`
- Modify: `internal/cli/pull_test.go`
- Modify: `cmd/platform-cli/main.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/server/http_server_routes_test.go`

- [ ] **Step 1: Write the failing artifact resolve and CLI report tests**

```go
func TestResolveArtifactByFormatAndVersion(t *testing.T) {
	svc := NewService()
	_, artifactID, err := svc.CreatePackageJob(PackageRequest{
		DatasetID:  1,
		SnapshotID: 2,
		Format:     "yolo",
		Version:    "v1",
	})
	if err != nil {
		t.Fatalf("create package job: %v", err)
	}

	artifact, err := svc.ResolveArtifact("yolo", "v1")
	if err != nil {
		t.Fatalf("resolve artifact: %v", err)
	}
	if artifact.ID != artifactID {
		t.Fatalf("expected artifact %d, got %d", artifactID, artifact.ID)
	}
}

func TestPullWritesEnvironmentContext(t *testing.T) {
	dir := t.TempDir()
	client := NewPullClientWithSource(dir, fakeSource{
		pkg: PulledArtifact{
			ArtifactID: 1,
			Version:    "v1",
			Entries: []ArtifactEntry{{
				Path:     "labels/0001.txt",
				Body:     []byte("0 0.5 0.5 0.2 0.2\n"),
				Checksum: "fe1d19931e4f3092800a55299efc6f6e0b806bed3838aa14aebbc94ba55aa549",
			}},
		},
	})

	if err := client.Pull(PullOptions{Format: "yolo", Version: "v1", OutputDir: dir}); err != nil {
		t.Fatalf("pull: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "verify-report.json"))
	if err != nil {
		t.Fatalf("read verify report: %v", err)
	}
	var report VerifyReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report.EnvironmentContext.OS == "" || report.EnvironmentContext.StorageDriver == "" {
		t.Fatalf("missing environment context: %+v", report.EnvironmentContext)
	}
}
```

- [ ] **Step 2: Run the artifact and CLI tests to verify they fail**

Run: `go test ./internal/artifacts ./internal/cli ./internal/server -run 'TestResolveArtifactByFormatAndVersion|TestPullWritesEnvironmentContext|TestMVPRoutesAreRegistered' -v`  
Expected: FAIL because artifact resolution and `EnvironmentContext` do not exist yet.

- [ ] **Step 3: Add minimal artifact resolution and an HTTP-backed CLI source**

```go
// internal/artifacts/service.go
type PackageRequest struct {
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
}

type Artifact struct {
	ID           int64             `json:"id"`
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	URI          string            `json:"uri"`
	ManifestURI  string            `json:"manifest_uri"`
	Checksum     string            `json:"checksum"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
	Status       string            `json:"status"`
	CreatedAt    time.Time         `json:"created_at"`
}

func (s *Service) ResolveArtifact(format, version string) (Artifact, error) {
	a, ok := s.repo.FindByFormatVersion(format, version)
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %s@%s not found", format, version)
	}
	return a, nil
}
```

```go
// internal/artifacts/repository.go
type Repository interface {
	Create(a Artifact) (Artifact, error)
	Get(id int64) (Artifact, bool)
	FindByFormatVersion(format, version string) (Artifact, bool)
}

func (r *InMemoryRepository) FindByFormatVersion(format, version string) (Artifact, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, artifact := range r.byID {
		if artifact.Format == format && artifact.Version == version {
			return artifact, true
		}
	}
	return Artifact{}, false
}
```

```go
// internal/artifacts/handler.go
func (h *Handler) ResolveArtifact(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	version := r.URL.Query().Get("version")
	if format == "" || version == "" {
		writeError(w, http.StatusBadRequest, errors.New("format and version are required"))
		return
	}
	a, err := h.svc.ResolveArtifact(format, version)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}
```

```go
// internal/server/http_server.go
type ArtifactRoutes struct {
	CreatePackage   http.HandlerFunc
	GetArtifact     http.HandlerFunc
	PresignArtifact http.HandlerFunc
	ResolveArtifact http.HandlerFunc
}

// inside NewHTTPServerWithModules
r.Get("/artifacts/resolve", handlerOrNotImplemented(m.Artifacts.ResolveArtifact))
```

```go
// internal/cli/verify.go
type EnvironmentContext struct {
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CLIVersion    string `json:"cli_version"`
	StorageDriver string `json:"storage_driver"`
}

type VerifyReport struct {
	ArtifactID          int64              `json:"artifact_id"`
	Snapshot            string             `json:"snapshot"`
	TotalFiles          int                `json:"total_files"`
	FailedFiles         int                `json:"failed_files"`
	VerifiedAt          string             `json:"verified_at"`
	EnvironmentContext  EnvironmentContext `json:"environment_context"`
}
```

```go
// internal/cli/api_source.go
type APIArtifactSource struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (s *APIArtifactSource) FetchArtifact(format, version string) (PulledArtifact, error) {
	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resolveURL := fmt.Sprintf("%s/v1/artifacts/resolve?format=%s&version=%s",
		strings.TrimRight(s.BaseURL, "/"),
		url.QueryEscape(format),
		url.QueryEscape(version),
	)

	var artifact struct {
		ID          int64  `json:"id"`
		Version     string `json:"version"`
		URI         string `json:"uri"`
		ManifestURI string `json:"manifest_uri"`
	}
	if err := fetchJSON(client, resolveURL, &artifact); err != nil {
		return PulledArtifact{}, err
	}

	var manifest artifacts.Manifest
	if err := fetchJSON(client, artifact.ManifestURI, &manifest); err != nil {
		return PulledArtifact{}, err
	}

	entries := make([]ArtifactEntry, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		fileURL := fmt.Sprintf("%s/%s", strings.TrimRight(artifact.URI, "/"), entry.Path)
		body, err := fetchBytes(client, fileURL)
		if err != nil {
			return PulledArtifact{}, err
		}
		entries = append(entries, ArtifactEntry{
			Path:     entry.Path,
			Body:     body,
			Checksum: entry.Checksum,
		})
	}

	return PulledArtifact{
		ArtifactID: artifact.ID,
		Version:    artifact.Version,
		Entries:    entries,
	}, nil
}

func fetchJSON(client *http.Client, url string, out any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func fetchBytes(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}
```

```go
// internal/server/http_server.go
r.Get("/artifacts/resolve", handlerOrNotImplemented(m.Artifacts.ResolveArtifact))
```

- [ ] **Step 4: Run artifact, CLI, and route tests**

Run: `go test ./internal/artifacts ./internal/cli ./internal/server -v`  
Expected: PASS.

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/platform-cli --help`  
Expected: includes `pull`, `--format`, `--version`, and `--allow-partial`.

- [ ] **Step 5: Commit**

```bash
git add internal/artifacts/repository.go internal/artifacts/service.go internal/artifacts/handler.go internal/artifacts/handler_test.go internal/artifacts/packager.go internal/cli/api_source.go internal/cli/pull.go internal/cli/verify.go internal/cli/pull_test.go cmd/platform-cli/main.go internal/server/http_server.go internal/server/http_server_routes_test.go
git commit -m "feat: add artifact resolution and real cli source"
```

### Task 8: Extend Smoke, Quickstart, And Final Verification

**Files:**
- Modify: `scripts/dev/smoke.sh`
- Modify: `docs/development/local-quickstart.md`
- Modify: `README.md`
- Modify: `Makefile`

- [ ] **Step 1: Add failing smoke assertions for scan and item listing**

```bash
scan_response="$(curl -fsS -X POST http://localhost:8080/v1/datasets/${dataset_id}/scan \
  -H 'Content-Type: application/json' \
  -d '{"object_keys":["train/a.jpg","train/b.jpg"]}')"

items_response="$(curl -fsS http://localhost:8080/v1/datasets/${dataset_id}/items)"

if [[ "$scan_response" != *"indexed"* ]]; then
  fail "scan response missing indexed count: $scan_response"
fi
if [[ "$items_response" != *"train/a.jpg"* ]]; then
  fail "items response missing scanned key: $items_response"
fi
```

- [ ] **Step 2: Run smoke to verify it fails first**

Run: `bash scripts/dev/smoke.sh`  
Expected: FAIL because the current script does not exercise scan/items or migration prerequisites.

- [ ] **Step 3: Update smoke, docs, and repo-level verification instructions**

```bash
# scripts/dev/smoke.sh
export DATABASE_URL="${DATABASE_URL:-postgres://platform:platform@localhost:5432/platform?sslmode=disable}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
export S3_ENDPOINT="${S3_ENDPOINT:-localhost:9000}"
export S3_ACCESS_KEY="${S3_ACCESS_KEY:-minioadmin}"
export S3_SECRET_KEY="${S3_SECRET_KEY:-minioadmin}"
export S3_BUCKET="${S3_BUCKET:-platform-dev}"

if command -v docker >/dev/null 2>&1; then
  make up-dev >/dev/null
fi
make migrate-up >/dev/null

scan_response="$(curl -fsS -X POST http://localhost:8080/v1/datasets/${dataset_id}/scan \
  -H 'Content-Type: application/json' \
  -d '{"object_keys":["train/a.jpg","train/b.jpg"]}')"
items_response="$(curl -fsS http://localhost:8080/v1/datasets/${dataset_id}/items)"
```

```md
<!-- docs/development/local-quickstart.md -->
1. Start local dependencies with `make up-dev`.
2. Apply the canonical baseline with `make migrate-up`.
3. Export MinIO and PostgreSQL env vars if you are not using defaults.
4. Run `make test`.
5. Run `bash scripts/dev/smoke.sh`.
```

```md
<!-- README.md -->
- `make up-dev` starts PostgreSQL, Redis, and MinIO.
- `make migrate-up` applies the canonical baseline schema.
- `bash scripts/dev/smoke.sh` now checks dataset create, scan, items, presign, and async job creation.
```

- [ ] **Step 4: Run the full verification suite**

Run: `make test`  
Expected: PASS for Go and Python unit tests.

Run: `bash scripts/dev/smoke.sh`  
Expected: PASS against the local PostgreSQL, Redis, and MinIO-backed runtime.

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add scripts/dev/smoke.sh docs/development/local-quickstart.md README.md Makefile
git commit -m "chore: finalize smoke coverage and developer workflow"
```

## Self-Review

### Spec Coverage

1. Schema control and baseline migration: Task 1.
2. API lifecycle and graceful shutdown: Task 2.
3. PostgreSQL-backed Data Hub runtime plus MinIO presign: Task 3.
4. Jobs idempotency, lane dispatch, worker attribution, lease recovery: Tasks 4 and 5.
5. Review trust chain and diff compatibility score: Task 6.
6. Artifact resolution, CLI verification report, and environment context: Task 7.
7. MinIO-backed smoke and updated docs: Task 8.

### Placeholder Scan

1. No placeholder markers or deferred implementation steps remain.
2. The only deliberate surface addition beyond the approved route list is artifact resolution for CLI pull; it is explicitly scoped in Task 7.

### Type Consistency

1. `worker_id` appears in job model, sweeper events, and worker heartbeat payloads.
2. `compatibility_score` is produced in the versioning service and asserted in versioning tests.
3. `EnvironmentContext` is written in CLI verification output and checked by CLI tests.
