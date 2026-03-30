# YOLO-Ave-Mujica

A production-oriented MVP foundation for dataset indexing, annotation workflow orchestration, and training artifact delivery.

## Documentation

- English local quickstart: `docs/development/local-quickstart.md`
- 简体中文总览: `README.zh-CN.md`
- 简体中文本地开发: `docs/development/local-quickstart.zh-CN.md`
- 简体中文架构说明: `docs/development/architecture.zh-CN.md`

## Current MVP Scope

- Go control plane entry points: `api-server`, `platform-cli`
- Data Hub APIs for dataset creation, scans, snapshots, item listing, and object presign
- Job primitives for idempotent create, lane dispatch, lease recovery, and event listing
- Review, diff, and artifact HTTP modules
- Artifact packaging, resolve, archive download, and CLI pull verification
- Python worker-side primitives for heartbeats, partial success accounting, and cleaning rules

Detailed planning docs remain available under `docs/superpowers/` for implementation history and design context.

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
make test
bash scripts/dev/smoke.sh
```

See `docs/development/local-quickstart.md` for the full local runbook.

## Implemented API Surface

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
