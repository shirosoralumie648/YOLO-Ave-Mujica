from __future__ import annotations

import io
import json
import os
import zipfile
from urllib import request

from workers.common.job_client import JobClient
from workers.common.queue_runner import QueueRunner, poll_forever


def _derive_object_key(label_path: str) -> str:
    if label_path.startswith("labels/"):
        label_path = label_path[len("labels/") :]
    if label_path.endswith(".txt"):
        return label_path[:-4] + ".jpg"
    return label_path


def _parse_yolo_labels(labels: dict, names: list[str], images: dict) -> list[dict]:
    boxes = []
    for label_path, raw in labels.items():
        object_key = images.get(label_path, _derive_object_key(label_path))
        for line in raw.splitlines():
            line = line.strip()
            if not line:
                continue
            parts = line.split()
            if len(parts) != 5:
                raise ValueError(f"invalid yolo label line: {line}")
            class_idx = int(parts[0])
            boxes.append(
                {
                    "object_key": object_key,
                    "category_name": names[class_idx],
                    "bbox_x": float(parts[1]),
                    "bbox_y": float(parts[2]),
                    "bbox_w": float(parts[3]),
                    "bbox_h": float(parts[4]),
                }
            )
    return boxes


def _parse_names_from_yaml(raw: str) -> list[str]:
    names: list[str] = []
    lines = raw.splitlines()
    in_names = False
    for line in lines:
        stripped = line.strip()
        if stripped == "names:":
            in_names = True
            continue
        if not in_names:
            continue
        if stripped.startswith("- "):
            names.append(stripped[2:].strip())
            continue
        if stripped and not stripped.startswith("#"):
            break
    return names


def _load_yolo_archive(url: str, fallback_names: list[str] | None = None) -> tuple[dict, list[str], dict]:
    with request.urlopen(url) as response:
        archive_bytes = response.read()

    labels: dict[str, str] = {}
    names = list(fallback_names or [])
    images: dict[str, str] = {}
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as zf:
        for member in zf.namelist():
            if member.endswith("/"):
                continue
            if member in {"data.yaml", "data.yml"} and not names:
                names = _parse_names_from_yaml(zf.read(member).decode("utf-8"))
                continue
            if member.endswith(".txt"):
                labels[member] = zf.read(member).decode("utf-8")
                images[member] = _derive_object_key(member)
    return labels, names, images


def parse_import_payload(payload: dict) -> list[dict]:
    fmt = payload.get("format")
    if fmt != "yolo":
        raise ValueError(f"unsupported format: {fmt}")

    labels = payload.get("labels") or {}
    names = payload.get("names") or []
    images = payload.get("images") or {}

    if not labels:
        source_download_url = payload.get("source_download_url")
        if not source_download_url:
            source_uri = payload.get("source_uri", "")
            if source_uri.startswith(("file://", "http://", "https://")):
                source_download_url = source_uri
        if not source_download_url:
            raise ValueError("no supported import source provided")
        labels, names, images = _load_yolo_archive(source_download_url, fallback_names=names)

    return _parse_yolo_labels(labels, names, images)


def complete_snapshot_import(snapshot_id: int, fmt: str, source_uri: str, entries: list[dict], base_url: str, opener=None):
    payload = json.dumps(
        {
            "format": fmt,
            "source_uri": source_uri,
            "entries": entries,
        }
    ).encode("utf-8")
    req = request.Request(
        url=base_url.rstrip("/") + f"/internal/snapshots/{snapshot_id}/import",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with (opener or request.urlopen)(req):
        return None


def run_import_job(job_payload: dict, base_url: str | None = None, opener=None) -> dict:
    payload = job_payload.get("payload", job_payload)
    boxes = parse_import_payload(payload)
    snapshot_id = job_payload.get("snapshot_id")
    if snapshot_id:
        complete_snapshot_import(
            snapshot_id,
            payload.get("format", ""),
            payload.get("source_uri", ""),
            boxes,
            base_url or os.getenv("API_BASE_URL", "http://127.0.0.1:8080"),
            opener=opener,
        )
    return {
        "status": "succeeded",
        "total_items": len(boxes),
        "succeeded_items": len(boxes),
        "failed_items": 0,
        "entries": boxes,
    }


def build_importer_runner(worker_id: str | None = None):
    return QueueRunner(worker_id=worker_id or os.getenv("WORKER_ID", "importer-local"), accepted_job_types={"snapshot-import"})


def main():
    runner = build_importer_runner()
    client = JobClient(base_url=os.getenv("API_BASE_URL", "http://127.0.0.1:8080"))
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:cpu", runner=runner, handler=run_import_job, job_client=client)


if __name__ == "__main__":
    main()
