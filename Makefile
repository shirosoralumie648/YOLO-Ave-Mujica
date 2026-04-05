.PHONY: up-dev down-dev test migrate-up migrate-down migrate-version api-dev web-install web-dev web-test web-build

up-dev:
	@if command -v docker >/dev/null 2>&1; then \
		docker compose -f deploy/docker/docker-compose.dev.yml up -d && \
		S3_ENDPOINT=$${S3_ENDPOINT:-localhost:9000} \
		S3_ACCESS_KEY=$${S3_ACCESS_KEY:-minioadmin} \
		S3_SECRET_KEY=$${S3_SECRET_KEY:-minioadmin} \
		S3_BUCKET=$${S3_BUCKET:-platform-dev} \
		GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/s3-bootstrap; \
	else \
		echo "docker not installed; skipping up-dev"; \
	fi

down-dev:
	@if command -v docker >/dev/null 2>&1; then \
		docker compose -f deploy/docker/docker-compose.dev.yml down -v; \
	else \
		echo "docker not installed; skipping down-dev"; \
	fi

migrate-up:
	@DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} \
		GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command up

migrate-down:
	@DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} \
		GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command down

migrate-version:
	@DATABASE_URL=$${DATABASE_URL:?DATABASE_URL is required} \
		GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command version

api-dev:
	GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/api-server

web-install:
	cd apps/web && npm install

web-dev:
	cd apps/web && npm run dev

web-test:
	cd apps/web && npm test

web-build:
	cd apps/web && npm run build

test:
	GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
	PYTHONPATH=. python3 -m unittest \
		workers.tests.test_partial_success \
		workers.tests.test_job_client \
		workers.tests.test_cleaning_rules -v
	cd apps/web && npm test
