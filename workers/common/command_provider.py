import json
import subprocess


def load_provider_items(payload: dict, result_key: str):
    provider = payload.get("provider") or {}
    if provider.get("type") != "command":
        return None

    argv = provider.get("argv") or []
    if not isinstance(argv, list) or not argv or not all(isinstance(arg, str) and arg for arg in argv):
        raise ValueError("provider.argv is required for command providers")

    completed = subprocess.run(
        argv,
        check=False,
        capture_output=True,
        text=True,
        input=json.dumps(payload),
    )
    if completed.returncode != 0:
        raise RuntimeError(completed.stderr.strip() or f"provider command failed with exit code {completed.returncode}")

    body = completed.stdout.strip()
    if body == "":
        return []

    document = json.loads(body)
    items = document.get(result_key)
    if items is None:
        raise ValueError(f"provider output missing {result_key}")
    if not isinstance(items, list):
        raise ValueError(f"provider output field {result_key} must be a list")
    return items
