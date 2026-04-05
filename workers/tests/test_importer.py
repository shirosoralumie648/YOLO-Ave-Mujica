import unittest
import tempfile
import zipfile
from pathlib import Path

from workers.importer.main import build_importer_runner, parse_import_payload, run_import_job


class _FakeResponse:
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


class ImporterContractTest(unittest.TestCase):
    def test_parse_yolo_payload_returns_boxes(self):
        payload = {
            "format": "yolo",
            "labels": {"train/a.txt": "0 0.5 0.5 0.2 0.2\n"},
            "names": ["person"],
            "images": {"train/a.txt": "train/a.jpg"},
        }

        boxes = parse_import_payload(payload)

        self.assertEqual(1, len(boxes))
        self.assertEqual("train/a.jpg", boxes[0]["object_key"])
        self.assertEqual("person", boxes[0]["category_name"])

    def test_run_import_job_returns_terminal_counters(self):
        result = run_import_job(
            {
                "format": "yolo",
                "labels": {"train/a.txt": "0 0.5 0.5 0.2 0.2\n"},
                "names": ["person"],
                "images": {"train/a.txt": "train/a.jpg"},
            }
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(1, result["total_items"])
        self.assertEqual(1, result["succeeded_items"])
        self.assertEqual(0, result["failed_items"])

    def test_run_import_job_posts_internal_import_callback(self):
        opener = _FakeOpener()

        result = run_import_job(
            {
                "snapshot_id": 3,
                "payload": {
                    "format": "yolo",
                    "labels": {"train/a.txt": "0 0.5 0.5 0.2 0.2\n"},
                    "names": ["person"],
                    "images": {"train/a.txt": "train/a.jpg"},
                },
            },
            base_url="http://api.local",
            opener=opener,
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(1, len(opener.requests))
        request = opener.requests[0]
        self.assertEqual("http://api.local/internal/snapshots/3/import", request.full_url)
        self.assertEqual("POST", request.get_method())
        self.assertIn(b'"entries"', request.data)

    def test_run_import_job_parses_source_download_url_zip(self):
        opener = _FakeOpener()
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "import.zip"
            with zipfile.ZipFile(archive_path, "w") as zf:
                zf.writestr(
                    "data.yaml",
                    "train: images/train\nval: images/val\nnames:\n  - person\n",
                )
                zf.writestr("labels/train/a.txt", "0 0.5 0.5 0.2 0.2\n")

            result = run_import_job(
                {
                    "snapshot_id": 4,
                    "payload": {
                        "format": "yolo",
                        "source_download_url": archive_path.as_uri(),
                    },
                },
                base_url="http://api.local",
                opener=opener,
            )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(1, result["total_items"])
        request = opener.requests[0]
        self.assertIn(b'"train/a.jpg"', request.data)
        self.assertIn(b'"person"', request.data)

    def test_build_importer_runner_accepts_snapshot_import_jobs(self):
        runner = build_importer_runner(worker_id="importer-a")

        handled = []
        dispatched = runner.handle_once(
            {"job_id": 8, "job_type": "snapshot-import", "payload": {"format": "yolo"}},
            lambda job: handled.append(job["job_id"]),
        )

        self.assertTrue(dispatched)
        self.assertEqual([8], handled)

    def test_build_importer_runner_accepts_smoke_capability_aliases(self):
        runner = build_importer_runner(worker_id="importer-a")

        self.assertTrue(
            runner.can_handle(
                {
                    "job_id": 9,
                    "job_type": "snapshot-import",
                    "required_resource_type": "cpu",
                    "required_capabilities": ["importer", "yolo"],
                    "payload": {"format": "yolo"},
                },
                lane="jobs:cpu",
            )
        )


if __name__ == "__main__":
    unittest.main()
