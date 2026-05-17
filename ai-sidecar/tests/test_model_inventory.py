import unittest
import sys
import shutil
import uuid
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
TEST_TMP = Path(__file__).resolve().parents[1] / ".test-tmp"


def empty_model_dir(name: str) -> Path:
    path = TEST_TMP / f"{name}-{uuid.uuid4().hex}"
    path.mkdir(parents=True)
    return path

from app.config import SidecarSettings
from app.models import ModelInventory


class ModelInventoryTests(unittest.TestCase):
    def test_missing_models_fail_fast(self) -> None:
        model_dir = empty_model_dir("missing-fail-fast")
        try:
            settings = SidecarSettings(models_path=model_dir, model_fail_fast=True)

            with self.assertRaises(FileNotFoundError) as ctx:
                ModelInventory.load(settings)

            message = str(ctx.exception)
            self.assertIn("yolo", message)
            self.assertIn("arcface", message)
        finally:
            safe_rmtree(model_dir)

    def test_non_fail_fast_reports_missing_models_as_unloaded(self) -> None:
        model_dir = empty_model_dir("missing-non-fail-fast")
        try:
            settings = SidecarSettings(models_path=model_dir, model_fail_fast=False)

            inventory = ModelInventory.load(settings)

            self.assertFalse(inventory.yolo.loaded)
            self.assertFalse(inventory.arcface.loaded)
            self.assertEqual(inventory.status(), "degraded")
        finally:
            safe_rmtree(model_dir)


def safe_rmtree(path: Path) -> None:
    try:
        shutil.rmtree(path)
    except PermissionError:
        # Windows sandbox can keep test dirs locked briefly; they are ignored by git.
        pass


if __name__ == "__main__":
    unittest.main()
