import unittest
import sys

from workers.video.main import run_video_job


def _event_by_type(result: dict, event_type: str) -> dict:
    for event in result["events"]:
        if event["event_type"] == event_type:
            return event
    raise AssertionError(f"missing event_type={event_type!r} in {result['events']!r}")


class VideoWorkerContractTest(unittest.TestCase):
    def test_run_video_job_rejects_negative_frame_index(self):
        with self.assertRaisesRegex(ValueError, "frame_index must be >= 0"):
            run_video_job(
                {
                    "job_id": 10,
                    "payload": {
                        "dataset_id": 1,
                        "frames": [
                            {"frame_index": -1, "timestamp_ms": 0, "object_key": "clips/a/frame-0000.jpg"},
                        ],
                    },
                }
            )

    def test_run_video_job_rejects_negative_timestamp(self):
        with self.assertRaisesRegex(ValueError, "timestamp_ms must be >= 0"):
            run_video_job(
                {
                    "job_id": 11,
                    "payload": {
                        "dataset_id": 1,
                        "frames": [
                            {"frame_index": 0, "timestamp_ms": -10, "object_key": "clips/a/frame-0000.jpg"},
                        ],
                    },
                }
            )

    def test_run_video_job_uses_provider_summary_counters(self):
        result = run_video_job(
            {
                "job_id": 9,
                "payload": {
                    "dataset_id": 1,
                    "provider": {
                        "type": "command",
                        "argv": [
                            sys.executable,
                            "-c",
                            (
                                "import json, sys; "
                                "json.load(sys.stdin); "
                                "print(json.dumps({"
                                "'frames': [{"
                                "'frame_index': 4, "
                                "'timestamp_ms': 2000, "
                                "'object_key': 'clips/a/frame-0004.jpg'"
                                "}], "
                                "'total_items': 5, "
                                "'succeeded_items': 4, "
                                "'failed_items': 1"
                                "}))"
                            ),
                        ],
                    },
                },
            }
        )

        self.assertEqual("succeeded_with_errors", result["status"])
        self.assertEqual(5, result["total_items"])
        self.assertEqual(4, result["succeeded_items"])
        self.assertEqual(1, result["failed_items"])
        self.assertEqual("progress", result["events"][0]["event_type"])
        self.assertEqual(1, _event_by_type(result, "video_frames_materialized")["detail_json"]["result_count"])

    def test_run_video_job_uses_builtin_provider_output_from_source_context(self):
        result = run_video_job(
            {
                "job_id": 6,
                "payload": {
                    "dataset_id": 1,
                    "fps": 2,
                    "duration_ms": 3000,
                    "source_object_key": "clips/a.mp4",
                    "provider": {
                        "type": "builtin",
                        "name": "video_decode_fake",
                    },
                },
            }
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(7, result["total_items"])
        materialized = _event_by_type(result, "video_frames_materialized")
        self.assertEqual(7, materialized["detail_json"]["result_count"])
        self.assertEqual(0, materialized["detail_json"]["frames"][0]["frame_index"])
        self.assertEqual("clips/a/frame-0006.jpg", materialized["detail_json"]["frames"][-1]["object_key"])

    def test_run_video_job_uses_command_provider_output(self):
        result = run_video_job(
            {
                "job_id": 8,
                "payload": {
                    "dataset_id": 1,
                    "provider": {
                        "type": "command",
                        "argv": [
                            sys.executable,
                            "-c",
                            (
                                "import json, sys; "
                                "json.load(sys.stdin); "
                                "print(json.dumps({'frames': [{"
                                "'frame_index': 3, "
                                "'timestamp_ms': 1500, "
                                "'object_key': 'clips/a/frame-0003.jpg'"
                                "}]}))"
                            ),
                        ],
                    },
                },
            }
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(1, result["total_items"])
        materialized = _event_by_type(result, "video_frames_materialized")
        self.assertEqual(1, materialized["detail_json"]["result_count"])
        self.assertEqual(3, materialized["detail_json"]["frames"][0]["frame_index"])
        self.assertEqual("clips/a/frame-0003.jpg", materialized["detail_json"]["frames"][0]["object_key"])

    def test_run_video_job_emits_progress_and_frame_failure_events_for_partial_provider_result(self):
        result = run_video_job(
            {
                "job_id": 12,
                "payload": {
                    "dataset_id": 1,
                    "provider": {
                        "type": "command",
                        "argv": [
                            sys.executable,
                            "-c",
                            (
                                "import json, sys; "
                                "json.load(sys.stdin); "
                                "print(json.dumps({"
                                "'frames': [{"
                                "'frame_index': 3, "
                                "'timestamp_ms': 1500, "
                                "'object_key': 'clips/a/frame-0003.jpg'"
                                "}], "
                                "'frame_errors': [{"
                                "'frame_index': 4, "
                                "'timestamp_ms': 2000, "
                                "'object_key': 'clips/a/frame-0004.jpg', "
                                "'message': 'decode failed', "
                                "'detail': {'reason': 'corrupt-frame'}"
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
        self.assertEqual("frame_failed", result["events"][1]["event_type"])
        self.assertEqual("decode failed", result["events"][1]["message"])
        self.assertEqual(4, result["events"][1]["detail_json"]["frame_index"])
        self.assertEqual("corrupt-frame", result["events"][1]["detail_json"]["reason"])
        materialized = _event_by_type(result, "video_frames_materialized")
        self.assertEqual(1, materialized["detail_json"]["result_count"])

    def test_run_video_job_emits_frame_results_and_result_ref(self):
        result = run_video_job(
            {
                "job_id": 7,
                "payload": {
                    "dataset_id": 1,
                    "fps": 2,
                    "frames": [
                        {"frame_index": 0, "timestamp_ms": 0, "object_key": "clips/a/frame-0000.jpg"},
                        {"frame_index": 6, "timestamp_ms": 3000, "object_key": "clips/a/frame-0006.jpg"},
                    ],
                },
            }
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(2, result["total_items"])
        self.assertEqual(2, result["succeeded_items"])
        self.assertEqual(0, result["failed_items"])
        self.assertEqual({"result_type": "video_frames", "result_count": 2}, result["result_ref"])
        self.assertEqual("progress", result["events"][0]["event_type"])
        materialized = _event_by_type(result, "video_frames_materialized")
        self.assertEqual("video_frames_materialized", materialized["event_type"])
        self.assertEqual(2, materialized["detail_json"]["result_count"])
        self.assertEqual(6, materialized["detail_json"]["frames"][1]["frame_index"])


if __name__ == "__main__":
    unittest.main()
