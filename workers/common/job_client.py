import json
from urllib import request


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
    return {
        "job_id": job_id,
        "event_level": "info",
        "event_type": "heartbeat",
        "detail_json": {"worker_id": worker_id, "lease_seconds": lease_seconds},
    }


def emit_progress(job_id: int, worker_id: str, total: int, ok: int, failed: int):
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
    return {
        "job_id": job_id,
        "worker_id": worker_id,
        "status": status,
        "total_items": total,
        "succeeded_items": ok,
        "failed_items": failed,
    }


class JobClient:
    def __init__(self, base_url: str, opener=None):
        self.base_url = base_url.rstrip("/")
        self.opener = opener or request.urlopen

    def build_heartbeat(self, job_id: int, worker_id: str, lease_seconds: int):
        payload = emit_heartbeat(job_id=job_id, worker_id=worker_id, lease_seconds=lease_seconds)
        return {
            "worker_id": payload["detail_json"]["worker_id"],
            "lease_seconds": payload["detail_json"]["lease_seconds"],
        }

    def build_progress(self, job_id: int, worker_id: str, total: int, ok: int, failed: int):
        payload = emit_progress(job_id=job_id, worker_id=worker_id, total=total, ok=ok, failed=failed)
        return {
            "worker_id": payload["detail_json"]["worker_id"],
            "total_items": payload["detail_json"]["total_items"],
            "succeeded_items": payload["detail_json"]["succeeded_items"],
            "failed_items": payload["detail_json"]["failed_items"],
        }

    def build_item_error(self, job_id: int, item_id: int, message: str, detail: dict):
        payload = emit_item_error(job_id=job_id, item_id=item_id, message=message, detail=detail)
        return {
            "item_id": payload["item_id"],
            "message": payload["message"],
            "detail_json": payload["detail_json"],
        }

    def build_terminal(self, job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int):
        payload = emit_terminal(job_id=job_id, worker_id=worker_id, status=status, total=total, ok=ok, failed=failed)
        return {
            "worker_id": payload["worker_id"],
            "status": payload["status"],
            "total_items": payload["total_items"],
            "succeeded_items": payload["succeeded_items"],
            "failed_items": payload["failed_items"],
        }

    def post_heartbeat(self, job_id: int, worker_id: str, lease_seconds: int):
        body = self.build_heartbeat(job_id=job_id, worker_id=worker_id, lease_seconds=lease_seconds)
        return self._post_json(f"/internal/jobs/{job_id}/heartbeat", body)

    def post_progress(self, job_id: int, worker_id: str, total: int, ok: int, failed: int):
        body = self.build_progress(job_id=job_id, worker_id=worker_id, total=total, ok=ok, failed=failed)
        return self._post_json(f"/internal/jobs/{job_id}/progress", body)

    def post_item_error(self, job_id: int, item_id: int, message: str, detail: dict):
        body = self.build_item_error(job_id=job_id, item_id=item_id, message=message, detail=detail)
        return self._post_json(f"/internal/jobs/{job_id}/events", body)

    def post_terminal(self, job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int):
        body = self.build_terminal(job_id=job_id, worker_id=worker_id, status=status, total=total, ok=ok, failed=failed)
        return self._post_json(f"/internal/jobs/{job_id}/complete", body)

    def _post_json(self, path: str, body: dict):
        encoded = json.dumps(body).encode("utf-8")
        req = request.Request(
            url=self.base_url + path,
            data=encoded,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with self.opener(req) as response:
            return response
