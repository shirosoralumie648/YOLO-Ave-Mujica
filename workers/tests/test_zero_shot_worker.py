import unittest
import sys

from workers.zero_shot.main import build_zero_shot_runner, run_zero_shot_job


def _event_by_type(result: dict, event_type: str) -> dict:
    for event in result["events"]:
        if event["event_type"] == event_type:
            return event
    raise AssertionError(f"missing event_type={event_type!r} in {result['events']!r}")


class ZeroShotWorkerContractTest(unittest.TestCase):
    def test_build_zero_shot_runner_advertises_generic_and_provider_capabilities(self):
        runner = build_zero_shot_runner(worker_id="zero-shot-a")

        self.assertEqual("zero-shot-a", runner.worker_id)
        self.assertEqual({"zero_shot_inference", "grounding_dino"}, runner.capabilities)

    def test_run_zero_shot_job_rejects_invalid_candidate_geometry(self):
        with self.assertRaisesRegex(ValueError, "bbox.w and bbox.h must be > 0"):
            run_zero_shot_job(
                {
                    "job_id": 45,
                    "payload": {
                        "dataset_id": 1,
                        "snapshot_id": 2,
                        "prompt": "person",
                        "candidates": [
                            {
                                "item_id": 100,
                                "object_key": "images/100.jpg",
                                "category_name": "person",
                                "confidence": 0.91,
                                "bbox": {"x": 1, "y": 2, "w": 0, "h": 4},
                            }
                        ],
                    },
                }
            )

    def test_run_zero_shot_job_rejects_candidate_without_item_id(self):
        with self.assertRaisesRegex(ValueError, "item_id must be > 0"):
            run_zero_shot_job(
                {
                    "job_id": 46,
                    "payload": {
                        "dataset_id": 1,
                        "snapshot_id": 2,
                        "prompt": "person",
                        "candidates": [
                            {
                                "object_key": "images/100.jpg",
                                "category_name": "person",
                                "confidence": 0.91,
                                "bbox": {"x": 1, "y": 2, "w": 3, "h": 4},
                            }
                        ],
                    },
                }
            )

    def test_run_zero_shot_job_uses_provider_summary_counters(self):
        result = run_zero_shot_job(
            {
                "job_id": 44,
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
                                "print(json.dumps({"
                                "'candidates': [{"
                                "'item_id': 100, "
                                "'object_key': 'images/100.jpg', "
                                "'category_name': 'person', "
                                "'confidence': 0.91, "
                                "'bbox': {'x': 1, 'y': 2, 'w': 3, 'h': 4}"
                                "}], "
                                "'total_items': 3, "
                                "'succeeded_items': 2, "
                                "'failed_items': 1"
                                "}))"
                            ),
                        ],
                    },
                },
            }
        )

        self.assertEqual("succeeded_with_errors", result["status"])
        self.assertEqual(3, result["total_items"])
        self.assertEqual(2, result["succeeded_items"])
        self.assertEqual(1, result["failed_items"])
        self.assertEqual("progress", result["events"][0]["event_type"])
        self.assertEqual(3, result["events"][0]["detail_json"]["total_items"])
        self.assertEqual(1, _event_by_type(result, "review_candidates_materialized")["detail_json"]["result_count"])

    def test_run_zero_shot_job_uses_builtin_provider_output_from_items(self):
        result = run_zero_shot_job(
            {
                "job_id": 41,
                "payload": {
                    "dataset_id": 1,
                    "snapshot_id": 2,
                    "prompt": "person",
                    "items": [
                        {"item_id": 21, "object_key": "train/a.jpg"},
                        {"item_id": 22, "object_key": "train/b.jpg"},
                    ],
                    "provider": {
                        "type": "builtin",
                        "name": "grounding_dino_fake",
                    },
                },
            }
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(2, result["total_items"])
        materialized = _event_by_type(result, "review_candidates_materialized")
        self.assertEqual(2, materialized["detail_json"]["result_count"])
        self.assertEqual(21, materialized["detail_json"]["candidates"][0]["item_id"])
        self.assertEqual("train/a.jpg", materialized["detail_json"]["candidates"][0]["object_key"])
        self.assertEqual("grounding_dino_fake", materialized["detail_json"]["candidates"][0]["model_name"])

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
        materialized = _event_by_type(result, "review_candidates_materialized")
        self.assertEqual(1, materialized["detail_json"]["result_count"])
        self.assertEqual(99, materialized["detail_json"]["candidates"][0]["item_id"])
        self.assertEqual("images/99.jpg", materialized["detail_json"]["candidates"][0]["object_key"])

    def test_run_zero_shot_job_emits_progress_and_item_failure_events_for_partial_provider_result(self):
        result = run_zero_shot_job(
            {
                "job_id": 47,
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
                                "print(json.dumps({"
                                "'candidates': [{"
                                "'item_id': 99, "
                                "'object_key': 'images/99.jpg', "
                                "'category_name': 'person', "
                                "'confidence': 0.88, "
                                "'bbox': {'x': 5, 'y': 6, 'w': 7, 'h': 8}"
                                "}], "
                                "'errors': [{"
                                "'item_id': 100, "
                                "'message': 'provider rejected item', "
                                "'object_key': 'images/100.jpg', "
                                "'detail': {'reason': 'blurry'}"
                                "}], "
                                "'total_items': 2, "
                                "'succeeded_items': 1, "
                                "'failed_items': 1"
                                "}))"
                            ),
                        ],
                    },
                },
            }
        )

        self.assertEqual("succeeded_with_errors", result["status"])
        self.assertEqual(2, result["total_items"])
        self.assertEqual(1, result["succeeded_items"])
        self.assertEqual(1, result["failed_items"])
        self.assertEqual("progress", result["events"][0]["event_type"])
        self.assertEqual("item_failed", result["events"][1]["event_type"])
        self.assertEqual(100, result["events"][1]["item_id"])
        self.assertEqual("provider rejected item", result["events"][1]["message"])
        self.assertEqual("blurry", result["events"][1]["detail_json"]["reason"])
        self.assertEqual("images/100.jpg", result["events"][1]["detail_json"]["object_key"])
        materialized = _event_by_type(result, "review_candidates_materialized")
        self.assertEqual(1, materialized["detail_json"]["result_count"])

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
        self.assertEqual("progress", result["events"][0]["event_type"])
        materialized = _event_by_type(result, "review_candidates_materialized")
        self.assertEqual("review_candidates_materialized", materialized["event_type"])
        self.assertEqual("info", materialized["event_level"])
        self.assertEqual(2, materialized["detail_json"]["result_count"])
        self.assertEqual("grounding-dino-mvp", materialized["detail_json"]["candidates"][0]["model_name"])
        self.assertTrue(materialized["detail_json"]["candidates"][0]["is_pseudo"])


if __name__ == "__main__":
    unittest.main()
