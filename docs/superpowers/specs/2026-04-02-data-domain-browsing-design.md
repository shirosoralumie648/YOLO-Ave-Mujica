# Data Domain Browsing Design

## 1. 背景

当前仓库已经有一条可运行的控制面主链路：

1. `internal/datahub` 已支持 dataset 创建、scan、item 列表、snapshot 创建与列表。
2. `internal/versioning` 已支持基于 snapshot id 的真实 diff。
3. `apps/web` 已经有可运行的 `Task Overview`、`Task List`、`Task Detail` 主线。

但 Data Manager 对应的数据域页面仍然缺失，导致用户无法在 Web Shell 中回答这些基础问题：

1. 现在有哪些 dataset。
2. 哪个 dataset 最近产出了什么 snapshot。
3. 某个 snapshot 是否值得继续进入 task 或 review 链路。
4. 两个 snapshot 之间到底改了什么。

这会直接阻断后续的 publish gate、review queue 和训练评测页面，因为这些页面都需要以 dataset / snapshot 的浏览与上下文链路作为入口。

## 2. 目标

这一轮只补齐 Data Manager 的最小浏览闭环：

1. `Dataset List`
2. `Dataset Detail`
3. `Snapshot Detail`
4. `Snapshot Diff`

用户应当能够从左侧导航进入 `Data`，再沿着：

`Dataset List -> Dataset Detail -> Snapshot Detail -> Snapshot Diff`

完成一条完整浏览路径，并且在 `Task Detail` 中看到的 dataset / snapshot 上下文能够反向跳回数据域页面。

## 3. 非目标

这一轮明确不做：

1. publish gate 语义
2. import / export run 独立页面
3. review queue 页面
4. ontology / calibration 管理页
5. 数据域内的复杂写操作
6. 项目切换器与多 project 上下文

这意味着页面可以展示 “当前状态未知” 或 “尚未接入” 的字段，但不能伪装成已经完成的正式发布流。

## 4. 方案比较

### 方案 A：只做数据浏览闭环

做 4 个数据域页面，并新增最小读接口，让前端不需要用 N+1 请求把列表硬拼出来。

优点：

1. 范围稳定。
2. 与当前 `datahub` / `versioning` 的已有能力高度贴合。
3. 能最快补齐 Data Manager 的基础工作面。
4. 不会把 publish gate 和 review 逻辑过早耦合进来。

缺点：

1. 这轮不会提供数据域内的新建任务、新建 snapshot 等主动操作。

### 方案 B：数据浏览 + 轻操作

在方案 A 基础上，再加 `Create Snapshot` 和从 dataset 发起 task 的最小表单。

优点：

1. 页面更接近“可操作产品”。

缺点：

1. 会显著扩大路由、测试、后端写接口和表单状态管理范围。
2. 容易把 Phase 2 的“浏览问题”又拉回到“流程编排问题”。

### 方案 C：只补前端页面壳，尽量不改后端

前端直接拼接现有 `datahub` / `versioning` 接口，多发请求凑页面。

优点：

1. 页面出得最快。

缺点：

1. 列表页一定会出现接口过碎和 N+1。
2. 后面补 publish / review / task 关联时要再次返工接口层。
3. 会让前端承担不该承担的聚合逻辑。

### 决策

采用方案 A。

原因：

1. 它正好补齐当前产品缺的“浏览主链”。
2. 不会越界到 publish / review / task 创建。
3. 它能为下一阶段的数据可信度和发布门禁提供稳定的页面入口。

## 5. 信息架构

### 5.1 导航

在当前 `AppShell` 左侧导航中新增：

1. `Data`

点击后进入 `/data`，默认显示 `Dataset List`。

### 5.2 路由

新增前端路由：

1. `/data`
2. `/data/datasets/:datasetId`
3. `/data/snapshots/:snapshotId`
4. `/data/diff?before=:beforeSnapshotId&after=:afterSnapshotId`

同时补充上下文跳转：

1. `Task Detail` 中的 dataset 名称跳到 `/data/datasets/:datasetId`
2. `Task Detail` 中的 snapshot 版本跳到 `/data/snapshots/:snapshotId`
3. `Snapshot Detail` 中若存在 `based_on_snapshot_id`，提供 “Compare with previous” 入口到 diff 页面

## 6. 后端设计

### 6.1 现有可复用接口

已有接口继续保留：

1. `GET /v1/datasets/{id}/items`
2. `GET /v1/datasets/{id}/snapshots`
3. `POST /v1/snapshots/diff`

其中 `POST /v1/snapshots/diff` 继续作为 `Snapshot Diff` 页的真实数据来源，不新增第二套 diff 协议。

### 6.2 新增读接口

新增以下最小读接口：

1. `GET /v1/datasets`
2. `GET /v1/datasets/{id}`
3. `GET /v1/snapshots/{id}`

### 6.3 数据模型

在 `internal/datahub` 中新增只读聚合结构：

#### DatasetSummary

用于 `Dataset List`：

1. `id`
2. `project_id`
3. `name`
4. `bucket`
5. `prefix`
6. `item_count`
7. `latest_snapshot_id`
8. `latest_snapshot_version`
9. `snapshot_count`

#### DatasetDetail

用于 `Dataset Detail` 顶部摘要：

1. `id`
2. `project_id`
3. `name`
4. `bucket`
5. `prefix`
6. `item_count`
7. `snapshot_count`
8. `latest_snapshot_id`
9. `latest_snapshot_version`

说明：

1. snapshot 列表和 items 列表继续通过现有接口单独取。
2. 这样可以保持后端聚合结构小而稳定。

#### SnapshotDetail

用于 `Snapshot Detail`：

1. `id`
2. `dataset_id`
3. `dataset_name`
4. `project_id`
5. `version`
6. `based_on_snapshot_id`
7. `note`
8. `annotation_count`

说明：

1. 这轮没有正式 publish 状态，因此不返回伪造的 `publish_status`。
2. 页面会明确显示 “publish status not wired yet” 的占位提示，而不是假装已经有正式语义。

### 6.4 Repository 变更

`internal/datahub/repository.go` 增加：

1. `ListDatasets(ctx context.Context, projectID int64) ([]DatasetSummary, error)`
2. `GetDatasetDetail(ctx context.Context, datasetID int64) (DatasetDetail, error)`
3. `GetSnapshotDetail(ctx context.Context, snapshotID int64) (SnapshotDetail, error)`

PostgreSQL 实现要求：

1. `ListDatasets` 必须在单次查询内返回 `item_count`、`snapshot_count` 与最近 snapshot 信息。
2. `GetDatasetDetail` 必须在单次查询内返回 dataset 基础信息与聚合计数。
3. `GetSnapshotDetail` 必须 join `datasets`，并返回 `annotation_count`。

In-memory 实现要求：

1. 行为上与 PostgreSQL 对齐。
2. 不要求复杂优化，只要求字段语义一致。

### 6.5 Service / Handler 变更

`internal/datahub/service.go` 新增：

1. `ListDatasets(projectID int64) ([]DatasetSummary, error)`
2. `GetDatasetDetail(datasetID int64) (DatasetDetail, error)`
3. `GetSnapshotDetail(snapshotID int64) (SnapshotDetail, error)`

`internal/datahub/handler.go` 新增：

1. `ListDatasets`
2. `GetDatasetDetail`
3. `GetSnapshotDetail`

HTTP 约束：

1. 这轮继续固定 `project_id = 1`
2. `GET /v1/datasets` 默认返回 project 1 的数据集
3. 如果资源不存在，返回 `404`

### 6.6 Server Wiring

`internal/server/http_server.go` 增加以下路由：

1. `GET /v1/datasets`
2. `GET /v1/datasets/{id}`
3. `GET /v1/snapshots/{id}`

`cmd/api-server/main.go` 继续通过 `internal/datahub` 注入，不新增新的 domain 模块。

## 7. 前端设计

### 7.1 Feature 结构

新增 `apps/web/src/features/data/`：

1. `api.ts`
2. `dataset-list-page.tsx`
3. `dataset-detail-page.tsx`
4. `snapshot-detail-page.tsx`
5. `snapshot-diff-page.tsx`
6. `data-pages.test.tsx`

### 7.2 Dataset List

页面必须显示：

1. dataset 名称
2. bucket / prefix
3. item 数
4. snapshot 数
5. 最近 snapshot 版本

交互要求：

1. 点击 dataset 名称进入 `Dataset Detail`
2. 点击最近 snapshot 进入 `Snapshot Detail`

页面不做：

1. 排序控件
2. 新建 dataset 表单
3. project 切换器

### 7.3 Dataset Detail

页面包含 3 个区块：

1. 基础摘要
2. Snapshot 列表
3. Item 列表摘要

基础摘要展示：

1. dataset 名称
2. bucket
3. prefix
4. item 数
5. snapshot 数
6. 最近 snapshot

Snapshot 列表要求：

1. 展示 `version`
2. 展示 `note`
3. 展示 `based_on_snapshot_id`
4. 每一行都可点进 `Snapshot Detail`
5. 若该 snapshot 有父版本，则提供 compare deep link 到 `/data/diff?before=<parent>&after=<current>`

Item 列表摘要要求：

1. 先展示前若干条 object key
2. 若 item 数过大，不在这轮做分页器

### 7.4 Snapshot Detail

页面必须展示：

1. snapshot 版本
2. dataset 名称并可跳回 dataset
3. note
4. parent snapshot
5. annotation_count

页面还需要明确显示：

1. 当前 publish gate 未接入

具体表现为一段说明文案，例如：

`Publish status is not wired in this slice yet. Use this page for metadata and diff inspection only.`

### 7.5 Snapshot Diff

页面从 URL 读取：

1. `before`
2. `after`

然后调用现有：

1. `POST /v1/snapshots/diff`

页面必须展示：

1. `added_count`
2. `removed_count`
3. `updated_count`
4. `compatibility_score`
5. 变更明细列表

这轮不做：

1. 图表
2. 样本预览
3. 训练影响推荐

## 8. 数据流

### 8.1 Dataset List

1. 页面加载
2. 请求 `GET /v1/datasets`
3. 渲染 summary rows

### 8.2 Dataset Detail

1. 页面加载
2. 并行请求：
   1. `GET /v1/datasets/:id`
   2. `GET /v1/datasets/:id/items`
   3. `GET /v1/datasets/:id/snapshots`
3. 页面渲染基础信息、snapshot 列表和 item 摘要

### 8.3 Snapshot Detail

1. 页面加载
2. 请求 `GET /v1/snapshots/:id`
3. 页面渲染 snapshot 元数据
4. 若存在父 snapshot，渲染 compare 链接

### 8.4 Snapshot Diff

1. 页面从 query 读取 `before` 和 `after`
2. 调用 `POST /v1/snapshots/diff`
3. 渲染 stats 和变更列表

## 9. 错误与空状态

必须覆盖以下状态：

1. `Dataset List` 空状态：没有 dataset 时给出明确说明
2. `Dataset Detail` 404：显示 dataset 不存在
3. `Snapshot Detail` 404：显示 snapshot 不存在
4. `Snapshot Diff` 缺少 before / after：显示参数错误
5. `Snapshot Diff` 返回空 diff：仍然展示 stats，不显示空白页面

## 10. 测试策略

### 后端

新增或修改：

1. `internal/datahub/service_test.go`
2. `internal/datahub/handler_test.go`
3. `internal/server/http_server_routes_test.go`
4. `cmd/api-server/main_test.go`

需要覆盖：

1. dataset list/detail/snapshot detail 读接口
2. 404 与 bad request 行为
3. server route wiring

### 前端

新增：

1. `apps/web/src/features/data/data-pages.test.tsx`

需要覆盖：

1. Data 导航存在
2. Dataset List 渲染
3. Dataset Detail 渲染 snapshot 链接
4. Snapshot Detail 渲染 compare 链接
5. Snapshot Diff 渲染 stats

## 11. 验收标准

这一轮完成后，必须满足：

1. 用户能从左侧 `Data` 导航进入数据域页面
2. `Dataset List -> Dataset Detail -> Snapshot Detail -> Snapshot Diff` 链路可真实走通
3. 页面使用真实后端数据，不是静态占位
4. `Task Detail` 的 dataset / snapshot 上下文可以跳回数据域页面
5. 仓库仍然不宣称 publish gate、review queue、import/export runs 已完成

## 12. 后续衔接

完成这一轮后，下一层最自然的扩展是：

1. Snapshot 的 publish / trust 状态
2. Review Queue 页面
3. Publish Candidates 页面

这些能力都应当建立在本轮的数据域浏览基础上，而不是跳过它直接做流程页。
