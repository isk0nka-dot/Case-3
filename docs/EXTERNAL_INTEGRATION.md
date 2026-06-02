# Argus AI — External Integration Guide

Partner onboarding guide for integrating Argus AI exam proctoring into your platform via the External API and JavaScript SDK.

## Quick Start (5 minutes)

### 1. Get API Credentials

Contact Argus AI to receive your organization's API credentials:
- **Client ID** (`X-CLIENT-ID`): Your organization's unique key identifier
- **API Secret** (`X-API-KEY`): Your secret key (shown only once at creation)

### 2. Create a Proctoring Session

```bash
curl -X POST https://argusai.kz/api/v1/external/sessions \
  -H "Content-Type: application/json" \
  -H "X-CLIENT-ID: your_key_id" \
  -H "X-API-KEY: your_secret" \
  -H "Idempotency-Key: exam-2024-math-101:student-12345" \
  -d '{
    "examId": "exam-2024-math-101",
    "studentId": "student-12345",
    "studentName": "Askar Nurzhanov",
    "examName": "Mathematics Final Exam"
  }'
```

Response:
```json
{
  "sessionId": "ses_a1b2c3d4e5",
  "argusSessionToken": "eyJhbGciOiJIUzI1NiIs...",
  "sdkUrl": "https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js",
  "expiresAt": "2026-06-01T13:30:00Z"
}
```

### 3. Load the SDK

```html
<script src="https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js"></script>
<script>
  const proctoring = new ArgusSDK({
    sessionToken: 'eyJhbGciOiJIUzI1NiIs...', // From step 2
    serverUrl: 'https://argusai.kz',
    onViolation: (event) => {
      console.warn('Violation:', event.label, event.severity);
    },
    onStatusChange: (status) => {
      console.log('Session status:', status);
    }
  });

  // Run preflight checks (camera, face detection, system)
  const passed = await proctoring.startPreflight();
  if (!passed) {
    alert('Preflight checks failed. Please check camera and microphone.');
    return;
  }

  // Start proctoring
  await proctoring.startSession();

  // When exam is finished:
  // await proctoring.endSession();
  // proctoring.destroy();
</script>
```

---

## Authentication

All External API requests require two headers:

| Header | Description |
|--------|-------------|
| `X-CLIENT-ID` | Your API key ID (public identifier) |
| `X-API-KEY` | Your API secret (keep this secure!) |
| `Idempotency-Key` | Optional but recommended for create-session retries |

### Permissions

API keys have granular permissions:
- `sessions:read` — View session details
- `sessions:write` — Create, complete, cancel sessions
- `webhooks:read` — List webhook endpoints
- `webhooks:write` — Create, delete webhook endpoints

Wildcard permissions are supported: `*` (all), `sessions:*` (all session ops).

### Security Notes

- API secrets are hashed with bcrypt — we never store plaintext secrets
- Always transmit credentials over HTTPS
- Rotate API keys periodically
- Store secrets in environment variables, never in source code
- Do not expose `X-API-KEY` in a production browser page. Your backend should create Argus sessions and pass only `argusSessionToken` to the student page.

### Idempotency

`POST /sessions` supports `Idempotency-Key` and `X-Idempotency-Key`.

Use a stable key for one intended session creation, for example `<examId>:<studentId>:<attemptNo>`. If the same organization sends the same idempotency key again, Argus returns the original `sessionId`, `argusSessionToken`, `sdkUrl`, and `expiresAt` with HTTP `200` instead of creating a duplicate session.

Important: reusing the same key with a different request body returns the original session. Generate a new key for a new exam attempt.

### SDK URL Configuration

The `sdkUrl` field in create-session responses is controlled by backend environment:

```bash
ARGUS_SDK_URL=https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js
```

If `ARGUS_SDK_URL` is not set, the backend falls back to `ARGUS_PUBLIC_SDK_URL`, then to the public CDN URL.

---

## API Reference

Base URL: `https://argusai.kz/api/v1/external`

### Sessions

#### Create Session

```
POST /sessions
```

Creates a new proctoring session and returns a JWT token for the SDK.

**Request Body:**
```json
{
  "examId": "exam-2024-math-101",
  "studentId": "student-12345",
  "studentName": "Askar Nurzhanov",
  "examName": "Mathematics Final Exam",
  "callbackUrl": "https://yoursite.com/webhook",
  "metadata": {"room": "A-101", "seat": 15}
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| examId | string | Yes | Your exam identifier |
| studentId | string | Yes | Your student identifier |
| studentName | string | No | Student display name |
| examName | string | No | Exam display name |
| callbackUrl | string | No | Per-session callback URL |
| metadata | object | No | Custom metadata (stored as JSONB) |

**Response (201):**
```json
{
  "sessionId": "ses_a1b2c3d4e5",
  "argusSessionToken": "eyJ...",
  "sdkUrl": "https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js",
  "expiresAt": "2026-06-01T13:30:00Z"
}
```

**Response (200 idempotency replay):**
Same response shape as above. The existing session is returned when the same `Idempotency-Key` was already used by the same organization.

#### Get Session

```
GET /sessions/{sessionId}
```

Returns session details including status and verdict.

**Response (200):**
```json
{
  "sessionId": "ses_a1b2c3d4e5",
  "status": "completed",
  "verdict": "clean",
  "integrityScore": 95.5,
  "violationCount": 2,
  "startedAt": "2024-01-15T09:30:00Z",
  "completedAt": "2024-01-15T11:30:00Z"
}
```

Session statuses: `created` → `preflight` → `active` → `completed` | `expired` | `cancelled`

#### Get Session Report

```
GET /sessions/{sessionId}/report
```

Returns the Phase 1 JSON report for partner systems.

**Response (200):**
```json
{
  "sessionId": "ses_a1b2c3d4e5",
  "examId": "exam-2024-math-101",
  "studentId": "student-12345",
  "status": "completed",
  "verdict": "clean",
  "integrityScore": 95.5,
  "riskScore": 4.5,
  "violationCount": 2,
  "reviewStatus": "ready",
  "timeline": [],
  "recordings": [],
  "evidenceIntegrity": {
    "status": "not_checked"
  },
  "reportUrl": "/api/v1/external/sessions/ses_a1b2c3d4e5/report",
  "generatedAt": "2026-06-01T13:30:00Z"
}
```

The timeline is read from ClickHouse when analytics storage is available. Full
PDF polish remains outside Phase 1.

#### Complete Session

```
POST /sessions/{sessionId}/complete
```

Marks a session as completed with a verdict.

**Request Body:**
```json
{
  "verdict": "clean",
  "verdictDetails": {"note": "No violations detected"},
  "integrityScore": 95.5,
  "violationCount": 2
}
```

Verdicts: `clean`, `suspicious`, `violation`

#### Cancel Session

```
POST /sessions/{sessionId}/cancel
```

Cancels an active session. No request body required.

### Webhooks

#### List Webhooks

```
GET /webhooks
```

Returns all webhook endpoints for your organization.

#### Create Webhook

```
POST /webhooks
```

**Request Body:**
```json
{
  "name": "Production Webhook",
  "url": "https://yoursite.com/webhooks/argus",
  "events": ["session.started", "session.completed", "violation.detected", "verdict.ready"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | No | Display name for the endpoint |
| url | string | Yes | HTTPS callback URL |
| events | string[] | Yes | Event types to subscribe to |

**Response (201):**
```json
{
  "id": "wh_abc123",
  "secret": "whsec_a1b2c3..."
}
```

The `secret` is shown **only once**. Store it securely for signature verification.

#### Delete Webhook

```
DELETE /webhooks/{webhookId}
```

---

## SDK Usage

### Installation

**Script tag (recommended for quick integration):**
```html
<script src="https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js"></script>
```

**npm (for build systems):**
```bash
npm install @argus-ai/sdk
```

```typescript
import { ArgusSDK } from '@argus-ai/sdk';
```

### Configuration

```typescript
const proctoring = new ArgusSDK({
  // Required
  sessionToken: 'eyJ...',              // JWT from POST /sessions
  serverUrl: 'https://argusai.kz',

  // Optional
  containerId: 'argus-camera',         // Mount point (default: floating widget)
  locale: 'kk',                        // 'kk' | 'ru' | 'en' (default: 'ru')
  mediapipeBasePath: '/path/to/wasm',  // Custom MediaPipe CDN (default: jsdelivr)

  // Callbacks
  onReady: () => console.log('SDK ready'),
  onError: (err) => console.error(err.code, err.message),
  onViolation: (event) => handleViolation(event),
  onStatusChange: (status) => updateUI(status),
  onPreflightStep: (step) => showPreflightProgress(step),
});
```

### Lifecycle

```typescript
// 1. Preflight (camera, face detection, system checks)
const passed = await proctoring.startPreflight();

// 2. Start session (begins monitoring)
await proctoring.startSession();

// 3. During exam — SDK runs automatically
// Access real-time data:
console.log(proctoring.health);         // { fps, rttMs, score, tier }
console.log(proctoring.violationCount); // Number of violations

// 4. End session
await proctoring.endSession();

// 5. Clean up
proctoring.destroy();
```

### Events

```typescript
// Listen to events
proctoring.on('violation', (event) => {
  // event.type: EventType enum
  // event.severity: 1=INFO, 2=WARNING, 3=CRITICAL
  // event.label: Human-readable description
  // event.confidence: 0-1 detection confidence
});

proctoring.on('healthUpdate', (metrics) => {
  // metrics.fps, metrics.rttMs, metrics.score, metrics.tier
});

proctoring.on('tierChange', ({ from, to, config }) => {
  // Adaptive quality: A=720p/10Hz, B=240p/5Hz, C=120p/store-forward
});
```

### Preflight Steps

The SDK runs 3 mandatory checks before allowing session start:

1. **Hardware**: Camera + microphone access (`getUserMedia`)
2. **Environment**: Face detected, single person, properly centered
3. **System**: Secure context (HTTPS), fullscreen support, browser compatibility

```typescript
proctoring.on('preflightStep', (step) => {
  // step.id: 'hardware' | 'environment' | 'system'
  // step.status: 'pending' | 'checking' | 'passed' | 'failed'
  // step.message: Localized status message
  // step.details: Error details (if failed)
});
```

### Advanced: Individual Modules

For advanced use cases, you can use SDK modules individually:

```typescript
import {
  GrpcWebTransport,
  SessionManager,
  HealthGovernor,
  VisionEngine,
  AudioEngine,
  BrowserIntegrityMonitor,
  CameraWidget
} from '@argus-ai/sdk';

// Use only the vision engine
const vision = new VisionEngine({ inferenceIntervalMs: 200 });
vision.on('frame', (frame) => {
  console.log('Face detected:', frame.faceDetected);
  console.log('Head pose:', frame.headPose);
});
await vision.start(videoElement);
```

---

## Webhooks

### Event Types

| Event | Description |
|-------|-------------|
| `session.started` | Session transitioned to `active` |
| `session.completed` | Session ended (verdict available) |
| `violation.detected` | Critical violation detected during session |
| `verdict.ready` | Final proctoring verdict computed; payload includes `reportUrl` |

### Delivery

- **Format**: JSON POST to your HTTPS endpoint
- **Retry**: Exponential backoff (1s, 2s, 4s, 8s, 16s) — max 5 attempts
- **Timeout**: 30 seconds per delivery attempt
- **Signing**: HMAC-SHA256 for payload verification

### Headers

Each webhook delivery includes:

| Header | Description |
|--------|-------------|
| `X-Argus-Signature` | `sha256=<hex_digest>` HMAC signature |
| `X-Argus-Timestamp` | Unix timestamp (seconds) |
| `X-Argus-Event` | Event type (e.g., `session.completed`) |
| `X-Argus-Delivery-Id` | Unique delivery ID |
| `Content-Type` | `application/json` |

### Signature Verification

Verify webhook authenticity using your endpoint secret:

**Python:**
```python
import hmac
import hashlib

def verify_webhook(secret, timestamp, body, signature):
    message = f"{timestamp}.{body}"
    expected = hmac.new(
        secret.encode(), message.encode(), hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

**Node.js:**
```javascript
const crypto = require('crypto');

function verifyWebhook(secret, timestamp, body, signature) {
  const message = `${timestamp}.${body}`;
  const expected = crypto
    .createHmac('sha256', secret)
    .update(message)
    .digest('hex');
  return `sha256=${expected}` === signature;
}
```

**Go:**
```go
func verifyWebhook(secret, timestamp, body, signature string) bool {
    message := timestamp + "." + body
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(message))
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(signature))
}
```

### Payload Examples

**session.completed:**
```json
{
  "event": "session.completed",
  "sessionId": "ses_a1b2c3d4e5",
  "orgId": "org_xyz",
  "timestamp": "2024-01-15T11:30:00Z",
  "data": {
    "verdict": "clean",
    "integrityScore": 95.5,
    "violationCount": 2,
    "duration": "2h0m0s"
  }
}
```

**violation.detected:**
```json
{
  "event": "violation.detected",
  "sessionId": "ses_a1b2c3d4e5",
  "orgId": "org_xyz",
  "timestamp": "2024-01-15T10:15:30Z",
  "data": {
    "type": "TAB_SWITCH",
    "severity": "WARNING",
    "label": "Tab switch detected",
    "confidence": 0.95
  }
}
```

---

## Troubleshooting

### Camera Not Working

- Ensure HTTPS (required for `getUserMedia`)
- Check browser permissions (camera + microphone must be allowed)
- Verify no other application is using the camera
- Supported browsers: Chrome 90+, Firefox 90+, Edge 90+, Safari 15+

### Face Not Detected

- Ensure adequate lighting (avoid backlighting)
- Face should be centered and clearly visible
- Remove hats, masks, or heavy sunglasses
- Camera should be at eye level

### Network Issues

The SDK automatically adapts to network conditions:
- **Tier A** (score ≥ 70): Full quality — 720p video, 10 FPS AI
- **Tier B** (score 40-69): Reduced — 240p video, 5 FPS AI
- **Tier C** (score < 40): Minimal — 120p, AI disabled, store-and-forward

### API Errors

| Status | Meaning |
|--------|---------|
| 400 | Invalid request (check required fields) |
| 401 | Invalid API credentials |
| 403 | Insufficient permissions |
| 404 | Resource not found (or belongs to another org) |
| 429 | Rate limit exceeded (50 req/s per IP) |
| 500 | Server error (retry with backoff) |

---

## Browser Compatibility

| Browser | Min Version | Notes |
|---------|------------|-------|
| Chrome | 90+ | Full support, GPU delegate for MediaPipe |
| Firefox | 90+ | Full support |
| Edge | 90+ | Full support (Chromium-based) |
| Safari | 15+ | Partial (no Screen Capture API) |

### Required Browser APIs

- `getUserMedia` (camera + microphone)
- `AudioContext` (voice activity detection)
- `requestAnimationFrame` (FPS monitoring)
- `IndexedDB` (offline event queue)
- `fetch` (event delivery)
- `Fullscreen API` (exam lockdown)

### Optional APIs (enhanced detection)

- `Screen Capture API` (screen sharing detection)
- `Screen Details API` (external display detection)
- `Web Crypto API` (payload signing)

---

## Rate Limits

| Endpoint | Limit | Burst |
|----------|-------|-------|
| External API (`/api/v1/external/*`) | 50 req/s per IP | 100 |
| SDK assets (`/sdk/*`) | Unlimited (static CDN) | N/A |
| gRPC events (from SDK) | 100 req/s per IP | 200 |

---

## Face Identity Verification (Phase 2)

Argus can verify the student's identity by comparing the live camera feed against a reference photo.

### 1. Enroll a reference photo

Pass `referencePhotoUrl` when creating a session — Argus fetches the photo and extracts a face embedding in the background:

```bash
curl -X POST https://argusai.kz/api/v1/external/sessions \
  -H "X-CLIENT-ID: your_key_id" \
  -H "X-API-KEY: your_secret" \
  -H "Content-Type: application/json" \
  -d '{
    "examId": "exam-001",
    "studentId": "student-001",
    "studentName": "Aiman Nurova",
    "referencePhotoUrl": "https://lms.example.kz/photos/student-001.jpg"
  }'
```

Or enroll explicitly (e.g., at account registration):

```bash
curl -X POST https://argusai.kz/api/v1/external/students/student-001/enroll \
  -H "X-CLIENT-ID: your_key_id" \
  -H "X-API-KEY: your_secret" \
  -H "Content-Type: application/json" \
  -d '{ "photoUrl": "https://lms.example.kz/photos/student-001.jpg" }'
```

Check enrollment status:

```bash
curl https://argusai.kz/api/v1/external/students/student-001/enrollment \
  -H "X-CLIENT-ID: your_key_id" -H "X-API-KEY: your_secret"
```

```json
{
  "studentId": "student-001",
  "enrolled": true,
  "enrolledBy": "api",
  "enrolledAt": "2026-06-01T12:00:00Z",
  "modelVersion": "arcface_r50_w600k",
  "embeddingDim": 512
}
```

### 2. Enable liveness challenge in SDK

For strict mode, require the student to demonstrate natural face movement:

```js
const proctoring = new ArgusSDK({
  sessionToken: '...',
  serverUrl: 'https://argusai.kz',
  preflight: {
    consentAccepted: true,
    requireLivenessChallenge: true,   // Detects live person vs static photo
  },
});
```

---

## AI Deep Scan Report

After a session completes, the backend automatically runs an AI deep scan
(object detection, identity verification, liveness scoring). Results appear
in the session report within minutes.

```bash
curl https://argusai.kz/api/v1/external/sessions/ses_abc123/report \
  -H "X-CLIENT-ID: your_key_id" -H "X-API-KEY: your_secret"
```

The `aiDetections` section of the response:

```json
{
  "aiDetections": {
    "scanned": true,
    "identityVerified": true,
    "avgFaceSimilarity": 0.83,
    "faceMismatchCount": 0,
    "livenessFailCount": 0,
    "avgLivenessScore": 0.91,
    "objectDetections": [
      { "objectType": "PHONE_DETECTED", "count": 2, "maxConfidence": 0.94 }
    ],
    "backendEventCount": 47
  }
}
```

---

## Student Exam Shell

For partners who need a ready-made student UI without building their own:

```
https://argusai.kz/exam.html
  ?token=<argusSessionToken>
  &serverUrl=https://argusai.kz
  &locale=ru
  &examName=Mathematics+Final
  &examUrl=https://lms.example.kz/exam/123   (optional: iframe)
  &livekit=true                               (optional: LiveKit recording)
```

Screens: consent → preflight → active exam → done. Supports RU/KK/EN.
The file is available at `argus-sdk/examples/student-exam-shell.html`.
