import os
from typing import Any, Dict, Iterable, List, Set

from workers.common.job_client import JobClient
from workers.common.queue_runner import QueueRunner, poll_forever
from workers.zero_shot.main import summarize_batch


def classify_bbox(item: Dict[str, Any]) -> str:
    """Classify a single annotation box as valid or invalid using width and height only."""
    if item.get("bbox_w", 0) <= 0 or item.get("bbox_h", 0) <= 0:
        return "invalid_bbox"
    return "ok"


def run_rules(items: Iterable[Dict[str, Any]], taxonomy: Set[str], dark_threshold: float = 0.2) -> Dict[str, Any]:
    """Run MVP cleaning rules and return summary counts, detailed issues, and removal candidates."""
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


def run_cleaning_job(job: Dict[str, Any]) -> Dict[str, Any]:
    payload = job.get("payload", {})
    items = payload.get("items", [])
    taxonomy = set(payload.get("taxonomy", []))
    dark_threshold = payload.get("rules", {}).get("dark_threshold", 0.2)

    report = run_rules(items, taxonomy, dark_threshold=dark_threshold)
    total = len(items)
    failed = len(report["issues"])
    ok = max(total - failed, 0)
    status, summary = summarize_batch(total=total, ok=ok, failed=failed)
    return {"status": status, **summary, "report": report}


def build_cleaning_runner(worker_id: str = "cleaning-local") -> QueueRunner:
    return QueueRunner(
        worker_id=worker_id,
        accepted_job_types={"cleaning"},
        resource_lane="jobs:cpu",
        capabilities={"rules_engine", "image_stats"},
    )


def main():
    runner = build_cleaning_runner()
    client = JobClient(base_url=os.getenv("API_BASE_URL", "http://127.0.0.1:8080"))
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:cpu", runner=runner, handler=run_cleaning_job, job_client=client)


if __name__ == "__main__":
    main()
