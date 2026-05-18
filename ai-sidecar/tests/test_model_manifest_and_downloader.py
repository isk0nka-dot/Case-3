import hashlib
import json
import shutil
import sys
import unittest
import uuid
import zipfile
from pathlib import Path

SIDECAR_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SIDECAR_ROOT))
TEST_TMP = SIDECAR_ROOT / ".test-tmp"


def scratch_dir(name: str) -> Path:
    path = TEST_TMP / f"{name}-{uuid.uuid4().hex}"
    path.mkdir(parents=True)
    return path


def safe_rmtree(path: Path) -> None:
    try:
        shutil.rmtree(path)
    except PermissionError:
        pass


class ModelManifestTests(unittest.TestCase):
    def test_manifest_declares_runtime_model_files(self) -> None:
        manifest_path = SIDECAR_ROOT / "model_manifest.json"

        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        by_name = {entry["name"]: entry for entry in manifest["models"]}

        self.assertEqual(by_name["yolo"]["filename"], "yolov8n.onnx")
        self.assertEqual(by_name["arcface"]["filename"], "arcface.onnx")
        self.assertIn("source_url", by_name["yolo"])
        self.assertIn("source_url", by_name["arcface"])


class ModelDownloaderTests(unittest.TestCase):
    def test_downloads_file_url_and_validates_checksum(self) -> None:
        from scripts.download_models import download_from_manifest

        tmp = scratch_dir("download-file")
        try:
            payload = b"fake-onnx-binary-for-downloader-test"
            source = tmp / "source.onnx"
            source.write_bytes(payload)
            manifest = {
                "models": [
                    {
                        "name": "yolo",
                        "filename": "yolov8n.onnx",
                        "download_url": source.as_uri(),
                        "sha256": hashlib.sha256(payload).hexdigest(),
                        "min_bytes": len(payload),
                    }
                ]
            }
            manifest_path = tmp / "manifest.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            models_path = tmp / "models"

            written = download_from_manifest(manifest_path, models_path)

            self.assertEqual(written, [models_path / "yolov8n.onnx"])
            self.assertEqual((models_path / "yolov8n.onnx").read_bytes(), payload)
        finally:
            safe_rmtree(tmp)

    def test_extracts_zip_member_to_target_model_file(self) -> None:
        from scripts.download_models import download_from_manifest

        tmp = scratch_dir("download-zip")
        try:
            payload = b"fake-arcface-onnx-binary-for-downloader-test"
            archive = tmp / "arcface.zip"
            with zipfile.ZipFile(archive, "w") as zf:
                zf.writestr("buffalo_l/w600k_r50.onnx", payload)

            manifest = {
                "models": [
                    {
                        "name": "arcface",
                        "filename": "arcface.onnx",
                        "download_url": archive.as_uri(),
                        "archive_member": "buffalo_l/w600k_r50.onnx",
                        "sha256": hashlib.sha256(payload).hexdigest(),
                        "min_bytes": len(payload),
                    }
                ]
            }
            manifest_path = tmp / "manifest.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            models_path = tmp / "models"

            written = download_from_manifest(manifest_path, models_path)

            self.assertEqual(written, [models_path / "arcface.onnx"])
            self.assertEqual((models_path / "arcface.onnx").read_bytes(), payload)
        finally:
            safe_rmtree(tmp)

    def test_check_only_fails_when_model_file_is_missing(self) -> None:
        from scripts.download_models import ManifestError, download_from_manifest

        tmp = scratch_dir("check-only-missing")
        try:
            manifest = {
                "models": [
                    {
                        "name": "yolo",
                        "filename": "yolov8n.onnx",
                        "download_url": "",
                        "sha256": "0" * 64,
                        "min_bytes": 1,
                    }
                ]
            }
            manifest_path = tmp / "manifest.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")

            with self.assertRaises(ManifestError):
                download_from_manifest(manifest_path, tmp / "models", check_only=True)
        finally:
            safe_rmtree(tmp)


if __name__ == "__main__":
    unittest.main()
