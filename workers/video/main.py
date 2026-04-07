import os

from workers.common.command_provider import load_provider_result, provider_items
from workers.common.job_client import JobClient
from workers.common.queue_runner import QueueRunner, poll_forever
from workers.common.structured_logging import WorkerLogger
from workers.zero_shot.main import summarize_batch


def summarize_video_extract(total_frames: int, ok_frames: int, failed_frames: int):
    return summarize_batch(total=total_frames, ok=ok_frames, failed=failed_frames)


def _build_frames(payload: dict, provider_result: dict | None = None) -> list[dict]:
    frames = provider_items(provider_result, "frames")
    if frames is None:
        frames = payload.get("frames")
    if frames:
        out = []
        for frame in frames:
            frame_index = int(frame.get("frame_index", 0))
            timestamp_ms = int(frame.get("timestamp_ms", 0))
            if frame_index < 0:
                raise ValueError("frame_index must be >= 0")
            if timestamp_ms < 0:
                raise ValueError("timestamp_ms must be >= 0")
            out.append(
                {
                    "frame_index": frame_index,
                    "timestamp_ms": timestamp_ms,
                    "object_key": frame.get("object_key", ""),
                }
            )
        return out

    total_frames = int(payload.get("total_frames", 0))
    fps = max(int(payload.get("fps", 1)), 1)
    return [
        {
            "frame_index": index,
            "timestamp_ms": int((index / fps) * 1000),
            "object_key": "",
        }
        for index in range(total_frames)
    ]


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


def _build_frame_failure_events(provider_result: dict | None) -> list[dict]:
    if provider_result is None:
        return []

    raw_errors = provider_result.get("frame_errors") or []
    if not raw_errors:
        return []
    if not isinstance(raw_errors, list):
        raise ValueError("provider output field frame_errors must be a list")

    events = []
    for raw in raw_errors:
        if not isinstance(raw, dict):
            raise ValueError("provider output frame_errors entries must be objects")
        message = str(raw.get("message", "")).strip()
        if not message:
            raise ValueError("frame_error.message is required")

        detail = raw.get("detail") or {}
        if not isinstance(detail, dict):
            raise ValueError("frame_error.detail must be an object")
        detail = dict(detail)

        if "frame_index" not in detail:
            detail["frame_index"] = int(raw.get("frame_index", 0))
        if "timestamp_ms" not in detail:
            detail["timestamp_ms"] = int(raw.get("timestamp_ms", 0))
        object_key = str(raw.get("object_key", "")).strip()
        if object_key and "object_key" not in detail:
            detail["object_key"] = object_key

        events.append(
            {
                "event_level": "error",
                "event_type": "frame_failed",
                "message": message,
                "detail_json": detail,
            }
        )
    return events


def run_video_job(job: dict):
    payload = job.get("payload", {})
    provider_result = load_provider_result(payload)
    frames = _build_frames(payload, provider_result=provider_result)
    total, ok, failed = _summary_from_provider(provider_result, len(frames))
    status, summary = summarize_video_extract(total_frames=total, ok_frames=ok, failed_frames=failed)
    result = {
        "status": status,
        **summary,
        "events": [_progress_event(total, ok, failed)],
    }
    result["events"].extend(_build_frame_failure_events(provider_result))
    if frames:
        result["events"].append(
            {
                "event_level": "info",
                "event_type": "video_frames_materialized",
                "message": "materialized video frame results",
                "detail_json": {
                    "result_type": "video_frames",
                    "result_count": len(frames),
                    "frames": frames,
                },
            }
        )
        result["result_ref"] = {"result_type": "video_frames", "result_count": len(frames)}
    return result


def build_video_runner(worker_id: str | None = None):
    return QueueRunner(
        worker_id=worker_id or os.getenv("WORKER_ID", "video-local"),
        accepted_job_types={"video-extract"},
        resource_lane="jobs:cpu",
        capabilities={"video_decode"},
    )


def main():
    runner = build_video_runner()
    client = JobClient(base_url=os.getenv("API_BASE_URL", "http://127.0.0.1:8080"))
    logger = WorkerLogger(component="video_worker")
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:cpu", runner=runner, handler=run_video_job, job_client=client, logger=logger)


if __name__ == "__main__":
    main()
