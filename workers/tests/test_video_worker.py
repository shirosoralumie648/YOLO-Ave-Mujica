import unittest
import sys

from workers.video.main import run_video_job


class VideoWorkerContractTest(unittest.TestCase):
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
        self.assertEqual(1, result["events"][0]["detail_json"]["result_count"])
        self.assertEqual(3, result["events"][0]["detail_json"]["frames"][0]["frame_index"])
        self.assertEqual("clips/a/frame-0003.jpg", result["events"][0]["detail_json"]["frames"][0]["object_key"])

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
        self.assertEqual(1, len(result["events"]))
        materialized = result["events"][0]
        self.assertEqual("video_frames_materialized", materialized["event_type"])
        self.assertEqual(2, materialized["detail_json"]["result_count"])
        self.assertEqual(6, materialized["detail_json"]["frames"][1]["frame_index"])


if __name__ == "__main__":
    unittest.main()
