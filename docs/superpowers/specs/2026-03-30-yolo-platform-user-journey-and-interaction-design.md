# YOLO Platform User Journey And Interaction Design

- Date: 2026-03-30
- Scope: V1-first user journeys and interaction experience, with V1.5 and V2 evolution appendix
- Status: Drafted from the approved product framework spec
- Owner: Platform team
- Parent Spec: [2026-03-30-yolo-platform-product-framework-design.md](./2026-03-30-yolo-platform-product-framework-design.md)

## 1. 文档定位与范围

### 1.1 文档角色

这是一份用户旅程与交互体验 spec。

它不再回答“系统有哪些模块”，而是回答：

1. 不同角色登录后先看到什么。
2. 用户如何在正确的上下文中完成主线工作。
3. 页面之间如何继承任务、资源、版本与责任上下文。
4. 什么时候应该直接推进，什么时候必须确认，什么时候必须阻断。
5. 当协作发生交接、返工、阻塞、失败时，界面应该怎样表达。

### 1.2 与产品框架 spec 的关系

产品框架 spec 负责定义：

1. 产品目标与边界。
2. 核心模块。
3. 权限模型、资源模型、状态机。
4. 部署形态与技术落点。
5. V1、V1.5、V2 分期。

本文档负责定义：

1. 角色主旅程。
2. 默认入口与默认视图。
3. 页面职责与上下文继承。
4. 状态反馈、空状态、异常状态。
5. 协作、交接与可追溯体验。

### 1.3 文档范围

本文档重点覆盖：

1. V1 五类核心角色的真实工作流体验。
2. `Task Overview` 作为默认入口的组织方式。
3. `Data -> Annotation -> Review -> Publish -> Training -> Evaluation -> Artifact` 的主线体验。
4. Review 和 Evaluation 反馈如何以可用的界面行为回流到下一轮。
5. V1.5 / V2 的体验升级附录。

### 1.4 本文档不重点覆盖

本文档不展开：

1. 数据库设计。
2. API 结构。
3. Worker 协议细节。
4. 插件 runtime 内部实现。
5. 像素级视觉稿与组件 CSS 细节。
6. 单个弹窗字段级表单实现。

### 1.5 适用对象

本文档主要供以下对象使用：

1. 产品设计与交互设计，用于定义流程与页面职责。
2. 前端实现，用于确定信息层级、状态呈现、路由与组件行为。
3. 后端与平台实现，用于反推哪些上下文、状态、版本信息必须暴露给前端。

### 1.6 体验成功标准

V1 的体验是否成立，不取决于页面数量，而取决于以下问题是否能在系统里被自然回答：

1. 我现在该做什么。
2. 这个任务属于哪个版本上下文。
3. 我完成这个动作后，会推进到什么状态。
4. 接下来应该交给谁。
5. 如果系统阻塞了，是阻塞在哪一段链路。
6. 如果结果不可信，是因为哪一个版本、权限或评测基准不满足。

## 2. 体验设计原则

### 2.1 任务优先，而不是模块优先

V1 的默认入口必须是 `Task Overview`，而不是模块导航落地页。

登录后的第一反应应该是：

1. 当前有哪些待处理工作。
2. 哪些工作被阻塞。
3. 哪些工作需要我立即决策。

而不是：

1. 系统里有多少模块。
2. 每个模块能做什么 CRUD。

### 2.2 版本上下文必须始终可见

任何会影响正式决策的页面，都必须清晰显示至少一个或多个版本上下文：

1. `snapshot_id`
2. `ontology_version`
3. `calibration_version`
4. `benchmark_snapshot_id`
5. `checkpoint`
6. `artifact version`

如果用户正在做关键动作，但界面上看不到当前版本上下文，那么这个设计就是不合格的。

### 2.3 页面跳转必须继承上下文

用户在数据、任务、审核、训练、评测之间跳转时，不能频繁重新选择：

1. 当前项目。
2. 当前数据集。
3. 当前 snapshot。
4. 当前 task。
5. 当前 training run。
6. 当前 benchmark。

因此：

1. 页面跳转必须尽量继承上下文。
2. 关键上下文应进入 URL Search Params。
3. 页面刷新后应尽量恢复到相同状态。

### 2.4 生产工作台与管理页必须分层

V1 页面必须分清三类职责：

1. 列表页：发现、筛选、分配、批量操作。
2. 详情页：看上下文、看血缘、看状态、看历史。
3. 工作台：高频生产动作。

不能把所有操作塞进详情页，也不能把工作台做成只是一个列表弹窗。

### 2.5 关键动作必须明确结果状态

对以下动作，系统不能只提示“成功”：

1. 提交任务。
2. Reject / Rework。
3. Publish。
4. TrainingRun 创建。
5. Evaluation 完成。
6. Artifact Promotion。

用户必须能看到：

1. 结果状态是什么。
2. 谁现在是下一责任方。
3. 是否已经进入正式链路。
4. 是否还缺少某个版本约束或审批约束。

### 2.6 人工审核永远强于模型建议

界面必须始终强化：

1. AI 是候选来源，不是事实来源。
2. Candidate 不等于 Canonical。
3. Publish 前的数据一定处于人审控制之下。

因此 AI 结果在界面上的表达应有明确身份区分，例如：

1. 来源模型。
2. confidence。
3. 生成时间。
4. 是否被人工接受、修改或拒绝。

### 2.7 结构化反馈优先于自由文本

系统必须引导 Reviewer 提供机器可消费反馈。

Reject 或 Rework 时：

1. `reason_code` 必填。
2. 自由文本只是补充备注。
3. `severity` 和 `influence_weight` 可作为进一步结构化输入。

### 2.8 高密度，但不能混乱

这是一个生产工具，不是营销页面。

V1 的界面应当：

1. 信息密度高。
2. 行为路径短。
3. 版本上下文常驻可见。
4. 支持键盘优先。
5. 颜色承担语义，而不是装饰。

但也必须避免：

1. 大量无层级数字堆砌。
2. 卡片墙。
3. 重复显示同一类信息。
4. 需要到处展开才能知道当前状态。

### 2.9 阻塞要可见、可解释、可处理

阻塞不是“报个错”就结束。

系统对阻塞必须同时提供三件事：

1. 是什么阻塞。
2. 为什么阻塞。
3. 现在应该去哪处理。

`Blockers View` 和 `Blocker Card` 都必须遵守这一原则。

### 2.10 可分享、可回放、可追溯

如果一个关键工作态无法通过 URL 分享给同事，或者刷新后丢失上下文，这个体验就不适合异步协作系统。

关键页面必须支持：

1. URL 深链。
2. 版本与筛选条件恢复。
3. 回到来源页面时的上下文恢复。

## 3. 全局旅程地图

### 3.1 全局生产主线

V1 的全局生产主线应在用户心智上呈现为一条连续链路：

1. 数据接入。
2. 数据整理与 snapshot。
3. 标注任务执行。
4. 审核与返工。
5. 发布正式 snapshot。
6. 训练与评测。
7. 推荐 artifact。
8. 反馈回流下一轮。

系统应让用户随时知道自己当前处于这条链路的哪一段。

### 3.2 默认角色入口

虽然系统默认首页统一是 `Task Overview`，但每类角色看到的重点不同。

#### Project Owner

重点看到：

1. 生产阻塞。
2. 发布节奏。
3. 待审批 artifact。
4. 最近训练结果变化。

#### Data Manager

重点看到：

1. 数据接入状态。
2. snapshot 创建状态。
3. ontology / schema / calibration 风险。
4. 哪些数据还未进入任务链路。

#### Annotator

重点看到：

1. 我的任务。
2. 即将超时任务。
3. 被退回任务。
4. AI 候选待处理任务。

#### Reviewer

重点看到：

1. Review Queue。
2. 按风险或优先级排序的待审项。
3. Reject 原因分布。
4. 可发布候选集合。

#### ML Engineer

重点看到：

1. 最新 TrainingRun。
2. 失败任务。
3. Evaluation Compare 入口。
4. 待提名 / 待审批模型结果。
5. 来自 Review / Evaluation 的反馈项。

### 3.3 全局导航模型

V1 的一级导航应稳定保持为：

1. `Overview`
2. `Data`
3. `Tasks`
4. `Review`
5. `Training`
6. `Artifacts`

设置类入口进入独立设置域，不占据生产导航的主心智。

### 3.4 全局上下文条

系统应在关键页面保留一条轻量但持续存在的上下文信息带。

推荐包含：

1. 当前 Project。
2. 当前 Dataset 或 Task。
3. 当前 Snapshot。
4. 当前用户角色。
5. 当前页面的关键状态。

这一层不是面包屑替代品，而是帮助用户确认自己正在哪条正式链路里。

### 3.5 全局搜索与切换

V1 不要求完整的全局命令面板，但必须至少支持以下能力中的一种或多种：

1. 数据集快速切换。
2. 任务快速定位。
3. TrainingRun 快速跳转。
4. Artifact 快速跳转。

目标不是做“全站搜索产品”，而是减少用户靠左侧导航层层点击的成本。

### 3.6 通知与工作提醒

V1 的通知应以“可执行提醒”为目标，而不是堆消息中心。

高优先级提醒至少包括：

1. 任务被退回。
2. Review Queue 积压异常。
3. Snapshot 已发布但迟迟未进入训练。
4. TrainingRun 失败。
5. Evaluation invalid。
6. Recommended Artifact 待审批。

每条提醒都应支持一跳进入处理页。

### 3.7 阻塞视图

`Blockers View` 是 `Task Overview` 的核心区块之一，不是附属卡片。

它必须至少发现以下类型：

1. Review 积压过高。
2. 返工率异常。
3. 已发布 snapshot 长时间未训练。
4. TrainingRun 连续失败。
5. Evaluation 缺失 benchmark。
6. Artifact 已 revoke 或 deprecated 但仍被引用。
7. `Longest Idle Task`。
8. Ontology / Calibration 异常阻断。

其中 `Longest Idle Task` 的设计目标是暴露“无声阻塞”，即没有报错、没有失败、但已经被遗忘的工作。

## 4. V1 五类核心角色旅程

### 4.1 Data Manager 旅程

#### 角色目标

Data Manager 的核心目标不是“上传文件”，而是：

1. 把一批数据变成可追溯、可分发、可下游消费的正式数据上下文。
2. 确保 `Dataset / Asset / Snapshot / Ontology / Calibration` 一致且可信。
3. 把数据顺利交给标注或训练链路。

#### 默认入口

Data Manager 登录后进入 `Task Overview`，默认高亮数据相关区块：

1. 未完成数据接入。
2. 待构建 snapshot。
3. 数据 schema 风险。
4. 仍未被任务消费的数据集。

#### 主线步骤

1. 进入 `Dataset List` 创建或选择数据集。
2. 绑定对象存储来源或导入入口。
3. 触发 scan / import。
4. 在 `Dataset Detail` 检查资产规模、导入状态、异常数据。
5. 配置或确认 ontology、schema、calibration 摘要。
6. 创建 snapshot。
7. 检查 `Snapshot Detail` 和 `Snapshot Diff`。
8. 将 dataset / snapshot 交给任务创建或训练链路。

#### 关键界面期待

Data Manager 在 `Dataset Detail` 必须立即看到：

1. 数据规模。
2. 数据类型。
3. 最近 snapshot。
4. 相关任务数量。
5. Import / Export 运行状态。
6. Ontology / Calibration 风险提示。

#### 关键决策点

Data Manager 在 V1 的关键决策通常有三类：

1. 数据是否足以形成正式 snapshot。
2. 当前 schema / ontology 是否可交给标注团队。
3. 当前 snapshot 是否可作为后续任务或训练的版本起点。

#### 关键阻塞态

系统必须明确提示：

1. Scan 失败。
2. Import 部分成功。
3. Snapshot building 卡住。
4. Ontology 缺失。
5. Calibration invalid。

阻塞提示要直接指向：

1. 重新扫描。
2. 查看导入日志。
3. 修复配置。
4. 联系 Project Owner 或 Reviewer 的后续步骤。

### 4.2 Annotator 旅程

#### 角色目标

Annotator 的目标不是“浏览数据”，而是：

1. 快速知道自己今天该处理哪些任务。
2. 在不丢上下文的情况下完成高频标注。
3. 明确知道哪些对象是 AI 候选、哪些是自己修改的结果。
4. 在提交前完成基本自检。

#### 默认入口

Annotator 默认进入 `Task Overview`，但最重要入口是 `My Tasks`。

首页应将标注员的注意力直接拉向：

1. 今天待处理任务。
2. 即将超时任务。
3. 被退回任务。
4. AI 辅助候选较多的任务。

#### 主线步骤

1. 从 `My Tasks` 进入任务。
2. 在 `Task Detail` 确认任务描述、目标、当前 snapshot。
3. 进入 `Annotation Workspace`。
4. 查看当前 asset 或 frame，以及 AI candidate panel。
5. 处理候选：
   - 接受。
   - 修改后接受。
   - 忽略或删除。
6. 补全人工标注。
7. 通过对象列表和时间轴自检一致性。
8. 检查 `Object Persistence Checker` 提示。
9. 提交任务。

#### Workspace 核心体验要求

Annotator 的主工作区必须优先满足效率，而不是“看起来像设计稿”。

必须做到：

1. 切帧快。
2. 选对象快。
3. 工具切换快。
4. AI 候选来源清楚。
5. 自动保存不打断操作。
6. 当前任务、当前 snapshot 和当前 frame 始终可见。

#### Annotator 需要看到的上下文

在 Workspace 右侧或固定信息区，应持续显示：

1. 当前 Task。
2. 当前 Dataset / Asset。
3. 当前 Snapshot。
4. 当前 Ontology Version。
5. 当前对象数量或候选数量。
6. 自动保存 / 提交状态。

#### 视频标注的关键体验

视频不是把图片列表换成视频播放器这么简单。

V1 至少要支持：

1. 时间轴定位。
2. 关键帧标记。
3. 基础插值。
4. 轨迹连续性可视化。
5. 对象 ID 持续性检查。

`Object Persistence Checker` 应提醒：

1. 某个对象 ID 中途消失。
2. 两个关键帧之间出现异常空洞。
3. 插值结果跳变。
4. 标注轨迹与前后帧明显不一致。

#### 标注员的完成感

Annotator 提交后，系统必须明确告诉他：

1. 当前任务已进入 `submitted`。
2. 下一责任方是 Reviewer。
3. 如果被退回，会在哪里重新接收。

### 4.3 Reviewer / QA 旅程

#### 角色目标

Reviewer 的目标不是简单点“通过/不通过”，而是：

1. 控制正式数据质量。
2. 将 Reject 或 Rework 转成结构化反馈。
3. 决定哪些任务可以进入发布。
4. 发现 QA 层面的系统性问题。

#### 默认入口

Reviewer 登录后仍从 `Task Overview` 进入，但首页重点是：

1. `Review Queue`
2. 高风险待审项
3. 大量积压任务
4. 可发布候选集合
5. 最近 Reject 原因分布

#### 主线步骤

1. 从 `Review Queue` 选择一项待审工作。
2. 进入 `Review Workspace` 查看改动、候选、来源和上下文。
3. 决定：
   - Accept
   - Reject
   - Rework
   - Escalate
4. 若 Reject 或 Rework，则填写结构化 `reason_code`。
5. 可选填写 `severity`、`influence_weight`、自由备注。
6. 完成一批任务后进入 `Publish Candidates`。
7. 核对 QA 状态和发布影响范围。
8. 发起或参与发布。

#### Review Queue 的体验重点

`Review Queue` 不是普通列表，而是 Reviewer 的主工作台。

它必须支持按以下维度快速排序或过滤：

1. 风险。
2. 优先级。
3. 候选来源模型。
4. Annotator。
5. Task。
6. Snapshot。
7. Reject 历史。

#### Review Workspace 的体验重点

Review Workspace 与 Annotation Workspace 共享渲染基础，但体验重心完全不同。

Reviewer 需要优先看到：

1. 本次变更的重点对象。
2. 候选的来源模型与 confidence。
3. 上次修改者。
4. 与历史版本的差异。
5. 当前任务是否适合进入正式发布。

#### Reject / Rework 的体验要求

Reject 与 Rework 不能是一个简单文本框。

系统应强制提供结构化路径：

1. `reason_code` 必填。
2. 可选 `severity`。
3. 可选 `influence_weight` 建议。
4. 自由文本作为补充。

这样做的目的不是多填表，而是让后续：

1. Feedback Bus 可消费。
2. Active Learning 可排序。
3. QA 统计可聚合。
4. 团队能看见系统性问题。

#### Reviewer 的完成感

Reviewer 完成审核后，系统必须明确表示：

1. 该任务是进入 `accepted` 还是 `rework_required`。
2. 若可发布，是否已经进入 `Publish Candidates`。
3. 若不能发布，是卡在什么条件上。

### 4.4 ML Engineer 旅程

#### 角色目标

ML Engineer 的目标不是“在页面上看个图”，而是：

1. 基于可信 snapshot 启动训练。
2. 把训练结果和环境上下文正规回传平台。
3. 在统一 benchmark 上对比多个结果。
4. 提名推荐 artifact。
5. 消费来自 Review / Evaluation 的反馈。

#### 默认入口

ML Engineer 登录后的首页重点应是：

1. 最近训练状态。
2. 失败任务。
3. 评测对比入口。
4. 待提名 checkpoint。
5. 需要处理的反馈项。

#### 主线步骤

1. 从 `Task Overview` 或 `Snapshot Detail` 进入训练链路。
2. 选择已发布 snapshot。
3. 生成命令或创建 `TrainingRun`。
4. 在外部终端、远程环境或受控后端执行训练。
5. 通过 CLI / SDK 回传：
   - logs
   - curves
   - checkpoints
   - evaluation
   - environment context
6. 进入 `Training Run Detail` 查看状态与输出。
7. 进入 `Evaluation Compare` 进行同 benchmark 对比。
8. 选择更优 checkpoint。
9. 进入 `Recommended Model Promotion` 发起提名。

#### Training Run Detail 的体验重点

ML Engineer 打开 `Training Run Detail` 时最关心的是：

1. 这个 run 绑定哪个 snapshot。
2. 它有没有在跑。
3. 为什么失败。
4. 日志和曲线是否正在回传。
5. 产生了哪些 checkpoint。
6. 哪些 checkpoint 已进入评测。

因此页面必须优先回答这六个问题，而不是先展示静态元信息表。

#### Evaluation Compare 的体验重点

`Evaluation Compare` 是 ML Engineer 的核心决策页之一。

页面必须把“能不能比”放在“谁更高分”之前。

用户首先看到的应该是：

1. `benchmark_snapshot_id`
2. 参与对比的 checkpoint
3. 评测协议与环境摘要
4. 是否可直接横向对比

如果不可对比：

1. 不给直接排名。
2. 明确写出原因。
3. 提供 “创建对齐评测” 入口。

#### 推荐模型提名

ML Engineer 完成对比后，可以从 Compare 或 TrainingRun 详情页进入 `Recommended Model Promotion`。

这一步的体验目标是：

1. 让提名理由结构化。
2. 明确提名的 checkpoint 与 artifact 来源。
3. 明确需要 Project Owner 审批。

### 4.5 Project Owner 旅程

#### 角色目标

Project Owner 的目标不是具体执行标注或训练，而是：

1. 看清项目当前卡在哪。
2. 决定何时推进发布。
3. 决定何时认可推荐模型。
4. 控制节奏、质量和资源优先级。

#### 默认入口

Project Owner 的首页体验重点必须集中在 `Task Overview`。

优先看到：

1. 阻塞视图。
2. 发布节奏。
3. 审核积压。
4. 最近训练变化。
5. 待审批 artifact。
6. `Longest Idle Task`。

#### 主线步骤

1. 从 `Task Overview` 判断项目当前瓶颈。
2. 若数据或审核阻塞，则进入对应列表或详情页追责和协调。
3. 若 snapshot 已发布，则关注 TrainingRun 是否及时启动。
4. 若 ML Engineer 已提名推荐模型，则进入 `Recommended Model Promotion`。
5. 审核推荐理由、评测依据、风险备注。
6. 决定是否批准 artifact 成为推荐版本。

#### Project Owner 需要的体验重点

Project Owner 的界面不应要求深入每个模块看细节才能做决策。

系统必须先聚合为：

1. 当前阶段。
2. 主要瓶颈。
3. 最近风险。
4. 下一决策动作。

他只在需要时再深入详情。

#### 审批体验

Project Owner 在审批推荐 artifact 时，应清晰看到：

1. 这是从哪个 checkpoint 来的。
2. 基于哪个 benchmark snapshot 得出的结论。
3. 是否存在不可忽略的风险备注。
4. 这次审批会把系统推进到什么状态。

## 5. 关键跨角色交接点

### 5.1 Data Manager -> Annotator

触发条件：

1. Dataset 已完成整理。
2. Snapshot 已可供任务使用。
3. Ontology / Schema 基础约束已就位。

交接方式：

1. 创建任务或任务集。
2. 让 `Task List / My Tasks` 可以直接消费当前 snapshot。

交接时必须带上的上下文：

1. Dataset。
2. Snapshot。
3. Ontology Version。
4. 任务说明。
5. 优先级。

### 5.2 Annotator -> Reviewer

触发条件：

1. Task 被提交。

Reviewer 接收面：

1. `Review Queue`

交接时必须带上的上下文：

1. 提交人。
2. 当前 snapshot。
3. 本次变更摘要。
4. AI candidate 来源信息。
5. 是否存在对象持续性异常。

### 5.3 Reviewer -> Annotator

触发条件：

1. Reject。
2. Rework。

Annotator 接收面：

1. `My Tasks`

交接时必须带上的上下文：

1. 结构化 `reason_code`
2. Reviewer 备注。
3. 哪些对象或帧存在问题。
4. 当前状态已变为 `rework_required`。

### 5.4 Reviewer -> Training Chain

触发条件：

1. 一组任务通过审核并进入可发布状态。
2. Publish 完成。

训练链接收面：

1. `Task Overview`
2. `Snapshot Detail`
3. `Training` 域相关入口

交接时必须带上的上下文：

1. 新 `published snapshot`
2. diff 摘要
3. 发布责任人
4. 下游影响范围

### 5.5 ML Engineer -> Project Owner

触发条件：

1. 有多个可比较评测结果。
2. 某个 checkpoint 被提名为推荐结果。

Project Owner 接收面：

1. `Task Overview`
2. `Recommended Model Promotion`

交接时必须带上的上下文：

1. 被提名 checkpoint
2. `benchmark_snapshot_id`
3. 比较依据
4. 风险备注

### 5.6 Review / Evaluation -> Learning Loop

触发条件：

1. Reject。
2. QA 失败。
3. Evaluation 暴露困难样本。

接收面：

1. 反馈视图
2. 数据与训练相关入口

交接时必须带上的上下文：

1. `reason_code`
2. `severity`
3. `influence_weight`
4. 来源 snapshot
5. 来源 annotation / candidate / evaluation

## 6. 关键页面体验规则

### 6.1 Task Overview

#### 页面目标

统一告诉用户：

1. 现在有哪些关键工作。
2. 当前主阻塞在哪。
3. 哪些工作需要我立即决策。

#### 主问题

Task Overview 必须优先回答：

1. 今天我该做什么。
2. 项目当前卡在哪。
3. 哪些版本正处在活跃流转中。
4. 哪些输出值得关注。

#### 必备区块

1. 角色化待办。
2. Review 积压。
3. 数据生产状态。
4. 训练与评测最近变化。
5. 推荐 artifact 状态。
6. `Blockers View`。
7. `Longest Idle Task`。

#### 绝对不要做成

1. 空 BI 面板。
2. 大面积欢迎卡片。
3. 没有操作入口的统计汇总。

### 6.2 Dataset List

#### 页面目标

快速帮助 Data Manager 和 Project Owner 看清：

1. 当前有哪些数据集。
2. 哪些数据集处于什么阶段。
3. 哪些数据集存在风险或停滞。

#### 必看字段

1. 名称。
2. 类型。
3. 规模摘要。
4. 最近 snapshot。
5. 状态。
6. 负责人。

### 6.3 Dataset Detail

#### 页面目标

帮助用户理解一个 dataset 当前能不能进入任务链路，或者为什么还不行。

#### 必备信息

1. 资产摘要。
2. 数据分布摘要。
3. 导入导出记录。
4. 相关 task。
5. 相关 snapshot。
6. ontology / schema / calibration 摘要。

#### 必备动作

1. 创建 snapshot。
2. 查看 snapshot diff。
3. 创建任务。
4. 查看导入日志。

### 6.4 Snapshot Detail

#### 页面目标

帮助用户判断一个 snapshot 是否可信、是否正式、是否已被下游消费。

#### 必备信息

1. Snapshot 状态。
2. 发布状态。
3. diff 摘要。
4. 与 dataset 的绑定。
5. 被哪些 TrainingRun / Evaluation / Artifact 引用。

### 6.5 Snapshot Diff

#### 页面目标

Diff 页面不只是版本比对，而是为发布、训练和评测服务。

#### 必备信息

1. 标注增删改统计。
2. 类别分布变化。
3. 问题样本入口。
4. 对下游训练的影响提示。

### 6.6 Task List

#### 页面目标

帮助负责人、数据管理员和审核员看清任务结构、积压与风险。

#### 必备能力

1. 状态筛选。
2. 分配视图。
3. 返工率查看。
4. 优先级与 SLA 排序。

### 6.7 My Tasks

#### 页面目标

这是 Annotator 的主操作入口，必须尽量简洁。

#### 优先顺序

1. 即将超时。
2. 被退回。
3. 当前进行中。
4. 新分配待领取。

#### 不应混入

1. 大量治理信息。
2. 无关指标。
3. 复杂统计视图。

### 6.8 Task Detail

#### 页面目标

在进入 Workspace 之前，告诉用户：

1. 这个任务是什么。
2. 绑定哪个 snapshot。
3. 谁负责。
4. 现在在哪个状态。

### 6.9 Annotation Workspace

#### 页面目标

让 Annotator 在高频操作中保持稳定效率。

#### 布局原则

主工作区通常分为：

1. 中心画布区。
2. 左侧工具区。
3. 右侧对象与上下文区。
4. 底部或顶部时间轴与帧导航。

#### 必备信息

1. 当前 task。
2. 当前 asset / frame。
3. 当前 snapshot。
4. 当前 ontology version。
5. autosave 状态。
6. submit 状态。

#### 必备行为

1. 快捷键优先。
2. 自动保存异步化。
3. AI candidate 可一键接受、修改或忽略。
4. 对象列表与时间轴虚拟化。
5. 支持从错误帧快速跳回问题位置。

### 6.10 Review Queue

#### 页面目标

让 Reviewer 快速定位最值得先审的内容。

#### 必备排序维度

1. 风险。
2. 优先级。
3. 候选来源。
4. Annotator。
5. 版本。
6. 积压时间。

### 6.11 Review Workspace

#### 页面目标

让 Reviewer 快速判断当前结果是否可接受、是否应返工，以及为什么。

#### 必备信息

1. 当前变更重点。
2. confidence。
3. 来源模型。
4. 上次修改者。
5. 当前 snapshot。

#### 必备动作

1. Accept。
2. Reject。
3. Rework。
4. Escalate。

#### Reject 表单要求

1. `reason_code` 必填。
2. 备注可选。
3. `severity` 可选。
4. `influence_weight` 建议值可选。

### 6.12 Publish Candidates

#### 页面目标

帮助 Reviewer 和 Project Owner 理解：

1. 哪批任务已经具备发布条件。
2. 这次发布会带来什么影响。
3. 发布后哪些下游链路将被触发。

#### 必备信息

1. 候选任务组。
2. QA 状态。
3. diff 范围。
4. 下游影响提示。

### 6.13 Training Run List

#### 页面目标

让 ML Engineer 和 Project Owner 快速看清：

1. 训练是否稳定。
2. 哪些 run 失败。
3. 哪些 run 值得继续看。

### 6.14 Training Run Detail

#### 页面目标

将一个训练的“运行态 + 版本态 + 输出态”统一放在一页。

#### 信息优先级

优先展示：

1. Bound Snapshot。
2. 当前状态。
3. 最近日志。
4. 曲线。
5. checkpoint 列表。
6. 失败原因或环境上下文。

### 6.15 Evaluation Compare

#### 页面目标

帮助用户判断：

1. 这些 checkpoint 能不能比。
2. 如果能比，谁更合适。
3. 如果不能比，应如何重新建立可比性。

#### 必备信息

1. `benchmark_snapshot_id`
2. 参与比较的 checkpoint
3. 指标表
4. 样本级对比
5. 环境与协议

#### 关键行为

1. 同 benchmark 时允许直接比较。
2. 不同 benchmark 时明确显示不可直接比较。
3. 提供 “创建对齐评测” 入口。

### 6.16 Recommended Model Promotion

#### 页面目标

把模型推荐变成一个正式决策，而不是一句口头结论。

#### 必备信息

1. 被提名 checkpoint。
2. 比较依据。
3. 推荐理由。
4. 风险备注。
5. 审批责任人。

### 6.17 Artifact Registry

#### 页面目标

帮助用户理解：

1. 当前有哪些可消费 artifact。
2. 它们来自哪里。
3. 哪些已经被推荐、废弃或撤回。

### 6.18 Artifact Detail

#### 页面目标

让用户清楚知道：

1. 这个 artifact 的来源链。
2. 如何下载和使用。
3. 当前是否可信和推荐。

#### 必备信息

1. 来源 checkpoint / evaluation / snapshot。
2. manifest 摘要。
3. 下载命令。
4. revoke / deprecate 状态。

### 6.19 CLI / SDK Access

#### 页面目标

让工程师快速拿到可执行命令，而不是被迫翻文档。

#### 必备内容

1. Pull 命令。
2. Upload 命令或接入方式。
3. Token 使用方式。
4. 环境变量模板。
5. 训练脚本集成示例。

## 7. 状态反馈、空状态与异常状态

### 7.1 加载状态

V1 的加载态应遵循：

1. 先显示结构。
2. 再逐步填内容。
3. 避免整页白屏。

对长列表、训练详情、评测对比页尤其重要。

### 7.2 空状态

空状态不能只写“暂无数据”。

应回答：

1. 为什么为空。
2. 下一步做什么。
3. 谁来做。

例如：

1. 无任务：去创建任务或等待分配。
2. 无 Review 候选：可能因为没有提交，或者筛选条件过严。
3. 无 Evaluation：可能还没上传结果，或者 benchmark 不完整。

### 7.3 成功状态

对以下动作应提供可确认成功反馈：

1. 创建 snapshot。
2. 提交 task。
3. 完成 review。
4. publish 完成。
5. TrainingRun 创建成功。
6. 上传 checkpoint 完成。
7. artifact 晋级完成。

成功反馈除提示外，应尽量提供：

1. 结果状态。
2. 下一跳入口。
3. 后续责任人。

### 7.4 警告状态

警告态用于告诉用户“当前仍可继续，但风险正在升高”，例如：

1. 某个 task 即将超时。
2. 视频对象持续性异常但仍可提交。
3. training log 长时间无更新。
4. 推荐 artifact 存在风险备注。

### 7.5 阻塞状态

阻塞态意味着用户当前不能继续推进正式链路。

典型阻塞包括：

1. Snapshot 未发布。
2. Calibration invalid。
3. Review 未完成。
4. Benchmark snapshot 不一致。
5. 权限不足。
6. Artifact 已 revoke。

阻塞态必须同时告诉用户：

1. 当前为什么不能继续。
2. 谁有权限解决。
3. 可以跳去哪个页面处理。

### 7.6 错误状态

错误态不能只显示“请求失败”。

至少应区分：

1. 权限错误。
2. 版本不一致。
3. 资源不存在。
4. 后台任务失败。
5. 上传失败。
6. 对比不成立。

### 7.7 异步任务状态

对 scan、import、snapshot build、training、evaluation、export 这类异步动作：

1. 需要有明显运行态。
2. 需要能查看事件流或日志。
3. 需要明确是暂时失败还是最终失败。
4. 需要明确是否可重试。

### 7.8 本地草稿与恢复

对 Workspace：

1. 自动保存状态必须可见。
2. 若本地暂存与服务器状态不一致，应给出恢复选项。
3. 若页面刷新，应尽量恢复最近上下文。

### 7.9 评测不可比状态

当用户尝试比较不同 benchmark snapshot 的结果时：

1. 不允许给直接排名。
2. 页面应以明显警告态说明不成立原因。
3. 提供创建对齐评测的下一步入口。

## 8. 协作、权限与版本可见性体验要求

### 8.1 协作原则

这个系统的协作不是“大家都能看”，而是“每个人看见自己该接的工作”。

因此界面应尽量体现：

1. 当前责任方。
2. 上一责任方。
3. 下一责任方。
4. 当前状态。

### 8.2 权限在界面中的表达

对不可执行动作，界面不能只灰掉按钮。

应尽量说明：

1. 为什么不可执行。
2. 是角色问题、scope 问题，还是资源状态问题。
3. 谁可以执行。

### 8.3 Guest / Contractor 体验

V1 对 Guest / Contractor 不要求完整管理体验，但必须满足：

1. 只看得到被授权项目和任务。
2. 页面上能明显感知自己是受限访问。
3. 到期或被收回权限时有清晰提示。

### 8.4 Service Account 与 CLI 体验

Service Account 不进入 UI 主流程，但系统对其行为必须可见。

用户在 Training 或 Artifact 相关页面中应能看到：

1. 某次上传是否来自 CLI / SDK / Service Account。
2. 它绑定的是哪个 TrainingRun。
3. 是否存在权限越界风险。

### 8.5 版本可见性规则

以下类型页面必须强制显示版本上下文：

1. Task Detail
2. Annotation Workspace
3. Review Workspace
4. Snapshot Detail
5. Training Run Detail
6. Evaluation Compare
7. Recommended Model Promotion
8. Artifact Detail

### 8.6 AI 与人工来源区分

界面必须始终能区分：

1. 人工标注。
2. AI 候选。
3. 人工接受后的 AI 结果。
4. 人工修改后的 AI 结果。

否则系统的信任链会在体验层被模糊掉。

### 8.7 审计与可追溯入口

V1 不需要把每一页都做成审计中心，但关键详情页必须能看到：

1. 最近状态变更。
2. 最近责任人。
3. 最近关键动作。

至少做到“知道应该去哪看完整审计”。

## 9. 通用交互模式与共享组件行为

### 9.1 Entity Header

用于展示页面当前主体资源。

必须能快速告诉用户：

1. 这是哪个资源。
2. 当前处于什么状态。
3. 当前属于哪个项目或数据链路。

### 9.2 Version Badge

用于统一表达：

1. Snapshot。
2. Ontology Version。
3. Calibration Version。
4. Artifact Version。

点击后应优先跳转到相应详情或 diff，而不是静态标签。

### 9.3 State Timeline

用于帮助用户理解：

1. 资源已经走到哪一步。
2. 下一步是什么。
3. 是否在异常态或返工态。

### 9.4 Diff Summary Panel

用于解释：

1. 当前版本相对上一个版本的主要变化。
2. 为什么这个变化值得发布或训练。

### 9.5 Lineage Breadcrumb

用于表达资源血缘。

最常见应至少支持：

1. Artifact -> Evaluation -> TrainingRun -> Snapshot
2. Snapshot -> Dataset
3. Task -> Snapshot -> Dataset

### 9.6 Blocker Card

用于 `Task Overview` 与相关列表页。

必须包含：

1. 阻塞标题。
2. 停留时长或风险级别。
3. 简短原因。
4. 直接处理入口。

### 9.7 Reason Code Tag

用于 review 反馈与 QA 聚合。

应支持：

1. 语义化颜色。
2. 统一文案。
3. 在列表、详情和聚合视图中保持一致。

### 9.8 Confidence Badge

用于 AI candidate 和模型输出。

应强调它是“参考信号”，不是最终判定。

### 9.9 Run Status Strip

用于 TrainingRun、EvaluationRun 等长时任务。

应统一表达：

1. 当前状态。
2. 最近更新时间。
3. 是否异常。
4. 是否可重试。

### 9.10 Promotion Decision Panel

用于 `Recommended Model Promotion`。

应同时呈现：

1. 推荐对象。
2. 推荐依据。
3. 风险说明。
4. 决策动作。

## 10. V1.5 / V2 体验升级附录

### 10.1 V1.5 体验升级方向

V1.5 的体验升级重点不是加大量新页面，而是让主链路更聪明、更省协作成本。

#### Blockers View 升级

从规则式阻塞升级到趋势式阻塞，例如：

1. 审核员容量不足趋势。
2. 某类任务返工率持续上升。
3. 发布后长期无训练的异常提醒。

#### Active Learning 入口升级

在 V1 的反馈回流基础上，V1.5 可以增加：

1. 难例优先任务集。
2. 负采样推荐集。
3. 低价值任务后移提示。

#### Evaluation Compare 升级

V1.5 可以在用户显式确认后支持：

1. 自动寻找可对齐交集。
2. 自动发起交集 benchmark 重评。
3. 返回新的可比结果集。

#### Guest / Contractor 体验升级

可增加：

1. 到期提醒。
2. 受限能力解释。
3. 外包任务专属视图。

### 10.2 V2 体验升级方向

V2 开始进入“平台化与扩展化”的体验。

#### 点云插件工作台

V2 可以引入：

1. 点云工作台入口。
2. 2D / 3D 同步定位体验。
3. 插件化多视图工作区。

#### 插件市场体验

可增加：

1. 私有插件目录。
2. 官方插件目录。
3. 插件能力声明与兼容信息展示。

#### 更强的交付与 benchmark 体验

可增加：

1. 多硬件 benchmark 对比页。
2. 模型转换流水线可视化。
3. 部署建议与适配说明。

#### 更强的组织与运维视角

V2 可以增加更完整的：

1. 组织级生产总览。
2. 成本与容量趋势。
3. 插件健康与风险。
4. 多项目横向效率对比。

## 11. 体验验收清单

### 11.1 V1 体验必须满足的事情

V1 的体验至少要让以下行为顺畅完成：

1. Data Manager 能从数据接入自然走到 snapshot。
2. Annotator 能从 `My Tasks` 一步进入 Workspace，并清楚知道当前版本上下文。
3. Reviewer 能从 `Review Queue` 一步进入 Review Workspace，并以结构化方式 Reject。
4. ML Engineer 能基于已发布 snapshot 发起或登记训练，并把输出回传平台。
5. ML Engineer 能在同一 `benchmark_snapshot_id` 下对比多个 checkpoint。
6. Project Owner 能从 `Task Overview` 发现阻塞并审批推荐 artifact。
7. 用户能通过 URL 把关键工作态分享给同事。
8. 系统能在关键页面上持续显示版本与责任上下文。

### 11.2 如果这些情况仍然存在，说明体验没有成立

如果用户仍然需要靠下面这些方式才能完成主线工作，说明 V1 体验还不够成熟：

1. 用表格记录任务卡在哪。
2. 用私聊解释为什么 Reject。
3. 用截图比较两个 checkpoint。
4. 用文件命名猜哪个模型是当前推荐版本。
5. 页面刷新后丢失关键上下文。
6. 无法判断某次训练到底绑定了哪个 snapshot。

## 12. 结论

这份用户旅程与交互体验 spec 的核心目标，是把平台从“功能集合”变成“可协作、可交接、可判断、可追溯的生产界面”。

对于 V1，最重要的不是页面做得多，而是以下五件事成立：

1. 首页就是工作入口，而不是装饰性仪表盘。
2. 关键角色都能在系统里完成自己的真实主线工作。
3. 版本上下文始终可见，不让用户在盲态下做关键决策。
4. Reject、Publish、Evaluate、Promote 这些动作都有明确状态反馈与责任交接。
5. 界面天然支持协作，而不是要求用户靠群聊把流程补齐。

V1.5 与 V2 的体验升级，应该建立在这条 V1 主链路已经稳定、可信、可用的前提之上，而不是反过来用未来愿景稀释当前体验。
