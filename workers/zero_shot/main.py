import os

from workers.common.job_client import JobClient, emit_terminal
from workers.common.queue_runner import QueueRunner, poll_forever


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
    return {
        "x": float(bbox.get("x", 0)),
        "y": float(bbox.get("y", 0)),
        "w": float(bbox.get("w", 0)),
        "h": float(bbox.get("h", 0)),
    }


def _build_candidates(job: dict) -> list[dict]:
    payload = job.get("payload", {})
    model_name = payload.get("model_name", "zero-shot-mvp")
    candidates = []
    for raw in payload.get("candidates", []):
        candidates.append(
            {
                "dataset_id": int(raw.get("dataset_id", payload.get("dataset_id", 0))),
                "snapshot_id": int(raw.get("snapshot_id", payload.get("snapshot_id", 0))),
                "item_id": int(raw.get("item_id", 0)),
                "object_key": raw.get("object_key", ""),
                "category_id": int(raw.get("category_id", 0)),
                "category_name": raw.get("category_name", payload.get("prompt", "")),
                "confidence": float(raw.get("confidence", 0)),
                "model_name": raw.get("model_name", model_name),
                "is_pseudo": True,
                "bbox": _coerce_bbox(raw),
            }
        )
    return candidates


def run_zero_shot_job(job: dict):
    candidates = _build_candidates(job)
    total = len(candidates)
    result = build_terminal_event(job["job_id"], total=total, ok=total, failed=0)
    result["events"] = [
        {
            "event_level": "info",
            "event_type": "review_candidates_materialized",
            "message": "persisted review candidates",
            "detail_json": {
                "result_type": "annotation_candidates",
                "result_count": total,
                "candidates": candidates,
            },
        }
    ]
    result["result_ref"] = {"result_type": "annotation_candidates", "result_count": total}
    return result


def build_zero_shot_runner(worker_id: str | None = None):
    return QueueRunner(
        worker_id=worker_id or os.getenv("WORKER_ID", "zero-shot-local"),
        accepted_job_types={"zero-shot"},
        resource_lane="jobs:gpu",
        capabilities={"zero_shot_inference"},
    )


def main():
    runner = build_zero_shot_runner()
    client = JobClient(base_url=os.getenv("API_BASE_URL", "http://127.0.0.1:8080"))
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:gpu", runner=runner, handler=run_zero_shot_job, job_client=client)


if __name__ == "__main__":
    main()
