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

## `api-server` 启动与模块装配

`cmd/api-server/main.go` 负责读取配置、初始化 PostgreSQL、Redis、MinIO 相关依赖，并把 Data Hub、Jobs、Versioning、Review、Artifacts 这些模块组装成 `server.Modules`，最后交给 `internal/server/http_server.go` 注册成 HTTP 路由。

除了 HTTP 服务本身，启动过程还会做两件后台工作：

- 启动 job sweeper 定时检查过期租约，做恢复或重试
- 启动 artifact build runner，异步消费待构建的导出任务

`/healthz` 只反映进程是否存活，`/readyz` 则会继续检查 PostgreSQL、Redis、MinIO 和 bucket 配置是否达到可服务状态。

## 路由与合同治理

公开 HTTP 面统一收敛在 `internal/server/http_server.go` 的 `/v1/*` 路由树下，面向 worker 或内部异步链路的回调则收敛在 `/internal/*` 下。当前需要特别注意两点：

- 对外合同以 `api/openapi/mvp.yaml` 为准，新增或修改公开 `/v1/*` 路由时，必须同步更新 OpenAPI。
- `/internal/*` 回调不属于外部公开合同，但它们的错误边界需要由各模块自己的 handler 测试守住，例如 job heartbeat、snapshot import complete、artifact complete。

仓库现在有一条自动化守卫：`internal/server/http_server_routes_test.go` 会比对实际注册的公开路由和 OpenAPI 中声明的 path + method，防止 `http_server.go`、README、OpenAPI 各自漂移。

当前已经固定的几个浏览型公开读取接口包括：

- `GET /v1/datasets`
- `GET /v1/datasets/{id}`
- `GET /v1/snapshots/{id}`
- `GET /v1/projects/{id}/overview`

如果修改了公开路由面，至少要重新执行：

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server -run TestOpenAPIPublicRoutesMatchRegisteredRoutes -count=1
```

## Data Hub 请求流

Data Hub 负责数据集及其快照相关的基础能力，包括：

- 创建数据集
- 扫描对象键并登记条目
- 创建和列出快照
- 为对象读取生成短期预签名 URL

在 MVP 阶段，Service 层对外暴露的输入输出模型已经稳定，底层既可以接内存仓储，也可以接 PostgreSQL 仓储，因此 handler 和测试主要围绕稳定的 service contract 编排。

## Jobs 创建、分发与恢复

`internal/jobs/service.go` 负责创建任务、保证幂等、写入初始事件，并在配置了 dispatcher 时把任务投递到与资源类型对应的 lane。

相关职责分层如下：

- Service：校验输入、调用仓储、决定是否投递
- Repository：持久化任务、事件、租约和状态
- Publisher：把任务放入 Redis 等分发通道
- Sweeper：检查租约超时任务，必要时重新排队

`internal/jobs/model.go` 里的状态字段和计数字段是任务运行态的核心描述，worker 事件、任务查询接口和租约恢复逻辑都会使用这些字段。

## Artifacts 构建与交付流

Artifacts 模块负责把某个数据集快照导出为可下载的训练包。主链路如下：

1. 客户端调用 `POST /v1/artifacts/packages` 创建导出请求
2. Service 在仓储中创建一条 `pending` artifact 记录
3. Build runner 异步取出 artifact ID 并进入 `building`
4. Export query 从数据库组装快照导出所需的数据
5. Builder 生成目录结构、标签文件、`data.yaml`、`manifest.json` 和归档文件
6. Filesystem storage 把构建结果原子写入本地存储目录
7. Repository 更新 artifact 状态为 `ready`，并写回 `uri`、`manifest_uri`、`checksum`、`size` 等元数据

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
