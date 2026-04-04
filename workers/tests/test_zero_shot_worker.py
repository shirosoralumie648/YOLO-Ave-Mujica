import unittest
import sys

from workers.zero_shot.main import run_zero_shot_job


class ZeroShotWorkerContractTest(unittest.TestCase):
    def test_run_zero_shot_job_uses_command_provider_output(self):
        result = run_zero_shot_job(
            {
                "job_id": 43,
                "payload": {
                    "dataset_id": 1,
                    "snapshot_id": 2,
                    "prompt": "person",
                    "provider": {
                        "type": "command",
                        "argv": [
                            sys.executable,
                            "-c",
                            (
                                "import json, sys; "
                                "json.load(sys.stdin); "
                                "print(json.dumps({'candidates': [{"
                                "'item_id': 99, "
                                "'object_key': 'images/99.jpg', "
                                "'category_name': 'person', "
                                "'confidence': 0.88, "
                                "'bbox': {'x': 5, 'y': 6, 'w': 7, 'h': 8}"
                                "}]}))"
                            ),
                        ],
                    },
                },
            }
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(1, result["total_items"])
        self.assertEqual(1, result["events"][0]["detail_json"]["result_count"])
        self.assertEqual(99, result["events"][0]["detail_json"]["candidates"][0]["item_id"])
        self.assertEqual("images/99.jpg", result["events"][0]["detail_json"]["candidates"][0]["object_key"])

    def test_run_zero_shot_job_emits_candidate_materialization_event_and_result_ref(self):
        result = run_zero_shot_job(
            {
                "job_id": 42,
                "payload": {
                    "dataset_id": 1,
                    "snapshot_id": 2,
                    "model_name": "grounding-dino-mvp",
                    "candidates": [
                        {
                            "item_id": 11,
                            "object_key": "images/11.jpg",
                            "category_name": "person",
                            "confidence": 0.93,
                            "bbox": {"x": 10, "y": 20, "w": 30, "h": 40},
                        },
                        {
                            "item_id": 12,
                            "object_key": "images/12.jpg",
                            "category_name": "person",
                            "confidence": 0.77,
                            "bbox": {"x": 1, "y": 2, "w": 3, "h": 4},
                        },
                    ],
                },
            }
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(2, result["total_items"])
        self.assertEqual(2, result["succeeded_items"])
        self.assertEqual(0, result["failed_items"])
        self.assertEqual({"result_type": "annotation_candidates", "result_count": 2}, result["result_ref"])
        self.assertEqual(1, len(result["events"]))
        materialized = result["events"][0]
        self.assertEqual("review_candidates_materialized", materialized["event_type"])
        self.assertEqual("info", materialized["event_level"])
        self.assertEqual(2, materialized["detail_json"]["result_count"])
        self.assertEqual("grounding-dino-mvp", materialized["detail_json"]["candidates"][0]["model_name"])
        self.assertTrue(materialized["detail_json"]["candidates"][0]["is_pseudo"])


if __name__ == "__main__":
    unittest.main()
