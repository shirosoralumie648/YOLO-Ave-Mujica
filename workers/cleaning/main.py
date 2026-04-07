import os
from typing import Any, Dict, Iterable, List, Set

from workers.common.job_client import JobClient
from workers.common.queue_runner import QueueRunner, poll_forever
from workers.common.structured_logging import WorkerLogger
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


def _progress_event(total: int, succeeded: int, failed: int) -> Dict[str, Any]:
    return {
        "event_level": "info",
        "event_type": "progress",
        "message": "worker progress",
        "detail_json": {
            "total_items": total,
            "succeeded_items": succeeded,
            "failed_items": failed,
        },
    }


def _item_failure_events(report: Dict[str, Any]) -> List[Dict[str, Any]]:
    issues_by_item: Dict[int, List[Dict[str, Any]]] = {}
    for issue in report.get("issues", []):
        item_id = issue.get("item_id")
        if not isinstance(item_id, int) or item_id <= 0:
            continue
        issues_by_item.setdefault(item_id, []).append(issue)

    events: List[Dict[str, Any]] = []
    for item_id in sorted(issues_by_item):
        issues = issues_by_item[item_id]
        events.append(
            {
                "event_level": "error",
                "event_type": "item_failed",
                "message": "cleaning rules flagged item",
                "item_id": item_id,
                "detail_json": {
                    "issue_count": len(issues),
                    "issues": issues,
                },
            }
        )
    return events


def run_cleaning_job(job: Dict[str, Any]) -> Dict[str, Any]:
    payload = job.get("payload", {})
    items = payload.get("items", [])
    taxonomy = set(payload.get("taxonomy", []))
    dark_threshold = payload.get("rules", {}).get("dark_threshold", 0.2)

    report = run_rules(items, taxonomy, dark_threshold=dark_threshold)
    total = len(items)
    failed = len(report["removal_candidates"])
    ok = max(total - failed, 0)
    status, summary = summarize_batch(total=total, ok=ok, failed=failed)
    result = {
        "status": status,
        **summary,
        "report": report,
        "events": [_progress_event(total, ok, failed)],
    }
    result["events"].extend(_item_failure_events(report))
    result["events"].append(
        {
            "event_level": "info",
            "event_type": "cleaning_report_materialized",
            "message": "persisted cleaning report",
            "detail_json": {
                "result_type": "cleaning_report",
                "result_count": len(report["removal_candidates"]),
                "report": report,
            },
        }
    )
    result["result_ref"] = {
        "result_type": "cleaning_report",
        "result_count": len(report["removal_candidates"]),
    }
    return result


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
    logger = WorkerLogger(component="cleaning_worker")
    poll_forever(redis_addr=os.getenv("REDIS_ADDR", "localhost:6379"), lane="jobs:cpu", runner=runner, handler=run_cleaning_job, job_client=client, logger=logger)


if __name__ == "__main__":
    main()
