def emit_item_error(job_id: int, item_id: int, message: str, detail: dict):
    """Build a normalized item-level failure event payload for job_events."""
    return {
        "job_id": job_id,
        "item_id": item_id,
        "event_level": "error",
        "event_type": "item_failed",
        "message": message,
        "detail_json": detail,
    }


def emit_heartbeat(job_id: int, worker_id: str, lease_seconds: int):
    """Build a heartbeat event payload for lease-based worker monitoring."""
    return {
        "job_id": job_id,
        "event_level": "info",
        "event_type": "heartbeat",
        "detail_json": {"worker_id": worker_id, "lease_seconds": lease_seconds},
    }


def emit_progress(job_id: int, worker_id: str, total: int, ok: int, failed: int):
    """Build a progress event payload with aggregate success and failure counters."""
    return {
        "job_id": job_id,
        "event_level": "info",
        "event_type": "progress",
        "detail_json": {
            "worker_id": worker_id,
            "total_items": total,
            "succeeded_items": ok,
            "failed_items": failed,
        },
    }


def emit_terminal(job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int):
    """Build a terminal job summary payload consumed by worker-side job updaters."""
    return {
        "job_id": job_id,
        "worker_id": worker_id,
        "status": status,
        "total_items": total,
        "succeeded_items": ok,
        "failed_items": failed,
    }
