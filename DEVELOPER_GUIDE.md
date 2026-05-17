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
  requirements.txt
  app/
    config.py
    inference.py
    main.py
    models.py
  tests/
    test_model_inventory.py
```

The sidecar fails at startup when `MODEL_FAIL_FAST=true` and either required model is missing or cannot be loaded by ONNX Runtime.

## Required Model Files

Default local paths:

```text
models/yolov8n.onnx
models/arcface.onnx
```

Do not commit large model binaries directly unless the repository is configured for Git LFS. For production, mount `/models` read-only into the sidecar container.

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

Full ONNX runtime tests require real `.onnx` files in `models/`. The included sidecar test verifies fail-fast inventory behavior without downloading model weights.

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
