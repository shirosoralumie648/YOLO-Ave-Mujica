# Documentation And Comments Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a maintainable documentation and comment pass that gives the repository a complete Simplified Chinese onboarding path, improves English entry-point navigation, and documents the core control-plane and worker flows inline.

**Architecture:** The work is split into five isolated deliverables: repository entry docs, development runbooks, Chinese architecture documentation, focused Go comments, and focused Python docstrings. Each task updates a small, well-defined file set and ends with targeted verification so the documentation stays aligned with the current MVP code rather than older planning docs.

**Tech Stack:** Markdown, Go, Python 3, `gofmt`, `go test`, `python3 -m unittest`, `rg`

---

### Task 1: Refresh Repository Entry Documentation

**Files:**
- Modify: `README.md`
- Create: `README.zh-CN.md`

- [ ] **Step 1: Replace `README.md` with a navigational English entry point**

```markdown
# YOLO-Ave-Mujica

A production-oriented MVP foundation for dataset indexing, annotation workflow orchestration, and training artifact delivery.

## Documentation

- English local quickstart: `docs/development/local-quickstart.md`
- 简体中文总览: `README.zh-CN.md`
- 简体中文本地开发: `docs/development/local-quickstart.zh-CN.md`
- 简体中文架构说明: `docs/development/architecture.zh-CN.md`

## Current MVP Scope

- Go control plane entry points: `api-server`, `platform-cli`
- Data Hub APIs for dataset creation, scans, snapshots, item listing, and object presign
- Job primitives for idempotent create, lane dispatch, lease recovery, and event listing
- Artifact packaging, resolve, archive download, and CLI pull verification
- Python worker-side primitives for heartbeats, partial success accounting, and cleaning rules

## Repository Layout

```text
cmd/                Entry points for api-server, platform-cli, migration, and local helpers
internal/           Go domain modules and runtime wiring
workers/            Python worker-side helper primitives and tests
migrations/         SQL schema bootstrap
deploy/docker/      Local PostgreSQL, Redis, and MinIO compose stack
scripts/dev/        Local smoke checks and helper scripts
docs/               Development docs, specs, and plans
```

## Quick Start

```bash
make up-dev
export DATABASE_URL=postgres://platform:platform@localhost:5432/platform?sslmode=disable
export REDIS_ADDR=localhost:6379
export S3_ENDPOINT=localhost:9000
export S3_ACCESS_KEY=minioadmin
export S3_SECRET_KEY=minioadmin
export S3_BUCKET=platform-dev
export ARTIFACT_STORAGE_DIR=/tmp/platform-artifacts
export ARTIFACT_BUILD_CONCURRENCY=2
make migrate-up
make test
bash scripts/dev/smoke.sh
```

See `docs/development/local-quickstart.md` for the detailed local runbook.

## Implemented API Surface

- `POST /v1/datasets`
- `POST /v1/datasets/{id}/scan`
- `POST /v1/datasets/{id}/snapshots`
- `GET /v1/datasets/{id}/snapshots`
- `GET /v1/datasets/{id}/items`
- `POST /v1/objects/presign`
- `POST /v1/jobs/zero-shot`
- `POST /v1/jobs/video-extract`
- `POST /v1/jobs/cleaning`
- `GET /v1/jobs/{job_id}`
- `GET /v1/jobs/{job_id}/events`
- `POST /v1/snapshots/diff`
- `GET /v1/review/candidates`
- `POST /v1/review/candidates/{id}/accept`
- `POST /v1/review/candidates/{id}/reject`
- `POST /v1/artifacts/packages`
- `GET /v1/artifacts/resolve`
- `GET /v1/artifacts/{id}`
- `GET /v1/artifacts/{id}/download`
- `POST /v1/artifacts/{id}/presign`
- `GET /healthz`
- `GET /readyz`

## Testing

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
PYTHONPATH=. python3 -m unittest \
  workers.tests.test_partial_success \
  workers.tests.test_job_client \
  workers.tests.test_cleaning_rules -v
```
```

- [ ] **Step 2: Create `README.zh-CN.md` with a complete Simplified Chinese onboarding overview**

```markdown
# YOLO-Ave-Mujica

一个面向生产场景的 MVP 仓库，用于验证数据集索引、标注工作流编排，以及训练产物导出与交付链路。

## 当前仓库包含什么

- Go 控制面入口：`api-server`、`platform-cli`
- Data Hub 能力：数据集创建、扫描、快照、条目列表、对象预签名
- Jobs 能力：任务创建、幂等、分发、租约恢复、事件查询
- Artifacts 能力：导出包构建、状态查询、版本解析、下载与校验
- Python worker 原语：心跳、部分成功统计、清洗规则判断

## 文档导航

- 英文入口：`README.md`
- 英文本地开发：`docs/development/local-quickstart.md`
- 中文本地开发：`docs/development/local-quickstart.zh-CN.md`
- 中文架构说明：`docs/development/architecture.zh-CN.md`

## 目录结构

```text
cmd/                可执行入口，如 api-server、platform-cli、migrate
internal/           Go 领域模块和服务装配代码
workers/            Python worker 侧辅助逻辑与测试
migrations/         数据库迁移脚本
deploy/docker/      本地开发依赖的 Docker Compose 配置
scripts/dev/        smoke 检查和开发脚本
docs/               开发文档、设计文档、实施计划
```

## 本地快速开始

```bash
make up-dev
export DATABASE_URL=postgres://platform:platform@localhost:5432/platform?sslmode=disable
export REDIS_ADDR=localhost:6379
export S3_ENDPOINT=localhost:9000
export S3_ACCESS_KEY=minioadmin
export S3_SECRET_KEY=minioadmin
export S3_BUCKET=platform-dev
export ARTIFACT_STORAGE_DIR=/tmp/platform-artifacts
export ARTIFACT_BUILD_CONCURRENCY=2
make migrate-up
make test
bash scripts/dev/smoke.sh
```

更完整的步骤、环境要求和常见说明见 `docs/development/local-quickstart.zh-CN.md`。

## 当前 API 能力

- `POST /v1/datasets`
- `POST /v1/datasets/{id}/scan`
- `POST /v1/datasets/{id}/snapshots`
- `GET /v1/datasets/{id}/snapshots`
- `GET /v1/datasets/{id}/items`
- `POST /v1/objects/presign`
- `POST /v1/jobs/zero-shot`
- `POST /v1/jobs/video-extract`
- `POST /v1/jobs/cleaning`
- `GET /v1/jobs/{job_id}`
- `GET /v1/jobs/{job_id}/events`
- `POST /v1/snapshots/diff`
- `GET /v1/review/candidates`
- `POST /v1/review/candidates/{id}/accept`
- `POST /v1/review/candidates/{id}/reject`
- `POST /v1/artifacts/packages`
- `GET /v1/artifacts/resolve`
- `GET /v1/artifacts/{id}`
- `GET /v1/artifacts/{id}/download`
- `POST /v1/artifacts/{id}/presign`

## CLI 与验证产物

- `platform-cli pull --format <format> --version <version>` 会解析可用 artifact、下载归档、解压内容并校验 `manifest.json`
- 命令完成后会在输出目录写入 `verify-report.json`
- 报告中的 `environment_context` 会记录 `os`、`arch`、`cli_version` 和 `storage_driver`

## 测试命令

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
PYTHONPATH=. python3 -m unittest \
  workers.tests.test_partial_success \
  workers.tests.test_job_client \
  workers.tests.test_cleaning_rules -v
```
```

- [ ] **Step 3: Review the two repository entry docs for consistency**

Run: `rg -n "README.zh-CN|local-quickstart|architecture.zh-CN|platform-cli pull|verify-report.json" README.md README.zh-CN.md`
Expected: both files contain the intended doc links and artifact CLI wording

- [ ] **Step 4: Commit the repository entry docs**

```bash
git add README.md README.zh-CN.md
git commit -m "docs: add bilingual repository entry docs"
```

### Task 2: Expand Local Development Runbooks

**Files:**
- Modify: `docs/development/local-quickstart.md`
- Create: `docs/development/local-quickstart.zh-CN.md`

- [ ] **Step 1: Rewrite the English quickstart as an operational runbook**

```markdown
# Local Quickstart

## Prerequisites

- Go 1.20+
- Python 3.12+
- Docker for the default local PostgreSQL, Redis, and MinIO stack
- Or equivalent local services already running on `5432`, `6379`, and `9000`

## Start Local Dependencies

```bash
make up-dev
export DATABASE_URL=postgres://platform:platform@localhost:5432/platform?sslmode=disable
export REDIS_ADDR=localhost:6379
export S3_ENDPOINT=localhost:9000
export S3_ACCESS_KEY=minioadmin
export S3_SECRET_KEY=minioadmin
export S3_BUCKET=platform-dev
export ARTIFACT_STORAGE_DIR=/tmp/platform-artifacts
export ARTIFACT_BUILD_CONCURRENCY=2
make migrate-up
```

`make up-dev` starts the Docker-backed dependency stack and also bootstraps the default MinIO bucket (`platform-dev`). If Docker is unavailable, you need PostgreSQL, Redis, and a MinIO-compatible endpoint running locally before the API server or smoke script can succeed.

## Run Tests

```bash
make test
```

This runs the full Go test suite and the worker unit tests:

- `workers.tests.test_partial_success`
- `workers.tests.test_job_client`
- `workers.tests.test_cleaning_rules`

## Run Smoke Checks

```bash
bash scripts/dev/smoke.sh
```

The smoke flow verifies:

- `/healthz`
- `/readyz`
- dataset creation
- dataset scan
- dataset item listing
- object presign response shape
- zero-shot job creation response shape
- artifact package build polling
- `platform-cli pull` archive download, extraction, and verification

`/readyz` checks PostgreSQL, Redis, and MinIO endpoint access with the configured credentials. A `503` means the API process is alive but one or more runtime dependencies are still unavailable.

If the API is not already running, the smoke script starts a temporary local API process after verifying that PostgreSQL, Redis, MinIO, and the baseline migration are available. In tightly sandboxed environments, binding `:8080` may still require elevated permissions.

`ARTIFACT_STORAGE_DIR` defaults to `/tmp/platform-artifacts`, and `ARTIFACT_BUILD_CONCURRENCY` defaults to `2`.

`platform-cli pull` writes `verify-report.json`, including an `environment_context` block with `os`, `arch`, `cli_version`, and `storage_driver`.

## Stop Local Dependencies

```bash
make down-dev
```
```

- [ ] **Step 2: Create the full Simplified Chinese runbook**

```markdown
# 本地开发快速开始

## 前置条件

- Go 1.20+
- Python 3.12+
- Docker，用于启动默认的 PostgreSQL、Redis、MinIO 本地依赖
- 如果不使用 Docker，需要自行在本机提供 `5432`、`6379`、`9000` 对应的兼容服务

## 启动本地依赖

```bash
make up-dev
export DATABASE_URL=postgres://platform:platform@localhost:5432/platform?sslmode=disable
export REDIS_ADDR=localhost:6379
export S3_ENDPOINT=localhost:9000
export S3_ACCESS_KEY=minioadmin
export S3_SECRET_KEY=minioadmin
export S3_BUCKET=platform-dev
export ARTIFACT_STORAGE_DIR=/tmp/platform-artifacts
export ARTIFACT_BUILD_CONCURRENCY=2
make migrate-up
```

`make up-dev` 会启动 Docker 依赖栈，并自动创建默认的 MinIO bucket `platform-dev`。如果本机没有 Docker，这一步会被跳过，此时需要你自行准备 PostgreSQL、Redis 和兼容 S3 的对象存储服务，否则 API 和 smoke 脚本都无法通过。

## 运行测试

```bash
make test
```

该命令会执行：

- 全量 Go 单元测试
- `workers.tests.test_partial_success`
- `workers.tests.test_job_client`
- `workers.tests.test_cleaning_rules`

## 运行 Smoke 检查

```bash
bash scripts/dev/smoke.sh
```

Smoke 脚本会验证以下链路：

- `/healthz`
- `/readyz`
- 创建数据集
- 扫描数据集对象
- 查询数据集条目
- 校验对象预签名返回结构
- 校验 zero-shot 任务创建返回结构
- 轮询 artifact 导出包构建状态
- 执行 `platform-cli pull` 下载、解压和校验导出包

其中 `/readyz` 会检查 PostgreSQL、Redis 和 MinIO 访问是否可用。如果返回 `503`，说明进程本身还活着，但至少有一个运行时依赖尚未就绪。

如果当前没有 API 进程在运行，脚本会先检查 PostgreSQL、Redis、MinIO 以及基础迁移是否已经准备好，然后临时启动一个本地 API 进程。在受限较强的环境里，即使只是本地 smoke，也可能因为绑定 `:8080` 而需要额外权限。

`ARTIFACT_STORAGE_DIR` 默认是 `/tmp/platform-artifacts`，用于保存导出包的原子落盘目录和归档文件。`ARTIFACT_BUILD_CONCURRENCY` 默认值是 `2`，用于限制进程内同时执行的 artifact 构建数。

`platform-cli pull` 会在输出目录生成 `verify-report.json`，其中 `environment_context` 会记录 `os`、`arch`、`cli_version` 和 `storage_driver`，便于排查不同机器上的本地校验差异。

## 停止本地依赖

```bash
make down-dev
```
```

- [ ] **Step 3: Verify the runbooks mention the same startup variables and smoke scope**

Run: `rg -n "ARTIFACT_STORAGE_DIR|ARTIFACT_BUILD_CONCURRENCY|verify-report.json|/readyz|make up-dev|make down-dev" docs/development/local-quickstart.md docs/development/local-quickstart.zh-CN.md`
Expected: both runbooks mention the same runtime variables and smoke responsibilities

- [ ] **Step 4: Commit the runbook updates**

```bash
git add docs/development/local-quickstart.md docs/development/local-quickstart.zh-CN.md
git commit -m "docs: add Chinese local quickstart and refine English runbook"
```

### Task 3: Add A Chinese Architecture Guide

**Files:**
- Create: `docs/development/architecture.zh-CN.md`

- [ ] **Step 1: Create the architecture overview with repository roles and reading order**

```markdown
# 项目架构说明

## 仓库定位

这个仓库实现的是一个面向数据集处理链路的 MVP 控制面，当前重点是把数据集注册、任务调度、导出构建与 CLI 交付这几条主链路打通，而不是一次性做完完整平台。

## 建议阅读顺序

1. `README.zh-CN.md`
2. `docs/development/local-quickstart.zh-CN.md`
3. `cmd/api-server/main.go`
4. `internal/server/http_server.go`
5. `internal/datahub/`
6. `internal/jobs/`
7. `internal/artifacts/`
8. `internal/cli/pull.go`
9. `workers/`

## 目录职责

- `cmd/`：应用入口和本地工具入口
- `internal/`：Go 领域模块和运行时装配
- `workers/`：Python worker 侧辅助逻辑
- `scripts/dev/`：本地 smoke 和开发辅助脚本
- `deploy/docker/`：本地依赖栈
```

- [ ] **Step 2: Add the `api-server`, Data Hub, and Jobs flow sections**

```markdown
## `api-server` 启动与模块装配

`cmd/api-server/main.go` 负责读取配置、初始化 PostgreSQL、Redis、MinIO 相关依赖，并把 Data Hub、Jobs、Versioning、Review、Artifacts 这些模块组装成 `server.Modules`，最后交给 `internal/server/http_server.go` 注册成 HTTP 路由。

除了 HTTP 服务本身，启动过程还会做两件后台工作：

- 启动 job sweeper 定时检查过期租约，做恢复或重试
- 启动 artifact build runner，异步消费待构建的导出任务

## Data Hub 请求流

Data Hub 负责数据集及其快照相关的基础能力，包括：

- 创建数据集
- 扫描对象键并登记条目
- 创建和列出快照
- 为对象读取生成短期预签名 URL

在 MVP 阶段，Service 层对外暴露的输入输出模型已经稳定，底层既可以接内存仓储，也可以接 PostgreSQL 仓储，因此文档和 handler 都围绕稳定的 service contract 编排。

## Jobs 创建、分发与恢复

`internal/jobs/service.go` 负责创建任务、保证幂等、写入初始事件，并在配置了 dispatcher 时把任务投递到与资源类型对应的 lane。

相关职责分层如下：

- Service：校验输入、调用仓储、决定是否投递
- Repository：持久化任务、事件、租约和状态
- Publisher：把任务放入 Redis 等分发通道
- Sweeper：检查租约超时任务，必要时重新排队
```

- [ ] **Step 3: Add the Artifacts, CLI pull, and worker sections**

```markdown
## Artifacts 构建与交付流

Artifacts 模块负责把某个数据集快照导出为可下载的训练包。主链路如下：

1. 客户端调用 `POST /v1/artifacts/packages` 创建导出请求
2. Service 在仓储中创建一条 `pending` artifact 记录
3. Build runner 异步取出 artifact ID 并进入 `building`
4. Export query 从数据库组装快照导出所需的数据
5. Builder 生成目录结构、标签文件、`data.yaml`、`manifest.json` 和归档文件
6. Filesystem storage 把构建结果原子写入本地存储目录
7. Repository 更新 artifact 状态为 `ready`，并写回 URI、checksum、size 等元数据

只有状态已经变为 `ready` 的 artifact，才能继续被 resolve、presign 或 download。

## `platform-cli pull` 下载与校验流

`internal/cli/pull.go` 负责实现本地拉取导出包的主流程：

1. 解析 `--format` 和 `--version`
2. 调用 artifact source 解析可用 artifact
3. 下载归档到临时文件
4. 解压到 `pulled-<version>` 目录
5. 读取 `manifest.json`
6. 对每个文件做 SHA-256 校验
7. 输出 `verify-report.json`

报告里的 `environment_context` 用于记录当前操作系统、CPU 架构、CLI 版本和底层存储驱动，方便排查机器差异。

## Python worker 原语

`workers/common/job_client.py` 提供的是事件 payload 生成原语，不直接负责真正的网络上报。它的职责是统一不同 worker 发出的事件结构，例如：

- item 级错误事件
- 心跳事件
- 进度事件
- 终态统计

`workers/zero_shot/main.py` 负责根据 `ok/failed` 统计汇总终态；`workers/cleaning/main.py` 则实现清洗规则判断，输出问题摘要、命中列表和可移除候选项。

这些模块都属于 MVP 阶段的 worker 侧基础积木，重点是保证事件结构和规则结果稳定可测。
```

- [ ] **Step 4: Verify the architecture guide references only implemented flows**

Run: `rg -n "build runner|verify-report.json|sweeper|POST /v1/artifacts/packages|platform-cli pull|workers/common/job_client.py" docs/development/architecture.zh-CN.md`
Expected: the guide references only flows and files that exist in the repository

- [ ] **Step 5: Commit the architecture guide**

```bash
git add docs/development/architecture.zh-CN.md
git commit -m "docs: add Chinese architecture guide"
```

### Task 4: Add Focused Go Doc Comments

**Files:**
- Modify: `cmd/api-server/main.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/datahub/service.go`
- Modify: `internal/jobs/model.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/cli/pull.go`

- [ ] **Step 1: Add orchestration comments to `cmd/api-server/main.go`**

```diff
func buildModules(ctx context.Context, cfg config.Config) (server.Modules, func(), func(time.Time) error, error) {
+	// Build shared infrastructure dependencies first because the control-plane
+	// modules all depend on PostgreSQL, Redis, or object storage clients.
	pool, err := store.NewPostgresPool(ctx, cfg)
+	// Recover unfinished artifact builds before accepting traffic so stale
+	// "building" rows do not survive an unclean restart indefinitely.
	if _, err := artifactService.MarkStaleBuildsFailed(ctx, "startup_recovery"); err != nil {
+		_ = redisClient.Close()
+		pool.Close()
+		return server.Modules{}, nil, nil, err
	}
+	// Start the in-process build runner only after recovery has completed.
	artifactService.StartBuildRunner(ctx, cfg.ArtifactBuildConcurrency)
}

func startBackgroundLoop(ctx context.Context, interval time.Duration, tick func(time.Time) error) {
+	// The shared loop currently drives job sweeping, but the helper stays
+	// generic so other lightweight periodic runtime tasks can reuse it.
}
```

- [ ] **Step 2: Add exported symbol comments to `internal/server/http_server.go`**

```diff
// HTTPServer owns the root handler for the control-plane HTTP surface.
type HTTPServer struct {
	Handler http.Handler
}

// ReadyCheck reports whether a required runtime dependency is ready to serve traffic.
type ReadyCheck func(ctx context.Context) error

// DataHubRoutes groups handlers for dataset and object-management endpoints.
type DataHubRoutes struct {
	CreateDataset  http.HandlerFunc
	ScanDataset    http.HandlerFunc
	CreateSnapshot http.HandlerFunc
	ListSnapshots  http.HandlerFunc
	ListItems      http.HandlerFunc
	PresignObject  http.HandlerFunc
}

// JobRoutes groups handlers for asynchronous job creation and inspection endpoints.
type JobRoutes struct {
	CreateZeroShot     http.HandlerFunc
	CreateVideoExtract http.HandlerFunc
	CreateCleaning     http.HandlerFunc
	GetJob             http.HandlerFunc
	ListEvents         http.HandlerFunc
}

// VersioningRoutes groups handlers for snapshot diff and compatibility endpoints.
type VersioningRoutes struct {
	DiffSnapshots http.HandlerFunc
}

// ReviewRoutes groups handlers for review candidate listing and decisions.
type ReviewRoutes struct {
	ListCandidates  http.HandlerFunc
	AcceptCandidate http.HandlerFunc
	RejectCandidate http.HandlerFunc
}

// ArtifactRoutes groups handlers for artifact creation, resolution, and download.
type ArtifactRoutes struct {
	CreatePackage    http.HandlerFunc
	GetArtifact      http.HandlerFunc
	PresignArtifact  http.HandlerFunc
	ResolveArtifact  http.HandlerFunc
	DownloadArtifact http.HandlerFunc
}

// Modules collects optional route groups so the server can expose a stable
// MVP route surface even while some handlers are still being implemented.
type Modules struct {
	DataHub     DataHubRoutes
	Jobs        JobRoutes
	Versioning  VersioningRoutes
	Review      ReviewRoutes
	Artifacts   ArtifactRoutes
	ReadyChecks []ReadyCheck
}

// NewHTTPServerWithModules wires all MVP route groups.
// Handlers left unset return 501 so clients can rely on route shape before
// every module is fully delivered.
func NewHTTPServerWithModules(m Modules) *HTTPServer {
}
```

- [ ] **Step 3: Add model and service comments to `internal/datahub/service.go` and `internal/jobs/model.go`**

```diff
// PresignFunc resolves a short-lived object URL for a dataset item.
type PresignFunc func(datasetID int64, objectKey string, ttlSeconds int) (string, error)

// Service exposes the Data Hub use cases used by HTTP handlers and tests.
// The contracts stay stable across in-memory and PostgreSQL repositories.
type Service struct {
	repo    Repository
	presign PresignFunc
}

// Dataset is the minimal dataset record exposed by the MVP control plane.
type Dataset struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
}

// Snapshot represents a logical dataset version that downstream jobs and
// artifact builds can reference.
type Snapshot struct {
	ID              int64  `json:"id"`
	DatasetID       int64  `json:"dataset_id"`
	Version         string `json:"version"`
	BasedOnSnapshot *int64 `json:"based_on_snapshot_id,omitempty"`
	Note            string `json:"note,omitempty"`
}
```

```diff
const (
	// StatusQueued means the job has been accepted and is waiting for a worker.
	StatusQueued = "queued"
	// StatusRunning means a worker currently holds the lease and is executing the job.
	StatusRunning = "running"
	// StatusSucceeded means all tracked work completed without item-level failures.
	StatusSucceeded = "succeeded"
	// StatusSucceededWithErrors means at least one item failed but the job produced usable output.
	StatusSucceededWithErrors = "succeeded_with_errors"
	// StatusFailed means the job terminated without producing a successful result.
	StatusFailed = "failed"
	// StatusCanceled means execution stopped due to an external cancellation request.
	StatusCanceled = "canceled"
	// StatusRetryWaiting means the job is paused until it is eligible to be re-queued.
	StatusRetryWaiting = "retry_waiting"
)

// Job is the canonical persisted runtime record for asynchronous work.
// It stores routing information, execution counters, lease data, and terminal error state.
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

- [ ] **Step 4: Add artifact lifecycle comments to `internal/artifacts/service.go`**

```diff
// PackageRequest describes a dataset-export build request accepted by the API.
type PackageRequest struct {
	ProjectID    int64             `json:"project_id"`
	DatasetID    int64             `json:"dataset_id"`
	SnapshotID   int64             `json:"snapshot_id"`
	Format       string            `json:"format"`
	Version      string            `json:"version"`
	LabelMapJSON map[string]string `json:"label_map_json,omitempty"`
}

// Artifact tracks the lifecycle and downloadable metadata of an export package.
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

// Service coordinates artifact creation, background building, and archive access.
type Service struct {
	repo    Repository
	query   *ExportQuery
	builder *Builder
	storage ArtifactStorage
	runner  *BuildRunner
}

// StartBuildRunner enables asynchronous artifact builds inside the API process.
func (s *Service) StartBuildRunner(ctx context.Context, concurrency int) {
	s.runner = NewBuildRunner(concurrency, s.buildArtifact)
	s.runner.Start(ctx)
}

// OpenArtifactArchive opens a ready artifact for HTTP download.
// Only artifacts in the ready state are exposed to callers.
func (s *Service) OpenArtifactArchive(ctx context.Context, id int64) (ReadSeekCloser, int64, Artifact, error) {
	if s.storage == nil {
		return nil, 0, Artifact{}, fmt.Errorf("artifact storage is not configured")
	}
}
```

- [ ] **Step 5: Add CLI comments to `internal/cli/pull.go`**

```diff
// RootCommand is the minimal CLI dispatcher for MVP artifact delivery commands.
type RootCommand struct{}

// PullOptions configures artifact resolution, download, and local verification.
type PullOptions struct {
	Format       string
	Version      string
	AllowPartial bool
	OutputDir    string
}

// PullClient resolves an artifact, downloads its archive, extracts it, and
// writes a local verification report for the pulled contents.
type PullClient struct {
	outputDir string
	source    ArtifactSource
}

// ArtifactSource abstracts where pull fetches artifacts from so tests can
// replace the live HTTP implementation with deterministic fixtures.
type ArtifactSource interface {
	ResolveArtifact(format, version string) (ResolvedArtifact, error)
	DownloadArchive(ctx context.Context, artifact ResolvedArtifact, tempPath string) error
}

// Pull downloads, extracts, and verifies the requested artifact version.
func (c *PullClient) Pull(opts PullOptions) error {
	if opts.Format == "" || opts.Version == "" {
		return fmt.Errorf("format and version are required")
	}
}
```

- [ ] **Step 6: Format the modified Go files**

Run: `gofmt -w cmd/api-server/main.go internal/server/http_server.go internal/datahub/service.go internal/jobs/model.go internal/artifacts/service.go internal/cli/pull.go`
Expected: command exits successfully with no stderr output

- [ ] **Step 7: Commit the Go comment updates**

```bash
git add cmd/api-server/main.go internal/server/http_server.go internal/datahub/service.go internal/jobs/model.go internal/artifacts/service.go internal/cli/pull.go
git commit -m "docs: add comments for core Go workflows"
```

### Task 5: Add Focused Python Docstrings

**Files:**
- Modify: `workers/common/job_client.py`
- Modify: `workers/zero_shot/main.py`
- Modify: `workers/cleaning/main.py`

- [ ] **Step 1: Add payload docstrings to `workers/common/job_client.py`**

```python
def emit_heartbeat(job_id: int, worker_id: str, lease_seconds: int):
    """Build a heartbeat event payload for lease-based worker monitoring."""
    return {
        "job_id": job_id,
        "event_level": "info",
        "event_type": "heartbeat",
        "detail_json": {"worker_id": worker_id, "lease_seconds": lease_seconds},
    }


def emit_progress(job_id: int, worker_id: str, total: int, ok: int, failed: int):
    """Build a progress event payload with aggregate success and failure counters."""
    return {
        "job_id": job_id,
        "event_level": "info",
        "event_type": "progress",
        "detail_json": {
            "worker_id": worker_id,
            "total_items": total,
            "succeeded_items": ok,
            "failed_items": failed,
        },
    }


def emit_terminal(job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int):
    """Build a terminal job summary payload consumed by worker-side job updaters."""
    return {
        "job_id": job_id,
        "worker_id": worker_id,
        "status": status,
        "total_items": total,
        "succeeded_items": ok,
        "failed_items": failed,
    }
```

- [ ] **Step 2: Add summarization docstrings to `workers/zero_shot/main.py`**

```python
def summarize_batch(total: int, ok: int, failed: int):
    """Convert batch counters into the terminal job status expected by the control plane."""
    if failed == 0:
        return "succeeded", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    if ok > 0:
        return "succeeded_with_errors", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    return "failed", {"total_items": total, "succeeded_items": ok, "failed_items": failed}


def build_terminal_event(job_id: int, total: int, ok: int, failed: int):
    """Build the terminal worker payload using the local worker identity fallback."""
    status, summary = summarize_batch(total=total, ok=ok, failed=failed)
    worker_id = os.getenv("WORKER_ID", "zero-shot-local")
    return emit_terminal(
        job_id,
        worker_id,
        status,
        summary["total_items"],
        summary["succeeded_items"],
        summary["failed_items"],
    )
```

- [ ] **Step 3: Add cleaning rule docstrings to `workers/cleaning/main.py`**

```python
def classify_bbox(item: Dict[str, Any]) -> str:
    """Classify a single annotation box as valid or invalid using width and height only."""
    if item.get("bbox_w", 0) <= 0 or item.get("bbox_h", 0) <= 0:
        return "invalid_bbox"
    return "ok"


def run_rules(items: Iterable[Dict[str, Any]], taxonomy: Set[str], dark_threshold: float = 0.2) -> Dict[str, Any]:
    """Run MVP cleaning rules and return summary counts, detailed issues, and removal candidates."""
    summary = {
        "invalid_bbox": 0,
        "category_mismatch": 0,
        "too_dark": 0,
    }
```

- [ ] **Step 4: Run the focused worker tests**

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success workers.tests.test_job_client workers.tests.test_cleaning_rules -v`
Expected: PASS for all listed worker test modules

- [ ] **Step 5: Commit the Python docstring updates**

```bash
git add workers/common/job_client.py workers/zero_shot/main.py workers/cleaning/main.py
git commit -m "docs: add worker docstrings"
```

### Task 6: Run Final Verification

**Files:**
- Modify: `README.md`
- Create: `README.zh-CN.md`
- Modify: `docs/development/local-quickstart.md`
- Create: `docs/development/local-quickstart.zh-CN.md`
- Create: `docs/development/architecture.zh-CN.md`
- Modify: `cmd/api-server/main.go`
- Modify: `internal/server/http_server.go`
- Modify: `internal/datahub/service.go`
- Modify: `internal/jobs/model.go`
- Modify: `internal/artifacts/service.go`
- Modify: `internal/cli/pull.go`
- Modify: `workers/common/job_client.py`
- Modify: `workers/zero_shot/main.py`
- Modify: `workers/cleaning/main.py`

- [ ] **Step 1: Run the Go test suite**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...`
Expected: PASS for all Go packages

- [ ] **Step 2: Re-run the focused worker tests**

Run: `PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success workers.tests.test_job_client workers.tests.test_cleaning_rules -v`
Expected: PASS for all listed worker test modules

- [ ] **Step 3: Check the new Chinese documentation files are linked from the entry docs**

Run: `rg -n "README.zh-CN|local-quickstart.zh-CN|architecture.zh-CN" README.md README.zh-CN.md docs/development/local-quickstart.md docs/development/local-quickstart.zh-CN.md`
Expected: entry docs and runbooks reference the Chinese documentation set consistently

- [ ] **Step 4: Review the final diff before handoff**

Run: `git diff -- README.md README.zh-CN.md docs/development/local-quickstart.md docs/development/local-quickstart.zh-CN.md docs/development/architecture.zh-CN.md cmd/api-server/main.go internal/server/http_server.go internal/datahub/service.go internal/jobs/model.go internal/artifacts/service.go internal/cli/pull.go workers/common/job_client.py workers/zero_shot/main.py workers/cleaning/main.py`
Expected: only documentation and comment changes appear; no behavioral edits

- [ ] **Step 5: Commit the final verification pass if needed**

```bash
git add README.md README.zh-CN.md \
  docs/development/local-quickstart.md docs/development/local-quickstart.zh-CN.md docs/development/architecture.zh-CN.md \
  cmd/api-server/main.go internal/server/http_server.go internal/datahub/service.go internal/jobs/model.go internal/artifacts/service.go internal/cli/pull.go \
  workers/common/job_client.py workers/zero_shot/main.py workers/cleaning/main.py
git commit -m "docs: finalize documentation and comments pass"
```
