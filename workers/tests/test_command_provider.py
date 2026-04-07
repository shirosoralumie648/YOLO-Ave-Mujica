import subprocess
import unittest
from unittest import mock

from workers.common.command_provider import load_provider_result


class CommandProviderContractTest(unittest.TestCase):
    def test_load_provider_result_supports_builtin_grounding_dino_fake(self):
        payload = {
            "dataset_id": 7,
            "snapshot_id": 3,
            "prompt": "person",
            "items": [
                {"item_id": 11, "object_key": "train/a.jpg"},
                {"item_id": 12, "object_key": "train/b.jpg"},
            ],
            "provider": {
                "type": "builtin",
                "name": "grounding_dino_fake",
            },
        }

        result = load_provider_result(payload)

        self.assertEqual(2, result["total_items"])
        self.assertEqual(2, result["succeeded_items"])
        self.assertEqual(0, result["failed_items"])
        self.assertEqual(2, len(result["candidates"]))
        self.assertEqual(11, result["candidates"][0]["item_id"])
        self.assertEqual("train/a.jpg", result["candidates"][0]["object_key"])
        self.assertEqual("person", result["candidates"][0]["category_name"])
        self.assertEqual("grounding_dino_fake", result["candidates"][0]["model_name"])

    def test_load_provider_result_supports_builtin_video_decode_fake(self):
        payload = {
            "dataset_id": 7,
            "fps": 2,
            "duration_ms": 3000,
            "source_object_key": "clips/a.mp4",
            "provider": {
                "type": "builtin",
                "name": "video_decode_fake",
            },
        }

        result = load_provider_result(payload)

        self.assertEqual(7, result["total_items"])
        self.assertEqual(7, result["succeeded_items"])
        self.assertEqual(0, result["failed_items"])
        self.assertEqual(7, len(result["frames"]))
        self.assertEqual(0, result["frames"][0]["frame_index"])
        self.assertEqual(3000, result["frames"][-1]["timestamp_ms"])
        self.assertEqual("clips/a/frame-0006.jpg", result["frames"][-1]["object_key"])

    def test_load_provider_result_passes_timeout_seconds(self):
        payload = {
            "provider": {
                "type": "command",
                "argv": ["python3", "/opt/provider.py"],
                "timeout_seconds": 7.5,
            }
        }

        completed = subprocess.CompletedProcess(
            args=["python3", "/opt/provider.py"],
            returncode=0,
            stdout='{"frames":[]}',
            stderr="",
        )

        with mock.patch("workers.common.command_provider.subprocess.run", return_value=completed) as run:
            load_provider_result(payload)

        self.assertEqual(7.5, run.call_args.kwargs["timeout"])

    def test_load_provider_result_reports_timeout(self):
        payload = {
            "provider": {
                "type": "command",
                "argv": ["python3", "/opt/provider.py"],
                "timeout_seconds": 3,
            }
        }

        with mock.patch(
            "workers.common.command_provider.subprocess.run",
            side_effect=subprocess.TimeoutExpired(cmd=["python3", "/opt/provider.py"], timeout=3),
        ):
            with self.assertRaisesRegex(RuntimeError, "timed out"):
                load_provider_result(payload)

    def test_load_provider_result_rejects_non_positive_timeout(self):
        payload = {
            "provider": {
                "type": "command",
                "argv": ["python3", "/opt/provider.py"],
                "timeout_seconds": 0,
            }
        }

        completed = subprocess.CompletedProcess(
            args=["python3", "/opt/provider.py"],
            returncode=0,
            stdout='{"frames":[]}',
            stderr="",
        )

        with mock.patch("workers.common.command_provider.subprocess.run", return_value=completed):
            with self.assertRaisesRegex(ValueError, "provider.timeout_seconds"):
                load_provider_result(payload)

    def test_load_provider_result_reports_invalid_json(self):
        payload = {
            "provider": {
                "type": "command",
                "argv": ["python3", "/opt/provider.py"],
            }
        }

        completed = subprocess.CompletedProcess(
            args=["python3", "/opt/provider.py"],
            returncode=0,
            stdout="not-json",
            stderr="",
        )

        with mock.patch("workers.common.command_provider.subprocess.run", return_value=completed):
            with self.assertRaisesRegex(RuntimeError, "invalid JSON"):
                load_provider_result(payload)

    def test_load_provider_result_rejects_non_object_document(self):
        payload = {
            "provider": {
                "type": "command",
                "argv": ["python3", "/opt/provider.py"],
            }
        }

        completed = subprocess.CompletedProcess(
            args=["python3", "/opt/provider.py"],
            returncode=0,
            stdout="[]",
            stderr="",
        )

        with mock.patch("workers.common.command_provider.subprocess.run", return_value=completed):
            with self.assertRaisesRegex(ValueError, "JSON object"):
                load_provider_result(payload)


if __name__ == "__main__":
    unittest.main()
