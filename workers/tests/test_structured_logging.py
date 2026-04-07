import io
import json
import unittest
from datetime import datetime, timezone

from workers.common.structured_logging import WorkerLogger


class StructuredLoggingContractTest(unittest.TestCase):
    def test_worker_logger_emits_json_line_with_context(self):
        stream = io.StringIO()
        logger = WorkerLogger(
            component="zero-shot-worker",
            stream=stream,
            now=lambda: datetime(2026, 4, 6, 8, 30, 0, tzinfo=timezone.utc),
        )

        logger.info(
            "job_dequeued",
            worker_id="zero-shot-a",
            queue_lane="jobs:gpu",
            job_id=7,
            trace_id="trace-zero-shot-7",
        )

        payload = json.loads(stream.getvalue().strip())
        self.assertEqual("2026-04-06T08:30:00Z", payload["ts"])
        self.assertEqual("info", payload["level"])
        self.assertEqual("zero-shot-worker", payload["component"])
        self.assertEqual("job_dequeued", payload["message"])
        self.assertEqual("zero-shot-a", payload["worker_id"])
        self.assertEqual("jobs:gpu", payload["queue_lane"])
        self.assertEqual(7, payload["job_id"])
        self.assertEqual("trace-zero-shot-7", payload["trace_id"])


if __name__ == "__main__":
    unittest.main()
