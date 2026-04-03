# Publish Gate And Review Workspace Design

- Date: 2026-04-03
- Scope: V1 Phase 2 product slice after Task Overview, Task resources, and Data browsing
- Status: Draft
- Owner: Platform team
- Related specs:
  - [2026-03-30-yolo-platform-product-framework-design.md](/home/shirosora/YOLO-Ave-Mujica/docs/superpowers/specs/2026-03-30-yolo-platform-product-framework-design.md)
  - [2026-03-30-yolo-platform-user-journey-and-interaction-design.md](/home/shirosora/YOLO-Ave-Mujica/docs/superpowers/specs/2026-03-30-yolo-platform-user-journey-and-interaction-design.md)
  - [2026-04-02-data-domain-browsing-design.md](/home/shirosora/YOLO-Ave-Mujica/docs/superpowers/specs/2026-04-02-data-domain-browsing-design.md)

## 1. 背景

当前仓库已经有三个可用基座：

1. `Task Overview -> Task List -> Task Detail` 的 task-first Web Shell 主入口已经存在。
2. `Dataset List -> Dataset Detail -> Snapshot Detail -> Snapshot Diff` 的数据浏览闭环已经存在。
3. `review`、`versioning`、`artifacts`、`jobs` 后端控制面已经具备基础能力。

但系统仍然缺少正式的发布门禁与审核工作台，这导致产品主线卡在 Review 与 Publish 之间：

1. Reviewer 只能处理零散 candidate，而不能将结果组织成正式发布候选批次。
2. Project Owner 没有双审批入口，无法对发布批次做最终决策。
3. Reject / Rework 仍缺少机器可消费的 batch 级与 item 级结构化反馈。
4. 审批通过后没有稳定的 `publish record`，也没有把结果推送到后续任务链路。
5. 当前 Web Shell 还没有完整的 Review Workspace，无法承载批次级、对象级、上下文级审批动作。

这会直接阻断产品框架 spec 中的 `Review And QA` 与后续 `Training And Evaluation Hub`：

1. 没有正式 publish gate，就没有可信的数据晋升入口。
2. 没有结构化反馈，就没有可回流的质量闭环。
3. 没有 publish record 和下游 task，就无法把审批结果自然地推给训练/晋升后续责任方。

## 2. 目标

本轮要交付一个真正可用的 `Review -> Publish Gate -> Downstream Task` 产品切片。

用户应当能够：

1. 在 `Review Queue` 中发现待处理 review / publish 工作。
2. 在 `Publish Candidates` 中查看系统建议的 publish batch，并手动调整。
3. 在 `Publish Batch Detail` 中查看批次摘要、审批历史、结构化反馈和上下文。
4. 在完整 `Review Workspace` 中基于画布、对象、时间轴和 diff 做审批判断。
5. 让 Reviewer 与 Project Owner 通过双审批链对 batch 进行正式决策。
6. 在 Owner 最终批准后生成 `publish record`，并自动创建后续 task。

## 3. 非目标

本轮明确不做：

1. 直接产出正式 `published snapshot version`
2. 训练运行、评测报告、artifact promotion 的完整 UI
3. IAM / auth / audit 的完整产品化落地
4. 点云、多模态、插件化审批工作台
5. 训练流水线自动触发执行

说明：

1. 本轮 publish gate 的正式输出是 `publish record`，不是新的 published snapshot。
2. 后续训练或晋升流程通过自动创建 downstream task 接住，而不是在本轮直接扩展成新的大模块。

## 4. 决策摘要

本轮设计已经锁定以下关键边界：

1. `publish gate` 使用 Reviewer + Project Owner 双审批链。
2. 发布单元是 `snapshot + approval batch`，不是单个 candidate，也不是 snapshot 整体一次通过。
3. 同一个 `snapshot` 下允许同时存在多个并行中的 publish batch。
4. 一个 batch 的内容是完整审批快照，包含 `candidate + task + snapshot summary`。
5. Owner 最终批准后生成 `publish record`，不直接把 snapshot 标成 published。
6. 系统采用“建议 batch + 人工编辑”的混合模式。
7. Owner 可以自由增删 batch 内容。
8. 只要 Owner 修改了 batch 内容，Reviewer 的已审批状态立即失效，必须回到 Reviewer 重审。
9. 结构化反馈同时支持 `batch` 级和 `item` 级。
10. 系统默认通过规则引擎生成建议 batch。
11. 本轮 Review Workspace 目标是完整工作台，不是只读详情页。
12. `publish record` 生成后要自动创建 downstream task。

## 5. 方案比较

### 方案 A：审批内核优先

只做 publish batch / publish record / 双审批 / feedback 内核，前端仅管理页。

优点：

1. 后端状态机最稳定。
2. 审计和回退路径最清晰。

缺点：

1. 无法满足完整 Review Workspace 目标。
2. 产品感偏弱，仍然像控制面。

### 方案 B：完整工作台一次到位

一次交付 Review Queue、Publish Candidates、完整 Review Workspace、双审批、publish record、下游 task。

优点：

1. 最接近目标产品体验。
2. Review 与 Publish 一次打通。

缺点：

1. 切片体量很大。
2. 如果状态机模型设计错误，返工成本高。

### 方案 C：双层切片

先以 publish gate 真实内核为主线，但前端同时交付足够支撑审批闭环的完整工作台框架。

优点：

1. 状态机与数据模型先稳定。
2. 仍能交付真实可用的 Review Workspace。

缺点：

1. 实施上需要严格控制边界。

### 决策

采用方案 C。

原因：

1. 它能优先确保 `publish gate` 语义正确。
2. 它仍然保留完整 Review Workspace 的产品目标。
3. 它把最昂贵的设计决策放在内核层，而不是画布层。

## 6. 资源模型

### 6.1 PublishBatch

`PublishBatch` 表示一次正式审批批次。

字段建议：

1. `id`
2. `snapshot_id`
3. `project_id`
4. `source`，取值如 `suggested`、`manual`
5. `status`
6. `rule_summary_json`
7. `owner_edit_version`
8. `review_approved_at`
9. `review_approved_by`
10. `owner_decided_at`
11. `owner_decided_by`
12. `created_at`
13. `updated_at`

说明：

1. 同一 snapshot 允许存在多个 active batch。
2. `owner_edit_version` 用于判断 Owner 修改后是否触发重新 review。

### 6.2 PublishBatchItem

`PublishBatchItem` 表示批次中的单个审批项。

字段建议：

1. `id`
2. `publish_batch_id`
3. `candidate_id`
4. `task_id`
5. `snapshot_id`
6. `dataset_id`
7. `item_payload_json`
8. `position`
9. `created_at`
10. `updated_at`

说明：

1. `item_payload_json` 用来冻结创建 batch 时的候选项、task 摘要、snapshot 摘要。
2. 这样审批过程不会依赖外部资源在运行时再次拼接，降低上下文漂移风险。

### 6.3 PublishFeedback

`PublishFeedback` 是结构化反馈资源。

字段建议：

1. `id`
2. `publish_batch_id`
3. `publish_batch_item_id`，batch 级反馈时为空
4. `scope`，取值 `batch` 或 `item`
5. `stage`，取值 `review` 或 `owner`
6. `action`，取值 `reject`、`rework`、`comment`
7. `reason_code`
8. `severity`
9. `influence_weight`
10. `comment`
11. `created_by`
12. `created_at`

说明：

1. batch 级反馈回答“整批为什么不能过”。
2. item 级反馈回答“哪一项为什么有问题”。

### 6.4 PublishRecord

`PublishRecord` 是 publish gate 的正式输出。

字段建议：

1. `id`
2. `project_id`
3. `snapshot_id`
4. `publish_batch_id`
5. `status`
6. `summary_json`
7. `approved_by_owner`
8. `approved_at`
9. `created_at`

说明：

1. 它不是新的 published snapshot。
2. 它是后续 training / promotion / delivery 流程的正式输入。

## 7. 状态机

### 7.1 PublishBatch.status

建议状态：

1. `draft`
2. `review_pending`
3. `review_approved`
4. `owner_pending`
5. `owner_changes_requested`
6. `rejected`
7. `published`
8. `superseded`

### 7.2 主路径

1. 系统生成建议 batch，初始为 `draft`
2. Reviewer 确认并提交审批，进入 `review_pending`
3. Reviewer 批准后进入 `review_approved`
4. 推送到 Owner 后进入 `owner_pending`
5. Owner 批准后生成 `publish record`
6. 系统自动创建 downstream task
7. batch 状态进入 `published`

### 7.3 Owner 修改回退规则

这是本轮最重要的硬规则：

1. Owner 可以自由增删 batch item。
2. 但只要内容发生任何改动，Reviewer 的已审批状态立即失效。
3. batch 回到 `owner_changes_requested`
4. 然后重新进入 `review_pending`

这样做的原因：

1. 审计清楚。
2. 双审批链不会被 Owner 直接绕过。
3. 所有最终进入 publish record 的内容都必须经过 Reviewer 再确认。

### 7.4 Reject / Rework

1. Reviewer 和 Owner 都可以提交 batch 级或 item 级反馈。
2. 如果是不可恢复否决，batch 进入 `rejected`。
3. 如果是要求修正重审，则保留 batch，并进入重新审批链。

### 7.5 Superseded

`superseded` 用于表示：

1. 某个旧 batch 已被新的更完整或更优先批次替代。
2. 它不是审批失败，而是被业务上淘汰。

## 8. 建议 Batch 生成

### 8.1 总体原则

系统先产出 `SuggestedPublishBatch`，Reviewer 再做增删、拆分、合并与确认。

系统不能直接替人决定正式发布边界。

### 8.2 硬约束

建议 batch 的候选项必须满足：

1. 属于同一个 `snapshot`
2. 处于 review 通过或等效可发布前置状态
3. 没有未决 blocker
4. 具备最小必要上下文

不满足硬约束的项不能进入建议 batch。

### 8.3 规则引擎分组信号

建议先支持以下聚合信号：

1. 风险等级
2. 来源模型 / 来源策略
3. review 通过时间窗口
4. task / owner / assignee 上下文
5. reason code 聚类信号

### 8.4 Reviewer 可编辑行为

Reviewer 可以：

1. 接受建议直接创建 batch
2. 从建议 batch 中移除项
3. 向建议 batch 中补充项
4. 合并两个建议 batch
5. 拆分一个建议 batch

## 9. 页面与交互结构

### 9.1 Review Queue

作用：

1. Reviewer 默认工作入口
2. 查看待 review、待 publish、被退回重审的批次与任务

页面职责：

1. 列表发现
2. 快速筛选
3. 跳转到 `Publish Candidates` 或 `Publish Batch Detail`

### 9.2 Publish Candidates

作用：

1. 展示规则引擎建议出来的 candidate groups
2. 让 Reviewer 将建议结果整理成正式 batch

页面职责：

1. 显示系统建议及其分组理由
2. 支持 merge / split / add / remove
3. 创建正式 `PublishBatch`

### 9.3 Publish Batch Detail

作用：

1. 承担正式审批详情页职责

页面必须展示：

1. batch 摘要
2. item 列表
3. batch 级反馈
4. item 级反馈
5. 审批历史
6. 当前责任人
7. 下游影响摘要

### 9.4 Review Workspace

作用：

1. 承担真正的审批现场

这次切片的目标是完整工作台，因此要支持：

1. 图像 / 帧预览
2. 候选框只读叠加
3. 对象级上下文查看
4. snapshot diff 辅助查看
5. 视频时间轴 / 轨迹检查
6. item 级反馈填写

说明：

1. 本轮是“完整工作台”，但职责仍然聚焦在审批，不扩展成全量标注工作台。
2. Reviewer 和 Owner 共用工作台框架，但动作权限不同。

## 10. 角色动作

### 10.1 Reviewer

Reviewer 可以：

1. 查看建议 batch
2. 创建正式 batch
3. 修改 batch 内容
4. 填写 batch/item 级反馈
5. `approve`
6. `reject`
7. `request rework`
8. 推送到 Owner

### 10.2 Project Owner

Owner 可以：

1. 查看 batch 详情与完整上下文
2. 查看审批历史
3. 修改 batch 内容
4. 填写 batch/item 级反馈
5. `approve`
6. `reject`
7. `request rework`

但 Owner 的修改会触发硬回退：

1. 任何内容变更都必须回到 Reviewer。

## 11. 后端模块边界

### 11.1 internal/publish

新增 `internal/publish` 模块，负责：

1. `PublishBatch`
2. `PublishBatchItem`
3. `PublishFeedback`
4. `PublishRecord`
5. 双审批状态机
6. Owner 修改导致 Reviewer 失效
7. downstream task 自动创建

### 11.2 internal/review

`internal/review` 继续负责：

1. candidate / review 基础能力
2. 可供 publish gate 消费的 review 视图

但不负责 publish batch 生命周期。

### 11.3 internal/tasks

`internal/tasks` 继续作为责任交接域：

1. `publish record` 生成后复用 task 模型自动创建 downstream task
2. 不单独新增一套下游资源体系

## 12. API 设计

### 12.1 建议批次

1. `GET /v1/publish/candidates`

### 12.2 正式批次

1. `POST /v1/publish/batches`
2. `GET /v1/publish/batches/{id}`
3. `POST /v1/publish/batches/{id}/items`

### 12.3 Reviewer 审批动作

1. `POST /v1/publish/batches/{id}/review-approve`
2. `POST /v1/publish/batches/{id}/review-reject`
3. `POST /v1/publish/batches/{id}/review-rework`

### 12.4 Owner 审批动作

1. `POST /v1/publish/batches/{id}/owner-approve`
2. `POST /v1/publish/batches/{id}/owner-reject`
3. `POST /v1/publish/batches/{id}/owner-rework`

### 12.5 反馈

1. `POST /v1/publish/batches/{id}/feedback`
2. `POST /v1/publish/batches/{id}/items/{itemId}/feedback`

### 12.6 PublishRecord

1. `GET /v1/publish/records/{id}`

### 12.7 Workspace 查询

1. `GET /v1/publish/batches/{id}/workspace`

返回内容要一次带齐：

1. batch 摘要
2. items
3. 可视审批上下文
4. snapshot / diff 摘要
5. 反馈和审批历史

目的：

1. 前端不要自己拼 N+1 请求。

## 13. Downstream Task

当 Owner 最终批准 batch 后，系统要执行两个动作：

1. 创建 `PublishRecord`
2. 自动创建 downstream task

下游 task 建议先复用现有 task 域，通过 `kind` 扩展实现，不新建另一套大资源体系。

本轮建议支持的 kind：

1. `training_candidate`
2. `promotion_review`

如果 task 域本轮不适合立即扩展枚举，可先保守映射到现有扩展位，但最终目标仍应是显式 kind。

## 14. 测试与验证

本轮设计要求覆盖以下验证：

1. publish batch 状态机
2. Owner 修改内容触发 Reviewer 失效
3. batch/item 双层结构化反馈
4. publish record 生成
5. downstream task 自动创建
6. `Review Queue`
7. `Publish Candidates`
8. `Publish Batch Detail`
9. `Review Workspace`

## 15. 成功标准

这次切片完成后，系统应该能自然回答：

1. 哪些候选项正在等待正式发布审批。
2. 某个 snapshot 下有哪些并行中的 publish batch。
3. Reviewer 批准过的内容是否被 Owner 修改过。
4. 某次发布为什么被 reject 或 rework。
5. 某次正式审批通过后生成了什么 publish record。
6. publish 结果已经交给了哪个下游 task 去继续推进。

## 16. 风险与控制

### 16.1 最大风险

最大风险不是画布实现，而是审批状态机和 batch 内容冻结语义做错。

### 16.2 控制策略

1. 先让 `internal/publish` 保持边界清晰。
2. 用状态机测试先锁住回退逻辑。
3. 用 workspace 聚合查询避免前端自己拼装语义。
4. 对 batch 内容采用冻结快照，而不是运行时拼接。

## 17. 下一步

本 spec 批准后，下一步应写独立 implementation plan，按任务拆成：

1. `internal/publish` 内核与迁移
2. publish API 与任务联动
3. `Publish Candidates` 与 `Publish Batch Detail`
4. `Review Queue` 与 `Review Workspace`
5. 双审批 / feedback / downstream task 收尾验证
