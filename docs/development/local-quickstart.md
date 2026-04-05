# Local Quickstart

## Prerequisites

- Go 1.20+
- Node.js 20+ and npm 10+
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

`make up-dev` starts the Docker-backed dependency stack and also bootstraps the default MinIO bucket (`platform-dev`). The command is fail-fast: if Docker is installed but unusable, startup stops immediately instead of continuing into the S3 bootstrap step. In WSL 2, the most common cause is Docker Desktop being installed without WSL integration enabled for the current distro. If you are not using Docker at all, you need PostgreSQL, Redis, and a MinIO-compatible endpoint running locally before the API server or smoke script can succeed.

## Run The API Server

```bash
make api-dev
```

The control plane listens on `http://127.0.0.1:8080` by default.

## Run The Web Console

```bash
make web-install
make web-dev
```

The Vite app listens on `http://127.0.0.1:5173` and proxies `/v1/*` to `http://127.0.0.1:8080` by default. If your API server listens elsewhere, set `VITE_API_PROXY_TARGET`.

## Run Tests

```bash
make test
```

This runs the full Go test suite and the worker unit tests:

- `workers.tests.test_partial_success`
- `workers.tests.test_job_client`
- `workers.tests.test_cleaning_rules`
- `apps/web` Vitest suite

You can also run frontend verification independently:

```bash
make web-test
make web-build
```

If you change public HTTP routes or `api/openapi/mvp.yaml`, rerun the route contract guard as well:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./internal/server -count=1
```

The public contract lives under `/v1/*` plus `/healthz` and `/readyz`. `internal/server/http_server_routes_test.go` now guards route registration drift, duplicate OpenAPI path/method entries, and the documented failure surface for datahub, tasks/workspace, jobs, review, publish, artifact, and snapshot diff/export APIs. Worker/internal callbacks such as snapshot import completion and job progress reporting live under `/internal/*` and are verified by module-specific tests rather than OpenAPI.

## Run Smoke Checks

```bash
bash scripts/dev/smoke.sh
```

The smoke flow verifies:

- task overview route shape
- project task list route shape
- task detail route shape
- `/healthz`
- `/readyz`
- dataset creation
- dataset scan
- dataset item listing
- snapshot creation
- duplicate annotation workspace submit remains idempotent
- object presign response shape
- zero-shot job creation response shape with the created snapshot id
- snapshot import response shape with resolved dataset/snapshot linkage
- both `coco` and `yolo` snapshot export requests are accepted and their build states can be polled
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
