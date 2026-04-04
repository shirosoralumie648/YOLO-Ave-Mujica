# YOLO-Ave-Mujica

A production-oriented YOLO platform foundation for Data Hub, task-first annotation operations, review flow, and artifact delivery.

## Documentation

- English local quickstart: `docs/development/local-quickstart.md`
- 简体中文总览: `README.zh-CN.md`
- 简体中文本地开发: `docs/development/local-quickstart.zh-CN.md`
- 简体中文架构说明: `docs/development/architecture.zh-CN.md`
- Technical audit snapshot: `docs/development/technical-audit-2026-04-04.md`

Detailed planning docs remain available under `docs/superpowers/` for implementation history and design context.

## Current Status

Implemented today:

- Go control plane entry points: `api-server`, `platform-cli`, `migrate`, `s3-bootstrap`
- Vite + React + TypeScript web console with overview, task, review, publish, data, and annotation workspace pages
- Data Hub APIs for dataset creation, scans, item listing, snapshots, snapshot detail, and object presign
- Task overview, task list/detail, annotation workspace draft/submit, and publish review flows
- Job primitives for idempotent create, lane dispatch, lease recovery, worker callbacks, and event listing
- Worker-side importer, packager, cleaning, zero-shot, and video contract runners
- Artifact packaging, resolve, archive download, presign, and CLI pull verification
- Local smoke checks, OpenAPI route guards, and migration guard tests

Not complete yet:

- `zero-shot` and `video` workers currently provide durable contract outputs, not real model-backed inference or media extraction pipelines
- Snapshot import supports `yolo` and `coco`; artifact export and package delivery are currently `yolo` only
- Authentication, RBAC, training/evaluation domains, and plugin runtime are still roadmap items

## Repository Layout

```text
cmd/                Entry points for api-server, platform-cli, migration, and local helpers
internal/           Go domain modules and runtime wiring
workers/            Python worker-side helper primitives and tests
migrations/         SQL schema bootstrap
deploy/docker/      Local PostgreSQL, Redis, and MinIO compose stack
scripts/dev/        Local smoke checks and helper scripts
docs/               Development docs, specs, and plans
```

## Quick Start

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
make web-install
make api-dev
```

In another terminal:

```bash
make web-dev
```

The web console opens on `http://127.0.0.1:5173` and proxies `/v1/*` to `http://127.0.0.1:8080` by default. The web shell uses root-scoped routes such as `/`, `/tasks`, `/review`, `/publish/candidates`, and `/data`, while the control-plane API stays project-scoped where appropriate under `/v1/projects/{id}/...`.

Run verification with:

```bash
make test
make web-build
bash scripts/dev/smoke.sh
```

See `docs/development/local-quickstart.md` for the full local runbook.

## Implemented API Surface

- `GET /v1/projects/{id}/overview`
- `GET /v1/projects/{id}/tasks`
- `POST /v1/projects/{id}/tasks`
- `GET /v1/tasks/{id}`
- `POST /v1/datasets`
- `POST /v1/datasets/{id}/scan`
- `POST /v1/datasets/{id}/snapshots`
- `GET /v1/datasets/{id}/snapshots`
- `GET /v1/datasets/{id}/items`
- `POST /v1/snapshots/{id}/import`
- `POST /v1/snapshots/{id}/export`
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
- `GET /healthz`
- `GET /readyz`

## CLI Artifact Delivery

`platform-cli pull --format <format> --version <version>` resolves a ready artifact, downloads the package archive, extracts it locally, and verifies every file declared in `manifest.json`.

The pull workflow writes `verify-report.json` with an `environment_context` block containing `os`, `arch`, `cli_version`, and `storage_driver`.

## Testing

Run all Go tests:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
```

Run the worker unit tests:

```bash
PYTHONPATH=. python3 -m unittest \
  workers.tests.test_partial_success \
  workers.tests.test_job_client \
  workers.tests.test_cleaning_rules -v
```

Run the web console tests:

```bash
cd apps/web && npm test
cd apps/web && npm run build
```
