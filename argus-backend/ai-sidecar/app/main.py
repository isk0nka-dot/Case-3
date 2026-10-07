from __future__ import annotations

from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from .config import SidecarSettings
from .inference import FrameAnalyzer
from .models import ModelInventory


class AnalyzeFrameRequest(BaseModel):
    content_type: str = "image/jpeg"
    frame_base64: str = Field(min_length=1)
    reference_embedding: list[float] | None = None


def create_app() -> FastAPI:
    settings = SidecarSettings.from_env()
    inventory = ModelInventory.load(settings)
    analyzer = FrameAnalyzer(inventory=inventory, settings=settings)

    app = FastAPI(title="Argus AI Inference Sidecar", version="0.1.0")

    @app.get("/healthz")
    def healthz() -> dict[str, Any]:
        return inventory.health_payload()

    @app.post("/v1/analyze-frame")
    def analyze_frame(req: AnalyzeFrameRequest) -> dict[str, Any]:
        if inventory.status() != "ok":
            raise HTTPException(status_code=503, detail="required ONNX models are not loaded")
        try:
            return analyzer.analyze_frame(req.frame_base64, req.content_type, req.reference_embedding)
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    return app


app = create_app()
