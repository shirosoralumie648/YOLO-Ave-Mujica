import unittest

from workers.importer.main import parse_import_payload


class CocoImporterContractTest(unittest.TestCase):
    def test_parse_coco_payload_returns_boxes(self):
        boxes = parse_import_payload(
            {
                "format": "coco",
                "images": [
                    {"id": 1, "file_name": "train/a.jpg"},
                    {"id": 2, "file_name": "train/b.jpg"},
                ],
                "categories": [
                    {"id": 10, "name": "person"},
                    {"id": 11, "name": "helmet"},
                ],
                "annotations": [
                    {"id": 100, "image_id": 1, "category_id": 10, "bbox": [10, 20, 30, 40]},
                    {"id": 101, "image_id": 2, "category_id": 11, "bbox": [1, 2, 3, 4]},
                ],
            }
        )

        self.assertEqual(
            [
                {
                    "object_key": "train/a.jpg",
                    "category_name": "person",
                    "bbox_x": 10.0,
                    "bbox_y": 20.0,
                    "bbox_w": 30.0,
                    "bbox_h": 40.0,
                },
                {
                    "object_key": "train/b.jpg",
                    "category_name": "helmet",
                    "bbox_x": 1.0,
                    "bbox_y": 2.0,
                    "bbox_w": 3.0,
                    "bbox_h": 4.0,
                },
            ],
            boxes,
        )


if __name__ == "__main__":
    unittest.main()
