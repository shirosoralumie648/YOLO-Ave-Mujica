# Repository Guidelines

## Project Structure & Module Organization
`cmd/` contains executable entrypoints such as `api-server`, `platform-cli`, `migrate`, and `s3-bootstrap`. Keep reusable application logic in `internal/<domain>/` packages such as `datahub`, `jobs`, `review`, `artifacts`, `server`, and `storage`. Python background workers live in `workers/`, with shared helpers under `workers/common/` and tests under `workers/tests/`. Database migrations are ordered SQL files in `migrations/`, the HTTP contract lives at `api/openapi/mvp.yaml`, and local runtime assets are under `deploy/docker/` and `scripts/dev/`.

## Build, Test, and Development Commands
Use `make up-dev` to start PostgreSQL, Redis, and MinIO from `deploy/docker/docker-compose.dev.yml` and bootstrap the S3 bucket. Run the control plane locally with `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/api-server`. Apply schema changes with `make migrate-up` after exporting `DATABASE_URL`. Use `make test` for the default Go and Python test suite. Run `bash scripts/dev/smoke.sh` before merging API, worker, or migration changes; it exercises `/healthz`, `/readyz`, dataset, snapshot, job, and artifact flows end to end.

## Coding Style & Naming Conventions
Format Go code with `gofmt`; keep package names lowercase and exported identifiers in `CamelCase`. Prefer thin `cmd/...` entrypoints and constructor-style `New...` functions that wire dependencies into `internal/...` services and handlers. Python code uses 4-space indentation, `snake_case` for modules and functions, and `CamelCase` for `unittest.TestCase` classes. Match existing domain-oriented names such as `postgres_repository.go`, `handler.go`, or `test_job_client.py`.

## Testing Guidelines
Place Go tests alongside the package under test as `*_test.go`; favor focused handler, repository, and state-machine coverage. Worker tests use `python3 -m unittest` and should live in `workers/tests/test_*.py`. There is no enforced coverage percentage today, so every behavior change should add or update targeted tests and leave `make test` passing. When contracts or local runtime wiring move, also rerun `bash scripts/dev/smoke.sh`.

## Commit & Pull Request Guidelines
Recent history follows Conventional Commit prefixes such as `feat:`, `fix:`, `docs:`, `chore:`, and `merge:`. Keep subjects short, imperative, and scoped, for example `feat: add dataset scan dedupe`. Pull requests should summarize the behavior change, list the verification commands you ran, link the relevant issue or spec, and include request/response samples or screenshots when API or UI output changes.

## Security & Configuration Tips
Keep runtime configuration in environment variables such as `DATABASE_URL`, `REDIS_ADDR`, `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, and `API_BASE_URL`; do not hardcode secrets in code, tests, or fixtures. Update `api/openapi/mvp.yaml` and any affected smoke coverage when changing external contracts.
