# Argus AI — Technical Architecture Guide

**Audience:** Senior engineers joining the Argus AI team.
**Purpose:** Understand the system's design contracts in 5 minutes. Break none of them.

## CI/CD Contract

The frontend pipeline runs `npm ci`, unit tests, lint, typecheck, and a production Nuxt build before deployment is available. Deployment is manual only.

Required GitLab CI variables:

| Variable | Type | Description |
| --- | --- | --- |
| `ARGUS_DEPLOY_HOST` | Variable | Production server IP or DNS name. |
| `ARGUS_DEPLOY_USER` | Variable | SSH user, usually `deploy`. |
| `ARGUS_DEPLOY_SSH_KEY` | File | Private deploy key. Use GitLab variable type `File`; multiline OpenSSH keys may not be accepted as hidden masked variables. |

Optional variables:

| Variable | Default | Description |
| --- | --- | --- |
| `ARGUS_PROJECT_ROOT` | `/opt/argus` | Root directory reserved for this Argus installation on a shared server. |
| `ARGUS_FRONTEND_DEPLOY_PATH` | `$ARGUS_PROJECT_ROOT/frontend` | Frontend checkout/build directory on the server. |
| `NUXT_PUBLIC_API_BASE_URL` | `https://argusai.kz` | Public backend URL baked into the Nuxt build. |

The manual `deploy-production` job runs `sh ci/scripts/deploy_frontend.sh`. It checks SSH access first, syncs only the frontend repo into `$ARGUS_PROJECT_ROOT/frontend`, builds `argus/frontend:latest`, and starts `argus-frontend-app`. It does not prune Docker globally or touch other projects on the server.

---

## 1. System Blueprint

Argus AI is a polyrepo ecosystem. Three repositories, three teams, one event pipeline.

```
┌─────────────────────────────────────────────────────────────────────┐
│                         argus-infra                                 │
│  Docker Compose · Nginx · Migrations · CI/CD                       │
│                                                                     │
│  ┌──────────────────────┐          ┌──────────────────────┐        │
│  │   argus-frontend     │  gRPC-Web │   argus-backend      │        │
│  │                      │◄────────►│                      │        │
│  │   Nuxt 4 / Vue 3     │  JSON/HTTP│   Go 1.24            │        │
│  │   TypeScript          │          │   gRPC + REST         │        │
│  └──────────┬───────────┘          └──────────┬───────────┘        │
│             │                                  │                    │
│             │ SPA served via Nginx             │ Writes to:        │
│             │                                  ├─► Kafka (primary) │
│             │                                  ├─► ClickHouse      │
│             │                                  ├─► MinIO (S3)      │
│             │                                  └─► PostgreSQL      │
└─────────────┴──────────────────────────────────┴────────────────────┘
```

**Repo boundaries are strict.** Frontend never talks to databases. Backend never serves HTML. Infra never contains application logic. Each repo has its own CI pipeline; infra orchestrates deployment via triggered downstream pipelines and image tags (`BACKEND_TAG`, `FRONTEND_TAG`).

**Communication contract:** The frontend speaks to the backend exclusively through gRPC-Web (JSON over HTTP, proxied by Nginx) and REST endpoints under `/api/v1/`. The proto definition at `argus-backend/api/proto/v1/event_collector.proto` is the single source of truth for the wire format.

---

## 2. The Event Pipeline

This is the core data flow. Every proctoring event from every browser session follows this path:

```
Browser (Vue composable)
  │
  ├─ CRITICAL events ──► IngestEvent (unary gRPC, immediate)
  │
  └─ WARNING/INFO/TELEMETRY ──► Client batch buffer
                                    │
                                    ▼
                              IngestBatch (gRPC, tier-adaptive interval)
                                    │
                           ┌────────┼────────────┐
                           ▼        ▼             ▼
                        Kafka   ClickHouse   Evidence Recorder
                      (primary) (secondary)    (tertiary)
                         │          │              │
                         │          │         Ring Buffer → MinIO
                         │          │              │
                         │          │         Chain Writer
                         │          │         (Forensic Ledger)
                         │          │
                    7-day retain  90-day TTL
                    Source of     Analytical
                    truth         queries
```

**Failure semantics are asymmetric by design:**
- Kafka write fails → event is **rejected** (client retries or queues offline)
- ClickHouse write fails → event is **accepted** (Kafka is source of truth; ClickHouse rebuilds from Kafka)
- Evidence capture fails → event is **accepted** (async, never blocks ingestion)

**Never change this hierarchy.** Kafka is the append-only log. ClickHouse is the query engine. They are not interchangeable.

---

## 3. Backend Architecture (Go)

### 3.1 Directory Contract

```
cmd/server/main.go              ← 18-step bootstrap, wires everything
internal/
  domain/
    entity/                     ← ProctoringEvent, EvidenceFragment, Appeal, Consent
    valueobject/                ← EventType, Severity, EventSource (typed enums)
  application/
    port/                       ← Interfaces: EventWriter, EvidenceStore, EvidenceChainWriter
    usecase/                    ← IngestUseCase (the only use case)
  infrastructure/
    kafka/                      ← AsyncProducer (sarama), idempotent, zstd
    clickhouse/                 ← Native protocol, two-stage buffer swap
    minio/                      ← S3 upload with single-pass SHA-256 (TeeReader)
    postgres/                   ← Admin/SaaS layer (orgs, users, audit, exports)
    ringbuffer/                 ← Per-session rolling video window (60s, ~3MB)
    evidence/                   ← Forensic Ledger chain writer (SHA-256 hash chaining)
    recorder/                   ← Orchestrates capture: snapshot → concat → upload → chain
  transport/
    grpc/                       ← gRPC server, worker pool, interceptor chain, proto mapper
    grpcweb/                    ← improbable-eng/grpc-web proxy
    http/                       ← REST admin handlers (analytics, monitoring, exports, appeals)
pkg/
  auth/                         ← JWT verification, HMAC crypto
  circuitbreaker/               ← Fail-fast on downstream outages
  ratelimiter/                  ← Token bucket (global + per-session + per-IP)
  cors/                         ← CORS middleware
  securityheaders/              ← HSTS, CSP, X-Frame-Options
  shutdown/                     ← Graceful shutdown orchestration (30s budget)
```

### 3.2 Design Rules

**Hexagonal architecture is mandatory.** Domain entities have zero infrastructure imports. Ports (interfaces) define what infrastructure must provide. Infrastructure implements ports. Transport maps wire formats to domain types. Dependency arrows point inward.

**Constructor injection, no globals.** Every struct receives its dependencies explicitly via `New*()` constructors. No `init()` functions, no service locators, no package-level state. This is non-negotiable; it makes the 18-step bootstrap in `main.go` readable and the shutdown sequence deterministic.

**The IngestUseCase is the single entry point for all event processing.** It performs a 3-stage fan-out: Kafka (primary) → ClickHouse (secondary) → Evidence Recorder (tertiary). If you need to add a new downstream (e.g., a notification service), add it as a fourth stage in the fan-out. Do not create a second use case.

### 3.3 The Forensic Ledger

The evidence chain writer maintains a per-session hash chain:

```
record_hash = SHA-256( sequence_num | previous_hash | fragment_id | sha256_hash | uploaded_at )
```

- First record uses `previous_hash = "GENESIS"`
- Sequence numbers are monotonic per session (no gaps)
- Chain is written to both Kafka (`argus.forensic.ledger`) and ClickHouse (`evidence_fragments`)
- Tampering detection: insertion breaks sequence continuity, deletion breaks hash linkage, reordering breaks record_hash verification

**Do not modify the hash computation formula.** It is a cryptographic contract. Changing it invalidates all existing chains.

### 3.4 Concurrency Model

```
10K+ gRPC streams ──► Worker Pool (256 goroutines) ──► IngestUseCase
                          │
                     Bounded via buffered channel (8192 jobs)
                     Submit() blocks when full (backpressure)
```

The worker pool uses `context.Background()` for ClickHouse writes so they complete even if the gRPC stream context is cancelled. The ring buffer uses `sync.RWMutex` with copy-on-read to prevent races between Push and Snapshot. The ClickHouse writer uses a two-stage buffer swap to minimize lock contention (~50ns per write).

---

## 4. Frontend Architecture (Nuxt 4)

### 4.1 Directory Contract

```
app/
  composables/                  ← Auto-imported. This is where shared logic lives.
    useColors.ts                ← Theme-aware RGBA helpers (accentBg, errorBg, etc.)
    useFormatters.ts            ← Date/time formatting (ru-RU locale, cached Intl)
    useStatusHelpers.ts         ← 25+ status/severity/event mappers
    useResilience.ts            ← Master orchestrator (wires health, tiers, queue, gRPC)
    useHealthGovernor.ts        ← Device/network health scoring (5s sampling loop)
    useTierEngine.ts            ← A/B/C state machine + tier-specific config
    useTransportMetrics.ts      ← Real-time gRPC health dashboard data
    useOfflineQueue.ts          ← IndexedDB write-ahead log (500MB default)
    useAdminAPI.ts              ← REST API client for admin operations
  components/                   ← Auto-imported. PascalCase in templates.
  pages/                        ← File-based routing.
  stores/                       ← Pinia. useAuthStore (JWT + RBAC), useDashboardStore, etc.
  lib/
    grpc/
      client.ts                 ← EventCollectorClient with smart batching
      transport.ts              ← GrpcWebTransport (JSON/HTTP, retry with backoff)
      interceptors.ts           ← Auth (JWT injection), Logging, Metrics
      chunked-upload.ts         ← Binary evidence upload (256KB chunks, pacing)
    proto/
      types.ts                  ← Hand-crafted TypeScript types (40 event types, enums)
      codec.ts                  ← JSON encode/decode
    storage/
      idb.ts                    ← IndexedDB wrapper (zero deps, 3 object stores)
  middleware/
    auth.global.ts              ← Route protection + Super Admin RBAC
```

### 4.2 The Composable Deduplication Rule

**No color helper, date formatter, or status mapper may be defined in a page or component.** All shared UI logic lives in `composables/`. Nuxt auto-imports them globally.

Before this rule, 16+ files had independent `makeAccentColor()` implementations. 12+ files had inconsistent date formatters (some `ru-RU`, some `en-US`). 8+ files had their own severity-to-color switches. All of that is now in three files:

| Composable | What it replaced | Functions |
|---|---|---|
| `useColors()` | `colorMode/isDark` + 6 RGBA helpers duplicated across 16 files | `accentBg`, `errorBg`, `successBg`, `warningBg`, `purpleBg`, `infoBg`, `isDark` |
| `useFormatters()` | Date/time formatters duplicated across 12 files | `formatDate`, `formatTime`, `formatDateTime`, `formatTimeShort`, `formatTimeAgo`, `formatVideoTimestamp` |
| `useStatusHelpers()` | Status/severity/event mappers duplicated across 8 files | `severityColor`, `severityBg`, `integrityColor`, `integrityGradient`, `eventIcon`, `sessionStatusLabel`, `appealStatusColor`, `exportStatusLabel`, + 17 more |

**When you need a new helper:** Add it to the appropriate composable. If it doesn't fit any of the three, create a new composable in `composables/`. Never define it inline in a page.

**Naming conflicts with auto-imports:** If a page needs a local function with the same name as an auto-imported one (different signature or thresholds), rename the local version with a suffix:
- `integrityGradient` (auto-imported, shared thresholds) vs `integrityGradientLocal` (page-specific thresholds)
- `severityBg` (auto-imported) vs `severityBgFn` (local, different arity)
- `formatTimeAgo` (auto-imported, takes ISO string) vs `formatTimeAgoMs` (local, takes epoch number)

### 4.3 Tier-Adaptive Resilience Engine

The frontend continuously measures device and network health, then adapts its behavior:

```
Health Governor (5s loop)
  Measures: FPS, RTT, packet loss, CPU pressure, bandwidth
  Weights:  25%, 25%, 20%, 15%, 15%
  Output:   Composite score 0-100
      │
      ▼
Tier Engine (state machine)
  Score ≥ 70 → Tier A (Optimal)
  Score ≥ 40 → Tier B (Strained)
  Score < 40 → Tier C (Critical)
      │
      │  Hysteresis: 3 consecutive checks to downgrade, 5 to upgrade
      │  No direct A↔C transitions (must pass through B)
      │  Bandwidth floor: <128kbps forces Tier C regardless of score
      ▼
gRPC Client reconfigures batch params:
  Tier A: 100 events/batch, 500ms flush   (low latency)
  Tier B: 200 events/batch, 2s flush      (reduced RPS)
  Tier C: 500 events/batch, 5s flush      (store-and-forward)
```

| Capability | Tier A | Tier B | Tier C |
|---|---|---|---|
| Video | 720p 15fps | 240p 10fps | Disabled (burst mode: 1fps every 30s for 5s) |
| AI inference | Full, 10Hz | Lite, 5Hz | Minimal, 2Hz |
| Snapshots | Disabled | 5s interval, 480p | 10s interval, 240p |
| Telemetry sampling | 100ms (10Hz) | 300ms (3Hz) | 1000ms (1Hz) |
| Chunk upload pacing | 0ms | 200ms between chunks | 500ms between chunks |

**Server-side backpressure:** The backend responds to heartbeats with `SessionDirective.telemetryMode` (NORMAL/HIGH_FREQ/LOW_FREQ). The client biases the health score by ±30 points, which can force tier transitions. The backend can also terminate a session with a reason.

### 4.4 Offline Queue (Write-Ahead Log)

Events are persisted to IndexedDB *before* gRPC upload:

```
emit(event) → IndexedDB.put(event) → gRPC.ingestBatch(events)
                                          │
                                    success? → IndexedDB.delete(ids)
                                    failure? → stays in IDB, retried later
```

Priority drain order: critical → high → normal → low (telemetry). Snapshots drain separately (5 per batch to limit bandwidth). Storage quota monitored every 30s; low-priority items evicted at 80% usage.

### 4.5 Browser AI Audio Bridge

`useSecurityShield()` owns the browser-side AI orchestration for a live exam session. When
`realtimeAudio.enabled` is true, it starts `useAudioEngine()` alongside the MediaPipe
vision engine. Raw microphone samples never leave the browser; only structured
`AudioAnalysisPayload` fields are emitted:

- `rmsDb`, `vadActive`, `vadConfidence`
- `spectralCentroidHz`, `zcr`
- `classification`, `classificationConfidence`
- `speakerCount`, `speakerMatch`, `speakerSimilarity`
- `segmentDurationMs`

`AUDIO_LEVEL_TELEMETRY` is a high-frequency telemetry event and must always use the
`EventCollectorClient` telemetry buffer, not the violation buffer. The client must rely on
`isTelemetryEvent()` from `app/lib/proto/types.ts` instead of hardcoded numeric ranges so
new AI telemetry event IDs (`104-106`) keep the same batching and offline-queue behavior
as gaze, mouse, keyboard, and focus telemetry.

Backend aggregation remains separate: sustained audio telemetry can produce a derived
`AUDIO_ANOMALY` in ClickHouse, but the frontend continues to send the original structured
telemetry asynchronously through the existing gRPC queue.

### 4.6 RBAC Model

```
super_admin (org_id='*') ⊃ org_admin ⊃ proctor ⊃ viewer
```

Route protection is in `middleware/auth.global.ts`. Super-admin-only routes: `/dashboard/infrastructure`, `/dashboard/executive`, `/api`, `/organizations`. The `useAuthStore` persists JWT + user data to localStorage; the `effectiveOrgId` computed property handles the super admin org-switching context.

---

## 5. Infrastructure Layer

### 5.1 Service Topology

Nine containers on a private bridge network (`argus-network`). Only Nginx exposes ports.

| Service | Role | Data Retention |
|---|---|---|
| **Nginx** | TLS termination, rate limiting (100 RPS general / 5 RPS auth), reverse proxy | — |
| **Backend** (Go) | gRPC ingestion + REST admin API | — |
| **Frontend** (Nuxt) | SPA serving | — |
| **PostgreSQL 16** | Transactional: orgs, users, API keys, audit, reviews, exports, consent, appeals | Indefinite |
| **ClickHouse** | Analytical: billions of events, 5 materialized views, evidence metadata | 90 days (raw), 180 days (hourly stats), 365 days (org stats), 730 days (evidence) |
| **Kafka** (KRaft) | Event streaming backbone, forensic ledger source of truth | 7 days |
| **MinIO** | S3 evidence storage with Object Lock GOVERNANCE | 365 days (WORM) |
| **Redis 7** | LiveKit session state | Ephemeral |
| **LiveKit** | WebRTC video | — |

### 5.2 Database Schemas

**PostgreSQL** (transactional, ACID):
- `organizations` — tenant boundary, plan limits (max_sessions, max_events_rps, retention_days)
- `users` — RBAC (super_admin, org_admin, proctor, viewer), phone-based login, bcrypt
- `api_keys` — M2M auth for external integrators (e.g., EDUSER), permission arrays, RPS limits
- `audit_log` — immutable compliance log (append-only, no UPDATE/DELETE)
- `review_decisions` — human review of violations (confirmed/dismissed/escalated)
- `export_jobs` — async bulk archive generation (pending → processing → completed/failed)
- `consent_records` — GDPR consent proof (immutable, append-only)
- `appeals` — student appeal state machine (submitted → under_review → upheld/overturned/withdrawn)

**ClickHouse** (analytical, columnar):
- `proctoring_events` — raw events, partitioned monthly, 90-day TTL, 9 data-skipping indices
- `session_event_counts` — materialized view, SummingMergeTree, per-session violation breakdown
- `hourly_event_stats` — materialized view, hourly aggregation for trend dashboards
- `critical_events_recent` — materialized view, 7-day window of critical-only events
- `student_session_summary` — materialized view, per-student risk scoring
- `global_org_stats` — materialized view, daily cross-org aggregation for super admin
- `evidence_fragments` — evidence metadata with forensic ledger columns (hash chain)

**Every row in every table carries `org_id`.** This is the tenant isolation boundary. Super admin queries use `org_id='*'` against the `global_org_stats` materialized view, never scanning raw billions-row tables.

### 5.3 Deployment

CI/CD is triggered downstream: when backend or frontend pushes a new image to the registry, it triggers the infra pipeline which runs `docker compose pull` + `up -d --no-deps --wait` per service. Zero-downtime via health check polling (15s start period, 3 retries). Rollback: re-run with previous image tag.

Migrations are idempotent SQL files in `migrations/postgres/` and `migrations/clickhouse/`. Run them in order. All use `IF NOT EXISTS` guards.

---

## 6. Coding Standards

### The No-Duplication Rule

If a function exists in a shared composable or package, you use it. You do not rewrite it. You do not "simplify" it in your component. If the shared version doesn't fit your needs, you extend the shared version or create a clearly-named local variant with a suffix explaining why (e.g., `integrityGradientLocal` — uses different score thresholds for a specific page context).

### Naming

**All API interactions use camelCase.** Proto field names on the wire are `snake_case` (gRPC convention) but are mapped to `camelCase` at the transport boundary — in the Go mapper (`internal/transport/grpc/mapper.go`) and the TypeScript proto types (`app/lib/proto/types.ts`). Application code, domain entities, Vue templates, and composable return values are always `camelCase`. No exceptions.

**Go packages:** lowercase, single-word where possible. Types are PascalCase. Unexported helpers are camelCase. Interface names describe capability (e.g., `EventWriter`, not `IEventWriter`).

**TypeScript/Vue:** composables are `use*` prefixed. Components are PascalCase. Stores are `use*Store`. Pages are kebab-case filenames. All auto-imported by Nuxt.

### Error Handling

**Go:** Errors are returned, not panicked. The only panic recovery is the outermost gRPC interceptor. Infrastructure failures in secondary/tertiary stages (ClickHouse, evidence recorder) are logged and tolerated. Kafka failures in the primary stage cause event rejection.

**TypeScript:** gRPC transport errors are classified by `TransportErrorCode` (NETWORK, SERVER, TIMEOUT, RATE_LIMITED, etc.). Retryable errors get exponential backoff with jitter. Non-retryable errors (4xx, INVALID_ARGUMENT) fail immediately. The offline queue catches everything that falls through.

### Adding a New Feature

1. **New event type:** Add to `event_collector.proto` → regenerate Go code → add to `types.ts` → add handler in `useStatusHelpers.ts` (icon, label, color) → done. The pipeline ingests it automatically.

2. **New admin endpoint:** Add handler in `internal/transport/http/` → register route in `main.go` → add corresponding method in `useAdminAPI.ts` → create page in `app/pages/`.

3. **New ClickHouse materialized view:** Add migration file in `migrations/clickhouse/` → run migration → backend queries it via `writer.Conn()` → expose via admin handler.

4. **New composable:** Create `app/composables/use*.ts` → Nuxt auto-imports it → use directly in any page or component.

### What Not To Do

- Do not bypass Kafka and write directly to ClickHouse. Kafka is the source of truth.
- Do not store secrets in `.env` files in the repo. Use `.env.example` as documentation; actual `.env` lives on the deployment host only.
- Do not add `import` statements for composables or components in the frontend. Nuxt auto-imports them. If your IDE complains, configure it for Nuxt, don't add manual imports.
- Do not create circular dependencies between composables. The dependency graph is: `useResilience` → `useHealthGovernor` → `useTierEngine` → `useOfflineQueue` → `useTransportMetrics`. Leaf composables (`useColors`, `useFormatters`, `useStatusHelpers`) depend on nothing.
- Do not commit `.env`, `.pem`, `.key`, `.crt`, `.jks`, or `credentials.json` files. The `.gitignore` blocks them, but the rule is: if it's a secret, it never enters version control.

---

## 7. Local Development

```bash
# Start infrastructure (from argus-infra/docker/)
cp .env.example .env           # Fill in local dev values
docker compose up -d postgres clickhouse kafka redis minio livekit
docker compose run --rm minio-init

# Run migrations
for f in ../migrations/postgres/*.sql; do
  docker compose exec postgres psql -U argus -d argus -f /dev/stdin < "$f"
done
cat ../migrations/clickhouse/init.sql ../migrations/clickhouse/002_forensic_ledger.sql | \
  docker compose exec -T clickhouse clickhouse-client --multiquery

# Start backend (from argus-backend/)
cp .env.example .env
go run cmd/server/main.go -config deployments/config.yaml

# Start frontend (from argus-frontend/)
cp .env.example .env
npm install && npm run dev
```

Frontend runs at `localhost:3000`. Backend gRPC at `:50051`, REST at `:8080`.

---

## 8. Repository Links

| Repository | URL | Stack |
|---|---|---|
| Backend | https://gitlab.com/argus_ai_group/argus-backend | Go 1.24 · gRPC · Kafka · ClickHouse · MinIO · PostgreSQL |
| Frontend | https://gitlab.com/argus_ai_group/argus-frontend | Nuxt 4 · Vue 3 · TypeScript · Pinia · gRPC-Web · IndexedDB |
| Infrastructure | https://gitlab.com/argus_ai_group/argus-infra | Docker · Nginx · PostgreSQL · ClickHouse · Kafka · MinIO · Redis · LiveKit |
