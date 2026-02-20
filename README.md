# argus-frontend — The Dashboard of Argus AI

> **You do not need to know Go to work on this repository.**
> The frontend communicates with the backend exclusively through the REST API defined in
> `argus-backend/api/openapi.yaml`. That file is your complete contract — every endpoint,
> every request schema, every response field is documented there.

---

## Table of Contents

1. [Overview](#overview)
2. [Tech Stack](#tech-stack)
3. [Key Modules](#key-modules)
   - [Student Connection Banner](#student-connection-banner)
   - [Offline-First Storage](#offline-first-storage)
   - [Tier-C Visualization](#tier-c-visualization)
4. [API Integration](#api-integration)
5. [Application Structure](#application-structure)
6. [Pinia Stores](#pinia-stores)
7. [Route Protection & RBAC](#route-protection--rbac)
8. [Getting Started](#getting-started)
9. [Development Workflow](#development-workflow)
10. [Environment Variables](#environment-variables)
11. [Related Repositories](#related-repositories)

---

## Overview

`argus-frontend` is a **Nuxt 4 Single-Page Application** (SSR disabled) that serves as
the administrative and monitoring dashboard for the Argus AI proctoring platform.

It is intentionally a **thin client**: no database access, no Go code, no AI algorithms,
no secrets. It renders data from the backend REST API and provides proctors, org admins,
and super admins with real-time monitoring, historical review, evidence playback, and
platform management tools.

The only network endpoint the frontend ever calls is the backend API at `NUXT_PUBLIC_API_BASE_URL`.

---

## Tech Stack

| Technology | Version | Role |
|------------|---------|------|
| **Nuxt** | 4.x | SPA framework (SSR disabled, `ssr: false`) |
| **Vue.js** | 3.x | Component model (Composition API throughout) |
| **TypeScript** | 5.x | Type safety across all composables, stores, and components |
| **Tailwind CSS** | 4.x | Utility-first styling |
| **Nuxt UI** | 4.x | Headless UI component library (built on Tailwind) |
| **Pinia** | 2.x | Application state management |
| **Chart.js + vue-chartjs** | 4.x / 5.x | Executive and analytics charts |
| **LiveKit Client** | 2.x | WebRTC video streaming for proctoring room video |
| **IndexedDB** (`app/lib/storage/idb.ts`) | Browser native | Offline write-ahead log for proctoring events |
| **Iconify** (Lucide + Simple Icons) | — | Icon sets |

---

## Key Modules

### Student Connection Banner

**File:** `app/components/StudentConnectionBanner.vue`

The `StudentConnectionBanner` is a non-interruptive overlay that communicates the current
network resilience tier to the student **without causing anxiety or panic**.

#### Design Goals

- The student's exam session must **never stop** due to network instability — the system
  handles offline buffering transparently.
- Colors are chosen deliberately: **green → amber → blue** (never red). Red connotes failure.
  Blue means "saving locally, will upload when reconnected" — safe and non-alarming.
- Auto-dismissing toasts (4 seconds) so students are informed but not distracted.

#### How It Works

The banner subscribes to the resilience state from `useResilience()` composable:

```
useHealthGovernor()  ─── samples every 5s ───►  healthScore (0–100)
     │                                               │
     │  FPS (25%) + RTT (25%) + packet loss (20%)   │
     │  + CPU estimate (15%) + bandwidth (15%)       │
     ▼                                               ▼
useTierEngine()  ─── state machine ───►  activeTier: 'A' | 'B' | 'C'
     │                                               │
     │  Hysteresis: 3 bad checks to downgrade        │
     │             5 good checks to upgrade          │
     ▼                                               ▼
StudentConnectionBanner.vue
     ├── Tier A: Green dot, "Connection: Excellent" (shown briefly, then hidden)
     ├── Tier B: Amber dot, "Connection: Degraded — recording locally"
     └── Tier C: Blue dot, "Offline — all activity saved, will sync automatically"
```

**Props:**

| Prop | Type | Description |
|------|------|-------------|
| `tier` | `'A' \| 'B' \| 'C'` | Current resilience tier |
| `healthScore` | `number` | 0–100 health score |
| `hasPendingItems` | `boolean` | Whether the offline queue has unsent items |
| `pendingCount` | `number` | Number of queued events |
| `isOffline` | `boolean` | Complete offline (no connectivity) |

---

### Offline-First Storage

**Files:**
- `app/lib/storage/idb.ts` — Zero-dependency IndexedDB wrapper
- `app/composables/useOfflineQueue.ts` — Write-ahead log with drain logic

#### The Problem

In Tier B and Tier C conditions (degraded or no network), the browser SDK must continue
capturing proctoring events. These events cannot be lost — they are potential evidence of
academic integrity violations. The system must buffer them durably until connectivity recovers.

#### Architecture: IndexedDB Write-Ahead Log

Every proctoring event is written to **IndexedDB before upload is attempted**.
This is the write-ahead log (WAL) pattern: durability before delivery.

```
Event captured (violation, gaze, keystroke...)
    │
    ▼  addEvents([event])  — synchronous from app perspective
IndexedDB: "events" object store
    │  status: 'pending', priority: 'critical'|'high'|'normal'|'low'
    │  sessionId, createdAt indexes
    ▼
Background drain loop (every 1,000ms)
    │  Queries: status='pending' ORDER BY priority DESC, createdAt ASC
    │  Batch: 50 events at a time
    ▼  setEventUploader() callback
gRPC streaming / REST API upload
    │
    ├── Success: updateEventStatus(id, 'sent')
    └── Failure: updateEventStatus(id, 'failed'), retry up to 10x with backoff
```

#### Drain Priority Order

When connectivity is restored, events are drained in priority order to ensure
critical evidence reaches the server first:

```
Priority 1: CRITICAL events  (face detection, identity mismatch, screen sharing)
Priority 2: HIGH events       (tab switch, multiple faces)
Priority 3: NORMAL events     (gaze deviation, keystroke burst)
Priority 4: LOW events        (heartbeat, periodic snapshots)
Priority 5: SNAPSHOTS         (binary JPEG evidence — larger, lower urgency)
```

#### Storage Quota Management (v2.1)

The queue monitors the browser's storage quota every 30 seconds via `navigator.storage.estimate()`:

- **< 80% used:** Normal operation
- **≥ 80% used:** Begin evicting `LOW` priority events (LRU — oldest first)
- **CRITICAL and HIGH events are never evicted** — they are potential legal evidence

Configuration defaults:
- Maximum queue size: 500 MB
- Maximum events per session: 50,000
- Drain interval: 1,000ms
- Maximum retries per event: 10

#### Cryptographic Signing

Each event stored in IndexedDB is HMAC-SHA256 signed with a per-session signing key
(derived from the JWT session secret). This ensures that any tampering with the local
IndexedDB store is detectable when the backend verifies the chain on upload.

---

### Tier-C Visualization

**Files:**
- `app/composables/useTierEngine.ts` — Tier state machine
- `app/composables/useHealthGovernor.ts` — Health metrics sampling
- `app/components/ResilienceIndicator.vue` — Admin/proctor view
- `app/assets/css/low-spec.css` — Reduced-motion, low-bandwidth CSS overrides

#### The Three Tiers

The Resilience Engine degrades the proctoring session gracefully as network conditions worsen.
The key insight: **it is better to capture degraded evidence than no evidence.**

| Tier | Condition | Video | AI Analysis | Upload Mode | UI Color |
|------|-----------|-------|-------------|------------|---------|
| **A — Optimal** | Score ≥ 70, RTT < 150ms | 720p continuous | Real-time, all models | Streaming (live) | Green |
| **B — Strained** | Score 40–70, RTT 150–500ms | 240p or snapshot mode | Reduced model set | Batched (every 30s) | Amber |
| **C — Critical** | Score < 40, RTT > 500ms or offline | No video — JPEG snapshots only | Offline, stored locally | Store-and-forward (drain on recovery) | Blue |

#### Tier Transition Rules (Hysteresis)

Tier transitions use hysteresis to prevent flapping — rapid oscillation between tiers
that would be disruptive for the student:

```
Downgrade (A→B or B→C):  3 consecutive health checks below threshold
  Upgrade (C→B or B→A):  5 consecutive health checks above threshold

Direct jumps (A→C or C→A) are NOT allowed — the system transitions through B.
```

#### Burst Mode (Tier C)

In Tier C, continuous video capture is suspended to conserve bandwidth and battery.
Instead, **Burst Mode** captures high-resolution JPEG snapshots:

- Baseline capture: 1 snapshot every 30 seconds
- Triggered capture: 1 snapshot immediately upon any violation event
- Snapshots are stored in IndexedDB (as `ArrayBuffer`) and uploaded when Tier A/B resumes

#### ResilienceIndicator Component

The `ResilienceIndicator.vue` component provides the admin/proctor view of a student's
current tier and queue state. It is typically placed in the monitoring dashboard overlay.

```vue
<ResilienceIndicator
  :tier="session.tier"
  :health-score="session.healthScore"
  :pending-count="session.queueDepth"
  :connection-message="session.statusMessage"
/>
```

---

## API Integration

> **The frontend is strictly decoupled from the backend implementation.**

The entire API surface is defined by `argus-backend/api/openapi.yaml`.
**This is the only document you need to understand what data is available, what requests
to make, and what responses to expect.**

### The `useAdminAPI` Composable

**File:** `app/composables/useAdminAPI.ts`

All HTTP communication with the backend is centralized in the `useAdminAPI` composable.
No component makes `fetch()` or `axios()` calls directly.

```typescript
// Usage in any component or page:
const api = useAdminAPI()

// Authentication
const { token, user } = await api.login(phone, password)

// Analytics
const overview = await api.getAnalyticsOverview()
const infra = await api.getInfrastructureStats()

// Monitoring
const sessions = await api.getActiveSessions()
await api.warnSession(sessionId)
await api.terminateSession(sessionId)

// Archive
const sessions = await api.getArchivedSessions({ orgId, examId, status })
const events = await api.getSessionEvents(sessionId)

// Export
const job = await api.createExportJob({ sessionIds: ['sess_abc', 'sess_def'] })
```

### Base URL Configuration

The composable reads the backend URL from the Nuxt runtime config:

```typescript
// app/composables/useAdminAPI.ts
const config = useRuntimeConfig()
const baseURL = config.public.apiBaseUrl  // Set via NUXT_PUBLIC_API_BASE_URL
```

This means **you never hardcode a server address** — the backend URL is injected at build time
or runtime via the environment variable.

### Authentication in API Calls

Once logged in, the JWT token is stored in `useAuthStore` and automatically attached
to every request:

```typescript
// Every request includes the Authorization header automatically
const headers = {
  'Authorization': `Bearer ${authStore.jwtToken}`,
  'Content-Type': 'application/json',
}
```

If the backend returns `401 Unauthorized`, the composable clears the auth state and
redirects to the login screen — **unless the session is a demo session**, in which case
the API failure is handled gracefully with demo data fallback.

---

## Application Structure

```
argus-frontend/
│
├── app/
│   ├── app.vue                          ← Root component
│   ├── app.config.ts                    ← Theme + UI config
│   │
│   ├── components/                      ← Reusable Vue components
│   │   ├── StudentConnectionBanner.vue  ← Student-facing tier indicator
│   │   ├── ResilienceIndicator.vue      ← Admin/proctor tier display
│   │   ├── TransportHealthIndicator.vue ← Network metrics overlay
│   │   ├── CriticalAlertsBanner.vue     ← Critical alert feed
│   │   ├── EvidenceViewer.vue           ← Evidence fragment player
│   │   ├── VideoPlayer.vue              ← LiveKit WebRTC player
│   │   ├── GazeHeatmap.vue              ← Canvas-based gaze visualization
│   │   ├── LatencyChart.vue             ← Real-time latency graph
│   │   ├── LiveEventFeed.vue            ← Real-time event stream
│   │   ├── MonitoringModal.vue          ← Full-screen session detail modal
│   │   ├── ReviewPanel.vue              ← Evidence review + decision UI
│   │   ├── ViolationChart.vue           ← Violation frequency chart
│   │   ├── OrgSwitcher.vue              ← Super admin org context selector
│   │   ├── LoginModal.vue               ← Auth modal (real + demo login)
│   │   ├── AppLogo.vue / ArgusLogo.vue  ← Branding
│   │   └── TemplateMenu.vue             ← Navigation template
│   │
│   ├── composables/                     ← Vue 3 Composition API hooks
│   │   ├── useAdminAPI.ts               ← All REST API calls (single entry point)
│   │   ├── useResilience.ts             ← Master resilience orchestrator
│   │   ├── useTierEngine.ts             ← Tier A/B/C state machine
│   │   ├── useHealthGovernor.ts         ← Network health sampling (5s interval)
│   │   ├── useOfflineQueue.ts           ← IndexedDB write-ahead log
│   │   ├── useProctoringSession.ts      ← Session lifecycle (start/stop/events)
│   │   ├── useProctoringAlerts.ts       ← Real-time alert management
│   │   ├── useSnapshotCapture.ts        ← JPEG snapshot capture (Tier B/C)
│   │   ├── useTransportMetrics.ts       ← RTT, packet loss, bandwidth metrics
│   │   └── useLowSpecMode.ts            ← Low-spec device detection + CSS class toggle
│   │
│   ├── stores/                          ← Pinia global state
│   │   ├── useAuthStore.ts              ← JWT, user profile, RBAC, org context
│   │   ├── useDashboardStore.ts         ← Dashboard data (exams, sessions, violations)
│   │   ├── useEventFeedStore.ts         ← Real-time event feed state
│   │   └── useTelemetryStore.ts         ← High-frequency telemetry (Float32Array + double-buffer)
│   │
│   ├── middleware/
│   │   └── auth.global.ts               ← Route guard (runs on every navigation)
│   │
│   ├── pages/                           ← File-system routing
│   │   ├── index.vue                    ← Home / login redirect
│   │   ├── landing.vue                  ← Public marketing page (no auth required)
│   │   ├── student-guide.vue            ← Public student guide (no auth required)
│   │   ├── monitoring.vue               ← Live session monitoring dashboard
│   │   ├── analytics.vue                ← Analytics overview page
│   │   ├── archive.vue                  ← Session archive + search
│   │   ├── organizations.vue            ← Org management (super_admin only)
│   │   └── dashboard/
│   │       ├── executive.vue            ← Executive analytics (super_admin)
│   │       ├── infrastructure.vue       ← Infrastructure health (super_admin)
│   │       ├── exams.vue                ← Exam management
│   │       ├── exports.vue              ← Bulk export jobs
│   │       ├── violations.vue           ← Violation review
│   │       └── regions.vue              ← Geographic distribution
│   │
│   ├── lib/
│   │   ├── grpc/                        ← gRPC-Web client for event streaming
│   │   │   ├── client.ts                ← Main gRPC client
│   │   │   ├── transport.ts             ← HTTP/2 transport layer
│   │   │   ├── chunked-upload.ts        ← Tier-C chunked binary upload
│   │   │   └── interceptors.ts          ← Auth + retry interceptors
│   │   ├── proto/                       ← TypeScript-generated Protobuf types
│   │   │   ├── types.ts                 ← ProctoringEvent, EventType, Severity
│   │   │   └── codec.ts                 ← Protobuf encode/decode
│   │   └── storage/
│   │       └── idb.ts                   ← IndexedDB wrapper (zero-dependency, ~684 lines)
│   │
│   ├── plugins/
│   │   └── grpc.client.ts               ← gRPC client initialization (Nuxt plugin)
│   │
│   └── assets/
│       └── css/
│           ├── main.css                 ← Global styles + Tailwind base
│           └── low-spec.css             ← Reduced-motion / low-bandwidth overrides
│
├── nuxt.config.ts                       ← Nuxt configuration
├── package.json
├── tsconfig.json
├── Dockerfile                           ← Multi-stage: node:22-alpine build + runtime
└── .env.example                         ← Environment variable documentation
```

---

## Pinia Stores

### `useAuthStore` — Authentication & RBAC

The central identity store. Persists to `localStorage` for session resume across page refreshes.

```typescript
const auth = useAuthStore()

// State
auth.jwtToken          // JWT string (null if not logged in)
auth.user              // { id, orgId, phone, fullName, email, role, isActive }
auth.isAuthenticated   // Boolean derived from jwtToken presence
auth.hasValidToken     // Boolean: jwtToken !== null (for demo vs real login distinction)

// Role helpers
auth.isSuperAdmin      // true if role === 'super_admin'
auth.isOrgAdmin        // true if role === 'org_admin' or 'super_admin'
auth.effectiveOrgId    // selectedOrgId for super_admin, own orgId for others

// Actions
await auth.login(phone, password)     // Sets token + user from API
auth.setToken(token, sessionId, userId, orgId)  // Direct set (demo login)
auth.logout()                         // Clears all auth state + localStorage
auth.switchOrg(orgId)                 // Super admin org context switch
```

**RBAC Role Hierarchy:**

| Role | Can access |
|------|-----------|
| `super_admin` | Everything + `/dashboard/executive`, `/dashboard/infrastructure`, `/organizations` |
| `org_admin` | Own org: users, API keys, exports, integrity |
| `proctor` | Live monitoring, archive review, session evidence |
| `viewer` | Read-only analytics and archive |

### `useTelemetryStore` — High-Frequency Telemetry

Optimized for recording 60–95 data points per second (gaze, mouse, keyboard) for 10,000
concurrent sessions without overwhelming Vue's reactivity system.

**The Problem:** At 95 events/sec × 10,000 sessions = 950,000 reactive updates/sec.
Standard Pinia reactive objects would freeze the browser.

**Solution: Double-Buffering with Float32Array**

```typescript
// Non-reactive write buffer (updated at event frequency: 30–60 Hz)
// Float32Array uses 4x less memory than Array<number>
const writeBuffer: Float32Array = new Float32Array(1024)

// Reactive read buffer (swapped at visualization frequency: 2 Hz)
const readBuffer = ref<Float32Array>(new Float32Array(1024))

// Every 500ms: atomically swap buffers
function swapBuffers() {
  readBuffer.value = writeBuffer.slice()  // Only triggers reactivity at 2 Hz
}
```

This means the Vue component tree re-renders at 2Hz (smooth enough for gaze heatmaps)
while capturing data at the full 60Hz rate.

---

## Route Protection & RBAC

**File:** `app/middleware/auth.global.ts`

The global route middleware runs before every page navigation. It reads auth state from
`localStorage` (not from the server — this is a pure SPA):

```
Navigation attempt
    │
    ▼
Is route public? (/landing, /student-guide)
    ├── Yes → allow
    └── No → check localStorage.argus_auth
                │
                ├── Not found / expired → redirect to /landing
                └── Found → check role for protected routes
                                │
                                ├── /dashboard/executive, /dashboard/infrastructure,
                                │   /organizations, /api → requires super_admin
                                │   └── Not super_admin → redirect to /?denied=1
                                │
                                └── All other routes → any authenticated user allowed
```

---

## Getting Started

### Prerequisites

| Tool | Minimum Version |
|------|----------------|
| Node.js | 20 LTS |
| npm | 10.x |

### 1. Clone and Install

```bash
git clone git@gitlab.argus.ai:argus/argus-frontend.git
cd argus-frontend

# Install all dependencies
npm install
```

### 2. Configure the Environment

```bash
cp .env.example .env
```

Edit `.env`:

```env
# Backend API URL — where argus-backend is running
NUXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

For local development with the full stack running via `argus-infra`:
```env
NUXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

For connecting to the staging environment:
```env
NUXT_PUBLIC_API_BASE_URL=https://api-staging.argus.ai
```

### 3. Start the Development Server

```bash
npm run dev
```

The dashboard will be available at **http://localhost:3000**.

**Demo login (no backend required):**
- Phone: `+7 (999) 000-00-00`
- Password: `Admin1234!`

Demo mode uses mock data for all dashboard pages and does not require the backend
to be running. The `StudentConnectionBanner` and `ResilienceIndicator` components
are fully functional in demo mode (you can test tier transitions by throttling
your network in browser DevTools).

---

## Development Workflow

### Available Scripts

```bash
npm run dev          # Start dev server with HMR at http://localhost:3000
npm run build        # Build for production (output in .output/)
npm run preview      # Preview the production build locally
npm run lint         # Run ESLint
npm run lint:fix     # Run ESLint with auto-fix
npm run typecheck    # Run vue-tsc type checking
```

### Adding a New Dashboard Page

1. Create `app/pages/dashboard/my-page.vue`
2. Nuxt auto-generates the route `/dashboard/my-page`
3. If the page requires `super_admin`, add it to the protected routes list in `app/middleware/auth.global.ts`
4. Fetch data via `useAdminAPI()` — check `api/openapi.yaml` for the endpoint

### Adding a New API Call

1. Open `api/openapi.yaml` (in `argus-backend`) to find the endpoint details
2. Add a method to `app/composables/useAdminAPI.ts`:

```typescript
async function getMyData(params: MyParams): Promise<MyResponse> {
  return request('GET', `/api/v1/my-endpoint?param=${params.value}`)
}

// Expose in the return object:
return {
  // ... existing methods
  getMyData,
}
```

3. The `request()` function automatically adds `Authorization: Bearer <token>`, handles
   401 (logout for real sessions, graceful fallback for demo), and throws typed errors.

### TypeScript Type Safety

All API request and response types should mirror the OpenAPI schemas exactly.
Define them as TypeScript interfaces in `useAdminAPI.ts` alongside the method:

```typescript
interface MyResponse {
  examId: string
  sessionCount: number
  generatedAt: string  // ISO 8601
}
```

Field names **must match the `camelCase` JSON property names** in the OpenAPI spec exactly.
The backend always sends camelCase — never `snake_case`.

---

## Environment Variables

| Variable | Default | Required | Description |
|----------|---------|:--------:|-------------|
| `NUXT_PUBLIC_API_BASE_URL` | `http://localhost:8080` | ✅ | Backend API base URL. The only required variable. |
| `NUXT_PUBLIC_GRPC_URL` | — | No | Backwards-compatible alias for `NUXT_PUBLIC_API_BASE_URL` |
| `NUXT_DEVTOOLS_ENABLED` | `true` | No | Disable Nuxt DevTools in CI: `false` |
| `NODE_ENV` | `development` | No | Set to `production` in Docker/CI |

> **Security note:** All `NUXT_PUBLIC_*` variables are embedded in the client-side JavaScript
> bundle at build time. **Never put secrets in `NUXT_PUBLIC_*` variables.**
> Secrets (JWT tokens, passwords) are runtime values stored in memory/localStorage — never in env vars.

---

## Related Repositories

| Repository | Path | Purpose |
|------------|------|---------|
| argus-backend | `~/Desktop/argus_ai/argus-backend` | Go REST API + gRPC event collector |
| **argus-frontend** | `~/Desktop/argus_ai/argus-frontend` | **This repo** — Dashboard SPA |
| argus-infra | `~/Desktop/argus_ai/argus-infra` | Docker Compose, Nginx, CI/CD |

**API Contract:** `argus-backend/api/openapi.yaml`

This is the complete, authoritative reference for every API endpoint.
If what you need is not in this spec, raise it with the backend team — do not
call undocumented endpoints directly.
