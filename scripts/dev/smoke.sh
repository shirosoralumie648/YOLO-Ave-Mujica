#!/usr/bin/env bash
set -euo pipefail

export DATABASE_URL="${DATABASE_URL:-postgres://platform:platform@localhost:5432/platform?sslmode=disable}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
export S3_ENDPOINT="${S3_ENDPOINT:-localhost:9000}"
export S3_ACCESS_KEY="${S3_ACCESS_KEY:-minioadmin}"
export S3_SECRET_KEY="${S3_SECRET_KEY:-minioadmin}"
export S3_BUCKET="${S3_BUCKET:-platform-dev}"
export API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"

api_base="${API_BASE_URL%/}"
api_log="/tmp/api-server.log"
started_local="false"
pid=""
pull_dir=""
cli_bin=""

cleanup() {
  if [[ "$started_local" == "true" && -n "$pid" ]]; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" 2>/dev/null || true
  fi
  if [[ -n "$pull_dir" ]]; then
    rm -rf "$pull_dir" >/dev/null 2>&1 || true
  fi
  if [[ -n "$cli_bin" ]]; then
    rm -f "$cli_bin" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

fail() {
  echo "smoke failed: $1" >&2
  if [[ "$started_local" == "true" && -f "$api_log" ]]; then
    echo "--- api-server.log ---" >&2
    tail -n 20 "$api_log" >&2 || true
  fi
  exit 1
}

require_endpoint() {
  local name="$1"
  local url="$2"
  if ! curl -fsS "$url" >/dev/null 2>&1; then
    fail "$name check failed for $url"
  fi
}

port_open() {
  local port="$1"
  (echo > /dev/tcp/127.0.0.1/"$port") >/dev/null 2>&1
}

missing_ports=()
for port in 5432 6379 9000; do
  if ! port_open "$port"; then
    missing_ports+=("$port")
  fi
done

if [[ "${#missing_ports[@]}" -gt 0 ]]; then
  if command -v docker >/dev/null 2>&1; then
    make up-dev >/dev/null || fail "failed to start local dependencies via make up-dev"
    sleep 2
  else
    fail "required local dependencies on ports ${missing_ports[*]} are unavailable; start PostgreSQL, Redis, and MinIO or install Docker for make up-dev"
  fi
fi

for port in 5432 6379 9000; do
  if ! port_open "$port"; then
    fail "required local dependency on port $port is unavailable after startup attempt"
  fi
done

GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/s3-bootstrap >/dev/null || fail "s3 bucket bootstrap failed"
make migrate-up >/dev/null || fail "database migration failed"

# If no API is running, start a temporary local one just for this smoke check.
if ! curl -fsS "${api_base}/healthz" >/dev/null 2>&1; then
  : > "$api_log"
  GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/api-server >"$api_log" 2>&1 &
  pid=$!
  started_local="true"
  for _ in $(seq 1 40); do
    if curl -fsS "${api_base}/healthz" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
fi

require_endpoint "healthz" "${api_base}/healthz"
require_endpoint "readyz" "${api_base}/readyz"

dataset_response="$(curl -fsS -X POST "${api_base}/v1/datasets" \
  -H 'Content-Type: application/json' \
  -d '{"project_id":1,"name":"smoke-dataset","bucket":"platform-dev","prefix":"train"}')" || fail "dataset create request failed"

dataset_id="$(printf '%s' "$dataset_response" | tr -d '\n' | sed -n 's/.*"dataset_id":[[:space:]]*\([0-9][0-9]*\).*/\1/p')"
if [[ -z "$dataset_id" ]]; then
  fail "dataset create response missing dataset_id: $dataset_response"
fi

scan_response="$(curl -fsS -X POST "${api_base}/v1/datasets/${dataset_id}/scan" \
  -H 'Content-Type: application/json' \
  -d '{"object_keys":["train/a.jpg","train/b.jpg"]}')" || fail "dataset scan request failed"

if [[ "$scan_response" != *"added_items"* ]]; then
  fail "scan response missing added_items: $scan_response"
fi

items_response="$(curl -fsS "${api_base}/v1/datasets/${dataset_id}/items")" || fail "items list request failed"

if [[ "$items_response" != *"train/a.jpg"* ]]; then
  fail "items response missing scanned key: $items_response"
fi

presign_response="$(curl -fsS -X POST "${api_base}/v1/objects/presign" \
  -H 'Content-Type: application/json' \
  -d "{\"dataset_id\":${dataset_id},\"object_key\":\"train/a.jpg\",\"ttl_seconds\":120}")" || fail "object presign request failed"

if [[ "$presign_response" != *"url"* ]]; then
  fail "presign response missing url: $presign_response"
fi

job_response="$(curl -fsS -X POST "${api_base}/v1/jobs/zero-shot" \
  -H 'Content-Type: application/json' \
  -d "{\"project_id\":1,\"dataset_id\":${dataset_id},\"snapshot_id\":1,\"prompt\":\"person\",\"idempotency_key\":\"smoke-zero-shot\",\"required_resource_type\":\"gpu\"}")" || fail "zero-shot job request failed"

if [[ "$job_response" != *"job_id"* ]]; then
  fail "job create response missing job_id: $job_response"
fi

artifact_seed_response="$(GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/dev-seed-artifact-smoke --dataset-id "${dataset_id}")" || fail "artifact smoke seed failed"

snapshot_id="$(printf '%s' "$artifact_seed_response" | tr -d '\n' | sed -n 's/.*"snapshot_id":[[:space:]]*\([0-9][0-9]*\).*/\1/p')"
if [[ -z "$snapshot_id" ]]; then
  fail "artifact seed response missing snapshot_id: $artifact_seed_response"
fi

artifact_version="v-smoke-${dataset_id}"
artifact_response="$(curl -fsS -X POST "${api_base}/v1/artifacts/packages" \
  -H 'Content-Type: application/json' \
  -d "{\"dataset_id\":${dataset_id},\"snapshot_id\":${snapshot_id},\"format\":\"yolo\",\"version\":\"${artifact_version}\"}")" || fail "artifact package request failed"

artifact_id="$(printf '%s' "$artifact_response" | tr -d '\n' | sed -n 's/.*"artifact_id":[[:space:]]*\([0-9][0-9]*\).*/\1/p')"
if [[ -z "$artifact_id" ]]; then
  fail "artifact package response missing artifact_id: $artifact_response"
fi

artifact_ready="false"
for _ in $(seq 1 60); do
  artifact_status="$(curl -fsS "${api_base}/v1/artifacts/${artifact_id}")" || fail "artifact status request failed"
  if [[ "$artifact_status" == *'"status":"ready"'* ]]; then
    artifact_ready="true"
    break
  fi
  if [[ "$artifact_status" == *'"status":"failed"'* ]]; then
    fail "artifact build failed: $artifact_status"
  fi
  sleep 0.5
done

if [[ "$artifact_ready" != "true" ]]; then
  fail "artifact did not become ready: $artifact_status"
fi

pull_dir="$(mktemp -d /tmp/platform-pull.XXXXXX)"
cli_bin="$(mktemp /tmp/platform-cli-smoke.XXXXXX)"

GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go build -o "$cli_bin" ./cmd/platform-cli >/dev/null || fail "platform-cli build failed"

(
  cd "$pull_dir"
  API_BASE_URL="${api_base}" "$cli_bin" pull --format yolo --version "${artifact_version}" >/dev/null
) || fail "artifact pull failed"

[[ -f "${pull_dir}/pulled-${artifact_version}/data.yaml" ]] || fail "missing pulled data.yaml"
[[ -f "${pull_dir}/pulled-${artifact_version}/train/images/a.jpg" ]] || fail "missing pulled train image"
[[ -f "${pull_dir}/pulled-${artifact_version}/train/labels/a.txt" ]] || fail "missing pulled train label"
