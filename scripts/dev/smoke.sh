#!/usr/bin/env bash
set -euo pipefail

export DATABASE_URL="${DATABASE_URL:-postgres://platform:platform@localhost:5432/platform?sslmode=disable}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
export S3_ENDPOINT="${S3_ENDPOINT:-localhost:9000}"
export S3_ACCESS_KEY="${S3_ACCESS_KEY:-minioadmin}"
export S3_SECRET_KEY="${S3_SECRET_KEY:-minioadmin}"
export S3_BUCKET="${S3_BUCKET:-platform-dev}"
export API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
export SMOKE_SKIP_PORT_CHECK="${SMOKE_SKIP_PORT_CHECK:-0}"

api_base="${API_BASE_URL%/}"
api_log="/tmp/api-server.log"
importer_log="/tmp/importer-worker.log"
zero_shot_log="/tmp/zero-shot-worker.log"
video_log="/tmp/video-worker.log"
started_local="false"
started_importer="false"
started_zero_shot="false"
started_video="false"
pid=""
importer_pid=""
zero_shot_pid=""
video_pid=""
pull_dir=""
cli_bin=""

cleanup() {
  if [[ "$started_video" == "true" && -n "$video_pid" ]]; then
    kill "$video_pid" >/dev/null 2>&1 || true
    wait "$video_pid" 2>/dev/null || true
  fi
  if [[ "$started_zero_shot" == "true" && -n "$zero_shot_pid" ]]; then
    kill "$zero_shot_pid" >/dev/null 2>&1 || true
    wait "$zero_shot_pid" 2>/dev/null || true
  fi
  if [[ "$started_importer" == "true" && -n "$importer_pid" ]]; then
    kill "$importer_pid" >/dev/null 2>&1 || true
    wait "$importer_pid" 2>/dev/null || true
  fi
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
  if [[ "$started_importer" == "true" && -f "$importer_log" ]]; then
    echo "--- importer-worker.log ---" >&2
    tail -n 20 "$importer_log" >&2 || true
  fi
  if [[ "$started_zero_shot" == "true" && -f "$zero_shot_log" ]]; then
    echo "--- zero-shot-worker.log ---" >&2
    tail -n 20 "$zero_shot_log" >&2 || true
  fi
  if [[ "$started_video" == "true" && -f "$video_log" ]]; then
    echo "--- video-worker.log ---" >&2
    tail -n 20 "$video_log" >&2 || true
  fi
  exit 1
}

latest_migration_version() {
  local path base version latest=0
  shopt -s nullglob
  for path in migrations/*.up.sql; do
    base="$(basename "$path")"
    version="${base%%_*}"
    version=$((10#$version))
    if (( version > latest )); then
      latest=$version
    fi
  done
  shopt -u nullglob

  if (( latest == 0 )); then
    return 1
  fi
  printf '%s' "$latest"
}

run_migrations() {
  local output latest_version
  if output="$(make migrate-up 2>&1)"; then
    return 0
  fi

  if [[ "$output" != *"no migration found for version"* ]]; then
    printf '%s\n' "$output" >&2
    return 1
  fi

  latest_version="$(latest_migration_version)" || {
    printf '%s\n' "$output" >&2
    return 1
  }

  if ! GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/migrate -command force -force-version "$latest_version" >/dev/null 2>&1; then
    printf '%s\n' "$output" >&2
    return 1
  fi

  if output="$(make migrate-up 2>&1)"; then
    return 0
  fi

  printf '%s\n' "$output" >&2
  return 1
}

json_int_field() {
  local json="$1"
  local field="$2"
  JSON_INPUT="$json" python3 - "$field" <<'PY'
import json
import os
import sys

field = sys.argv[1]
text = os.environ.get("JSON_INPUT", "")

try:
    value = json.loads(text).get(field)
except Exception:
    sys.exit(0)

if isinstance(value, int):
    sys.stdout.write(str(value))
PY
}

json_item_id_for_object_key() {
  local json="$1"
  local object_key="$2"
  JSON_INPUT="$json" python3 - "$object_key" <<'PY'
import json
import os
import sys

object_key = sys.argv[1]
text = os.environ.get("JSON_INPUT", "")

try:
    items = json.loads(text).get("items") or []
except Exception:
    sys.exit(0)

for item in items:
    if item.get("object_key") == object_key and isinstance(item.get("id"), int):
        sys.stdout.write(str(item["id"]))
        break
PY
}

clear_job_queue_lanes() {
  python3 - <<'PY'
import os
import socket
import sys

addr = os.environ.get("REDIS_ADDR", "localhost:6379")
if ":" in addr:
    host, port = addr.rsplit(":", 1)
else:
    host, port = addr, "6379"

parts = ["DEL", "jobs:cpu", "jobs:gpu", "jobs:mixed"]
payload = [f"*{len(parts)}\r\n".encode("utf-8")]
for part in parts:
    raw = part.encode("utf-8")
    payload.append(f"${len(raw)}\r\n".encode("utf-8"))
    payload.append(raw + b"\r\n")

with socket.create_connection((host, int(port)), timeout=2) as conn:
    conn.sendall(b"".join(payload))
    response = conn.recv(64)

if not response.startswith(b":"):
    sys.stderr.write(f"unexpected redis response while clearing lanes: {response!r}\n")
    sys.exit(1)
PY
}

wait_for_artifact_ready() {
  local artifact_id="$1"
  local artifact_detail=""
  for _ in $(seq 1 60); do
    artifact_detail="$(curl -fsS "${api_base}/v1/artifacts/${artifact_id}")" || fail "artifact detail request failed"
    if [[ "$artifact_detail" == *"\"status\":\"ready\""* ]]; then
      printf '%s' "$artifact_detail"
      return 0
    fi
    if [[ "$artifact_detail" == *"\"status\":\"failed\""* ]]; then
      fail "artifact ${artifact_id} build failed: ${artifact_detail}"
    fi
    sleep 0.5
  done
  fail "artifact ${artifact_id} did not become ready: ${artifact_detail}"
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

if [[ "${SMOKE_SKIP_PORT_CHECK}" != "1" ]]; then
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
fi

GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/s3-bootstrap >/dev/null || fail "s3 bucket bootstrap failed"
run_migrations || fail "database migration failed"

if [[ "${SMOKE_SKIP_PORT_CHECK}" != "1" ]]; then
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

  clear_job_queue_lanes || fail "failed to clear stale job queue lanes"

  : > "$importer_log"
  PYTHONPATH=. API_BASE_URL="${api_base}" REDIS_ADDR="${REDIS_ADDR}" python3 -m workers.importer.main >"$importer_log" 2>&1 &
  importer_pid=$!
  started_importer="true"
  sleep 0.5
  if ! kill -0 "$importer_pid" >/dev/null 2>&1; then
    fail "importer worker exited before smoke requests began"
  fi

  : > "$zero_shot_log"
  PYTHONPATH=. API_BASE_URL="${api_base}" REDIS_ADDR="${REDIS_ADDR}" python3 -m workers.zero_shot.main >"$zero_shot_log" 2>&1 &
  zero_shot_pid=$!
  started_zero_shot="true"
  sleep 0.5
  if ! kill -0 "$zero_shot_pid" >/dev/null 2>&1; then
    fail "zero-shot worker exited before smoke requests began"
  fi

  : > "$video_log"
  PYTHONPATH=. API_BASE_URL="${api_base}" REDIS_ADDR="${REDIS_ADDR}" python3 -m workers.video.main >"$video_log" 2>&1 &
  video_pid=$!
  started_video="true"
  sleep 0.5
  if ! kill -0 "$video_pid" >/dev/null 2>&1; then
    fail "video worker exited before smoke requests began"
  fi
fi

dataset_response="$(curl -fsS -X POST "${api_base}/v1/datasets" \
  -H 'Content-Type: application/json' \
  -d "{\"project_id\":1,\"name\":\"smoke-dataset\",\"bucket\":\"${S3_BUCKET}\",\"prefix\":\"train\"}")" || fail "dataset create request failed"

dataset_id="$(json_int_field "$dataset_response" "dataset_id")"
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
train_a_item_id="$(json_item_id_for_object_key "$items_response" "train/a.jpg")"
train_b_item_id="$(json_item_id_for_object_key "$items_response" "train/b.jpg")"
if [[ -z "$train_a_item_id" || -z "$train_b_item_id" ]]; then
  fail "items response missing item ids for train/a.jpg or train/b.jpg: $items_response"
fi

snapshot_response="$(curl -fsS -X POST "${api_base}/v1/datasets/${dataset_id}/snapshots" \
  -H 'Content-Type: application/json' \
  -d '{"note":"smoke snapshot"}')" || fail "snapshot create request failed"

snapshot_id="$(json_int_field "$snapshot_response" "id")"
if [[ -z "$snapshot_id" ]]; then
  fail "snapshot create response missing id: $snapshot_response"
fi

task_response="$(curl -fsS -X POST "${api_base}/v1/projects/1/tasks" \
  -H 'Content-Type: application/json' \
  -d "{\"snapshot_id\":${snapshot_id},\"title\":\"Annotate smoke image\",\"kind\":\"annotation\",\"status\":\"in_progress\",\"assignee\":\"annotator-1\",\"asset_object_key\":\"train/a.jpg\",\"media_kind\":\"image\",\"ontology_version\":\"v1\",\"priority\":\"high\"}")" || fail "task create request failed"

task_id="$(json_int_field "$task_response" "id")"
if [[ -z "$task_id" ]]; then
  fail "task create response missing id: $task_response"
fi

workspace_response="$(curl -fsS "${api_base}/v1/tasks/${task_id}/workspace")" || fail "workspace request failed"
if [[ "$workspace_response" != *"\"object_key\":\"train/a.jpg\""* && "$workspace_response" != *"\"asset_object_key\":\"train/a.jpg\""* ]]; then
  fail "workspace response missing task asset context: $workspace_response"
fi

draft_response="$(curl -fsS -X PUT "${api_base}/v1/tasks/${task_id}/workspace/draft" \
  -H 'Content-Type: application/json' \
  -d '{"actor":"annotator-1","body":{"objects":[{"id":"box-1","label":"person"}]}}')" || fail "workspace draft save failed"
if [[ "$draft_response" != *"\"revision\":"* ]]; then
  fail "workspace draft response missing revision: $draft_response"
fi

submit_response="$(curl -fsS -X POST "${api_base}/v1/tasks/${task_id}/workspace/submit" \
  -H 'Content-Type: application/json' \
  -d '{"actor":"annotator-1"}')" || fail "workspace submit failed"
if [[ "$submit_response" != *"\"status\":\"submitted\""* ]]; then
  fail "workspace submit response missing submitted status: $submit_response"
fi

resubmit_response="$(curl -fsS -X POST "${api_base}/v1/tasks/${task_id}/workspace/submit" \
  -H 'Content-Type: application/json' \
  -d '{"actor":"annotator-1"}')" || fail "workspace resubmit failed"
if [[ "$resubmit_response" != *"\"status\":\"submitted\""* ]]; then
  fail "workspace resubmit response missing submitted status: $resubmit_response"
fi

zero_shot_idempotency_key="smoke-zero-shot-${dataset_id}-${snapshot_id}"
import_idempotency_key="smoke-snapshot-import-${dataset_id}-${snapshot_id}"

presign_response="$(curl -fsS -X POST "${api_base}/v1/objects/presign" \
  -H 'Content-Type: application/json' \
  -d "{\"dataset_id\":${dataset_id},\"object_key\":\"train/a.jpg\",\"ttl_seconds\":120}")" || fail "object presign request failed"

if [[ "$presign_response" != *"url"* ]]; then
  fail "presign response missing url: $presign_response"
fi

GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/dev-seed-artifact-smoke --dataset-id "${dataset_id}" --category-only >/dev/null || fail "category smoke seed failed"

job_response="$(curl -fsS -X POST "${api_base}/v1/jobs/zero-shot" \
  -H 'Content-Type: application/json' \
  -d "{\"project_id\":1,\"dataset_id\":${dataset_id},\"snapshot_id\":${snapshot_id},\"prompt\":\"person\",\"items\":[{\"item_id\":${train_a_item_id},\"object_key\":\"train/a.jpg\"},{\"item_id\":${train_b_item_id},\"object_key\":\"train/b.jpg\"}],\"provider\":{\"type\":\"builtin\",\"name\":\"grounding_dino_fake\"},\"idempotency_key\":\"${zero_shot_idempotency_key}\",\"required_resource_type\":\"gpu\",\"required_capabilities\":[\"grounding_dino\"]}")" || fail "zero-shot job request failed"

zero_shot_job_id="$(json_int_field "$job_response" "job_id")"
if [[ -z "$zero_shot_job_id" ]]; then
  fail "job create response missing job_id: $job_response"
fi

zero_shot_job_detail=""
for _ in $(seq 1 120); do
  zero_shot_job_detail="$(curl -fsS "${api_base}/v1/jobs/${zero_shot_job_id}")" || fail "zero-shot job status request failed"
  if [[ "$zero_shot_job_detail" == *"\"status\":\"succeeded\""* || "$zero_shot_job_detail" == *"\"status\":\"succeeded_with_errors\""* ]]; then
    break
  fi
  if [[ "$zero_shot_job_detail" == *"\"status\":\"failed\""* ]]; then
    fail "zero-shot job ${zero_shot_job_id} failed: ${zero_shot_job_detail}"
  fi
  sleep 0.25
done
if [[ "$zero_shot_job_detail" != *"\"result_type\":\"annotation_candidates\""* || "$zero_shot_job_detail" != *"\"result_count\":2"* ]]; then
  fail "zero-shot job detail missing durable candidate output: ${zero_shot_job_detail}"
fi

zero_shot_events="$(curl -fsS "${api_base}/v1/jobs/${zero_shot_job_id}/events")" || fail "zero-shot job events request failed"
if [[ "$zero_shot_events" != *"\"event_type\":\"progress\""* || "$zero_shot_events" != *"\"event_type\":\"review_candidates_materialized\""* ]]; then
  fail "zero-shot job events missing progress/materialization lifecycle: ${zero_shot_events}"
fi

review_candidates_response="$(curl -fsS "${api_base}/v1/review/candidates")" || fail "review candidates request failed"
if [[ "$review_candidates_response" != *"grounding_dino_fake"* || "$review_candidates_response" != *"train/a.jpg"* ]]; then
  fail "review candidates response missing zero-shot output: ${review_candidates_response}"
fi

video_idempotency_key="smoke-video-${dataset_id}-${snapshot_id}"
video_job_response="$(curl -fsS -X POST "${api_base}/v1/jobs/video-extract" \
  -H 'Content-Type: application/json' \
  -d "{\"project_id\":1,\"dataset_id\":${dataset_id},\"fps\":2,\"duration_ms\":3000,\"source_object_key\":\"clips/a.mp4\",\"frame_prefix\":\"clips/a\",\"provider\":{\"type\":\"builtin\",\"name\":\"video_decode_fake\"},\"idempotency_key\":\"${video_idempotency_key}\",\"required_resource_type\":\"cpu\",\"required_capabilities\":[\"video_decode\"]}")" || fail "video job request failed"

video_job_id="$(json_int_field "$video_job_response" "job_id")"
if [[ -z "$video_job_id" ]]; then
  fail "video job response missing job_id: $video_job_response"
fi

video_job_detail=""
for _ in $(seq 1 120); do
  video_job_detail="$(curl -fsS "${api_base}/v1/jobs/${video_job_id}")" || fail "video job status request failed"
  if [[ "$video_job_detail" == *"\"status\":\"succeeded\""* || "$video_job_detail" == *"\"status\":\"succeeded_with_errors\""* ]]; then
    break
  fi
  if [[ "$video_job_detail" == *"\"status\":\"failed\""* ]]; then
    fail "video job ${video_job_id} failed: ${video_job_detail}"
  fi
  sleep 0.25
done
if [[ "$video_job_detail" != *"\"result_type\":\"video_frames\""* || "$video_job_detail" != *"\"result_count\":7"* || "$video_job_detail" != *"\"object_key\":\"clips/a/frame-0006.jpg\""* ]]; then
  fail "video job detail missing durable frame output: ${video_job_detail}"
fi

video_job_events="$(curl -fsS "${api_base}/v1/jobs/${video_job_id}/events")" || fail "video job events request failed"
if [[ "$video_job_events" != *"\"event_type\":\"progress\""* || "$video_job_events" != *"\"event_type\":\"video_frames_materialized\""* ]]; then
  fail "video job events missing progress/materialization lifecycle: ${video_job_events}"
fi

import_response="$(curl -fsS -X POST "${api_base}/v1/snapshots/${snapshot_id}/import" \
  -H 'Content-Type: application/json' \
  -d "{\"format\":\"yolo\",\"idempotency_key\":\"${import_idempotency_key}\",\"required_resource_type\":\"cpu\",\"required_capabilities\":[\"importer\",\"yolo\"],\"labels\":{\"train/a.txt\":\"0 0.5 0.5 0.2 0.2\\n\"},\"names\":[\"person\"],\"images\":{\"train/a.txt\":\"train/a.jpg\"}}")" || fail "snapshot import request failed"

import_job_id="$(json_int_field "$import_response" "job_id")"
import_dataset_id="$(json_int_field "$import_response" "dataset_id")"
import_snapshot_id="$(json_int_field "$import_response" "snapshot_id")"
if [[ -z "$import_job_id" || -z "$import_dataset_id" || -z "$import_snapshot_id" ]]; then
  fail "snapshot import response missing job_id/dataset_id/snapshot_id: $import_response"
fi
if [[ "$import_dataset_id" != "$dataset_id" || "$import_snapshot_id" != "$snapshot_id" ]]; then
  fail "snapshot import linkage mismatch: $import_response"
fi

import_job_detail=""
for _ in $(seq 1 120); do
  import_job_detail="$(curl -fsS "${api_base}/v1/jobs/${import_job_id}")" || fail "snapshot import job status request failed"
  if [[ "$import_job_detail" == *"\"status\":\"succeeded\""* ]]; then
    break
  fi
  if [[ "$import_job_detail" == *"\"status\":\"failed\""* ]]; then
    fail "snapshot import job ${import_job_id} failed: ${import_job_detail}"
  fi
  sleep 0.25
done
if [[ "$import_job_detail" != *"\"status\":\"succeeded\""* ]]; then
  fail "snapshot import job ${import_job_id} did not complete successfully: ${import_job_detail}"
fi

import_job_events="$(curl -fsS "${api_base}/v1/jobs/${import_job_id}/events")" || fail "snapshot import job events request failed"
if [[ "$import_job_events" != *"\"event_type\":\"progress\""* ]]; then
  fail "snapshot import job events missing progress lifecycle: ${import_job_events}"
fi

publish_candidates_response="$(curl -fsS "${api_base}/v1/publish/candidates?project_id=1")" || fail "publish candidates request failed"
if [[ "$publish_candidates_response" != *"\"items\""* ]]; then
  fail "publish candidates response missing items: $publish_candidates_response"
fi

publish_batch_response="$(curl -fsS -X POST "${api_base}/v1/publish/batches" \
  -H 'Content-Type: application/json' \
  -d "{\"project_id\":1,\"snapshot_id\":${snapshot_id},\"source\":\"manual\",\"rule_summary\":{\"reason\":\"smoke-flow\"},\"items\":[{\"candidate_id\":401,\"task_id\":51,\"dataset_id\":${dataset_id},\"snapshot_id\":${snapshot_id},\"item_payload\":{\"overlay\":{\"boxes\":[{\"label\":\"person\"}]},\"diff\":{\"added\":1,\"updated\":0,\"removed\":0},\"context\":{\"source\":\"smoke\"}}}]}")" || fail "publish batch create request failed"

publish_batch_id="$(json_int_field "$publish_batch_response" "id")"
if [[ -z "$publish_batch_id" ]]; then
  fail "publish batch response missing id: $publish_batch_response"
fi

publish_batch_detail="$(curl -fsS "${api_base}/v1/publish/batches/${publish_batch_id}")" || fail "publish batch detail request failed"
if [[ "$publish_batch_detail" != *"\"id\":${publish_batch_id}"* ]]; then
  fail "publish batch detail mismatch: $publish_batch_detail"
fi

publish_feedback_response="$(curl -fsS -X POST "${api_base}/v1/publish/batches/${publish_batch_id}/feedback" \
  -H 'Content-Type: application/json' \
  -d '{"stage":"review","action":"comment","reason_code":"smoke_ready","severity":"low","influence_weight":1,"comment":"smoke batch feedback","actor":"reviewer-1"}')" || fail "publish batch feedback request failed"
if [[ "$publish_feedback_response" != *"\"scope\":\"batch\""* ]]; then
  fail "publish batch feedback response missing batch scope: $publish_feedback_response"
fi

review_approve_response="$(curl -fsS -X POST "${api_base}/v1/publish/batches/${publish_batch_id}/review-approve" \
  -H 'Content-Type: application/json' \
  -d '{"actor":"reviewer-1"}')" || fail "publish review approve request failed"
if [[ "$review_approve_response" != *"\"ok\":true"* ]]; then
  fail "publish review approve response missing ok=true: $review_approve_response"
fi

owner_approve_response="$(curl -fsS -X POST "${api_base}/v1/publish/batches/${publish_batch_id}/owner-approve" \
  -H 'Content-Type: application/json' \
  -d '{"actor":"owner-1"}')" || fail "publish owner approve request failed"

publish_record_id="$(json_int_field "$owner_approve_response" "publish_record_id")"
if [[ -z "$publish_record_id" ]]; then
  fail "publish owner approve response missing publish_record_id: $owner_approve_response"
fi

publish_workspace_response="$(curl -fsS "${api_base}/v1/publish/batches/${publish_batch_id}/workspace")" || fail "publish workspace request failed"
if [[ "$publish_workspace_response" != *"\"history\""* || "$publish_workspace_response" != *"\"overlay\""* ]]; then
  fail "publish workspace response missing history or overlay: $publish_workspace_response"
fi

publish_record_response="$(curl -fsS "${api_base}/v1/publish/records/${publish_record_id}")" || fail "publish record request failed"
if [[ "$publish_record_response" != *"\"publish_batch_id\":${publish_batch_id}"* ]]; then
  fail "publish record response missing publish_batch linkage: $publish_record_response"
fi

artifact_seed_response="$(GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go run ./cmd/dev-seed-artifact-smoke --dataset-id "${dataset_id}")" || fail "artifact smoke seed failed"
seed_snapshot_id="$(json_int_field "$artifact_seed_response" "snapshot_id")"
if [[ -n "$seed_snapshot_id" ]]; then
  snapshot_id="$seed_snapshot_id"
fi

artifact_version="v-smoke-${dataset_id}"
coco_artifact_version="${artifact_version}-coco"
coco_export_response="$(curl -fsS -X POST "${api_base}/v1/snapshots/${snapshot_id}/export" \
  -H 'Content-Type: application/json' \
  -d "{\"dataset_id\":${dataset_id},\"format\":\"coco\",\"version\":\"${coco_artifact_version}\"}")" || fail "snapshot coco export request failed"

coco_artifact_id="$(json_int_field "$coco_export_response" "artifact_id")"
coco_package_job_id="$(json_int_field "$coco_export_response" "job_id")"
if [[ -z "$coco_artifact_id" || -z "$coco_package_job_id" ]]; then
  fail "snapshot coco export response missing job_id or artifact_id: $coco_export_response"
fi

wait_for_artifact_ready "$coco_artifact_id" >/dev/null

export_response="$(curl -fsS -X POST "${api_base}/v1/snapshots/${snapshot_id}/export" \
  -H 'Content-Type: application/json' \
  -d "{\"dataset_id\":${dataset_id},\"format\":\"yolo\",\"version\":\"${artifact_version}\"}")" || fail "snapshot export request failed"

artifact_id="$(json_int_field "$export_response" "artifact_id")"
package_job_id="$(json_int_field "$export_response" "job_id")"
if [[ -z "$artifact_id" || -z "$package_job_id" ]]; then
  fail "snapshot export response missing job_id or artifact_id: $export_response"
fi

pull_dir="$(mktemp -d /tmp/platform-pull.XXXXXX)"
cli_bin="$(mktemp /tmp/platform-cli-smoke.XXXXXX)"
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go build -o "$cli_bin" ./cmd/platform-cli >/dev/null || fail "platform-cli build failed"

(
  cd "$pull_dir"
  API_BASE_URL="${api_base}" "$cli_bin" pull --dataset smoke-dataset --format yolo --version "${artifact_version}" >/dev/null
) || fail "artifact pull failed"

resolve_response="$(curl -fsS "${api_base}/v1/artifacts/resolve?dataset=smoke-dataset&format=yolo&version=${artifact_version}")" || fail "artifact resolve request failed"
resolved_artifact_id="$(json_int_field "$resolve_response" "id")"
if [[ -z "$resolved_artifact_id" ]]; then
  fail "artifact resolve response missing id: $resolve_response"
fi
if [[ "$resolved_artifact_id" != "$artifact_id" ]]; then
  fail "artifact resolve id ${resolved_artifact_id} does not match exported artifact ${artifact_id}"
fi

metrics_response="$(curl -fsS "${api_base}/metrics")" || fail "metrics request failed"
for required_metric in 'yolo_http_requests_total' 'yolo_job_creations_total' 'yolo_queue_depth{lane="jobs:cpu"}' 'yolo_review_backlog'; do
  if [[ "$metrics_response" != *"${required_metric}"* ]]; then
    fail "metrics response missing ${required_metric}: ${metrics_response}"
  fi
done

[[ -f "${pull_dir}/verify-report.json" ]] || fail "missing verify-report.json"
[[ -f "${pull_dir}/pulled-${artifact_version}/manifest.json" ]] || fail "missing pulled manifest"
[[ -f "${pull_dir}/pulled-${artifact_version}/data.yaml" ]] || fail "missing pulled data.yaml"
[[ -f "${pull_dir}/pulled-${artifact_version}/train/images/a.jpg" ]] || fail "missing pulled train image"
[[ -f "${pull_dir}/pulled-${artifact_version}/train/labels/a.txt" ]] || fail "missing pulled train label"
