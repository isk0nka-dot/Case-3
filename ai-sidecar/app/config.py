from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


def _bool_env(name: str, default: bool) -> bool:
    raw = os.getenv(name)
    if raw is None:
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def _float_env(name: str, default: float) -> float:
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    return float(raw)


@dataclass(frozen=True)
class SidecarSettings:
    models_path: Path = Path("models")
    yolo_model_file: str = "yolov8n.onnx"
    arcface_model_file: str = "arcface.onnx"
    model_fail_fast: bool = True
    yolo_confidence_threshold: float = 0.35
    face_mismatch_threshold: float = 0.62
    audio_vad_threshold: float = 0.65
    frame_extraction_interval_sec: float = 4.0

    @classmethod
    def from_env(cls) -> "SidecarSettings":
        return cls(
            models_path=Path(os.getenv("ONNX_MODELS_PATH", "models")),
            yolo_model_file=os.getenv("YOLO_MODEL_FILE", "yolov8n.onnx"),
            arcface_model_file=os.getenv("ARCFACE_MODEL_FILE", "arcface.onnx"),
            model_fail_fast=_bool_env("MODEL_FAIL_FAST", True),
            yolo_confidence_threshold=_float_env("YOLO_CONFIDENCE_THRESHOLD", 0.35),
            face_mismatch_threshold=_float_env("FACE_MISMATCH_THRESHOLD", 0.62),
            audio_vad_threshold=_float_env("AUDIO_VAD_THRESHOLD", 0.65),
            frame_extraction_interval_sec=_float_env("FRAME_EXTRACTION_INTERVAL_SEC", 4.0),
        )

    def model_path(self, file_name: str) -> Path:
        return Path(os.path.abspath(os.path.join(os.fspath(self.models_path.expanduser()), file_name)))
