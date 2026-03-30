import unittest

from workers.packager.main import run_package_job


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


class PackagerWorkerTest(unittest.TestCase):
    def test_run_package_job_completes_artifact_with_bundle_entries(self):
        opener = _FakeOpener()

        result = run_package_job(
            {
                "job_id": 7,
                "payload": {
                    "artifact_id": 9,
                    "names": ["person"],
                },
            },
            base_url="http://api.local",
            opener=opener,
        )

        self.assertEqual("succeeded", result["status"])
        self.assertEqual(1, len(opener.requests))
        request = opener.requests[0]
        self.assertEqual("http://api.local/internal/artifacts/9/complete", request.full_url)
        self.assertEqual("POST", request.get_method())
        self.assertIn(b'"entries"', request.data)


if __name__ == "__main__":
    unittest.main()
