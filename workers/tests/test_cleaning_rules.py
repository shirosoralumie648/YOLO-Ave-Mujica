import unittest

from workers.cleaning.main import run_cleaning_job, run_rules


def _event_by_type(result: dict, event_type: str) -> dict:
    for event in result["events"]:
        if event["event_type"] == event_type:
            return event
    raise AssertionError(f"missing event_type={event_type!r} in {result['events']!r}")


class CleaningRulesTest(unittest.TestCase):
    def test_cleaning_flags_zero_area_and_dark_score(self):
        report = run_rules(
            [
                {"item_id": 1, "bbox_w": 0, "bbox_h": 10, "brightness": 0.2, "category": "person"},
                {"item_id": 2, "bbox_w": 8, "bbox_h": 8, "brightness": 0.8, "category": "unknown"},
            ],
            taxonomy={"person"},
            dark_threshold=0.3,
        )

        self.assertEqual(report["summary"]["invalid_bbox"], 1)
        self.assertEqual(report["summary"]["too_dark"], 1)
        self.assertEqual(report["summary"]["category_mismatch"], 1)

    def test_run_cleaning_job_emits_item_failures_and_report_result_ref(self):
        result = run_cleaning_job(
            {
                "job_id": 14,
                "payload": {
                    "items": [
                        {"item_id": 1, "bbox_w": 0, "bbox_h": 10, "brightness": 0.2, "category": "person"},
                        {"item_id": 2, "bbox_w": 8, "bbox_h": 8, "brightness": 0.8, "category": "unknown"},
                        {"item_id": 3, "bbox_w": 8, "bbox_h": 8, "brightness": 0.8, "category": "person"},
                    ],
                    "taxonomy": ["person"],
                    "rules": {"dark_threshold": 0.3},
                },
            }
        )

        self.assertEqual("succeeded_with_errors", result["status"])
        self.assertEqual(3, result["total_items"])
        self.assertEqual(1, result["succeeded_items"])
        self.assertEqual(2, result["failed_items"])
        self.assertEqual("progress", result["events"][0]["event_type"])
        self.assertEqual("item_failed", result["events"][1]["event_type"])
        self.assertEqual(1, result["events"][1]["item_id"])
        self.assertEqual(2, result["events"][1]["detail_json"]["issue_count"])
        self.assertEqual("item_failed", result["events"][2]["event_type"])
        self.assertEqual(2, result["events"][2]["item_id"])

        materialized = _event_by_type(result, "cleaning_report_materialized")
        self.assertEqual("cleaning_report", materialized["detail_json"]["result_type"])
        self.assertEqual([1, 2], materialized["detail_json"]["report"]["removal_candidates"])
        self.assertEqual({"result_type": "cleaning_report", "result_count": 2}, result["result_ref"])


if __name__ == "__main__":
    unittest.main()
