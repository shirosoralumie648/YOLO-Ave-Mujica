from __future__ import annotations

import io
import json
import zipfile
from urllib import request


def parse_coco_document(document: dict) -> list[dict]:
    images = {int(image["id"]): image["file_name"] for image in document.get("images", [])}
    categories = {int(category["id"]): category["name"] for category in document.get("categories", [])}

    boxes = []
    for annotation in document.get("annotations", []):
        bbox = annotation.get("bbox") or []
        if len(bbox) != 4:
            raise ValueError(f"invalid coco bbox: {bbox}")
        image_id = int(annotation["image_id"])
        category_id = int(annotation["category_id"])
        if image_id not in images:
            raise ValueError(f"unknown image_id: {image_id}")
        if category_id not in categories:
            raise ValueError(f"unknown category_id: {category_id}")
        boxes.append(
            {
                "object_key": images[image_id],
                "category_name": categories[category_id],
                "bbox_x": float(bbox[0]),
                "bbox_y": float(bbox[1]),
                "bbox_w": float(bbox[2]),
                "bbox_h": float(bbox[3]),
            }
        )
    return boxes


def load_coco_document(url: str) -> dict:
    with request.urlopen(url) as response:
        payload = response.read()

    if url.endswith(".zip"):
        with zipfile.ZipFile(io.BytesIO(payload)) as zf:
            for member in zf.namelist():
                if member.endswith("/") or not member.endswith(".json"):
                    continue
                return json.loads(zf.read(member).decode("utf-8"))
        raise ValueError("no coco json found in archive")
    return json.loads(payload.decode("utf-8"))
