#!/usr/bin/env bash
set -euo pipefail

started_local="false"
pid=""

cleanup() {
  if [[ "$started_local" == "true" && -n "$pid" ]]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if ! curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; then
  GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/api-server >/tmp/api-server.log 2>&1 &
  pid=$!
  started_local="true"
  for _ in $(seq 1 40); do
    if curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
fi

curl -fsS http://localhost:8080/healthz >/dev/null
curl -fsS http://localhost:8080/readyz >/dev/null
