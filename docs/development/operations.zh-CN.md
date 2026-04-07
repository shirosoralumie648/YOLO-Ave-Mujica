# 本地运维与排障

## 目标

当前仓库的最小可观测性基线用于回答三个问题：

1. 请求有没有进来，落到了哪条路由。
2. job 有没有被创建、完成、重试恢复。
3. 队列和 review backlog 当前是不是在堆积。

## 访问日志

API 进程现在会输出 JSON 访问日志，核心字段包括：

- `component`
- `event`
- `trace_id`
- `method`
- `path`
- `route`
- `status`
- `duration_ms`

如果请求头里带了 `X-Request-Id`，服务端会复用这个值，并同步写回 `X-Request-Id` / `X-Correlation-Id` 响应头。

## Trace 关联

建议对所有人工触发的写请求主动带上 request id：

```bash
curl -fsS -X POST http://127.0.0.1:8080/v1/jobs/zero-shot \
  -H 'Content-Type: application/json' \
  -H 'X-Request-Id: ops-trace-001' \
  -d '{"project_id":1,"dataset_id":1,"snapshot_id":1,"prompt":"person","idempotency_key":"ops-demo"}'
```

这个 `trace_id` 会进入 job payload，并由 worker callback 继续带回 `/internal/jobs/*` 路径，所以同一条链路可以在 API 日志里按 request id 串起来。

## Metrics

查看当前指标：

```bash
curl -fsS http://127.0.0.1:8080/metrics
```

当前重点指标：

- `yolo_http_requests_total`
- `yolo_job_creations_total`
- `yolo_job_completions_total`
- `yolo_job_duration_seconds_sum`
- `yolo_job_duration_seconds_count`
- `yolo_job_lease_recoveries_total`
- `yolo_artifact_build_outcomes_total`
- `yolo_queue_depth`
- `yolo_review_backlog`

## 常见排障动作

1. API 活性与依赖就绪：

```bash
curl -i http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/readyz
```

2. 检查队列是否堆积：

```bash
curl -fsS http://127.0.0.1:8080/metrics | rg 'yolo_queue_depth|yolo_review_backlog'
```

3. 检查某个 job 是否卡住：

```bash
curl -fsS http://127.0.0.1:8080/v1/jobs/<job_id>
curl -fsS http://127.0.0.1:8080/v1/jobs/<job_id>/events
```

4. 本地端到端回归：

```bash
bash scripts/dev/smoke.sh
```

## 典型故障信号

- `/readyz` 返回 `503`
  说明 PostgreSQL、Redis 或 MinIO 至少有一个不可用。
- `yolo_queue_depth` 持续增长
  说明 worker 没有消费、能力不匹配，或 callback 没有成功回写。
- `yolo_job_lease_recoveries_total` 持续增长
  说明 worker 经常超时或崩溃，需要结合 job events 看 `lease_recovered` 和 `lease_timeout`。
- `yolo_artifact_build_outcomes_total{status="failed"}` 增长
  说明 artifact build 或 storage 路径出现失败，优先检查 artifact 详情与 API 日志。
