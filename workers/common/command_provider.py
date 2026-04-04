import json
import subprocess


def load_provider_result(payload: dict):
    provider = payload.get("provider") or {}
    if provider.get("type") != "command":
        return None

    argv = provider.get("argv") or []
    if not isinstance(argv, list) or not argv or not all(isinstance(arg, str) and arg for arg in argv):
        raise ValueError("provider.argv is required for command providers")

    timeout_seconds = provider.get("timeout_seconds")
    if timeout_seconds is not None:
        if isinstance(timeout_seconds, bool) or not isinstance(timeout_seconds, (int, float)) or timeout_seconds <= 0:
            raise ValueError("provider.timeout_seconds must be a positive number")

    try:
        completed = subprocess.run(
            argv,
            check=False,
            capture_output=True,
            text=True,
            input=json.dumps(payload),
            timeout=timeout_seconds,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"provider command timed out after {exc.timeout} seconds") from exc

    if completed.returncode != 0:
        raise RuntimeError(completed.stderr.strip() or f"provider command failed with exit code {completed.returncode}")

    body = completed.stdout.strip()
    if body == "":
        return {}

    return json.loads(body)


def provider_items(document: dict | None, result_key: str):
    if document is None:
        return None
    items = document.get(result_key)
    if items is None:
        raise ValueError(f"provider output missing {result_key}")
    if not isinstance(items, list):
        raise ValueError(f"provider output field {result_key} must be a list")
    return items
