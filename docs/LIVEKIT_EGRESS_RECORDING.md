# LiveKit Egress Recording

This document is the operational contract for Argus AI session recording via
LiveKit Track Composite Egress. It covers the backend API, PostgreSQL metadata,
LiveKit webhook handling, MinIO output, and infra dependencies.

## Current Scope

The implementation records selected LiveKit tracks, not full browser-composited
layouts. This is intentional: `TrackCompositeEgress` avoids UI rendering and
keeps CPU usage lower than room/browser composite egress.

Implemented components:

| Layer | Location | Responsibility |
|-------|----------|----------------|
| Backend egress adapter | `internal/infrastructure/livekit/egress.go` | Calls LiveKit SDK `StartTrackCompositeEgress` and `StopEgress` |
| Backend HTTP API | `internal/transport/http/recording_handler.go` | Starts/stops recordings through authenticated admin media endpoints |
| Backend webhook | `internal/infrastructure/livekit/webhook.go` | Validates LiveKit webhook signatures and handles `egress_ended` |
| PostgreSQL metadata | `internal/infrastructure/postgres/repository.go` | Persists `egress_id` to session/user mapping and final file status |
| Domain entity | `internal/domain/entity/recording.go` | In-memory representation of a recording row |
| Infra compose | `argus-infra/docker/docker-compose.yaml` | Runs LiveKit server, LiveKit Egress, Redis, MinIO, and backend wiring |
| Infra migration | `argus-infra/migrations/postgres/000007_livekit_recordings.up.sql` | Creates `livekit_recordings` |

## Data Flow

1. The frontend obtains a LiveKit token from `POST /api/v1/media/token`.
2. The student publishes camera/microphone tracks into room
   `argus-session-{sessionId}`.
3. A proctor/admin calls `POST /api/v1/media/recordings/start` with the
   `sessionId`, `studentId`, and LiveKit track IDs.
4. Backend calls LiveKit `StartTrackCompositeEgress` with:
   - video encoding: `1280x720`, `15 FPS`, H.264 main profile
   - output type: MP4
   - S3-compatible output: MinIO bucket from backend config
5. LiveKit returns an `egress_id`.
6. Backend stores the mapping in PostgreSQL table `livekit_recordings`.
7. When recording ends, LiveKit sends a signed webhook to:
   `POST /api/v1/livekit/webhook`.
8. Backend validates the webhook signature with LiveKit API key/secret.
9. On `egress_ended`, backend updates the matching `livekit_recordings` row by
   `egress_id`.

The webhook update never parses `session_id` or `student_id` from the object
storage path. The durable join key is always `egress_id`.

## API

All admin media recording endpoints require the same Bearer JWT auth pattern as
the existing admin/media API.

### Start Recording

`POST /api/v1/media/recordings/start`

Request:

```json
{
  "sessionId": "session-1",
  "studentId": "student-1",
  "roomName": "argus-session-session-1",
  "videoTrackId": "TR_VC...",
  "audioTrackId": "TR_AM..."
}
```

Fields:

| Field | Required | Notes |
|-------|:--------:|-------|
| `sessionId` | yes | Argus proctoring session ID |
| `studentId` | yes | Student/user ID associated with the recording |
| `roomName` | no | Defaults to `argus-session-{sessionId}` |
| `videoTrackId` | yes | LiveKit video track ID to record |
| `audioTrackId` | no | Optional LiveKit audio track ID |

Response:

```json
{
  "egressId": "EG_...",
  "status": "EGRESS_ACTIVE"
}
```

Status codes:

| Code | Meaning |
|------|---------|
| `202` | Egress started and metadata persisted |
| `400` | Missing required fields or invalid JSON |
| `401` | Missing/invalid admin JWT |
| `502` | LiveKit rejected or failed to start egress |
| `500` | Metadata persistence failed |

If LiveKit starts successfully but PostgreSQL persistence fails, backend calls
`StopEgress` immediately to avoid an orphan recording.

### Stop Recording

`POST /api/v1/media/recordings/stop`

Request:

```json
{
  "egressId": "EG_..."
}
```

Response:

```json
{
  "egressId": "EG_...",
  "status": "EGRESS_ENDING"
}
```

The final status and file URL are still written by the LiveKit webhook once the
egress process finishes.

### LiveKit Webhook

`POST /api/v1/livekit/webhook`

This route is called by LiveKit server, not by the frontend. The handler uses
`webhook.ReceiveWebhookEvent` from the LiveKit SDK and validates the request
signature with `LIVEKIT_API_KEY` and `LIVEKIT_API_SECRET`.

Only `egress_ended` is handled today. Unknown events are acknowledged with
`200 OK` so LiveKit does not retry unrelated events indefinitely.

## Storage Path

Backend requests MP4 output under:

```text
content/recordings/{roomName}/{sessionId}-{studentId}.mp4
```

Example:

```text
content/recordings/argus-session-session-1/session-1-student-1.mp4
```

The filename is for human inspection and S3 organization only. Application logic
must use `egress_id` as the authoritative mapping key.

## PostgreSQL Table

Migration:

```text
argus-infra/migrations/postgres/000007_livekit_recordings.up.sql
```

Table:

```sql
CREATE TABLE IF NOT EXISTS livekit_recordings (
    egress_id      TEXT PRIMARY KEY,
    session_id     TEXT        NOT NULL,
    user_id        TEXT        NOT NULL,
    room_name      TEXT        NOT NULL,
    video_track_id TEXT        NOT NULL DEFAULT '',
    audio_track_id TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL,
    file_url       TEXT        NOT NULL DEFAULT '',
    error_message  TEXT        NOT NULL DEFAULT '',
    started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at       TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Indexes:

| Index | Purpose |
|-------|---------|
| `egress_id` primary key | Exact webhook update |
| `idx_livekit_recordings_session` | Session review/history lookup |
| `idx_livekit_recordings_user` | Student-centric lookup |

Runtime startup also calls `EnsureLiveKitRecordingsTable` as a defensive guard,
but production schema changes should still be delivered through migrations.

## Configuration

Backend reads LiveKit values from environment:

| Variable | Required | Description |
|----------|:--------:|-------------|
| `LIVEKIT_API_KEY` | yes | LiveKit API key used by backend SDK and webhook validation |
| `LIVEKIT_API_SECRET` | yes | LiveKit API secret used by backend SDK and webhook validation |
| `LIVEKIT_WS_URL` | yes | Internal LiveKit WebSocket URL, usually `ws://livekit:7880` |
| `EVENT_COLLECTOR_MINIO_ENDPOINT` | yes | S3-compatible endpoint, usually `minio:9000` in Docker |
| `EVENT_COLLECTOR_MINIO_ACCESS_KEY` | yes | MinIO/S3 access key |
| `EVENT_COLLECTOR_MINIO_SECRET_KEY` | yes | MinIO/S3 secret key |
| `EVENT_COLLECTOR_MINIO_BUCKET` | yes | Bucket for recording output |

In the Docker stack these are provided by `argus-infra/docker/docker-compose.yaml`.

## Infrastructure Requirements

LiveKit Egress requires:

| Dependency | Why |
|------------|-----|
| LiveKit server | Room/tracks and Egress API |
| Redis | Shared LiveKit/Egress state |
| MinIO/S3 | MP4 output |
| Backend HTTP | Webhook receiver and metadata API |
| PostgreSQL | `egress_id` mapping and file status |

The self-hosted LiveKit server must include:

```yaml
webhook:
  api_key: ${LIVEKIT_API_KEY}
  urls:
    - http://backend:8080/api/v1/livekit/webhook
```

`livekit-egress` must receive config through `EGRESS_CONFIG_BODY` with API key,
secret, `ws_url`, Redis, and S3 settings.

## Failure Handling

| Failure | Behavior |
|---------|----------|
| LiveKit start fails | API returns `502`, no DB row is written |
| DB insert fails after LiveKit start | Backend attempts `StopEgress`, API returns `500` |
| Stop request fails in LiveKit | API returns `502`, DB status remains unchanged |
| Stop succeeds but DB update fails | API returns `500`; webhook can still finalize the row later |
| Webhook has invalid signature | Backend returns `401` |
| Webhook references unknown `egress_id` | Backend logs a warning and returns `200` |

## Operational Rules

1. Do not enable continuous full-session recording by default.
2. Prefer selective recording around violation windows.
3. Move `livekit-egress` to a dedicated CPU worker node before high concurrency.
4. Keep MinIO/S3 credentials in environment/secrets, never in source code.
5. Do not parse identity from S3 paths. Use `egress_id`.
6. Keep `livekit_recordings` append/update behavior idempotent for webhook retries.

## Verification

Backend verification:

```bash
GOCACHE=../.gocache go test ./...
```

Infra compose verification:

```bash
MINIO_ROOT_PASSWORD=dummy-minio-password \
LIVEKIT_API_SECRET=dummy-livekit-secret-must-be-at-least-32-chars \
JWT_SIGNING_KEY=dummy-jwt-signing-key-must-be-at-least-32chars \
docker compose -f docker/docker-compose.yaml config --quiet
```

Expected result: both commands exit with code `0`.
