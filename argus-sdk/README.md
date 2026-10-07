# Argus AI SDK

Browser SDK for embedding Argus AI proctoring into LMS, HR, and custom exam pages.

The SDK is the student-side runtime from `ARGUS_PROCTORING_TZ.md`: it runs preflight checks, starts a proctoring session, sends browser/vision/audio events to the backend, and exposes callbacks for partner pages.

## Current Scope

- Preflight checks: camera, microphone, face presence, browser/system compatibility.
- Session lifecycle: `startPreflight()`, `startSession()`, `endSession()`, `destroy()`.
- Event ingestion through the canonical backend gRPC-Web service:
  - `/argus.eventcollector.v1.EventCollectorService/IngestEvent`
  - `/argus.eventcollector.v1.EventCollectorService/IngestBatch`
  - `/argus.eventcollector.v1.EventCollectorService/Heartbeat`
- Browser integrity monitoring: tab switch, fullscreen exit, copy/paste, context menu, print screen heuristics.
- MediaPipe-based face/vision telemetry.
- Audio activity telemetry.
- Health governor and tier-aware degradation primitives.

LiveKit publishing and offline queue hardening are Phase 1 work items tracked in `../ARGUS_PHASE0_PHASE1_BACKLOG.md`.

## Install

For local development:

```bash
npm install
npm run typecheck
npm run contract:paths
npm run build
```

## Usage

```html
<div id="argus-camera"></div>
<script src="https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js"></script>
<script>
  const proctoring = new ArgusSDK({
    sessionToken: '<argusSessionToken from External API>',
    serverUrl: 'https://argusai.kz',
    containerId: 'argus-camera',
    locale: 'ru',
    preflight: {
      consentAccepted: true,
      // Production can self-host MediaPipe assets:
      // mediapipeBasePath: 'https://cdn.example.kz/mediapipe/wasm',
      // mediapipeModelAssetPath: 'https://cdn.example.kz/mediapipe/face_landmarker.task',
    },
    liveKit: {
      enabled: false,
      // To enable recording, pass livekit-client module or load its UMD bundle:
      // enabled: true,
      // client: LivekitClient,
    },
    onViolation(event) {
      console.warn('Argus violation', event);
    },
    onStatusChange(status) {
      console.log('Argus status', status);
    },
    onPreflightComplete(result) {
      console.log('Argus preflight result', result);
    },
    onDeliveryUpdate(state) {
      console.log('Argus delivery queue', state);
    },
    onLiveKitStateChange(state) {
      console.log('Argus LiveKit state', state);
    },
  });

  const passed = await proctoring.startPreflight();
  if (!passed) {
    throw new Error('Argus preflight failed');
  }

  await proctoring.startSession();

  // When the exam is finished:
  // await proctoring.endSession();
  // proctoring.destroy();
</script>
```

## External API Flow

1. Partner backend calls `POST /api/v1/external/sessions` with `X-CLIENT-ID`, `X-API-KEY`, and a stable `Idempotency-Key`.
2. Argus backend returns `sessionId`, `argusSessionToken`, `sdkUrl`, and `expiresAt`.
3. Partner exam page initializes `ArgusSDK` with the token.
4. SDK sends events to the backend using `Authorization: Bearer <argusSessionToken>`.
5. Partner receives status/verdict through External API and webhooks.

For manual local checks, open `examples/partner-quickstart.html` in a browser. It is a sandbox only: a production browser page must never receive the partner `X-API-KEY`.

## Preflight Contract

`startPreflight()` keeps the backward-compatible boolean return value, but the structured result is available through `lastPreflightResult` and `onPreflightComplete`.

Required Phase 1 checks:

- consent accepted;
- camera and microphone permission;
- exactly one face in frame;
- secure context (`https` or localhost);
- fullscreen support;
- browser compatibility;
- backend health/network ping.

The SDK emits `PREFLIGHT_STARTED`, `PREFLIGHT_PASSED`, and `PREFLIGHT_FAILED` lifecycle evidence through the regular ingestion path as system events.

## Session Ingestion Reliability

Session events are batched and sent through the canonical gRPC-Web JSON endpoints with `Authorization: Bearer <argusSessionToken>`.

Reliability behavior:

- failed batches are re-queued at the front of the queue;
- queue retry runs on the flush timer and immediately after the browser returns online;
- queue is bounded to prevent unbounded memory growth;
- low-severity events are dropped first if the queue limit is reached;
- queue state is persisted in `sessionStorage` for the current browser tab session;
- backend batch and heartbeat responses are decoded from proto JSON snake_case to SDK camelCase.

## LiveKit Recording

LiveKit publishing is optional and disabled by default until the partner enables recording:

```html
<script src="https://cdn.jsdelivr.net/npm/livekit-client/dist/livekit-client.umd.min.js"></script>
<script>
  const proctoring = new ArgusSDK({
    sessionToken: '<argusSessionToken>',
    serverUrl: 'https://argusai.kz',
    preflight: { consentAccepted: true },
    liveKit: {
      enabled: true,
      client: window.LivekitClient,
      required: false,
    },
  });
</script>
```

When enabled, the SDK calls:

- `POST /api/v1/external/sessions/{sessionId}/student-token`;
- publishes camera/audio tracks to the returned room;
- `POST /api/v1/external/sessions/{sessionId}/recording-ready`.

## Contract Checks

The SDK keeps its gRPC-Web method paths in `src/core/service-paths.ts`.

Run this before changing proto namespaces or SDK transport code:

```bash
npm run contract:paths
```

The check parses `../argus-backend/api/proto/v1/event_collector.proto` and verifies that SDK paths match the backend package, service, and RPC names.

## Development Notes

- Do not hardcode alternate EventCollector namespaces. The canonical namespace is `argus.eventcollector.v1.EventCollectorService`.
- Keep the SDK framework-agnostic. Shared Vue/Nuxt logic belongs in `argus-frontend`, not here.
- Heavy AI inference must not block the student session path. The SDK sends structured telemetry; backend workers handle deep inference.
