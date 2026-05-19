from __future__ import annotations

import base64
import io
import time
from dataclasses import dataclass
from typing import Any

from .config import SidecarSettings
from .models import ModelInventory


COCO_CLASS_MAP = {
    0: "person",
    67: "phone",
    73: "book",
}


@dataclass
class FrameAnalyzer:
    inventory: ModelInventory
    settings: SidecarSettings

    def analyze_frame(
        self,
        frame_base64: str,
        content_type: str,
        reference_embedding: list[float] | None = None,
    ) -> dict[str, Any]:
        started = time.perf_counter()
        raw = base64.b64decode(frame_base64)
        image = _decode_image(raw, content_type)

        objects = []
        if self.inventory.yolo.loaded:
            objects = _run_yolo(self.inventory.yolo.session, image, self.settings.yolo_confidence_threshold)

        faces = []
        if self.inventory.arcface.loaded and reference_embedding:
            face = _run_arcface(self.inventory.arcface.session, image, reference_embedding)
            if face:
                faces.append(face)

        latency_ms = (time.perf_counter() - started) * 1000.0
        return {
            "faces": faces,
            "objects": objects,
            "liveness": {
                "score": 0.0,
                "is_live": False,
                "method": "not_configured",
            },
            "latency_ms": latency_ms,
        }


def _decode_image(raw: bytes, content_type: str):
    media_type = content_type.split(";", 1)[0].strip().lower()
    if media_type not in {"image/jpeg", "image/jpg", "image/png", "image/webp", ""}:
        raise ValueError(f"unsupported content_type: {content_type}")

    from PIL import Image

    return Image.open(io.BytesIO(raw)).convert("RGB")


def _run_yolo(session: Any, image: Any, confidence_threshold: float) -> list[dict[str, Any]]:
    import numpy as np

    input_meta = session.get_inputs()[0]
    input_name = input_meta.name
    input_shape = list(input_meta.shape)
    height = _shape_dim(input_shape, 2, 640)
    width = _shape_dim(input_shape, 3, 640)

    resized = image.resize((width, height))
    tensor = np.asarray(resized, dtype=np.float32) / 255.0
    tensor = np.transpose(tensor, (2, 0, 1))[None, ...]

    outputs = session.run(None, {input_name: tensor})
    if not outputs:
        return []

    raw = np.asarray(outputs[0])
    if raw.ndim == 3:
        raw = raw[0]
    if raw.ndim != 2:
        return []
    if raw.shape[0] < raw.shape[1] and raw.shape[0] <= 128:
        raw = raw.T

    detections: list[dict[str, Any]] = []
    for row in raw:
        if row.shape[0] < 6:
            continue

        if row.shape[0] >= 85:
            objectness = float(row[4])
            scores = row[5:]
            class_id = int(np.argmax(scores))
            confidence = objectness * float(scores[class_id])
        else:
            scores = row[4:]
            class_id = int(np.argmax(scores))
            confidence = float(scores[class_id])

        mapped = COCO_CLASS_MAP.get(class_id)
        if mapped is None or confidence < confidence_threshold:
            continue

        cx, cy, w, h = [float(v) for v in row[:4]]
        detections.append({
            "object_type": mapped,
            "confidence": confidence,
            "bbox": {
                "x": max(0.0, (cx - w / 2.0) / width),
                "y": max(0.0, (cy - h / 2.0) / height),
                "w": min(1.0, w / width),
                "h": min(1.0, h / height),
            },
        })

    return detections[:20]


def _run_arcface(session: Any, image: Any, reference_embedding: list[float]) -> dict[str, Any] | None:
    import numpy as np

    input_meta = session.get_inputs()[0]
    input_name = input_meta.name
    input_shape = list(input_meta.shape)
    height = _shape_dim(input_shape, 2, 112)
    width = _shape_dim(input_shape, 3, 112)

    resized = image.resize((width, height))
    tensor = np.asarray(resized, dtype=np.float32)
    tensor = (tensor - 127.5) / 128.0
    tensor = np.transpose(tensor, (2, 0, 1))[None, ...]

    outputs = session.run(None, {input_name: tensor})
    if not outputs:
        return None

    embedding = np.asarray(outputs[0]).reshape(-1).astype(np.float32)
    reference = np.asarray(reference_embedding, dtype=np.float32)
    if embedding.size != reference.size:
        raise ValueError(f"reference embedding size {reference.size} does not match model output {embedding.size}")

    similarity = float(np.dot(embedding, reference) / ((np.linalg.norm(embedding) * np.linalg.norm(reference)) + 1e-8))
    return {
        "confidence": 1.0,
        "similarity": similarity,
        "embedding": embedding.tolist(),
        "is_spoof": False,
        "spoof_type": "",
        "bbox": {"x": 0.0, "y": 0.0, "w": 1.0, "h": 1.0},
        "head_yaw": 0.0,
        "head_pitch": 0.0,
        "head_roll": 0.0,
    }


def _shape_dim(shape: list[Any], idx: int, default: int) -> int:
    if idx >= len(shape):
        return default
    value = shape[idx]
    if isinstance(value, int) and value > 0:
        return value
    return default
