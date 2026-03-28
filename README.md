# YOLO-Ave-Mujica

A production-oriented MVP foundation for dataset indexing, annotation workflow orchestration, and training artifact delivery.

## Current Scope

This branch focuses on the platform foundation layer:

- Go control plane skeleton (`api-server`, `platform-cli`)
- Data Hub basics (dataset/scan/items/snapshot APIs + object presign)
- Job primitives (state machine, idempotent create model, lane dispatch, lease sweeper)
- Review, diff, and artifact HTTP modules
- Artifact packager helpers (`label_map`, `manifest`, `data.yaml`)
- Worker-side partial-success, heartbeat, and cleaning primitives
- Local smoke and quickstart docs

Detailed architecture and implementation planning docs:

- [MVP architecture spec](docs/superpowers/specs/2026-03-28-yolo-platform-mvp-design.md)
- [MVP foundation plan](docs/superpowers/plans/2026-03-28-yolo-platform-mvp-foundation-plan.md)
- [Local quickstart](docs/development/local-quickstart.md)

## Repository Layout

```text
cmd/                Entry points for api-server and platform-cli
internal/           Go domain modules (server/datahub/jobs/artifacts/cli/...)
workers/            Python worker-side primitives and tests
migrations/         SQL schema bootstrap
deploy/docker/      Local compose file
scripts/dev/        Smoke script and local helpers
docs/               Specs, plans, and development docs
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
make migrate-up
make test
bash scripts/dev/smoke.sh
make down-dev
```

Notes:

- `make up-dev/down-dev` are Docker-backed; if Docker is missing you need PostgreSQL, Redis, and MinIO already running locally.
- `make up-dev` also bootstraps the default MinIO bucket (`platform-dev`) used by the local smoke path.
- `make migrate-up` applies the canonical baseline schema and seeds the default `project_id=1` used by the current smoke path.
- `make test` runs Go tests plus Python worker unit tests.
- `/readyz` reflects dependency readiness for PostgreSQL, Redis, and MinIO endpoint access with the configured credentials, while `/healthz` remains pure process liveness.
- `scripts/dev/smoke.sh` checks health/readiness and exercises dataset create, dataset scan, item listing, object presign, and zero-shot job creation. It can start a temporary local API process if one is not already running.
- `platform-cli pull` writes `verify-report.json` with `environment_context` fields for OS, architecture, CLI version, and the active storage driver.

## Implemented API Surface (MVP Skeleton)

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
- `POST /v1/artifacts/{id}/presign`
- `GET /healthz`
- `GET /readyz`

## Testing

Run all Go tests:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
```

Run worker test directly:

```bash
PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success -v
```

Additional worker tests:

```bash
PYTHONPATH=. python3 -m unittest workers.tests.test_job_client workers.tests.test_cleaning_rules -v
```
