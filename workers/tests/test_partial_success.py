import unittest

from workers.zero_shot.main import summarize_batch


class PartialSuccessTest(unittest.TestCase):
    def test_summarize_batch_partial_success(self):
        status, summary = summarize_batch(total=1000, ok=995, failed=5)
        self.assertEqual(status, "succeeded_with_errors")
        self.assertEqual(summary["failed_items"], 5)


if __name__ == "__main__":
    unittest.main()
