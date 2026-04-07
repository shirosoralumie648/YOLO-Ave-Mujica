import unittest

from workers.common.job_client import JobClient, emit_heartbeat, emit_terminal


class _FakeResponse:
    def __init__(self, code: int = 200):
        self.code = code

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        return False


class _FakeOpener:
    def __init__(self):
        self.requests = []

    def __call__(self, request):
        self.requests.append(request)
        return _FakeResponse()


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

    def test_job_client_build_terminal_payload(self):
        client = JobClient(base_url="http://api.local")

        body = client.build_terminal(job_id=5, worker_id="worker-a", status="succeeded", total=3, ok=3, failed=0)

        self.assertEqual("worker-a", body["worker_id"])
        self.assertEqual("succeeded", body["status"])
        self.assertEqual(3, body["total_items"])

    def test_job_client_posts_terminal_update(self):
        opener = _FakeOpener()
        client = JobClient(base_url="http://api.local", opener=opener)

        client.post_terminal(job_id=5, worker_id="worker-a", status="succeeded", total=3, ok=3, failed=0)

        self.assertEqual(1, len(opener.requests))
        request = opener.requests[0]
        self.assertEqual("http://api.local/internal/jobs/5/complete", request.full_url)
        self.assertEqual("POST", request.get_method())
        self.assertIn(b'"status": "succeeded"', request.data)

    def test_job_client_posts_terminal_result_ref(self):
        opener = _FakeOpener()
        client = JobClient(base_url="http://api.local", opener=opener)

        client.post_terminal(
            job_id=5,
            worker_id="worker-a",
            status="succeeded",
            total=3,
            ok=3,
            failed=0,
            result_ref={"result_type": "annotation_candidates", "result_count": 3},
        )

        request = opener.requests[0]
        self.assertIn(b'"result_ref": {"result_type": "annotation_candidates", "result_count": 3}', request.data)

    def test_job_client_posts_dispatch_event(self):
        opener = _FakeOpener()
        client = JobClient(base_url="http://api.local", opener=opener)

        client.post_event(
            job_id=5,
            event_type="dispatch_rejected",
            message="worker cannot handle dispatched job",
            detail={"reason": "missing_capabilities"},
            level="warn",
        )

        self.assertEqual(1, len(opener.requests))
        request = opener.requests[0]
        self.assertEqual("http://api.local/internal/jobs/5/events", request.full_url)
        self.assertEqual("POST", request.get_method())
        self.assertIn(b'"event_type": "dispatch_rejected"', request.data)

    def test_job_client_propagates_trace_headers(self):
        opener = _FakeOpener()
        client = JobClient(base_url="http://api.local", opener=opener, trace_id="trace-worker-123")

        client.post_terminal(job_id=5, worker_id="worker-a", status="succeeded", total=3, ok=3, failed=0)

        request = opener.requests[0]
        self.assertEqual("trace-worker-123", request.get_header("X-request-id"))
        self.assertEqual("trace-worker-123", request.get_header("X-correlation-id"))

    def test_job_client_registers_worker_metadata(self):
        opener = _FakeOpener()
        client = JobClient(base_url="http://api.local", opener=opener)

        client.register_worker(
            {
                "worker_id": "zero-shot-a",
                "resource_lane": "jobs:gpu",
                "job_types": ["zero-shot"],
                "capabilities": ["zero_shot_inference", "grounding_dino"],
            }
        )

        self.assertEqual(1, len(opener.requests))
        request = opener.requests[0]
        self.assertEqual("http://api.local/internal/jobs/workers/register", request.full_url)
        self.assertEqual("POST", request.get_method())
        self.assertIn(b'"worker_id": "zero-shot-a"', request.data)
        self.assertIn(b'"resource_lane": "jobs:gpu"', request.data)
        self.assertIn(b'"job_types": ["zero-shot"]', request.data)


if __name__ == "__main__":
    unittest.main()
