# 本地开发快速开始

## 前置条件

- Go 1.20+
- Node.js 20+ 与 npm 10+
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
export AUTH_BEARER_TOKEN=
export AUTH_DEFAULT_PROJECT_IDS=1
export MUTATION_RATE_LIMIT_PER_MINUTE=60
make migrate-up
```

`make up-dev` 会启动 Docker 依赖栈，并自动创建默认的 MinIO bucket `platform-dev`。该命令现在是失败即停：如果 Docker 虽然已安装但当前环境不可用，它会在依赖启动阶段立刻退出，不会继续执行 S3 bootstrap。对 WSL 2 来说，最常见原因是 Docker Desktop 已安装，但没有给当前发行版开启 WSL integration。如果你完全不使用 Docker，则需要自行准备 PostgreSQL、Redis 和兼容 S3 的对象存储服务，否则 API 和 smoke 脚本都无法通过。

## 启动 API 服务

```bash
make api-dev
```

默认监听地址是 `http://127.0.0.1:8080`。

`AUTH_BEARER_TOKEN` 是可选项。设置后，公开 `/v1/*` 变更接口会要求 `Authorization: Bearer <token>`；公开 `GET` 接口保持开放，`/internal/*` worker 回调暂时仍然可用，这样不会先打断当前队列 worker 链路。

`AUTH_DEFAULT_PROJECT_IDS` 默认值是 `1`，会作为调用方默认允许访问的项目范围。单个请求可以通过 `X-Project-Scopes` 覆盖默认范围，也可以通过受信任反向代理注入 `X-Actor` 来记录调用方身份。当前这套能力只是最小项目级 authz，不是完整 RBAC。

`MUTATION_RATE_LIMIT_PER_MINUTE` 默认值是 `60`。限流只作用在公开 `/v1/*` 写接口上；优先按 bearer token 计数，没有 token 时回退到客户端 IP。纯本地开发如果想关闭节流，可以显式设置为 `0`。

API 现在还会暴露 `GET /metrics`，返回 Prometheus 文本格式指标；同时每个 HTTP 响应都会带上 `X-Request-Id` 和 `X-Correlation-Id`。如果你在创建 job 的请求里主动传入 `X-Request-Id`，这个值会继续透传到 worker callback，便于把 API 访问日志和 worker 回调串起来排障。

## 启动 Web 控制台

```bash
make web-install
make web-dev
```

默认前端地址是 `http://127.0.0.1:5173`，并会把 `/v1/*` 请求代理到 `http://127.0.0.1:8080`。如果 API 不在这个地址，设置 `VITE_API_PROXY_TARGET` 即可。

## 运行测试

```bash
make test
```

该命令会执行：

- 全量 Go 单元测试
- `workers.tests.test_partial_success`
- `workers.tests.test_job_client`
- `workers.tests.test_cleaning_rules`
- `apps/web` 的 Vitest 测试

前端也可以单独验证：

```bash
make web-test
make web-build
```

如果你修改了公开 HTTP 路由或 `api/openapi/mvp.yaml`，还应该额外执行一次路由合同守卫：

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server -count=1
```

公开合同统一位于 `/v1/*`，再加上 `/healthz` 和 `/readyz`。`internal/server/http_server_routes_test.go` 现在会同时守护公开路由注册漂移、OpenAPI 重复 path / method，以及 datahub、tasks/workspace、jobs、review、publish、artifact、snapshot diff/export 这些接口的失败响应面。面向 worker 的内部回调，例如 snapshot import complete、job progress、artifact complete，则位于 `/internal/*`，它们通过各模块自己的测试守护，而不是写进对外 OpenAPI。

## 运行 Smoke 检查

```bash
bash scripts/dev/smoke.sh
```

Smoke 脚本会验证以下链路：

- 任务总览路由形状
- 项目任务列表路由形状
- 单任务详情路由形状
- `/healthz`
- `/readyz`
- 创建数据集
- 扫描数据集对象
- 查询数据集条目
- 重复 annotation workspace submit 仍保持幂等
- 校验对象预签名返回结构
- 校验 zero-shot 任务创建返回结构
- 验证 `coco` 与 `yolo` 两种 snapshot export 请求都会被接受，并轮询构建状态
- 轮询 artifact 导出包构建状态
- 校验 `/metrics` 暴露基础 HTTP、job、queue、review backlog 指标
- 执行 `platform-cli pull`，等待 yolo artifact ready 后完成下载、解压和校验

其中 `/readyz` 会检查 PostgreSQL、Redis 和 MinIO 访问是否可用。如果返回 `503`，说明进程本身还活着，但至少有一个运行时依赖尚未就绪。

如果当前没有 API 进程在运行，脚本会先检查 PostgreSQL、Redis、MinIO 以及基础迁移是否已经准备好，然后临时启动一个本地 API 进程。在受限较强的环境里，即使只是本地 smoke，也可能因为绑定 `:8080` 而需要额外权限。

`ARTIFACT_STORAGE_DIR` 默认是 `/tmp/platform-artifacts`，用于保存导出包的原子落盘目录和归档文件。`ARTIFACT_BUILD_CONCURRENCY` 默认值是 `2`，用于限制进程内同时执行的 artifact 构建数。

当前格式支持矩阵已经冻结为：snapshot import 支持 `yolo`/`coco`，snapshot export 支持 `yolo`/`coco`，`platform-cli pull` 可拉取这两类导出包。当前 smoke 会验证 `coco` 导出可接受，并对 `yolo` 拉取链路做端到端校验。

`platform-cli pull` 会在输出目录生成 `verify-report.json`，其中会记录每个文件的 `path`、`size`、`checksum`、`status`、可选 `error`，以及 `environment_context`，便于排查不同机器上的本地校验差异。

## 观测与排障

快速检查命令：

```bash
curl -fsS http://127.0.0.1:8080/metrics
curl -i -H 'X-Request-Id: smoke-trace-1' http://127.0.0.1:8080/healthz
```

当前最小指标基线包括：

- `yolo_http_requests_total`
- `yolo_job_creations_total`
- `yolo_job_completions_total`
- `yolo_job_lease_recoveries_total`
- `yolo_artifact_build_outcomes_total`
- `yolo_queue_depth`
- `yolo_review_backlog`

更完整的本地运维和排障说明见 `docs/development/operations.zh-CN.md`。

## 停止本地依赖

```bash
make down-dev
```
