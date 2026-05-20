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

The YOLO parser supports Ultralytics-style `[1,84,N]` outputs and objectness-column `[1,85,N]` outputs. In the objectness format, final confidence is `objectness * class_score`, which keeps YOLOv5/custom exports compatible without changing the Go inference contract.

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

GitLab CI verifies the backend automatically and exposes production deployment as a manual job only. The server may host multiple projects, so the backend deploy is scoped to the Argus project root and never deletes broad server paths.

Required GitLab CI variables:

| Variable | Type | Description |
| --- | --- | --- |
| `ARGUS_DEPLOY_HOST` | Variable | Production server IP or DNS name. |
| `ARGUS_DEPLOY_USER` | Variable | SSH user, usually `deploy`. |
| `ARGUS_DEPLOY_SSH_KEY` | File | Private deploy key. Use GitLab variable type `File`; do not use hidden masking if GitLab rejects multiline OpenSSH keys. |

Optional variables:

| Variable | Default | Description |
| --- | --- | --- |
| `ARGUS_PROJECT_ROOT` | `/opt/argus-ai` | Root directory reserved for this Argus installation. |
| `ARGUS_BACKEND_DEPLOY_PATH` | `$ARGUS_PROJECT_ROOT/argus-backend` | Backend checkout/build directory on the server. |

Backward-compatible fallbacks are still accepted for older GitLab settings: `DEPLOY_HOST`, `DEPLOY_USER`, and `DEPLOY_SSH_KEY`.

The manual `deploy-production` job runs:

```bash
sh ci/scripts/deploy_backend.sh
```

It performs SSH key preflight, syncs the repository to `$ARGUS_PROJECT_ROOT/argus-backend`, builds `app`, `worker`, `inference`, and `ai-sidecar`, then waits for `argus-backend-app` health. Runtime DLQ data is mounted under `${ARGUS_PROJECT_ROOT:-/opt/argus-ai}/dlq-data`, not a broad server path.

### Single-server production compose

For a small production server, use the repository `docker-compose.yml` instead of the multi-node infra stack. The infra stack is intended for larger hosts; this backend compose keeps Kafka and ClickHouse single-node so an 8 GB server is not overloaded by three Kafka brokers and three ClickHouse replicas.

Before starting it, create `argus-backend/.env` from `.env.example` and set real secrets:

| Variable | Required | Description |
| --- | --- | --- |
| `ARGUS_PROJECT_ROOT` | yes | Persistent host root, normally `/opt/argus-ai`. |
| `ARGUS_POSTGRES_DB` | yes | PostgreSQL database, normally `argus_db`. |
| `ARGUS_POSTGRES_USER` | yes | PostgreSQL application user. |
| `ARGUS_POSTGRES_PASSWORD` | yes | PostgreSQL password. The compose file refuses to render without it. |
| `ARGUS_CLICKHOUSE_DATABASE` | yes | ClickHouse analytics database, normally `argus_analytics`. Must match `EVENT_COLLECTOR_CH_DATABASE`. |
| `ARGUS_CLICKHOUSE_USER` | yes | ClickHouse user. |
| `ARGUS_CLICKHOUSE_PASSWORD` | optional | ClickHouse password. Empty is allowed only when the server is not exposed publicly. |
| `ARGUS_MINIO_ROOT_USER` | yes | MinIO root user used by backend evidence storage. |
| `ARGUS_MINIO_ROOT_PASSWORD` | yes | MinIO root password. The compose file refuses to render without it. |
| `ARGUS_MINIO_BUCKET` | yes | Evidence bucket, normally `argus-evidence`. |
| `ARGUS_MINIO_EXPORT_BUCKET` | yes | Export archive bucket, normally `argus-exports`. |
| `EVENT_COLLECTOR_JWT_SIGNING_KEY` | yes | HS256 signing key, minimum 32 characters. The compose file refuses to render without it. |
| `EVENT_COLLECTOR_CORS_ORIGINS` | yes | Comma-separated frontend origins, normally `https://argusai.kz,https://www.argusai.kz`. |
| `DOCKER_GROUP_ID` | yes | Numeric group id owning `/var/run/docker.sock` on the server. Get it with `stat -c %g /var/run/docker.sock`. |
| `MODEL_FAIL_FAST` | yes | Keep `true` in production so missing/corrupt ONNX models stop the sidecar at startup. |

The compose file persists PostgreSQL, ClickHouse, Kafka, ZooKeeper, MinIO, and Badger DLQ data with named Docker volumes. Do not remove these volumes during normal redeploys. For a deliberate wipe/reinstall, capture an inventory first and delete only known Argus containers, networks, and volumes.

DLQ storage intentionally uses the `argus_dlq` named volume instead of a host bind mount. The backend containers run as non-root `appuser`, and host bind mounts commonly fail with `permission denied` on `/data/argus-dlq/LOCK`. A named volume preserves the image-owned directory permissions and keeps retry data persistent.

`minio-init` creates the evidence and export buckets before `app` and `worker` start. This is required because the MinIO adapter intentionally fails startup when the evidence bucket is missing.

Required model files must exist before `MODEL_FAIL_FAST=true` can start the sidecar:

```text
argus-backend/models/yolov8n.onnx
argus-backend/models/arcface.onnx
```

The model directory is mounted read-only into `ai-sidecar` as `/models`.

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

For ArcFace identity verification, callers may include `reference_embedding` on `AnalyzeFrameRequest`. This field is optional for backward compatibility. When present, the Go inference gateway copies it into `FrameAnalysisOptions.ReferenceEmbedding`; the Python bridge forwards it to `/v1/analyze-frame` as JSON `reference_embedding`. When absent, the sidecar can still return a detected face embedding, but identity similarity is not computed.

The Python sidecar returns normalized bounding boxes as JSON objects (`{"x":0.1,"y":0.2,"w":0.3,"h":0.4}`). The Go bridge also accepts the legacy array shape (`[x,y,w,h]`) so older test fixtures and external sidecars do not break.

Image content types are normalized by media type before sidecar decoding, so `image/jpeg; charset=binary` is handled the same as `image/jpeg`.

The legacy `AnalyzeVideo` streaming RPC is intentionally narrowed for Step 2: it accepts only pre-extracted image frames streamed in chunks with `Content-Type` `image/jpeg`, `image/png`, `image/jpg`, or `image/webp`. Raw `video/mp4` or `video/webm` fragments return `Unimplemented` at the gateway. This prevents the production sidecar from receiving arbitrary video bytes as fake JPEG frames. Worker-owned evidence analysis should prefer `AnalyzeFrame` for both direct image fragments and frames extracted from video.

`AIAnalysisHandler` handles raw `video/mp4` and `video/webm` evidence through a bounded FFmpeg extractor before calling inference. The extractor samples at `inference.frame_sample_interval_sec`, caps output with `inference.max_video_dur_sec / inference.frame_sample_interval_sec`, and sends each JPEG to `AnalyzeFrame` with its sampled timestamp. Direct image evidence is sent to `AnalyzeFrame` as a single timestamp-0 frame, but it is rejected before inference if it exceeds `inference.max_frame_bytes` (default 10 MiB). If the queued `AIAnalysisPayload` includes `reference_embedding`, the same vector is copied onto every frame request. Unsupported content types are skipped before MinIO download.

Worker hosts that run backend video deep scans must have `ffmpeg` on `PATH`. If the worker runs in Docker, install `ffmpeg` in that worker image before enabling video evidence analysis.

## Backend AI Event Mapping

`AIAnalysisHandler` writes model findings back to ClickHouse as `source=backend_ai` events. These events are not sent through Kafka and do not change the student-facing session API.

Object detections use `payload_type=object_detection` with this JSON shape:

```json
{
  "object_type": "phone",
  "bbox_x": 0.11,
  "bbox_y": 0.22,
  "bbox_w": 0.33,
  "bbox_h": 0.44,
  "detection_confidence": 0.91
}
```

Face mismatch/spoof detections use `payload_type=face_detection` and also populate denormalized ClickHouse fields where available:

```json
{
  "match": false,
  "similarity": 0.42,
  "face_count": 1,
  "is_spoof": false
}
```

The worker sets `face_similarity`, `face_bbox`, and optional `face_embedding` for ArcFace results. Liveness failures use `payload_type=liveness` and populate `liveness_score`.

ArcFace cosine similarity can be negative. Treat any computed similarity below `inference.face_mismatch_threshold` as a mismatch; do not require it to be greater than zero. Missing reference data is still not fraud because no computed embedding/similarity is available.

Liveness anomalies are emitted only for configured liveness methods. Placeholder methods such as `not_configured`, `unavailable`, `unknown`, or an empty method are ignored so an unavailable liveness model does not create false-positive fraud events.

Do not treat a standalone YOLO `person` object as fraud. The student's own webcam frame may contain a person detection; extra-person fraud must come from face count or identity-specific evidence.

Do not emit `BACKEND_AI_FACE_MISMATCH` solely because a reference embedding is missing. Missing enrollment data means identity similarity was not computed; it is an operational/enrollment gap, not proof of fraud.

Unused denormalized AI columns are deliberately initialized with sentinel values before insert: `face_similarity=-1`, `liveness_score=-1`, and `audio_rms_db=-100`. Do not remove these sentinels; otherwise ClickHouse analytics may interpret empty backend vision events as real zero-valued face or audio readings.

## Audio Bridge Contract

Frontend `useAudioEngine` emits structured audio features through the existing `sendEvent` pipeline. The backend bridge observes only `AUDIO_LEVEL_TELEMETRY`; it does not ingest raw microphone samples.

When sustained RMS/VAD thresholds are exceeded, the bridge writes one derived `audio_anomaly` event to ClickHouse with `source=backend_ai`. It does not write this derived event to Kafka, does not modify PostgreSQL session transactions, and does not reject the original event if ClickHouse is unavailable.

The bridge parser accepts both JSON shapes that can appear at the boundary:

- Frontend-facing camelCase: `vadConfidence`, `classificationConfidence`
- Protobuf/Go mapper snake_case: `vad_confidence`, `classification_confidence`

Keep this compatibility when changing mapper or payload code; otherwise derived anomaly confidence can silently fall back to the configured default.

## Safety Rules

- Never enable `allow_stub=true` in production.
- Omitted inference config defaults to `python_bridge`; use `engine_type: "stub"` only in explicit local development configs with `allow_stub: true`.
- Keep audio and vision event payloads structured; do not transmit raw microphone audio.
- Keep backend audio anomalies ClickHouse-only unless a separate realtime contract is explicitly introduced.
- Keep frame extraction interval-based or event-triggered. Do not continuously decode full video streams.
- Do not reintroduce pseudo-frame splitting of raw video bytes in `AnalyzeVideo`; add a bounded extractor worker instead.
- If inference capacity is saturated, drop frames and keep the student session alive.
- Tune thresholds through config/env first; do not hardcode production sensitivity in handlers.
