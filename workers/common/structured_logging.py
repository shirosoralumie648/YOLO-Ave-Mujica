import json
import sys
from datetime import datetime, timezone


def _format_timestamp(ts: datetime) -> str:
    if ts.tzinfo is None:
        ts = ts.replace(tzinfo=timezone.utc)
    return ts.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


class WorkerLogger:
    def __init__(self, component: str, stream=None, now=None):
        self.component = component
        self.stream = stream or sys.stdout
        self.now = now or (lambda: datetime.now(timezone.utc))

    def log(self, level: str, message: str, **fields):
        payload = {
            "ts": _format_timestamp(self.now()),
            "level": level,
            "component": self.component,
            "message": message,
            **fields,
        }
        self.stream.write(json.dumps(payload, separators=(",", ":")) + "\n")
        self.stream.flush()

    def info(self, message: str, **fields):
        self.log("info", message, **fields)

    def warn(self, message: str, **fields):
        self.log("warn", message, **fields)

    def error(self, message: str, **fields):
        self.log("error", message, **fields)
