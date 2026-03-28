from typing import Any, Dict, Iterable, List, Set


def classify_bbox(item: Dict[str, Any]) -> str:
    if item.get("bbox_w", 0) <= 0 or item.get("bbox_h", 0) <= 0:
        return "invalid_bbox"
    return "ok"


def run_rules(items: Iterable[Dict[str, Any]], taxonomy: Set[str], dark_threshold: float = 0.2) -> Dict[str, Any]:
    summary = {
        "invalid_bbox": 0,
        "category_mismatch": 0,
        "too_dark": 0,
    }
    issues: List[Dict[str, Any]] = []

    for item in items:
        item_id = item.get("item_id")

        bbox_result = classify_bbox(item)
        if bbox_result == "invalid_bbox":
            summary["invalid_bbox"] += 1
            issues.append({"item_id": item_id, "rule": "invalid_bbox"})

        category = item.get("category")
        if category is not None and category not in taxonomy:
            summary["category_mismatch"] += 1
            issues.append({"item_id": item_id, "rule": "category_mismatch", "category": category})

        if item.get("brightness", 1.0) < dark_threshold:
            summary["too_dark"] += 1
            issues.append({"item_id": item_id, "rule": "too_dark", "brightness": item.get("brightness")})

    removal_candidates = sorted({issue["item_id"] for issue in issues if issue.get("item_id") is not None})
    return {
        "summary": summary,
        "issues": issues,
        "removal_candidates": removal_candidates,
    }
