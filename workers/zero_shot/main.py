import os

from workers.common.job_client import emit_terminal


def summarize_batch(total: int, ok: int, failed: int):
    """Return terminal job status and counters for batch-oriented workers."""
    if failed == 0:
        return "succeeded", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    if ok > 0:
        return "succeeded_with_errors", {"total_items": total, "succeeded_items": ok, "failed_items": failed}
    return "failed", {"total_items": total, "succeeded_items": ok, "failed_items": failed}


def build_terminal_event(job_id: int, total: int, ok: int, failed: int):
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
