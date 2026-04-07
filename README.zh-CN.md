# YOLO-Ave-Mujica

一个面向生产场景的 YOLO 平台基础仓库，当前重点是打通 Data Hub、任务驱动的标注协作、审核流和产物交付。

## 文档导航

- 英文入口：`README.md`
- 英文本地开发：`docs/development/local-quickstart.md`
- 中文本地开发：`docs/development/local-quickstart.zh-CN.md`
- 中文架构说明：`docs/development/architecture.zh-CN.md`
- 技术审计快照：`docs/development/technical-audit-2026-04-04.md`

`docs/superpowers/` 下保留了更细的设计和实施计划，用于追踪历史方案与后续路线。

## 当前实现状态

已实现：

- Go 控制面入口：`api-server`、`platform-cli`、`migrate`、`s3-bootstrap`
- Vite + React + TypeScript Web 控制台：overview、task、review、publish、data、annotation workspace 页面
- Data Hub：数据集创建、扫描、条目浏览、快照、快照详情、对象预签名
- 任务总览、任务列表/详情、标注 workspace draft/submit、publish review 流程
- Jobs：幂等创建、资源 lane 分发、租约恢复、worker 回调、事件查询
- Python worker：importer、packager、cleaning、zero-shot、video 契约型执行入口
- Artifact：打包、状态查询、版本解析、下载、presign、CLI pull 校验
- 公开 `/v1/*` 写接口的静态 bearer 鉴权基线
- dataset、snapshot、job、artifact、publish 变更链路的审计日志基线
- request-id 透传、API JSON access log、基础 Prometheus 指标、worker 结构化 JSON 日志
- 本地 smoke、OpenAPI 路由守卫、迁移链路守卫

尚未完成：

- `zero-shot` 和 `video` 当前是稳定契约输出，不是真实模型推理或媒体抽帧流水线
- 快照导入与 artifact 导出都支持 `yolo` 和 `coco`；其中 YOLO 导出会额外生成 `data.yaml`
- RBAC、更完整的身份体系、训练/评测域、插件运行时仍属于路线图项

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
export AUTH_BEARER_TOKEN=
export AUTH_DEFAULT_PROJECT_IDS=1
export MUTATION_RATE_LIMIT_PER_MINUTE=60
make migrate-up
make web-install
make api-dev
```

`make up-dev` 现在是失败即停的行为：如果 Docker 虽然已安装但当前环境不可用，依赖启动会立刻中止，不会再静默继续。在 WSL 2 里，这通常意味着 Docker Desktop 已安装，但没有给当前发行版开启 WSL integration。

再开一个终端：

```bash
make web-dev
```

默认前端地址是 `http://127.0.0.1:5173`，会把 `/v1/*` 代理到 `http://127.0.0.1:8080`。前端 Web Shell 当前使用根路径导航，例如 `/`、`/tasks`、`/review`、`/publish/candidates`、`/data`；控制面 API 在需要项目上下文的地方仍保持 `/v1/projects/{id}/...` 风格。

当前运维基线：

- `AUTH_BEARER_TOKEN` 可为公开 `/v1/*` 写接口开启 `Authorization: Bearer <token>` 鉴权
- `AUTH_DEFAULT_PROJECT_IDS` 默认值是 `1`，在没有额外请求头覆盖时作为默认允许访问的项目范围
- `X-Project-Scopes: 1,2` 可在受信任的本地/反向代理场景里覆盖默认项目范围
- `X-Actor` 可携带调用方身份，便于审计和事件追踪
- `MUTATION_RATE_LIMIT_PER_MINUTE` 默认值为 `60`，会按 bearer token 或客户端 IP 对公开 `/v1/*` 写接口做节流
- 每个 HTTP 响应都会返回 `X-Request-Id` 和 `X-Correlation-Id`
- 在创建 job 的写请求里主动带 `X-Request-Id`，同一个 trace id 会继续透传到 worker callback
- `GET /metrics` 会暴露基础 HTTP、job lifecycle、queue depth、review backlog、artifact build 指标
- 更细的排障说明见 `docs/development/operations.zh-CN.md`

常用验证命令：

```bash
make test
make web-build
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server -count=1
bash scripts/dev/smoke.sh
```

更完整的步骤、环境要求和常见说明见 `docs/development/local-quickstart.zh-CN.md`。

## 当前 API 能力

- `GET /v1/projects/{id}/overview`
- `GET /v1/projects/{id}/tasks`
- `POST /v1/projects/{id}/tasks`
- `GET /v1/tasks/{id}`
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

如果设置了 `AUTH_BEARER_TOKEN`，公开 `/v1/*` 变更接口将要求 `Authorization: Bearer <token>`。公开 `GET` 接口保持开放，`/internal/*` worker 回调在当前基线中仍然不受该鉴权影响。`AUTH_DEFAULT_PROJECT_IDS` 默认是 `1`；单个请求可以用 `X-Project-Scopes` 覆盖默认项目范围，也可以用 `X-Actor` 注入调用方身份。当前这层能力只是最小项目级 scope 控制，不是完整 RBAC。`MUTATION_RATE_LIMIT_PER_MINUTE` 默认值是 `60`，只作用在公开 `/v1/*` 写接口；纯本地开发如果需要关闭节流，可以显式设为 `0`。

## CLI 与验证产物

- 当前 artifact 格式支持矩阵：
- snapshot import：`yolo`、`coco`
- snapshot export：`yolo`、`coco`
- `platform-cli pull`：`yolo`、`coco`
- 导出包布局：`yolo` 包含 `data.yaml`、`train/images`、`train/labels`；`coco` 包含 `images` 与 `annotations.json`
- `platform-cli pull --dataset <dataset> --format <format> --version <version>` 会轮询 artifact resolve，直到匹配版本进入 ready，然后下载归档、解压内容并校验 `manifest.json`
- `--wait-timeout`、`--poll-interval`、`--verify-workers` 可分别控制等待时长、轮询间隔和本地并发校验数
- 命令完成后会在输出目录写入 `verify-report.json`
- 报告会记录每个文件的 `path`、`size`、`checksum`、`status`、可选 `error`，以及 `environment_context`

## 测试命令

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
PYTHONPATH=. python3 -m unittest \
  workers.tests.test_partial_success \
  workers.tests.test_job_client \
  workers.tests.test_cleaning_rules -v
```

```bash
cd apps/web && npm test
cd apps/web && npm run build
```
