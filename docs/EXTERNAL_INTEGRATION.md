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
  "sdkUrl": "https://argusai.kz/sdk/argus-sdk.umd.js",
  "expiresAt": "2024-01-15T13:30:00Z"
}
```

### 3. Load the SDK

```html
<script src="https://argusai.kz/sdk/argus-sdk.umd.js"></script>
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
  "sdkUrl": "https://argusai.kz/sdk/argus-sdk.umd.js",
  "expiresAt": "2024-01-15T13:30:00Z"
}
```

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
<script src="https://argusai.kz/sdk/argus-sdk.umd.js"></script>
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
| `verdict.ready` | Final proctoring verdict computed |

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
