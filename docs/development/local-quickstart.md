# Local Quickstart

## Prerequisites

- Go 1.20+
- Python 3.12+
- Docker for the default local PostgreSQL, Redis, and MinIO stack
- Or equivalent local services already running on `5432`, `6379`, and `9000`

## Start Local Dependencies

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

`make up-dev` starts the Docker-backed dependency stack and also bootstraps the default MinIO bucket (`platform-dev`). If Docker is unavailable, you need PostgreSQL, Redis, and a MinIO-compatible endpoint running locally before the API server or smoke script can succeed.

## Run Tests

```bash
make test
```

This runs the full Go test suite and the worker unit tests:

- `workers.tests.test_partial_success`
- `workers.tests.test_job_client`
- `workers.tests.test_cleaning_rules`

## Run Smoke Checks

```bash
bash scripts/dev/smoke.sh
```

The smoke flow verifies:

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

`/readyz` checks PostgreSQL, Redis, and MinIO endpoint access with the configured credentials. A `503` means the API process is alive but one or more runtime dependencies are still unavailable.

If the API is not already running, the smoke script starts a temporary local API process after verifying that PostgreSQL, Redis, MinIO, and the baseline migration are available. In tightly sandboxed environments, binding `:8080` may still require elevated permissions.

`ARTIFACT_STORAGE_DIR` defaults to `/tmp/platform-artifacts`, and `ARTIFACT_BUILD_CONCURRENCY` defaults to `2`.

`platform-cli pull` writes `verify-report.json`, including an `environment_context` block with `os`, `arch`, `cli_version`, and `storage_driver`.

## Stop Local Dependencies

```bash
make down-dev
```

### Run the web console

1. `make web-install`
2. `make web-dev`
3. Open the Vite URL and keep the API server running on `http://localhost:8080`
