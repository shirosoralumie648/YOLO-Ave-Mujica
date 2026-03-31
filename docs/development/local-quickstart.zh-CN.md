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

### 运行 Web 控制台

1. `make web-install`
2. `make web-dev`
3. 打开 Vite 输出的本地地址，并保持 API 服务运行在 `http://localhost:8080`
