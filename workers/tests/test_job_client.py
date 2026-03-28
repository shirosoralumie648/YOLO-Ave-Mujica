import unittest

from workers.common.job_client import emit_heartbeat, emit_terminal


class JobClientContractTest(unittest.TestCase):
    def test_emit_heartbeat_payload(self):
        payload = emit_heartbeat(job_id=1, worker_id="worker-a", lease_seconds=30)
        self.assertEqual(payload["event_type"], "heartbeat")
        self.assertEqual(payload["job_id"], 1)
        self.assertEqual(payload["detail_json"]["worker_id"], "worker-a")
        self.assertEqual(payload["detail_json"]["lease_seconds"], 30)

    def test_emit_terminal_payload(self):
        payload = emit_terminal(job_id=2, worker_id="worker-a", status="succeeded_with_errors", total=10, ok=9, failed=1)
        self.assertEqual(payload["worker_id"], "worker-a")
        self.assertEqual(payload["status"], "succeeded_with_errors")
        self.assertEqual(payload["failed_items"], 1)


if __name__ == "__main__":
    unittest.main()
