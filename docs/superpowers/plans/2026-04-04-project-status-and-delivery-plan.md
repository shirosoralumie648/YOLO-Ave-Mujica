# YOLO Platform Project Status And Delivery Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于当前代码库、原始设计和路线图，建立一份面向交付的真实状态评估与推进计划，明确哪些能力已经达到可交付基线、哪些正在推进但尚未合并、哪些仍未启动，以及后续如何按既定目标高质量完成。

**Architecture:** 保持现有 Go 模块化单体 + Python worker + React Web Shell 的总体架构不变。短期不做大规模重写，而是在现有模块边界上继续推进真实 worker 输出、导入导出语义、artifact 交付、鉴权审计与可观测性。当前计划把“已合并基线”和“当前 worktree 已验证增量”明确分层，避免状态误判。

**Tech Stack:** Go 1.24, Chi, pgx/v5, PostgreSQL, Redis, MinIO/S3, Python 3.11+, unittest, React 19, React Router 7, TanStack Query, Vitest, bash smoke checks.

---

## 1. Assessment Baseline

### 1.1 Source Of Truth

- `main` / `origin/main` 当前都在 `096cad995641c7f052e5d730032c4de05c4fd731`
- 当前工作发生在 `.worktrees/codex-phase1-foundation`
- worktree 内已有额外未提交改动，且已通过局部与全量测试验证

### 1.2 Latest Verified Health

验证时间：2026-04-04

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
PYTHONPATH=. python3 -m unittest discover -s workers/tests -p 'test_*.py' -v
cd apps/web && npm test
cd apps/web && npm run build
```

最新结果：

- Go：`go test ./...` 全量通过
- Python workers：27/27 通过
- Web：9 个测试文件、28 个用例全部通过
- Web build：通过
- Smoke：在当前执行环境尝试运行 `bash scripts/dev/smoke.sh`，但因缺少 `make` 命令而未能完成依赖拉起；这反映的是本地工具链缺口，不是当前仓库再次出现编译失败

说明：

- `docs/development/technical-audit-2026-04-04.md` 中“主干不可编译、前端测试失败”的结论已经被后续修复覆盖，不再代表当前真实状态。
- 当前轮次已确认模块级与前端构建基线稳定，但端到端依赖联调与迁移冷启动仍未在可用依赖环境中完成，因此链路级稳定性仍保留残余风险。

## 2. Current Progress Snapshot

### 2.1 Overall Delivery Estimate

以下完成度是基于代码、测试与路线图的工程判断，不是仓库内自动生成指标：

| Workstream | Estimated Completion | Status |
| --- | --- | --- |
| Control-plane API and persistence | 85% | 已达到稳定开发基线 |
| Web shell and core product pages | 80% | 已稳定，可继续承接后续功能 |
| Worker execution contracts | 75% | 协议稳定，真实模型/媒体执行仍不足 |
| Real AI/media behavior | 40% | 结果契约存在，但仍偏模拟/轻实现 |
| Artifact export and CLI pull | 75% | 主链可用，性能与后端语义仍需增强 |
| Security / auth / audit | 10% | 设计明确，代码基本未启动 |
| Observability / telemetry / ops | 10% | 设计明确，代码基本未启动 |
| Training / evaluation / plugin extensibility | 5% | 只有任务类型预留，域实现未开始 |

### 2.2 Completed And Stable Modules

| Module | Status | Quality | Stability | Evidence |
| --- | --- | --- | --- | --- |
| `datahub` | 已完成浏览、快照、对象索引、导入回调 | B+ | 高 | Go tests green；YOLO/COCO import parsing available |
| `jobs` | 已完成创建、幂等、lane dispatch、callback、lease recovery | B+ | 高 | Go tests green；worker queue contract tests green |
| `tasks` + `overview` | 已完成任务列表/详情/状态流转/概览聚合 | B | 高 | Go tests green；Web pages green |
| `annotations` | 已完成 workspace、draft、submit、revision/context 校验 | B+ | 高 | Go tests green；HTTP status 分类已补强 |
| `review` + `publish` | 已完成 candidate lifecycle、publish batch、双审批、反馈、record | B | 中高 | Go tests green；Web pages green |
| `artifacts` + `platform-cli pull` | 已完成导出、构建、resolve、download、校验 | B | 中高 | Go tests green；CLI tests green |
| `apps/web` | Overview / Tasks / Data / Review / Publish / Workspace 页面可用 | B | 高 | Vitest 28/28；build 通过 |

### 2.3 Completed But Still Below Original Product Intent

| Area | Current Reality | Gap To Original Design |
| --- | --- | --- |
| Capability-aware scheduling | 已做 lane + capabilities 匹配、回队/拒绝事件 | 缺 worker 注册表、退避重试策略、运营视图 |
| Zero-shot worker | 已产出 candidate 事件和 `result_ref` | 仍是契约驱动输出，不是真实模型推理 |
| Video worker | 已产出 frame result 事件和 `result_ref` | 仍缺真实媒体解码与对象落盘 |
| COCO support | 已支持 COCO import parsing | COCO export 仍未实现 |
| Diff performance | 已增加 benchmark，并优化 exact-match | update matching 仍可进一步裁剪 |
| Artifact delivery | 状态语义和 ready gating 已清晰 | 构建仍整块读取对象，CLI 仍串行 |

## 3. In-Progress Workstreams

以下工作已经在当前 worktree 中完成开发并通过验证，但尚未提交/合并到 `main`：

### 3.1 API Contract Hardening

- `jobs`：`/internal/jobs/*` 回调已经按 `404/409/422` 分类，而不是统一 `400`
- `annotations`：`revision mismatch`、`already submitted`、workspace 校验已有稳定 HTTP 错误语义
- `datahub`：`/internal/snapshots/{id}/import` 已区分 `404` 与 `422`

**Current blocker:** 这些改动仍处于未提交状态；没有完成 “提交 -> 合并 -> smoke -> 推远端” 的闭环。

### 3.2 Route Contract Governance

- 新增了 `http_server` 公共路由与 OpenAPI path+method 的自动化比对
- 增加了 OpenAPI 重复 path / method 的守卫测试
- 开发文档已补充公开 `/v1/*` 与内部 `/internal/*` 的区分说明

**Current blocker:** `technical-audit-2026-04-04.md` 仍保留旧结论，尚未同步最新状态。

### 3.3 Versioning Performance Baseline

- `internal/versioning/service_bench_test.go` 已新增 benchmark
- `DiffSnapshots()` 对 exact bbox match 已从双层扫描改为索引查找

**Current blocker:** 只建立了第一层基线；未继续优化 update matching，也未形成运维说明文档。

## 4. Not Started Or Largely Unstarted Work

### 4.1 P0 / Immediate Next

1. 合并当前 worktree 已验证改动到 `main`
2. 重新执行 `bash scripts/dev/smoke.sh`
3. 用真实依赖重跑迁移冷启动验证

### 4.2 P1 / Highest Product Value

1. 把 `zero_shot` 从 contract stub 升级为真实推理适配层
2. 把 `video` 从 frame-summary stub 升级为真实抽帧执行
3. 明确 COCO export 策略：实现或从对外合同移除
4. 将 artifact builder 从 `[]byte` 全量读改为更节制的读取路径

### 4.3 P2 / Required For Production Readiness

1. `internal/auth/*` 仍不存在
2. `internal/observability/*` 仍不存在
3. 结构化日志、指标、trace correlation 仍不存在
4. 认证、鉴权、速率限制、审计扩展仍不存在

### 4.4 P3 / Longer-Horizon Features

1. `training` / `evaluation` 领域模块未启动
2. 插件运行时/扩展边界未启动
3. Web 端没有 auth 页面，也没有 training/evaluation 页面

## 5. Priority And Dependency Analysis

### 5.1 Priority Order

遵循原始设计和用户优先级，推荐排序如下：

1. 核心算法逻辑闭环
2. 当前 worktree 的鲁棒性改动合并并端到端验证
3. Artifact / CLI 性能与交付语义
4. 安全、审计、可观测性
5. 训练/评测/插件扩展

### 5.2 Dependency Graph

```text
Merge current verified worktree changes
  -> smoke / migrate cold-start verification
    -> zero-shot real execution
    -> video real execution
    -> COCO export decision
      -> artifact delivery semantics cleanup
        -> CLI parallel pull + richer verify report
          -> auth/audit/observability baseline
            -> training/evaluation/plugin work
```

### 5.3 Key Blocking Decisions

1. COCO export 是要在当前仓库实现，还是正式收窄为“仅 YOLO export”
2. zero-shot / video 是引入真实执行依赖，还是先做 provider interface + fake backend
3. artifact 交付是否继续以 filesystem 为 MVP 主路径，还是转到 S3/MinIO
4. auth 第一版采用静态 token、反向代理 header，还是本地账号

## 6. Detailed Delivery Plan

以下时间按 2026-04-05 起排，采用角色负责制。若团队尚未指定实名 owner，先按角色落位。

### Milestone 0: Stabilize And Merge Verified Work

**Window:** 2026-04-05 to 2026-04-07

**Goal:** 把当前 worktree 已验证的 contract hardening、route governance、diff benchmark 全部正式纳入主线。

**Owners:**

- Accountable: Backend Lead
- Responsible: Backend Engineer A
- Support: QA / Release Engineer

**Scope:**

1. 提交并合并当前 worktree 中 `jobs`、`annotations`、`datahub`、`server`、`versioning` 改动
2. 更新 `technical-audit-2026-04-04.md`，移除已过时的“构建失败”判断
3. 重新运行 smoke 与迁移冷启动

**Technical Approach:**

- 不再继续扩散新功能，先把已验证增量收口
- 保持现有 typed error 方案，不再回退到字符串判断
- 用 route contract tests 作为公开合同守卫的默认入口

**Risks:**

- 依赖服务不可用导致 smoke 无法形成真实结论
- 当前 worktree 改动面较广，合并时需要避免遗漏文件

**Acceptance:**

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
PYTHONPATH=. python3 -m unittest discover -s workers/tests -p 'test_*.py' -v
cd apps/web && npm test
cd apps/web && npm run build
bash scripts/dev/smoke.sh
```

**Deliverables:**

- merged `main`
- 更新后的技术审计文档
- smoke 验证记录

### Milestone 1: Close Core Worker Reality Gap

**Window:** 2026-04-08 to 2026-04-15

**Goal:** 把 zero-shot / video 从“契约完整”推进到“真实执行”。

**Owners:**

- Accountable: Backend Lead
- Responsible: Worker / ML Engineer
- Support: Backend Engineer A, QA / Release Engineer

**Scope:**

1. `workers/zero_shot/main.py` 引入真实推理 provider interface
2. `workers/video/main.py` 引入真实抽帧执行与对象键落地
3. `internal/jobs/service.go` 与 `internal/review/*` 保持结果写回语义稳定
4. 增加至少一个从 job 创建到 durable output 的 smoke 场景

**Technical Approach:**

- 先做 provider interface，保留 deterministic fake backend 以便测试
- domain output 继续通过现有 `result_ref + event` 语义回写，不推翻当前 contract
- 禁止把模型执行细节直接耦合进 handler

**Risks:**

- 真实模型或媒体解码依赖会拉高本地环境复杂度
- 若没有统一对象写回策略，视频抽帧结果会停留在内存结构

**Acceptance:**

```bash
PYTHONPATH=. python3 -m unittest workers.tests.test_zero_shot_worker workers.tests.test_video_worker workers.tests.test_queue_runner -v
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/jobs ./internal/review -v
```

**Deliverables:**

- zero-shot durable candidate outputs
- video durable frame outputs
- 对应测试与 smoke 记录

### Milestone 2: Finish Format And Delivery Semantics

**Window:** 2026-04-16 to 2026-04-23

**Goal:** 冻结数据格式支持矩阵，并收口 artifact / CLI 的交付语义。

**Owners:**

- Accountable: Backend Lead
- Responsible: Backend Engineer A
- Support: Worker / ML Engineer, QA / Release Engineer

**Scope:**

1. 明确 COCO export：实现或收缩合同
2. `internal/artifacts/builder.go` 改为更节制的读对象策略
3. `internal/cli/pull.go` 增加受控并发与 readiness 处理
4. `manifest.json` / `verify-report.json` 增强文件大小与失败摘要

**Technical Approach:**

- 若 COCO export 不做，必须同步更新 OpenAPI、README、技术审计
- builder 优先改为流式写盘或分段读取，不引入大而重的额外中间层
- CLI 并发度受配置控制，默认保守

**Risks:**

- 交付语义改动会波及 artifact tests、CLI tests、smoke
- 若先改存储后端再改 CLI，会导致验证面急剧扩大

**Acceptance:**

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/datahub ./internal/artifacts ./internal/cli ./internal/versioning -v
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/versioning -bench . -run ^$
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/artifacts -bench . -run ^$
bash scripts/dev/smoke.sh
```

**Deliverables:**

- 冻结后的 format support matrix
- artifact / CLI benchmark 基线
- 更新后的 API / docs / smoke

### Milestone 3: Security, Audit, And Observability Baseline

**Window:** 2026-04-24 to 2026-05-06

**Goal:** 给系统补上最小生产化护栏，而不是继续以匿名本地控制面形态扩容。

**Owners:**

- Accountable: Tech Lead / Backend Lead
- Responsible: Backend Engineer B
- Support: Frontend Engineer, QA / Release Engineer

**Scope:**

1. 新建 `internal/auth/*`
2. 新建 `internal/observability/*`
3. 扩展审计覆盖 dataset / snapshot / job / artifact / publish mutation
4. Web 增加最小登录/身份态入口（若采用 token 模式则至少支持注入）

**Technical Approach:**

- 第一版 auth 采用最小可行方案：静态 bearer token 或 trusted header
- observability 先做结构化日志字段和最小 metrics，不追求完整 tracing 平台
- 所有新增安全要求必须回写 OpenAPI 和开发文档

**Risks:**

- 鉴权一旦落地，会影响几乎所有 mutating handlers
- 没有统一身份模型时，前后端都容易各自解释 scope

**Acceptance:**

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/auth ./internal/server ./internal/publish ./internal/artifacts -v
```

**Deliverables:**

- auth middleware
- audit coverage baseline
- operations / local runbook 文档

## 7. Resource Plan

### 7.1 Minimum Recommended Staffing

| Role | Allocation | Responsibility |
| --- | --- | --- |
| Backend Lead | 1.0 FTE | 主线控制、API contract、artifact/versioning/datahub |
| Worker / ML Engineer | 1.0 FTE | zero-shot、video、importer、queue runtime |
| Frontend Engineer | 0.5 FTE | Web shell、auth entry、contract-driven page integration |
| QA / Release Engineer | 0.5 FTE | smoke、migration validation、release checklist、regression matrix |

### 7.2 Owner Mapping

若暂无实名分配，建议采用如下 owner 占位：

- `Owner-A / Backend Lead`
- `Owner-B / Worker-ML`
- `Owner-C / Frontend`
- `Owner-D / QA-Release`

## 8. Collaboration And Governance

### 8.1 Cadence

- 每日 15 分钟站会：状态、阻塞、当日目标
- 每周 2 次技术评审：合同变更、schema 变更、worker 协议变更
- 每个 milestone 结束后 1 次验收评审：测试结果、风险复盘、是否准入下一阶段

### 8.2 Branching And Review

- 每个 milestone 使用独立 worktree / branch
- 每个 PR 必须附带：
  - 变更范围
  - 契约变化说明
  - 运行过的验证命令
  - 是否涉及 OpenAPI / docs / smoke 更新

### 8.3 Progress Tracking States

统一采用以下状态：

- `Not Started`
- `In Progress`
- `Code Complete`
- `Verified`
- `Merged`
- `Released`

## 9. Quality Assurance System

### 9.1 Definition Of Done

一个功能流只有在满足以下条件后才算完成：

1. 对应模块单测通过
2. 若涉及公开路由，对应 OpenAPI 与 route contract tests 已更新
3. 若涉及 worker/internal callback，对应 handler tests 已覆盖失败路径
4. 若涉及文档承诺，README / development docs / plan docs 至少同步一处真实状态
5. 若涉及导出或跨模块行为，smoke 或等价集成验证已通过

### 9.2 Mandatory Quality Gates

- `go test ./...`
- `python3 -m unittest discover -s workers/tests -p 'test_*.py' -v`
- `cd apps/web && npm test`
- `cd apps/web && npm run build`
- `bash scripts/dev/smoke.sh` for route / callback / artifact affecting changes

### 9.3 Additional Gates By Domain

- Contract changes: `go test ./internal/server -run TestOpenAPIPublicRoutesMatchRegisteredRoutes -count=1`
- Versioning changes: benchmark rerun
- Artifact / CLI changes: benchmark + smoke rerun
- Auth changes: unauthorized / forbidden handler tests mandatory

## 10. Summary Recommendation

当前项目不是“功能太少”，而是“多数主链路已经存在，下一步必须把剩余产品缺口和工程护栏按顺序补齐”。最优推进策略不是平均分摊资源，而是：

1. 先把当前 worktree 中已验证的 hardening 和 governance 正式并入主线
2. 然后集中火力完成真实 zero-shot / video 执行
3. 再收口 COCO export、artifact / CLI 性能和交付语义
4. 最后补安全、审计和可观测性

如果跳过第 2 步直接做安全或运维，团队会把时间投入到一个“主链路仍偏模拟”的系统上，ROI 明显较差。
