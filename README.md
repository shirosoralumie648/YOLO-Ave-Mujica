# YOLO-Ave-Mujica

A production-oriented MVP foundation for dataset indexing, annotation workflow orchestration, and training artifact delivery.

## Current Scope

This branch focuses on the platform foundation layer:

- Go control plane skeleton (`api-server`, `platform-cli`)
- Data Hub basics (dataset/snapshot APIs + object presign)
- Job primitives (state machine + idempotent create model)
- Artifact packager helpers (`label_map`, `manifest`, `data.yaml`)
- Worker-side partial-success primitives
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
make test
bash scripts/dev/smoke.sh
make down-dev
```

Notes:

- `make up-dev/down-dev` are Docker-backed; if Docker is missing they print a skip message.
- `make test` runs Go tests and the Python worker unit test.
- `scripts/dev/smoke.sh` checks `/healthz` and `/readyz`; it can start a temporary local API process if one is not already running.

## Implemented API Surface (MVP Skeleton)

- `POST /v1/datasets`
- `POST /v1/datasets/{id}/snapshots`
- `GET /v1/datasets/{id}/snapshots`
- `POST /v1/objects/presign`
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
