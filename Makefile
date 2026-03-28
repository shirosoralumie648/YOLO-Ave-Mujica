.PHONY: up-dev down-dev test

up-dev:
	@if command -v docker >/dev/null 2>&1; then \
		docker compose -f deploy/docker/docker-compose.dev.yml up -d; \
	else \
		echo "docker not installed; skipping up-dev"; \
	fi

down-dev:
	@if command -v docker >/dev/null 2>&1; then \
		docker compose -f deploy/docker/docker-compose.dev.yml down -v; \
	else \
		echo "docker not installed; skipping down-dev"; \
	fi

test:
	GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
	PYTHONPATH=. python3 -m unittest workers.tests.test_partial_success -v
