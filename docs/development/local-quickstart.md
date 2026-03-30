# Local Quickstart

## Prerequisites

- Go 1.20+
- Python 3.12+
- Docker for the default local PostgreSQL/Redis/MinIO stack, or equivalent local services already running on `5432`, `6379`, and `9000`

## Start local dependencies

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

`make up-dev` also bootstraps the default MinIO bucket (`platform-dev`). If Docker is not installed, `make up-dev` is skipped. In that case you need your own PostgreSQL, Redis, and MinIO-compatible endpoints running locally before the API server or smoke script can succeed.

## Run tests

```bash
make test
```

## Run smoke checks

```bash
bash scripts/dev/smoke.sh
```

The smoke script verifies:

- `/healthz`
- `/readyz`
- dataset creation
- dataset scan
- dataset item listing
- snapshot creation
- object presign response shape
- zero-shot job creation response shape with the created snapshot id
- snapshot import response shape with resolved dataset/snapshot linkage
- snapshot export/build response shape
- dataset-aware artifact resolve response shape
- `platform-cli pull --dataset smoke-dataset --format yolo --version v-smoke-<dataset_id>` archive download, extraction, and verification

`/readyz` checks PostgreSQL, Redis, and MinIO endpoint access with the configured credentials, so a `503` means the process is alive but one or more runtime dependencies are still unavailable.

If API is not running, it launches a temporary local API instance automatically after verifying PostgreSQL, Redis, MinIO, and the baseline migration are available. In heavily sandboxed environments, binding `:8080` may require elevated permissions even for local smoke runs.

`ARTIFACT_STORAGE_DIR` defaults to `/tmp/platform-artifacts`, and `ARTIFACT_BUILD_CONCURRENCY` defaults to `2`.

`platform-cli pull` writes `verify-report.json` with an `environment_context` block (`os`, `arch`, `cli_version`, `storage_driver`) so local verification mismatches are easier to compare across machines.

## Stop local dependencies

```bash
make down-dev
```
