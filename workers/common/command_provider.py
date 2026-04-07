import hashlib
import json
import posixpath
import subprocess


def load_provider_result(payload: dict):
    provider = payload.get("provider") or {}
    provider_type = provider.get("type")
    if provider_type == "builtin":
        return _load_builtin_provider_result(payload, provider)
    if provider_type != "command":
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

    try:
        document = json.loads(body)
    except json.JSONDecodeError as exc:
        raise RuntimeError("provider command returned invalid JSON") from exc
    if not isinstance(document, dict):
        raise ValueError("provider output must be a JSON object")
    return document


def _load_builtin_provider_result(payload: dict, provider: dict):
    name = str(provider.get("name", "")).strip()
    if not name:
        raise ValueError("provider.name is required for builtin providers")
    if name == "grounding_dino_fake":
        return _run_builtin_grounding_dino_fake(payload, name)
    if name == "video_decode_fake":
        return _run_builtin_video_decode_fake(payload, name)
    raise ValueError(f"unsupported builtin provider.name {name!r}")


def _run_builtin_grounding_dino_fake(payload: dict, provider_name: str):
    prompt = str(payload.get("prompt", "")).strip()
    items = payload.get("items") or []
    if not prompt:
        raise ValueError("prompt is required for builtin zero-shot providers")
    if not isinstance(items, list) or not items:
        raise ValueError("items are required for builtin zero-shot providers")

    dataset_id = int(payload.get("dataset_id", 0))
    snapshot_id = int(payload.get("snapshot_id", 0))
    model_name = str(payload.get("model_name") or provider_name).strip() or provider_name
    candidates = []
    for raw in items:
        if not isinstance(raw, dict):
            raise ValueError("items entries must be objects")
        item_id = int(raw.get("item_id", 0))
        object_key = str(raw.get("object_key", "")).strip()
        if item_id <= 0:
            raise ValueError("items.item_id must be > 0")
        if not object_key:
            raise ValueError("items.object_key is required")
        x, y, w, h, confidence = _grounding_dino_fake_geometry(prompt, item_id, object_key)
        candidates.append(
            {
                "dataset_id": dataset_id,
                "snapshot_id": snapshot_id,
                "item_id": item_id,
                "object_key": object_key,
                "category_name": prompt,
                "confidence": confidence,
                "model_name": model_name,
                "bbox": {
                    "x": x,
                    "y": y,
                    "w": w,
                    "h": h,
                },
            }
        )
    return {
        "candidates": candidates,
        "total_items": len(candidates),
        "succeeded_items": len(candidates),
        "failed_items": 0,
    }


def _grounding_dino_fake_geometry(prompt: str, item_id: int, object_key: str):
    seed = int(hashlib.sha256(f"{prompt}|{item_id}|{object_key}".encode("utf-8")).hexdigest()[:8], 16)
    x = float(1 + (seed % 90))
    y = float(1 + ((seed // 90) % 90))
    w = float(20 + ((seed // 8100) % 30))
    h = float(20 + ((seed // 243000) % 30))
    confidence = round(0.55 + ((seed % 40) / 100), 4)
    return x, y, w, h, confidence


def _run_builtin_video_decode_fake(payload: dict, provider_name: str):
    source_object_key = str(payload.get("source_object_key", "")).strip()
    if not source_object_key:
        raise ValueError("source_object_key is required for builtin video providers")

    fps = int(payload.get("fps", 0))
    if fps <= 0:
        raise ValueError("fps must be > 0 for builtin video providers")

    duration_ms = int(payload.get("duration_ms", 0))
    if duration_ms <= 0:
        raise ValueError("duration_ms must be > 0 for builtin video providers")

    frame_prefix = str(payload.get("frame_prefix", "")).strip() or _default_frame_prefix(source_object_key)
    interval_ms = max(int(round(1000 / fps)), 1)
    timestamps = list(range(0, duration_ms + 1, interval_ms))
    if timestamps[-1] != duration_ms:
        timestamps.append(duration_ms)

    frames = []
    for frame_index, timestamp_ms in enumerate(timestamps):
        frames.append(
            {
                "frame_index": frame_index,
                "timestamp_ms": timestamp_ms,
                "object_key": f"{frame_prefix}/frame-{frame_index:04d}.jpg",
                "provider_name": provider_name,
            }
        )

    return {
        "frames": frames,
        "total_items": len(frames),
        "succeeded_items": len(frames),
        "failed_items": 0,
    }


def _default_frame_prefix(source_object_key: str) -> str:
    root, _ = posixpath.splitext(source_object_key)
    return root or source_object_key


def provider_items(document: dict | None, result_key: str):
    if document is None:
        return None
    items = document.get(result_key)
    if items is None:
        raise ValueError(f"provider output missing {result_key}")
    if not isinstance(items, list):
        raise ValueError(f"provider output field {result_key} must be a list")
    return items
