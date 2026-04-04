import json
import socket
import subprocess
import time


def _split_redis_addr(addr: str):
    if ":" not in addr:
        return addr, 6379
    host, port = addr.rsplit(":", 1)
    return host, int(port)


class RedisQueueClient:
    def __init__(self, redis_addr: str, redis_cli_bin: str = "redis-cli"):
        self.redis_addr = redis_addr
        self.redis_cli_bin = redis_cli_bin

    def pop_json(self, lane: str, timeout_seconds: int = 5):
        host, port = _split_redis_addr(self.redis_addr)
        cmd = [
            self.redis_cli_bin,
            "-h",
            host,
            "-p",
            str(port),
            "--raw",
            "BRPOP",
            lane,
            str(timeout_seconds),
        ]
        try:
            completed = subprocess.run(cmd, check=False, capture_output=True, text=True)
        except FileNotFoundError:
            return _pop_json_via_socket(host, port, lane, timeout_seconds)
        if completed.returncode not in (0, 1):
            raise RuntimeError(completed.stderr.strip() or f"redis-cli failed with exit code {completed.returncode}")

        lines = [line for line in completed.stdout.splitlines() if line]
        if len(lines) < 2:
            return None
        return json.loads(lines[-1])

    def push_json(self, lane: str, payload: dict):
        host, port = _split_redis_addr(self.redis_addr)
        body = json.dumps(payload)
        cmd = [
            self.redis_cli_bin,
            "-h",
            host,
            "-p",
            str(port),
            "--raw",
            "RPUSH",
            lane,
            body,
        ]
        try:
            completed = subprocess.run(cmd, check=False, capture_output=True, text=True)
        except FileNotFoundError:
            return _push_json_via_socket(host, port, lane, body)
        if completed.returncode != 0:
            raise RuntimeError(completed.stderr.strip() or f"redis-cli failed with exit code {completed.returncode}")
        return None


def _pop_json_via_socket(host: str, port: int, lane: str, timeout_seconds: int):
    command = _encode_resp_array(["BRPOP", lane, str(timeout_seconds)])
    with socket.create_connection((host, port), timeout=timeout_seconds + 1) as conn:
        conn.settimeout(timeout_seconds + 1)
        file = conn.makefile("rwb")
        file.write(command)
        file.flush()

        head = file.readline()
        if not head or head == b"*-1\r\n":
            return None
        if not head.startswith(b"*"):
            raise RuntimeError(f"unexpected redis response: {head!r}")

        _ = _read_resp_bulk_string(file)
        payload = _read_resp_bulk_string(file)
        if payload is None:
            return None
        return json.loads(payload)


def _push_json_via_socket(host: str, port: int, lane: str, payload: str):
    command = _encode_resp_array(["RPUSH", lane, payload])
    with socket.create_connection((host, port), timeout=2) as conn:
        conn.settimeout(2)
        file = conn.makefile("rwb")
        file.write(command)
        file.flush()
        response = file.readline()
        if not response or not response.startswith(b":"):
            raise RuntimeError(f"unexpected redis response: {response!r}")


def _encode_resp_array(parts: list[str]) -> bytes:
    encoded = [f"*{len(parts)}\r\n".encode("utf-8")]
    for part in parts:
        raw = part.encode("utf-8")
        encoded.append(f"${len(raw)}\r\n".encode("utf-8"))
        encoded.append(raw + b"\r\n")
    return b"".join(encoded)


def _read_resp_bulk_string(file) -> str | None:
    length_line = file.readline()
    if not length_line:
        raise RuntimeError("unexpected EOF from redis socket")
    if not length_line.startswith(b"$"):
        raise RuntimeError(f"unexpected redis bulk header: {length_line!r}")
    length = int(length_line[1:-2])
    if length == -1:
        return None
    payload = file.readline()
    if not payload:
        raise RuntimeError("unexpected EOF reading redis payload")
    return payload[:length].decode("utf-8")


def _lane_for_resource(resource_type: str | None) -> str | None:
    if resource_type == "gpu":
        return "jobs:gpu"
    if resource_type == "mixed":
        return "jobs:mixed"
    if resource_type == "cpu":
        return "jobs:cpu"
    return None


class QueueRunner:
    def __init__(
        self,
        worker_id: str,
        accepted_job_types: set[str],
        resource_lane: str | None = None,
        capabilities: set[str] | None = None,
    ):
        self.worker_id = worker_id
        self.accepted_job_types = accepted_job_types
        self.resource_lane = resource_lane
        self.capabilities = set(capabilities or set())

    def worker_descriptor(self) -> dict:
        return {
            "worker_id": self.worker_id,
            "resource_lane": self.resource_lane,
            "capabilities": sorted(self.capabilities),
            "job_types": sorted(self.accepted_job_types),
        }

    def mismatch_reason(self, payload: dict, lane: str | None = None) -> str | None:
        if not payload:
            return "empty_payload"
        if payload.get("job_type") not in self.accepted_job_types:
            return "job_type_mismatch"

        required_lane = payload.get("resource_lane") or _lane_for_resource(payload.get("required_resource_type"))
        if self.resource_lane and required_lane and self.resource_lane != required_lane:
            return "resource_lane_mismatch"
        if lane and required_lane and lane != required_lane:
            return "resource_lane_mismatch"

        required_capabilities = set(payload.get("required_capabilities") or [])
        if not required_capabilities.issubset(self.capabilities):
            return "missing_capabilities"
        return None

    def can_handle(self, payload: dict, lane: str | None = None) -> bool:
        return self.mismatch_reason(payload, lane=lane) is None

    def handle_once(self, payload: dict, handler, lane: str | None = None):
        if not self.can_handle(payload, lane=lane):
            return False
        handler(payload)
        return True


def dispatch_once(queue_client, lane: str, runner: QueueRunner, handler, timeout_seconds: int = 5, job_client=None, lease_seconds: int = 30):
    payload = queue_client.pop_json(lane, timeout_seconds=timeout_seconds)
    if payload is None:
        return False
    mismatch_reason = runner.mismatch_reason(payload, lane=lane)
    if mismatch_reason is not None:
        target_lane = payload.get("resource_lane") or _lane_for_resource(payload.get("required_resource_type")) or lane
        queue_client.push_json(target_lane, payload)
        if job_client is not None and payload.get("job_id") is not None and hasattr(job_client, "post_event"):
            event_type = "dispatch_requeued" if target_lane != lane else "dispatch_rejected"
            job_client.post_event(
                payload["job_id"],
                event_type,
                "worker cannot handle dispatched job",
                {
                    "reason": mismatch_reason,
                    "queue_lane": lane,
                    "target_lane": target_lane,
                    "worker": runner.worker_descriptor(),
                },
                level="warn",
            )
        return False
    if job_client is not None:
        job_client.post_heartbeat(payload["job_id"], runner.worker_id, lease_seconds)

    result = {"status": "succeeded", "total_items": 0, "succeeded_items": 0, "failed_items": 0}

    def wrapped(job):
        nonlocal result
        handled = handler(job)
        if isinstance(handled, dict):
            result = {**result, **handled}

    dispatched = runner.handle_once(payload, wrapped, lane=lane)
    if dispatched and job_client is not None:
        for event in result.get("events", []):
            job_client.post_event(
                payload["job_id"],
                event["event_type"],
                event["message"],
                event.get("detail_json", {}),
                level=event.get("event_level", "info"),
                item_id=event.get("item_id"),
            )
        job_client.post_terminal(
            payload["job_id"],
            runner.worker_id,
            result["status"],
            result["total_items"],
            result["succeeded_items"],
            result["failed_items"],
            result_ref=result.get("result_ref"),
        )
    return dispatched


def poll_forever(redis_addr: str, lane: str, runner: QueueRunner, handler, timeout_seconds: int = 5, idle_sleep_seconds: float = 0.25, job_client=None):
    queue_client = RedisQueueClient(redis_addr=redis_addr)
    while True:
        dispatched = dispatch_once(queue_client, lane, runner, handler, timeout_seconds=timeout_seconds, job_client=job_client)
        if not dispatched:
            time.sleep(idle_sleep_seconds)
