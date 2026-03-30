import unittest

from workers.cleaning.main import run_rules


class CleaningRulesTest(unittest.TestCase):
    def test_cleaning_flags_zero_area_and_dark_score(self):
        report = run_rules(
            [
                {"item_id": 1, "bbox_w": 0, "bbox_h": 10, "brightness": 0.2, "category": "person"},
                {"item_id": 2, "bbox_w": 8, "bbox_h": 8, "brightness": 0.8, "category": "unknown"},
            ],
            taxonomy={"person"},
            dark_threshold=0.3,
        )

        self.assertEqual(report["summary"]["invalid_bbox"], 1)
        self.assertEqual(report["summary"]["too_dark"], 1)
        self.assertEqual(report["summary"]["category_mismatch"], 1)


if __name__ == "__main__":
    unittest.main()

