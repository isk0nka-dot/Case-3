# Developer Guide

## Backend AI Step 2

The production AI path is intentionally split:

1. Nuxt runs lightweight MediaPipe and Web Audio checks in the browser.
2. The Go backend queues heavier frame analysis asynchronously.
3. The Go inference gateway calls the Python ONNX sidecar.
4. ClickHouse stores anomaly/telemetry events for analytics.

This avoids blocking exam sessions when ONNX inference is slow or unavailable.

## Python Sidecar Layout

```text
ai-sidecar/
  Dockerfile
  model_manifest.json
  requirements.txt
  requirements-dev.txt
  app/
    config.py
    inference.py
    main.py
    models.py
  scripts/
    download_models.py
  tests/
    test_model_inventory.py
    test_model_manifest_and_downloader.py
    test_synthetic_onnx_runtime.py
```

The sidecar fails at startup when `MODEL_FAIL_FAST=true` and either required model is missing or cannot be loaded by ONNX Runtime.

## Required Model Files

Default local paths:

```text
models/yolov8n.onnx
models/arcface.onnx
```

Do not commit large model binaries directly unless the repository is configured for Git LFS. For production, mount `/models` read-only into the sidecar container.

The repository intentionally tracks only `ai-sidecar/model_manifest.json`, not the model weights. The manifest records expected filenames, source references, minimum size checks, and environment-variable names for private artifact URLs and SHA256 values.

Download or validate models:

```bash
python ai-sidecar/scripts/download_models.py --models-path models
python ai-sidecar/scripts/download_models.py --models-path models --check-only --require-sha256
```

The downloader reads these optional variables:

| Variable | Description |
| --- | --- |
| `ARGUS_YOLO_ONNX_URL` | Private artifact URL or `file://` URL for `yolov8n.onnx`. |
| `ARGUS_YOLO_ONNX_SHA256` | Expected SHA256 for the YOLO ONNX file. |
| `ARGUS_ARCFACE_ONNX_URL` | Private artifact URL or archive URL for `arcface.onnx`. |
| `ARGUS_ARCFACE_ONNX_SHA256` | Expected SHA256 for the extracted ArcFace ONNX file. |
| `ARGUS_ARCFACE_ARCHIVE_MEMBER` | Exact member path inside an ArcFace zip archive, if the URL is an archive. |

Production should use checksummed internal artifacts. Public model sources move over time; do not hardcode an unverified third-party binary URL into deployment config.

## Running Tests

Backend:

```bash
go test ./cmd/inference ./internal/infrastructure/inference ./internal/infrastructure/worker ./internal/transport/grpc
go test ./...
go vet ./...
```

Frontend:

```bash
node tests/unit/realtime-ai-rules.test.mjs
npx vue-tsc --noEmit -p tsconfig.json
npm run lint
npm run build
```

Sidecar:

```bash
python -m unittest discover ai-sidecar/tests -v
```

Install sidecar dev dependencies for synthetic ONNX runtime loading tests:

```bash
python -m venv ai-sidecar/.venv
ai-sidecar/.venv/bin/pip install -r ai-sidecar/requirements.txt -r ai-sidecar/requirements-dev.txt
ai-sidecar/.venv/bin/python -m unittest discover ai-sidecar/tests -v
```

On Windows PowerShell, use `ai-sidecar\.venv\Scripts\python.exe` instead of `ai-sidecar/.venv/bin/python`. The synthetic ONNX test skips when `onnx` or `onnxruntime` is not installed; the downloader and fail-fast tests still run without external model weights.

## Ubuntu Dependencies

The current implementation uses a Python sidecar, so the Go backend does not require CGO or `libonnxruntime.so`.

If a future engineer adds Go-native ONNX Runtime bindings, install the ONNX Runtime shared library on Ubuntu and expose it to the linker:

```bash
sudo mkdir -p /opt/onnxruntime
sudo tar -xzf onnxruntime-linux-x64-*.tgz -C /opt/onnxruntime --strip-components=1
echo /opt/onnxruntime/lib | sudo tee /etc/ld.so.conf.d/onnxruntime.conf
sudo ldconfig
export CGO_ENABLED=1
export CGO_CFLAGS="-I/opt/onnxruntime/include"
export CGO_LDFLAGS="-L/opt/onnxruntime/lib -lonnxruntime"
```

Keep Python sidecar deployment as the default unless there is a measured reason to move model execution into Go.

## Deployment Notes

CI pushes code only. Server deployment should be done separately after the pipeline is green:

```bash
git pull
docker build -t argus/backend:latest .
docker build -t argus/ai-sidecar:latest ai-sidecar
```

The backend config must point to the sidecar:

```yaml
inference:
  engine_type: "python_bridge"
  python_bridge_url: "http://ai-sidecar:8091"
  allow_stub: false
```

## Safety Rules

- Never enable `allow_stub=true` in production.
- Keep audio and vision event payloads structured; do not transmit raw microphone audio.
- Keep frame extraction interval-based or event-triggered. Do not continuously decode full video streams.
- If inference capacity is saturated, drop frames and keep the student session alive.
- Tune thresholds through config/env first; do not hardcode production sensitivity in handlers.
