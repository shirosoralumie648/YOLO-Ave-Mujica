import json
from urllib import request


def emit_item_error(job_id: int, item_id: int, message: str, detail: dict):
    """Build a normalized item-level failure event payload for job_events."""
    return emit_event(job_id=job_id, event_type="item_failed", message=message, detail=detail, level="error", item_id=item_id)


def emit_event(job_id: int, event_type: str, message: str, detail: dict, level: str = "warn", item_id: int | None = None):
    payload = {
        "job_id": job_id,
        "event_level": level,
        "event_type": event_type,
        "message": message,
        "detail_json": detail,
    }
    if item_id is not None:
        payload["item_id"] = item_id
    return payload


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


def emit_terminal(job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int, result_ref: dict | None = None):
    """Build a terminal job summary payload consumed by worker-side job updaters."""
    payload = {
        "job_id": job_id,
        "worker_id": worker_id,
        "status": status,
        "total_items": total,
        "succeeded_items": ok,
        "failed_items": failed,
    }
    if result_ref:
        payload["result_ref"] = result_ref
    return payload


class JobClient:
    def __init__(self, base_url: str, opener=None, trace_id: str | None = None):
        self.base_url = base_url.rstrip("/")
        self.opener = opener or request.urlopen
        self.trace_id = trace_id

    def set_trace_id(self, trace_id: str | None):
        self.trace_id = trace_id

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
            "event_level": payload["event_level"],
            "event_type": payload["event_type"],
            "message": payload["message"],
            "detail_json": payload["detail_json"],
        }

    def build_event(self, job_id: int, event_type: str, message: str, detail: dict, level: str = "warn", item_id: int | None = None):
        payload = emit_event(
            job_id=job_id,
            event_type=event_type,
            message=message,
            detail=detail,
            level=level,
            item_id=item_id,
        )
        body = {
            "event_level": payload["event_level"],
            "event_type": payload["event_type"],
            "message": payload["message"],
            "detail_json": payload["detail_json"],
        }
        if item_id is not None:
            body["item_id"] = item_id
        return body

    def build_terminal(self, job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int, result_ref: dict | None = None):
        payload = emit_terminal(job_id=job_id, worker_id=worker_id, status=status, total=total, ok=ok, failed=failed, result_ref=result_ref)
        body = {
            "worker_id": payload["worker_id"],
            "status": payload["status"],
            "total_items": payload["total_items"],
            "succeeded_items": payload["succeeded_items"],
            "failed_items": payload["failed_items"],
        }
        if result_ref:
            body["result_ref"] = payload["result_ref"]
        return body

    def post_heartbeat(self, job_id: int, worker_id: str, lease_seconds: int):
        body = self.build_heartbeat(job_id=job_id, worker_id=worker_id, lease_seconds=lease_seconds)
        return self._post_json(f"/internal/jobs/{job_id}/heartbeat", body)

    def post_progress(self, job_id: int, worker_id: str, total: int, ok: int, failed: int):
        body = self.build_progress(job_id=job_id, worker_id=worker_id, total=total, ok=ok, failed=failed)
        return self._post_json(f"/internal/jobs/{job_id}/progress", body)

    def post_item_error(self, job_id: int, item_id: int, message: str, detail: dict):
        body = self.build_item_error(job_id=job_id, item_id=item_id, message=message, detail=detail)
        return self._post_json(f"/internal/jobs/{job_id}/events", body)

    def post_event(self, job_id: int, event_type: str, message: str, detail: dict, level: str = "warn", item_id: int | None = None):
        body = self.build_event(
            job_id=job_id,
            event_type=event_type,
            message=message,
            detail=detail,
            level=level,
            item_id=item_id,
        )
        return self._post_json(f"/internal/jobs/{job_id}/events", body)

    def post_terminal(self, job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int, result_ref: dict | None = None):
        body = self.build_terminal(job_id=job_id, worker_id=worker_id, status=status, total=total, ok=ok, failed=failed, result_ref=result_ref)
        return self._post_json(f"/internal/jobs/{job_id}/complete", body)

    def register_worker(self, descriptor: dict):
        body = {
            "worker_id": descriptor.get("worker_id", ""),
            "resource_lane": descriptor.get("resource_lane", ""),
            "capabilities": descriptor.get("capabilities", []),
            "job_types": descriptor.get("job_types", []),
        }
        return self._post_json("/internal/jobs/workers/register", body)

    def _post_json(self, path: str, body: dict):
        encoded = json.dumps(body).encode("utf-8")
        headers = {"Content-Type": "application/json"}
        if self.trace_id:
            headers["X-Request-Id"] = self.trace_id
            headers["X-Correlation-Id"] = self.trace_id
        req = request.Request(
            url=self.base_url + path,
            data=encoded,
            headers=headers,
            method="POST",
        )
        with self.opener(req) as response:
            return response
