# Local Quickstart

## Prerequisites

- Go 1.20+
- Python 3.12+
- Docker (optional for local infra)

## Start local dependencies

```bash
make up-dev
```

If Docker is not installed, the command prints a skip message and you can still run API/worker unit tests.

## Run tests

```bash
make test
```

## Run smoke checks

```bash
bash scripts/dev/smoke.sh
```

The smoke script verifies `/healthz` and `/readyz`. If API is not running, it launches a temporary local API instance automatically.

## Stop local dependencies

```bash
make down-dev
```
