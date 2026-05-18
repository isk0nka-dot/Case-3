# Developer Guide

## Backend AI Step 2

The production AI path is intentionally split:

1. Nuxt runs lightweight MediaPipe and Web Audio checks in the browser.
2. The Go backend queues heavier frame analysis asynchronously.
3. The Go inference gateway calls the Python ONNX sidecar.
4. The backend audio bridge derives low-frequency ClickHouse-only audio anomalies from sustained telemetry.
5. ClickHouse stores anomaly/telemetry events for analytics.

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
    test_ci_contract.py
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
The local `docker-compose.yml` mounts repository-root `./models` to `/models`, matching the downloader examples below.

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
go test ./internal/application/usecase ./internal/infrastructure/config
go test ./cmd/inference ./internal/infrastructure/inference ./internal/infrastructure/worker ./internal/transport/grpc
go test ./...
go vet ./...
```

GitLab CI also runs `ai-sidecar-test` in `python:3.12-slim` before the manual deploy job becomes available. That job installs `ai-sidecar/requirements.txt` plus `ai-sidecar/requirements-dev.txt`, so the synthetic ONNX Runtime test runs in CI even when a local Windows machine only has Python 3.13/3.14.

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
docker build -f Dockerfile.inference -t argus/inference:latest .
docker build -f Dockerfile.worker -t argus/worker:latest .
```

The backend config must point to the sidecar:

```yaml
inference:
  engine_type: "python_bridge"
  python_bridge_url: "http://ai-sidecar:8091"
  allow_stub: false
```

Audio telemetry aggregation is controlled separately:

```yaml
audio_bridge:
  enabled: true
  noise_threshold_db: -35
  vad_confidence_threshold: 0.70
  consecutive_events: 3
  cooldown_events: 12
  derived_event_confidence: 0.85
```

Environment overrides:

| Variable | Description |
| --- | --- |
| `EVENT_COLLECTOR_AUDIO_BRIDGE_ENABLED` | Enables/disables backend audio telemetry aggregation. |
| `EVENT_COLLECTOR_AUDIO_NOISE_THRESHOLD_DB` | RMS dB threshold for sustained noise. |
| `EVENT_COLLECTOR_AUDIO_VAD_CONFIDENCE_THRESHOLD` | Minimum VAD confidence for sustained voice. |
| `EVENT_COLLECTOR_AUDIO_CONSECUTIVE_EVENTS` | Consecutive telemetry frames before deriving `audio_anomaly`. |
| `EVENT_COLLECTOR_AUDIO_COOLDOWN_EVENTS` | Duplicate suppression window after a derived anomaly fires. |
| `EVENT_COLLECTOR_AUDIO_DERIVED_EVENT_CONFIDENCE` | Fallback confidence for derived backend audio anomalies. |

`Dockerfile.inference` runs `cmd/inference`, which exposes the Go gRPC inference gateway and calls the Python sidecar over HTTP. `Dockerfile.worker` runs `cmd/worker`, installs `ffmpeg`, consumes Asynq jobs, extracts bounded video frames, and calls the inference gateway. The local `docker-compose.yml` declares `ai-sidecar`, `inference`, and `worker` as separate services so model execution stays isolated from the API server.

The root `.dockerignore` intentionally excludes `models/`, `ai-sidecar/`, `.git/`, Go cache files, and large media/model artifacts from Go image builds. The sidecar image is built from `ai-sidecar/` as a separate context, and runtime models are mounted from `./models:/models:ro`.

## Inference Gateway Contract

The `AnalyzeFrame` RPC accepts a single image frame and is the preferred backend inference entry point.

The legacy `AnalyzeVideo` streaming RPC is intentionally narrowed for Step 2: it accepts only pre-extracted image frames streamed in chunks with `Content-Type` `image/jpeg`, `image/png`, `image/jpg`, or `image/webp`. Raw `video/mp4` or `video/webm` fragments return `Unimplemented` at the gateway. This prevents the production sidecar from receiving arbitrary video bytes as fake JPEG frames.

`AIAnalysisHandler` handles raw `video/mp4` and `video/webm` evidence through a bounded FFmpeg extractor before calling inference. The extractor samples at `inference.frame_sample_interval_sec`, caps output with `inference.max_video_dur_sec / inference.frame_sample_interval_sec`, and sends each JPEG to `AnalyzeFrame` with its sampled timestamp. Unsupported content types are skipped before MinIO download.

Worker hosts that run backend video deep scans must have `ffmpeg` on `PATH`. If the worker runs in Docker, install `ffmpeg` in that worker image before enabling video evidence analysis.

## Audio Bridge Contract

Frontend `useAudioEngine` emits structured audio features through the existing `sendEvent` pipeline. The backend bridge observes only `AUDIO_LEVEL_TELEMETRY`; it does not ingest raw microphone samples.

When sustained RMS/VAD thresholds are exceeded, the bridge writes one derived `audio_anomaly` event to ClickHouse with `source=backend_ai`. It does not write this derived event to Kafka, does not modify PostgreSQL session transactions, and does not reject the original event if ClickHouse is unavailable.

## Safety Rules

- Never enable `allow_stub=true` in production.
- Omitted inference config defaults to `python_bridge`; use `engine_type: "stub"` only in explicit local development configs with `allow_stub: true`.
- Keep audio and vision event payloads structured; do not transmit raw microphone audio.
- Keep backend audio anomalies ClickHouse-only unless a separate realtime contract is explicitly introduced.
- Keep frame extraction interval-based or event-triggered. Do not continuously decode full video streams.
- Do not reintroduce pseudo-frame splitting of raw video bytes in `AnalyzeVideo`; add a bounded extractor worker instead.
- If inference capacity is saturated, drop frames and keep the student session alive.
- Tune thresholds through config/env first; do not hardcode production sensitivity in handlers.
