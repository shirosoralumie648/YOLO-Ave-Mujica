import os

from workers.common.job_client import JobClient
from workers.common.queue_runner import QueueRunner, poll_forever
from workers.zero_shot.main import summarize_batch


def summarize_video_extract(total_frames: int, ok_frames: int, failed_frames: int):
    return summarize_batch(total=total_frames, ok=ok_frames, failed=failed_frames)


def run_video_job(job: dict):
    payload = job.get("payload", {})
    total = payload.get("total_frames", 0)
    ok = payload.get("ok_frames", total)
    failed = payload.get("failed_frames", 0)
    status, summary = summarize_video_extract(total_frames=total, ok_frames=ok, failed_frames=failed)
    return {"status": status, **summary}


def build_video_runner(worker_id: str | None = None):
    return QueueRunner(worker_id=worker_id or os.getenv("WORKER_ID", "video-local"), accepted_job_types={"video-extract"})


def main():
    runner = build_video_runner()
    client = JobClient(base_url=os.getenv("API_BASE_URL", "http://127.0.0.1:8080"))
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:cpu", runner=runner, handler=run_video_job, job_client=client)


if __name__ == "__main__":
    main()
