import unittest
from unittest import mock

from workers.common.queue_runner import QueueRunner, RedisQueueClient, dispatch_once


class _FakeQueueClient:
    def __init__(self, payload):
        self.payload = payload
        self.calls = []
        self.requeued = []

    def pop_json(self, lane: str, timeout_seconds: int = 5):
        self.calls.append((lane, timeout_seconds))
        payload = self.payload
        self.payload = None
        return payload

    def push_json(self, lane: str, payload: dict):
        self.requeued.append((lane, payload))


class _FakeJobClient:
    def __init__(self):
        self.calls = []

    def post_heartbeat(self, job_id: int, worker_id: str, lease_seconds: int):
        self.calls.append(("heartbeat", job_id, worker_id, lease_seconds))

    def post_terminal(self, job_id: int, worker_id: str, status: str, total: int, ok: int, failed: int, result_ref=None):
        self.calls.append(("terminal", job_id, worker_id, status, total, ok, failed, result_ref))

    def post_event(self, job_id: int, event_type: str, message: str, detail: dict, level: str = "warn", item_id=None):
        self.calls.append(("event", job_id, event_type, message, detail, level))


class _FakeRedisFile:
    def __init__(self, response: bytes):
        self.response = response.splitlines(keepends=True)
        self.written = b""

    def write(self, data: bytes):
        self.written += data

    def flush(self):
        return None

    def readline(self):
        if not self.response:
            return b""
        return self.response.pop(0)


class _FakeRedisConn:
    def __init__(self, response: bytes):
        self.file = _FakeRedisFile(response)

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        return False

    def settimeout(self, timeout: int):
        return None

    def makefile(self, mode: str):
        return self.file


class QueueRunnerContractTest(unittest.TestCase):
    def test_queue_runner_dispatches_matching_job_type(self):
        runner = QueueRunner(worker_id="packager-a", accepted_job_types={"artifact-package"})
        payload = {"job_id": 1, "job_type": "artifact-package", "payload": {"format": "yolo"}}
        handled = []

        dispatched = runner.handle_once(payload, lambda job: handled.append(job["job_id"]))

        self.assertTrue(dispatched)
        self.assertEqual([1], handled)

    def test_queue_runner_ignores_non_matching_job_type(self):
        runner = QueueRunner(worker_id="packager-a", accepted_job_types={"artifact-package"})
        payload = {"job_id": 2, "job_type": "zero-shot", "payload": {"prompt": "person"}}
        handled = []

        dispatched = runner.handle_once(payload, lambda job: handled.append(job["job_id"]))

        self.assertFalse(dispatched)
        self.assertEqual([], handled)

    def test_dispatch_once_polls_queue_and_invokes_runner(self):
        queue_client = _FakeQueueClient({"job_id": 3, "job_type": "artifact-package", "payload": {"format": "yolo"}})
        runner = QueueRunner(worker_id="packager-a", accepted_job_types={"artifact-package"})
        handled = []

        dispatched = dispatch_once(queue_client, "jobs:cpu", runner, lambda job: handled.append(job["job_id"]))

        self.assertTrue(dispatched)
        self.assertEqual([("jobs:cpu", 5)], queue_client.calls)
        self.assertEqual([3], handled)

    def test_dispatch_once_posts_worker_callbacks(self):
        queue_client = _FakeQueueClient({"job_id": 4, "job_type": "artifact-package", "payload": {"format": "yolo"}})
        runner = QueueRunner(worker_id="packager-a", accepted_job_types={"artifact-package"})
        job_client = _FakeJobClient()

        dispatched = dispatch_once(
            queue_client,
            "jobs:cpu",
            runner,
            lambda job: {"status": "succeeded", "total_items": 1, "succeeded_items": 1, "failed_items": 0},
            job_client=job_client,
        )

        self.assertTrue(dispatched)
        self.assertEqual(
            [
                ("heartbeat", 4, "packager-a", 30),
                ("terminal", 4, "packager-a", "succeeded", 1, 1, 0, None),
            ],
            job_client.calls,
        )

    def test_dispatch_once_posts_result_events_before_terminal(self):
        queue_client = _FakeQueueClient({"job_id": 10, "job_type": "zero-shot", "payload": {"prompt": "person"}})
        runner = QueueRunner(worker_id="zero-shot-a", accepted_job_types={"zero-shot"})
        job_client = _FakeJobClient()

        dispatched = dispatch_once(
            queue_client,
            "jobs:gpu",
            runner,
            lambda job: {
                "status": "succeeded",
                "total_items": 2,
                "succeeded_items": 2,
                "failed_items": 0,
                "events": [
                    {
                        "event_type": "review_candidates_materialized",
                        "message": "persisted review candidates",
                        "detail_json": {
                            "result_type": "annotation_candidates",
                            "result_count": 2,
                            "candidates": [{"item_id": 1}, {"item_id": 2}],
                        },
                        "event_level": "info",
                    }
                ],
                "result_ref": {
                    "result_type": "annotation_candidates",
                    "result_count": 2,
                },
            },
            job_client=job_client,
        )

        self.assertTrue(dispatched)
        self.assertEqual(
            [
                ("heartbeat", 10, "zero-shot-a", 30),
                (
                    "event",
                    10,
                    "review_candidates_materialized",
                    "persisted review candidates",
                    {
                        "result_type": "annotation_candidates",
                        "result_count": 2,
                        "candidates": [{"item_id": 1}, {"item_id": 2}],
                    },
                    "info",
                ),
                (
                    "terminal",
                    10,
                    "zero-shot-a",
                    "succeeded",
                    2,
                    2,
                    0,
                    {
                        "result_type": "annotation_candidates",
                        "result_count": 2,
                    },
                ),
            ],
            job_client.calls,
        )

    def test_dispatch_once_requeues_non_matching_job_with_dispatch_rejected_event(self):
        payload = {"job_id": 6, "job_type": "snapshot-import", "payload": {"format": "yolo"}}
        queue_client = _FakeQueueClient(payload)
        runner = QueueRunner(worker_id="packager-a", accepted_job_types={"artifact-package"})
        job_client = _FakeJobClient()

        dispatched = dispatch_once(
            queue_client,
            "jobs:cpu",
            runner,
            lambda job: {"status": "succeeded", "total_items": 1, "succeeded_items": 1, "failed_items": 0},
            job_client=job_client,
        )

        self.assertFalse(dispatched)
        self.assertEqual([("jobs:cpu", payload)], queue_client.requeued)
        self.assertEqual(
            [
                (
                    "event",
                    6,
                    "dispatch_rejected",
                    "worker cannot handle dispatched job",
                    {
                        "reason": "job_type_mismatch",
                        "queue_lane": "jobs:cpu",
                        "target_lane": "jobs:cpu",
                        "worker": {
                            "worker_id": "packager-a",
                            "resource_lane": None,
                            "capabilities": [],
                            "job_types": ["artifact-package"],
                        },
                    },
                    "warn",
                )
            ],
            job_client.calls,
        )

    def test_redis_queue_client_falls_back_to_socket_when_redis_cli_is_missing(self):
        client = RedisQueueClient(redis_addr="redis.local:6379", redis_cli_bin="missing-cli")
        fake_conn = _FakeRedisConn(
            b'*2\r\n$8\r\njobs:cpu\r\n$42\r\n{"job_id":5,"job_type":"artifact-package"}\r\n'
        )

        with mock.patch("workers.common.queue_runner.subprocess.run", side_effect=FileNotFoundError()), \
             mock.patch("workers.common.queue_runner.socket.create_connection", return_value=fake_conn):
            payload = client.pop_json("jobs:cpu", timeout_seconds=5)

        self.assertEqual(5, payload["job_id"])
        self.assertEqual("artifact-package", payload["job_type"])
        self.assertIn(b"BRPOP", fake_conn.file.written)

    def test_queue_runner_requires_matching_lane_and_capabilities(self):
        runner = QueueRunner(
            worker_id="zero-shot-a",
            accepted_job_types={"zero-shot"},
            resource_lane="jobs:gpu",
            capabilities={"zero_shot_inference"},
        )

        self.assertTrue(
            runner.can_handle(
                {
                    "job_id": 8,
                    "job_type": "zero-shot",
                    "resource_lane": "jobs:gpu",
                    "required_capabilities": ["zero_shot_inference"],
                },
                lane="jobs:gpu",
            )
        )

    def test_dispatch_once_requeues_capability_mismatch_and_posts_dispatch_rejected_event(self):
        payload = {
            "job_id": 7,
            "job_type": "zero-shot",
            "resource_lane": "jobs:gpu",
            "required_capabilities": ["zero_shot_inference", "sampler"],
            "payload": {"prompt": "person"},
        }
        queue_client = _FakeQueueClient(payload)
        runner = QueueRunner(
            worker_id="zero-shot-a",
            accepted_job_types={"zero-shot"},
            resource_lane="jobs:gpu",
            capabilities={"zero_shot_inference"},
        )
        job_client = _FakeJobClient()

        dispatched = dispatch_once(queue_client, "jobs:gpu", runner, lambda job: {"status": "succeeded"}, job_client=job_client)

        self.assertFalse(dispatched)
        self.assertEqual([("jobs:gpu", payload)], queue_client.requeued)
        self.assertEqual(
            [
                (
                    "event",
                    7,
                    "dispatch_rejected",
                    "worker cannot handle dispatched job",
                    {
                        "reason": "missing_capabilities",
                        "queue_lane": "jobs:gpu",
                        "target_lane": "jobs:gpu",
                        "worker": {
                            "worker_id": "zero-shot-a",
                            "resource_lane": "jobs:gpu",
                            "capabilities": ["zero_shot_inference"],
                            "job_types": ["zero-shot"],
                        },
                    },
                    "warn",
                )
            ],
            job_client.calls,
        )

    def test_dispatch_once_requeues_to_expected_lane_and_posts_dispatch_requeued_event(self):
        payload = {
            "job_id": 9,
            "job_type": "zero-shot",
            "resource_lane": "jobs:gpu",
            "required_capabilities": ["zero_shot_inference"],
            "payload": {"prompt": "person"},
        }
        queue_client = _FakeQueueClient(payload)
        runner = QueueRunner(
            worker_id="zero-shot-a",
            accepted_job_types={"zero-shot"},
            resource_lane="jobs:cpu",
            capabilities={"zero_shot_inference"},
        )
        job_client = _FakeJobClient()

        dispatched = dispatch_once(queue_client, "jobs:cpu", runner, lambda job: {"status": "succeeded"}, job_client=job_client)

        self.assertFalse(dispatched)
        self.assertEqual([("jobs:gpu", payload)], queue_client.requeued)
        self.assertEqual("dispatch_requeued", job_client.calls[0][2])


if __name__ == "__main__":
    unittest.main()
