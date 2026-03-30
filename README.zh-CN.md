# YOLO-Ave-Mujica

一个面向生产场景的 MVP 仓库，用于验证数据集索引、标注工作流编排，以及训练产物导出与交付链路。

## 当前仓库包含什么

- Go 控制面入口：`api-server`、`platform-cli`
- Data Hub 能力：数据集创建、扫描、快照、条目列表、对象预签名
- Jobs 能力：任务创建、幂等、分发、租约恢复、事件查询
- Review 和 Versioning 基础接口
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
