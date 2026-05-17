from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .config import SidecarSettings


@dataclass
class ModelHandle:
    name: str
    path: Path
    loaded: bool
    session: Any | None = None
    error: str | None = None
    input_shape: list[Any] | None = None
    output_shape: list[Any] | None = None

    def health_payload(self) -> dict[str, Any]:
        return {
            "loaded": self.loaded,
            "path": str(self.path),
            "error": self.error,
            "input_shape": self.input_shape,
            "output_shape": self.output_shape,
        }


@dataclass
class ModelInventory:
    yolo: ModelHandle
    arcface: ModelHandle

    @classmethod
    def load(cls, settings: SidecarSettings) -> "ModelInventory":
        missing = []
        yolo_path = settings.model_path(settings.yolo_model_file)
        arcface_path = settings.model_path(settings.arcface_model_file)

        if not yolo_path.is_file():
            missing.append(f"yolo={yolo_path}")
        if not arcface_path.is_file():
            missing.append(f"arcface={arcface_path}")

        if missing and settings.model_fail_fast:
            raise FileNotFoundError("missing required ONNX models: " + ", ".join(missing))

        return cls(
            yolo=_load_model("yolo", yolo_path, settings.model_fail_fast),
            arcface=_load_model("arcface", arcface_path, settings.model_fail_fast),
        )

    def status(self) -> str:
        if self.yolo.loaded and self.arcface.loaded:
            return "ok"
        return "degraded"

    def health_payload(self) -> dict[str, Any]:
        return {
            "status": self.status(),
            "models": {
                "yolo": self.yolo.health_payload(),
                "arcface": self.arcface.health_payload(),
            },
        }


def _load_model(name: str, path: Path, fail_fast: bool) -> ModelHandle:
    if not path.is_file():
        return ModelHandle(name=name, path=path, loaded=False, error="file not found")

    try:
        import onnxruntime as ort

        session = ort.InferenceSession(str(path), providers=["CPUExecutionProvider"])
        input_shape = list(session.get_inputs()[0].shape) if session.get_inputs() else None
        output_shape = list(session.get_outputs()[0].shape) if session.get_outputs() else None
        return ModelHandle(
            name=name,
            path=path,
            loaded=True,
            session=session,
            input_shape=input_shape,
            output_shape=output_shape,
        )
    except Exception as exc:
        if fail_fast:
            raise RuntimeError(f"failed to load {name} ONNX model at {path}: {exc}") from exc
        return ModelHandle(name=name, path=path, loaded=False, error=str(exc))
