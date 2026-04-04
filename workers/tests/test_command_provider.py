import subprocess
import unittest
from unittest import mock

from workers.common.command_provider import load_provider_result


class CommandProviderContractTest(unittest.TestCase):
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
