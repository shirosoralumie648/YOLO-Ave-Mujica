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


def run_zero_shot_job(job: dict):
    payload = job.get("payload", {})
    total = payload.get("total_items", 0)
    ok = payload.get("succeeded_items", total)
    failed = payload.get("failed_items", 0)
    return build_terminal_event(job["job_id"], total=total, ok=ok, failed=failed)


def build_zero_shot_runner(worker_id: str | None = None):
    return QueueRunner(worker_id=worker_id or os.getenv("WORKER_ID", "zero-shot-local"), accepted_job_types={"zero-shot"})


def main():
    runner = build_zero_shot_runner()
    client = JobClient(base_url=os.getenv("API_BASE_URL", "http://127.0.0.1:8080"))
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:gpu", runner=runner, handler=run_zero_shot_job, job_client=client)


if __name__ == "__main__":
    main()
