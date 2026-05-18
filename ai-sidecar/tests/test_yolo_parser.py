import unittest
import sys
from pathlib import Path

from PIL import Image

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from app.inference import _run_yolo


class _InputMeta:
    name = "images"
    shape = [1, 3, 640, 640]


class _YoloSession:
    def __init__(self, output):
        self.output = output

    def get_inputs(self):
        return [_InputMeta()]

    def run(self, _outputs, _inputs):
        return [self.output]


class YoloParserTests(unittest.TestCase):
    def test_detects_phone_from_yolo_output_with_objectness_column(self):
        import numpy as np

        output = np.zeros((1, 85, 100), dtype=np.float32)
        output[0, 0, 0] = 320
        output[0, 1, 0] = 240
        output[0, 2, 0] = 160
        output[0, 3, 0] = 120
        output[0, 4, 0] = 0.9
        output[0, 5 + 67, 0] = 0.95

        image = Image.new("RGB", (640, 640), color="white")

        detections = _run_yolo(_YoloSession(output), image, confidence_threshold=0.35)

        self.assertEqual(len(detections), 1)
        self.assertEqual(detections[0]["object_type"], "phone")
        self.assertGreater(detections[0]["confidence"], 0.85)


if __name__ == "__main__":
    unittest.main()
