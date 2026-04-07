import os

from workers.common.command_provider import load_provider_result, provider_items
from workers.common.job_client import JobClient, emit_terminal
from workers.common.queue_runner import QueueRunner, poll_forever
from workers.common.structured_logging import WorkerLogger


def summarize_batch(total: int, ok: int, failed: int):
    """Convert batch counters into the terminal job status expected by the control plane."""
    if failed == 0:
        return "succeeded", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    if ok > 0:
        return "succeeded_with_errors", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    return "failed", {"total_items": total, "succeeded_items": ok, "failed_items": failed}


def build_terminal_event(job_id: int, total: int, ok: int, failed: int):
    """Build the terminal worker payload using the local worker identity fallback."""
    status, summary = summarize_batch(total=total, ok=ok, failed=failed)
    worker_id = os.getenv("WORKER_ID", "zero-shot-local")
    return emit_terminal(
        job_id,
        worker_id,
        status,
        summary["total_items"],
        summary["succeeded_items"],
        summary["failed_items"],
    )


def _coerce_bbox(candidate: dict) -> dict:
    bbox = candidate.get("bbox", {})
    x = float(bbox.get("x", 0))
    y = float(bbox.get("y", 0))
    w = float(bbox.get("w", 0))
    h = float(bbox.get("h", 0))
    if w <= 0 or h <= 0:
        raise ValueError("bbox.w and bbox.h must be > 0")
    if x < 0 or y < 0:
        raise ValueError("bbox.x and bbox.y must be >= 0")
    return {"x": x, "y": y, "w": w, "h": h}


def _positive_int(value, field_name: str) -> int:
    coerced = int(value)
    if coerced <= 0:
        raise ValueError(f"{field_name} must be > 0")
    return coerced


def _required_text(value, field_name: str) -> str:
    text = str(value or "").strip()
    if not text:
        raise ValueError(f"{field_name} is required")
    return text


def _coerce_confidence(value) -> float:
    confidence = float(value)
    if confidence < 0 or confidence > 1:
        raise ValueError("confidence must be between 0 and 1")
    return confidence


def _build_candidates(job: dict, provider_result: dict | None = None) -> list[dict]:
    payload = job.get("payload", {})
    model_name = payload.get("model_name", "zero-shot-mvp")
    raw_candidates = provider_items(provider_result, "candidates")
    if raw_candidates is None:
        raw_candidates = payload.get("candidates", [])
    candidates = []
    for raw in raw_candidates:
        dataset_id = _positive_int(raw.get("dataset_id", payload.get("dataset_id", 0)), "dataset_id")
        snapshot_id = _positive_int(raw.get("snapshot_id", payload.get("snapshot_id", 0)), "snapshot_id")
        item_id = _positive_int(raw.get("item_id", 0), "item_id")
        object_key = _required_text(raw.get("object_key", ""), "object_key")
        category_id = int(raw.get("category_id", 0))
        category_name = str(raw.get("category_name", payload.get("prompt", ""))).strip()
        if category_id <= 0 and not category_name:
            raise ValueError("category_id or category_name is required")
        candidates.append(
            {
                "dataset_id": dataset_id,
                "snapshot_id": snapshot_id,
                "item_id": item_id,
                "object_key": object_key,
                "category_id": category_id,
                "category_name": category_name,
                "confidence": _coerce_confidence(raw.get("confidence", 0)),
                "model_name": raw.get("model_name", model_name),
                "is_pseudo": True,
                "bbox": _coerce_bbox(raw),
            }
        )
    return candidates


def _summary_from_provider(provider_result: dict | None, default_total: int) -> tuple[int, int, int]:
    if provider_result is None:
        return default_total, default_total, 0

    total = int(provider_result.get("total_items", default_total))
    succeeded = int(provider_result.get("succeeded_items", default_total))
    failed = int(provider_result.get("failed_items", max(total-succeeded, 0)))
    return total, succeeded, failed


def _progress_event(total: int, succeeded: int, failed: int) -> dict:
    return {
        "event_level": "info",
        "event_type": "progress",
        "message": "worker progress",
        "detail_json": {
            "total_items": total,
            "succeeded_items": succeeded,
            "failed_items": failed,
        },
    }


def _build_item_failure_events(provider_result: dict | None) -> list[dict]:
    if provider_result is None:
        return []

    raw_errors = provider_result.get("errors") or []
    if not raw_errors:
        return []
    if not isinstance(raw_errors, list):
        raise ValueError("provider output field errors must be a list")

    events = []
    for raw in raw_errors:
        if not isinstance(raw, dict):
            raise ValueError("provider output errors entries must be objects")
        item_id = _positive_int(raw.get("item_id", 0), "item_id")
        message = _required_text(raw.get("message", ""), "message")
        detail = raw.get("detail") or {}
        if not isinstance(detail, dict):
            raise ValueError("provider error detail must be an object")
        detail = dict(detail)

        object_key = str(raw.get("object_key", "")).strip()
        if object_key and "object_key" not in detail:
            detail["object_key"] = object_key

        events.append(
            {
                "event_level": "error",
                "event_type": "item_failed",
                "message": message,
                "detail_json": detail,
                "item_id": item_id,
            }
        )
    return events


def run_zero_shot_job(job: dict):
    payload = job.get("payload", {})
    provider_result = load_provider_result(payload)
    candidates = _build_candidates(job, provider_result=provider_result)
    total, succeeded, failed = _summary_from_provider(provider_result, len(candidates))
    result = build_terminal_event(job["job_id"], total=total, ok=succeeded, failed=failed)
    events = [_progress_event(total, succeeded, failed)]
    events.extend(_build_item_failure_events(provider_result))
    if candidates:
        events.append(
            {
                "event_level": "info",
                "event_type": "review_candidates_materialized",
                "message": "persisted review candidates",
                "detail_json": {
                    "result_type": "annotation_candidates",
                    "result_count": len(candidates),
                    "candidates": candidates,
                },
            }
        )
        result["result_ref"] = {"result_type": "annotation_candidates", "result_count": len(candidates)}
    result["events"] = events
    return result


def build_zero_shot_runner(worker_id: str | None = None):
    return QueueRunner(
        worker_id=worker_id or os.getenv("WORKER_ID", "zero-shot-local"),
        accepted_job_types={"zero-shot"},
        resource_lane="jobs:gpu",
        capabilities={"zero_shot_inference", "grounding_dino"},
    )


def main():
    runner = build_zero_shot_runner()
    client = JobClient(base_url=os.getenv("API_BASE_URL", "http://127.0.0.1:8080"))
    logger = WorkerLogger(component="zero_shot_worker")
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:gpu", runner=runner, handler=run_zero_shot_job, job_client=client, logger=logger)


if __name__ == "__main__":
    main()
