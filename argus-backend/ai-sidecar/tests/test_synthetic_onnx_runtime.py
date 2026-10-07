import importlib.util
import shutil
import sys
import unittest
import uuid
from pathlib import Path

SIDECAR_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SIDECAR_ROOT))
TEST_TMP = SIDECAR_ROOT / ".test-tmp"

from app.config import SidecarSettings
from app.models import ModelInventory


def scratch_dir(name: str) -> Path:
    path = TEST_TMP / f"{name}-{uuid.uuid4().hex}"
    path.mkdir(parents=True)
    return path


def safe_rmtree(path: Path) -> None:
    try:
        shutil.rmtree(path)
    except PermissionError:
        pass


@unittest.skipIf(
    importlib.util.find_spec("onnx") is None or importlib.util.find_spec("onnxruntime") is None,
    "onnx and onnxruntime are required for synthetic runtime loading",
)
class SyntheticOnnxRuntimeTests(unittest.TestCase):
    def test_inventory_loads_valid_synthetic_onnx_models(self) -> None:
        model_dir = scratch_dir("synthetic-onnx")
        try:
            _write_identity_model(model_dir / "yolov8n.onnx")
            _write_identity_model(model_dir / "arcface.onnx")
            settings = SidecarSettings(models_path=model_dir, model_fail_fast=True)

            inventory = ModelInventory.load(settings)

            self.assertEqual(inventory.status(), "ok")
            self.assertTrue(inventory.yolo.loaded)
            self.assertTrue(inventory.arcface.loaded)
            self.assertEqual(inventory.yolo.input_shape, [1, 3, 8, 8])
        finally:
            safe_rmtree(model_dir)


def _write_identity_model(path: Path) -> None:
    import onnx
    from onnx import TensorProto, helper

    input_tensor = helper.make_tensor_value_info("input", TensorProto.FLOAT, [1, 3, 8, 8])
    output_tensor = helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, 3, 8, 8])
    graph = helper.make_graph(
        [helper.make_node("Identity", ["input"], ["output"])],
        "identity-model",
        [input_tensor],
        [output_tensor],
    )
    model = helper.make_model(graph, opset_imports=[helper.make_opsetid("", 13)])
    model.ir_version = 10
    onnx.checker.check_model(model)
    onnx.save(model, path)


if __name__ == "__main__":
    unittest.main()
