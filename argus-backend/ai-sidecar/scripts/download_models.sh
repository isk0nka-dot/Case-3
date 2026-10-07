#!/usr/bin/env bash
# =============================================================================
#  Argus AI — ONNX Model Downloader
#
#  Downloads YOLOv8n (object detection) and ArcFace R50 (face identity)
#  ONNX models into ai-sidecar/models/.
#
#  Usage:
#    cd argus-backend/ai-sidecar
#    bash scripts/download_models.sh
#
#  Override URLs via env vars (e.g. internal artifact server):
#    ARGUS_YOLO_ONNX_URL=https://... bash scripts/download_models.sh
#    ARGUS_ARCFACE_ONNX_URL=https://... bash scripts/download_models.sh
#
#  After download, restart the ai-sidecar container:
#    docker compose -f argus-infra/docker/docker-compose.dev.yml restart ai-sidecar
# =============================================================================

set -euo pipefail

MODELS_DIR="$(cd "$(dirname "$0")/.." && pwd)/models"
mkdir -p "$MODELS_DIR"

# ── Helper ────────────────────────────────────────────────────────────────────
download() {
  local url="$1"
  local dest="$2"
  local description="$3"

  echo ""
  echo "⬇  Downloading $description..."
  echo "   URL: $url"
  echo "   → $dest"

  if command -v curl &>/dev/null; then
    curl -fL --progress-bar -o "$dest" "$url"
  elif command -v wget &>/dev/null; then
    wget -q --show-progress -O "$dest" "$url"
  else
    echo "ERROR: curl or wget is required" >&2
    exit 1
  fi

  local size
  size=$(du -sh "$dest" | cut -f1)
  echo "   ✓ $description saved ($size)"
}

verify_min_size() {
  local file="$1"
  local min_bytes="$2"
  local name="$3"
  local actual
  actual=$(wc -c < "$file")
  if [ "$actual" -lt "$min_bytes" ]; then
    echo "ERROR: $name is too small ($actual bytes < $min_bytes minimum)" >&2
    echo "       The downloaded file may be corrupt or the URL is wrong." >&2
    rm -f "$file"
    exit 1
  fi
}

# ── YOLOv8n ──────────────────────────────────────────────────────────────────
YOLO_FILE="$MODELS_DIR/yolov8n.onnx"
YOLO_DEFAULT_URL="https://github.com/ultralytics/assets/releases/download/v8.2.0/yolov8n.pt"
# Note: we download the .pt and convert, OR use a pre-exported ONNX
# Pre-exported YOLOv8n ONNX (opset 17, dynamic batch):
YOLO_ONNX_DEFAULT="https://github.com/ultralytics/assets/releases/download/v8.2.0/yolov8n.onnx"

YOLO_URL="${ARGUS_YOLO_ONNX_URL:-$YOLO_ONNX_DEFAULT}"

if [ -f "$YOLO_FILE" ]; then
  echo "✓ YOLOv8n already present: $YOLO_FILE"
else
  download "$YOLO_URL" "$YOLO_FILE" "YOLOv8n ONNX"
  verify_min_size "$YOLO_FILE" 1000000 "yolov8n.onnx"
fi

# ── ArcFace R50 ───────────────────────────────────────────────────────────────
ARCFACE_FILE="$MODELS_DIR/arcface.onnx"
# InsightFace w600k_r50 — from the official model zoo
# Primary: direct ONNX from InsightFace releases
ARCFACE_DEFAULT_URL="https://huggingface.co/ashleykza/insightface-models/resolve/main/w600k_r50.onnx"

ARCFACE_URL="${ARGUS_ARCFACE_ONNX_URL:-$ARCFACE_DEFAULT_URL}"

if [ -f "$ARCFACE_FILE" ]; then
  echo "✓ ArcFace R50 already present: $ARCFACE_FILE"
else
  download "$ARCFACE_URL" "$ARCFACE_FILE" "ArcFace R50 ONNX"
  verify_min_size "$ARCFACE_FILE" 1000000 "arcface.onnx"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "========================================"
echo " Models directory: $MODELS_DIR"
ls -lh "$MODELS_DIR"
echo "========================================"
echo ""
echo "Next steps:"
echo "  1. Restart ai-sidecar:"
echo "     docker compose -f argus-infra/docker/docker-compose.dev.yml restart ai-sidecar"
echo ""
echo "  2. Verify health:"
echo "     curl http://localhost:8000/healthz"
echo '     # Expected: {"status":"ok","models":{"yolo":true,"arcface":true}}'
echo ""
