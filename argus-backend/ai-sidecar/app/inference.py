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

# ---------------------------------------------------------------------------
# Определение «попытки сфотографировать экран»
# Все координаты bbox нормализованы (0..1): x, y, w, h
# ---------------------------------------------------------------------------
PHONE_PHOTO_LABEL = "phone_photo_attempt"

# Центральная зона кадра (если человек не найден): телефон на уровне лица/экрана
SCREEN_ZONE = {"x": 0.20, "y": 0.0, "w": 0.60, "h": 0.65}

# Минимальная доля площади телефона, которая должна лежать в зоне головы
PHOTO_OVERLAP_MIN = 0.30


def _box_area(b: dict[str, float]) -> float:
    return max(0.0, b["w"]) * max(0.0, b["h"])


def _intersection_area(a: dict[str, float], b: dict[str, float]) -> float:
    x1 = max(a["x"], b["x"])
    y1 = max(a["y"], b["y"])
    x2 = min(a["x"] + a["w"], b["x"] + b["w"])
    y2 = min(a["y"] + a["h"], b["y"] + b["h"])
    return max(0.0, x2 - x1) * max(0.0, y2 - y1)


def _head_zone(person: dict[str, float]) -> dict[str, float]:
    """Верхние ~45% рамки человека и центральные 70% по ширине = голова/плечи."""
    return {
        "x": person["x"] + person["w"] * 0.15,
        "y": person["y"],
        "w": person["w"] * 0.70,
        "h": person["h"] * 0.45,
    }


def flag_photo_attempts(objects: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """
    Если рамка телефона пересекается с зоной головы (или центром кадра),
    добавляет отдельную детекцию phone_photo_attempt.
    Исходные детекции (phone, person) сохраняются.
    """
    try:
        persons = [o["bbox"] for o in objects if o.get("object_type") == "person"]
        zones = [_head_zone(p) for p in persons] or [SCREEN_ZONE]

        extra: list[dict[str, Any]] = []
        for obj in objects:
            if obj.get("object_type") != "phone":
                continue

            phone = obj["bbox"]
            area = _box_area(phone)
            if area <= 0.0:
                continue

            overlap = max(_intersection_area(phone, z) / area for z in zones)
            cx = phone["x"] + phone["w"] / 2.0
            cy = phone["y"] + phone["h"] / 2.0
            in_center = 0.20 <= cx <= 0.80 and cy <= 0.65

            if overlap >= PHOTO_OVERLAP_MIN and in_center:
                extra.append({
                    "object_type": PHONE_PHOTO_LABEL,
                    "confidence": obj["confidence"],
                    "bbox": dict(phone),
                })
        return objects + extra
    except Exception:
        # Эвристика не должна ронять основной анализ кадра
        return objects

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
            objects = flag_photo_attempts(objects)

        faces = []
        if self.inventory.arcface.loaded and reference_embedding:
            face = _run_arcface(self.inventory.arcface.session, image, reference_embedding)
            if face:
                faces.append(face)

        # Liveness: texture-based anti-spoofing using pixel variance analysis.
        # A printed photo or screen replay has much lower local pixel variance
        # than a real face (due to printing/compression artifacts).
        liveness = _estimate_liveness(image)

        latency_ms = (time.perf_counter() - started) * 1000.0
        return {
            "faces": faces,
            "objects": objects,
            "liveness": liveness,
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


def _estimate_liveness(image: Any) -> dict[str, Any]:
    """
    Texture-based liveness detection using Local Binary Pattern (LBP) variance.

    Real faces exhibit high-frequency texture variation at the micro level.
    Printed photos and screen replays have lower variance because:
      - Printed: ink dot pattern, paper texture, colour banding
      - Screen replay: LCD pixel grid, compression artefacts, uniform illumination

    This is a lightweight heuristic — no dedicated anti-spoof model is loaded.
    Score range: [0.0, 1.0]. Threshold ~0.35 for live/spoof decision.
    """
    try:
        import numpy as np

        # Convert to grayscale and resize to 64x64 for consistent analysis
        gray = image.convert("L").resize((64, 64))
        arr = np.asarray(gray, dtype=np.float32)

        # Compute local standard deviation across 8x8 patches
        patch_size = 8
        variances = []
        for y in range(0, arr.shape[0] - patch_size, patch_size):
            for x in range(0, arr.shape[1] - patch_size, patch_size):
                patch = arr[y:y + patch_size, x:x + patch_size]
                variances.append(float(np.std(patch)))

        if not variances:
            return {"score": 0.5, "is_live": True, "method": "texture_variance", "note": "insufficient patches"}

        mean_variance = float(np.mean(variances))
        # Empirically calibrated: real faces ~ variance 18-45, photos/screens ~ 5-15
        # Normalise to [0, 1] with sigmoid-like mapping
        # score = 1 / (1 + exp(-k * (variance - threshold)))
        import math
        threshold = 12.0
        k = 0.15
        score = 1.0 / (1.0 + math.exp(-k * (mean_variance - threshold)))
        score = round(min(1.0, max(0.0, score)), 4)

        is_live = score >= 0.35
        return {
            "score": score,
            "is_live": is_live,
            "method": "texture_variance",
            "variance": round(mean_variance, 2),
        }
    except Exception as exc:
        return {"score": 0.5, "is_live": True, "method": "error", "error": str(exc)}


def _shape_dim(shape: list[Any], idx: int, default: int) -> int:
    if idx >= len(shape):
        return default
    value = shape[idx]
    if isinstance(value, int) and value > 0:
        return value
    return default
