# ARCHITECTURE.md — argus-backend

> Deep technical reference for the Argus AI event-collector service.
> Start with `README.md` for getting started. This document covers the *why*
> behind every architectural decision.

---

## Table of Contents

1. [Architectural Philosophy](#architectural-philosophy)
2. [System Context](#system-context)
3. [Clean Architecture Layers](#clean-architecture-layers)
4. [Request Lifecycle: Event Ingestion](#request-lifecycle-event-ingestion)
5. [Request Lifecycle: Admin REST API](#request-lifecycle-admin-rest-api)
6. [Data Flow Architecture](#data-flow-architecture)
7. [Forensic Ledger Deep Dive](#forensic-ledger-deep-dive)
8. [Resilience Tier Architecture](#resilience-tier-architecture)
9. [Concurrency Model](#concurrency-model)
10. [Storage Architecture](#storage-architecture)
11. [Security Architecture](#security-architecture)
12. [Observability](#observability)
13. [Deployment Architecture](#deployment-architecture)
14. [Decision Log (ADRs)](#decision-log-adrs)
    - ADR-001: gRPC + gRPC-Web
    - ADR-002: ClickHouse over TimescaleDB
    - ADR-003: In-memory ring buffer
    - ADR-004: Single binary → API/Worker split (superseded by ADR-006)
    - ADR-005: HMAC-SHA256 → RSA-256 JWT (superseded by ADR-006)
    - ADR-006: Microservice Split & Distributed Inference

---

## Architectural Philosophy

The event-collector is designed around three non-negotiable constraints:

1. **Zero data loss** — Every event that enters the system must be durably persisted,
   even during partial infrastructure failures. Kafka is the write-ahead log; ClickHouse
   is the read-optimized replica. Losing an event means losing legal evidence.

2. **Cryptographic auditability** — It must be mathematically impossible for an attacker
   (including a compromised operator) to modify, delete, or reorder evidence without detection.
   This is the Forensic Ledger requirement.

3. **Horizontal scalability** — The service must handle 10,000 concurrent proctoring sessions
   with sub-5ms event acknowledgement latency. This drives the gRPC + worker pool design.

These constraints are in tension with each other. The architecture resolves them by separating
concerns: gRPC handles high-throughput ingestion (latency-sensitive), while REST handles
low-frequency admin operations (correctness-sensitive).

---

## System Context

```
┌──────────────────────────────────────────────────────────────┐
│                     Internet / CDN                           │
└───────────────────────────┬──────────────────────────────────┘
                            │ HTTPS
                   ┌────────▼────────┐
                   │      Nginx      │   TLS termination
                   │  (argus-infra)  │   Rate limiting (5 RPS auth)
                   │                 │   CSP / HSTS / Security headers
                   └──┬──────────┬───┘
           /api/ │    │          │  /argus.proctoring.*
                 │    │          │  (gRPC-Web)
    ┌────────────▼─┐  │     ┌────▼──────────────────┐
    │  Nuxt 3 SPA  │  │     │    argus-backend       │  RSA Private Key (sign)
    │ (argus-front)│  │     │    :8080 (HTTP)        │  RSA Public Key (verify)
    └──────────────┘  │     │    :50051 (gRPC)       │
                      │     └──┬─────────┬─────┬─────┘
             REST ────┘        │         │     │
                               │  asynq  │     │
                    ┌──────────┘  tasks   │     │
                    │         ┌───┘       │     │
               ┌────▼────┐  ┌▼───────┐   │     │
               │  Kafka  │  │ Redis  │   │     │
               │(3-broker│  │ (4GB)  │   │     │
               │ cluster)│  │ AOF+RDB│   │     │
               └─────────┘  └───┬────┘   │     │
                                │        │     │
                    ┌───────────▼──┐     │     │
                    │ argus-worker │     │     │  RSA Public Key (verify only)
                    │ (background) │     │     │
                    └──────────────┘     │     │
                                   ┌────▼──────────┐
                                   │  ClickHouse   │
                                   │ (3-node HA    │
                                   │  + 3 Keepers) │
                                   └───────────────┘
                               │
                    ┌──────────▼──────────┐
                    │     PostgreSQL      │
                    │  (transactional +   │
                    │   outbox_jobs)      │
                    └─────────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │       MinIO         │
                    │  (evidence S3)      │
                    └─────────────────────┘
```

---

## Clean Architecture Layers

The codebase implements **Clean Architecture** (Robert C. Martin, 2017).
Layers are strictly enforced by Go's package import rules — any violation
causes a compilation error.

```
╔══════════════════════════════════════════════════════╗
║  Layer 4: Transport (internal/transport/)            ║
║  gRPC server, gRPC-Web proxy, HTTP REST handlers     ║
║  — Protocol adapters. Zero business logic.           ║
╠══════════════════════════════════════════════════════╣
║  Layer 3: Infrastructure (internal/infrastructure/)  ║
║  Kafka, ClickHouse, PostgreSQL, MinIO, Config        ║
║  — Implements application/port interfaces.           ║
╠══════════════════════════════════════════════════════╣
║  Layer 2: Application (internal/application/)        ║
║  UseCase orchestration + Port interfaces (contracts) ║
╠══════════════════════════════════════════════════════╣
║  Layer 1: Domain (internal/domain/)                  ║
║  Entities, Value Objects — pure Go, zero imports     ║
╚══════════════════════════════════════════════════════╝

pkg/* — Shared utilities, imported by any layer.
         Zero imports from internal/.
```

**Import direction rule:** Arrows only point inward (toward Domain).

```
Transport  →  Application  →  Domain
Infrastructure  →  Application  →  Domain
pkg  (no direction — standalone)
```

---

## Request Lifecycle: Event Ingestion

A proctoring event (tab switch, gaze deviation, face detection) flows through the system
in approximately **2–8ms end-to-end** from browser SDK to Kafka ACK.

```
Browser SDK
    │  gRPC-Web stream (Protobuf binary, Content-Type: application/grpc-web)
    │  POST /argus.proctoring.EventCollectorService/StreamEvents
    ▼
Nginx  ──HTTP/1.1 upgrade──►  argus-backend :8080
    │
    ▼
grpcweb.Handler (internal/transport/grpcweb/proxy.go)
    │  Detects gRPC-Web Content-Type, translates to native gRPC frames
    ▼
gRPC Interceptor Chain (internal/transport/grpc/interceptors.go)
    │  1. Recovery      — panic → graceful error (prevents goroutine leak)
    │  2. RateLimit     — token bucket (global 50K RPS + per-session 100 RPS)
    │  3. Auth          — JWT verification (RS256 asymmetric) + session cache lookup
    │  4. Logging       — structured zap entry with request metadata
    ▼
grpcTransport.Server.StreamEvents() (internal/transport/grpc/server.go)
    │  Validates stream context, extracts session metadata
    ▼
WorkerPool.Submit(job) (internal/transport/grpc/worker_pool.go)
    │  Non-blocking enqueue (512 goroutines, 16,384-deep queue)
    │  Backpressure: returns RESOURCE_EXHAUSTED if queue full
    ▼
IngestUseCase.Ingest(event) (internal/application/usecase/ingest.go)
    │  Maps Protobuf → domain entity
    │  Triggers evidence recording if event is a violation
    ├──►  kafka.Producer.Publish(event)  ─────────────────► Kafka topic: argus.proctoring.events
    │       (async, idempotent, zstd compressed)
    └──►  clickhouse.Writer.Write(event) ─────────────────► ClickHouse: proctoring_events table
              (batched 5,000 / flush 3s)
```

**Evidence capture path** (only triggered on critical/warning events):

```
IngestUseCase
    │  Violation event detected (severity = critical or warning)
    ▼
recorder.Recorder.OnViolation(event)
    │  Pulls pre-capture window from ring buffer (sliding window, configurable seconds)
    │  Triggers post-capture timer
    ▼
minio.Store.Upload(fragment)  ─────────────────────────► MinIO: argus-evidence bucket
    │  Content-Addressed storage (SHA-256 as object key prefix)
    │  Object Lock: GOVERNANCE mode (365-day WORM retention)
    ▼
evidence.ChainWriter.Write(fragment)
    ├──► Kafka: argus.evidence.chain (full metadata + chain fields)
    ├──► Kafka: argus.forensic.ledger (minimal chain proof)
    └──► ClickHouse: evidence_fragments (queryable, with sequence_num + hashes)
```

---

## Request Lifecycle: Admin REST API

An admin API request (e.g., `GET /api/v1/analytics/overview`) follows a simpler path:

```
Nuxt 3 SPA  (fetch from useAdminAPI.ts)
    │  HTTP GET /api/v1/analytics/overview
    │  Authorization: Bearer <jwt>
    ▼
Nginx  ──proxy_pass──►  argus-backend :8080
    ▼
securityheaders.Middleware  — adds HSTS, X-Frame-Options, etc.
    ▼
cors.Middleware  — validates Origin, adds Access-Control-* headers
    ▼
ratelimiter.HTTPRateLimiter.Middleware  — token bucket (5K global RPS + per-IP)
    ▼
grpcweb.Handler  — HTTP request, not gRPC-Web → falls through to httpMux
    ▼
http.ServeMux  — routes "GET /api/v1/analytics/overview" → analyticsHandler
    ▼
AnalyticsHandler.handleGetOverview()
    │  1. Extract JWT from Authorization header
    │  2. Validate + decode JWT → claims (sub, org_id, role)
    │  3. RBAC: proctor role or higher required
    │  4. Call postgres.Repository + clickhouse.Writer for data
    │  5. Map domain entities → JSON response DTOs
    │  6. writeJSON(w, 200, resp)
    ▼
HTTP 200 JSON response
```

---

## Data Flow Architecture

### Write Path (Hot Path — event ingestion)

```
Event  →  [gRPC Stream]  →  WorkerPool  →  IngestUseCase
                                               ├──► Kafka (source of truth, async)
                                               └──► ClickHouse (analytics, batched)
```

**Kafka** receives every event first. It is the source of truth and can replay events
to ClickHouse if the ClickHouse writer falls behind or fails.

**ClickHouse** receives events in batches (5,000 events or 3 seconds, whichever comes first).
Materialized views pre-aggregate data for sub-millisecond dashboard queries.

### Read Path (Admin API)

```
Dashboard  →  [REST]  →  Handler  →  ClickHouse (analytics queries)
                                   →  PostgreSQL (org/user/session data)
```

Dashboard queries hit ClickHouse materialized views, never the raw `proctoring_events` table.
This is the key to achieving sub-10ms response times for dashboards with billions of events.

### Evidence Path (Forensic Chain)

```
Violation Event
    │
    ▼
Ring Buffer (pre-capture window)  →  MinIO (S3 WORM)
                                          │
                                          ▼
                               ChainWriter (hash-chaining)
                                     ├──► Kafka (evidence.chain)
                                     ├──► Kafka (forensic.ledger)
                                     └──► ClickHouse (evidence_fragments)
```

---

## Forensic Ledger Deep Dive

### Hash Chain Formula

For a session with `n` evidence fragments, the chain is computed as:

```
H₀ = "GENESIS"
H₁ = SHA-256( encode(1)  ‖ H₀  ‖ fragment_id₁ ‖ sha256(S3_object₁) ‖ encode(uploaded_at₁) )
H₂ = SHA-256( encode(2)  ‖ H₁  ‖ fragment_id₂ ‖ sha256(S3_object₂) ‖ encode(uploaded_at₂) )
...
Hₙ = SHA-256( encode(n)  ‖ Hₙ₋₁ ‖ fragment_idₙ ‖ sha256(S3_objectₙ) ‖ encode(uploaded_atₙ) )
```

Where `encode(x)` is the UTF-8 byte representation of `x` serialized as a string,
and `‖` denotes concatenation. The chain is computed in `internal/infrastructure/evidence/chain_writer.go`.

### Why Dual Kafka Topics?

| Topic | Content | Consumer |
|-------|---------|----------|
| `argus.evidence.chain` | Full `EvidenceFragment` struct + chain fields | Archive service, ClickHouse sink |
| `argus.forensic.ledger` | Minimal: `(session_id, seq, prev_hash, record_hash)` | Independent auditor, legal discovery |

The minimal forensic ledger topic is intentionally lean — an external auditor can verify chain
integrity without needing access to the full evidence content. This separation supports
privacy-preserving audits.

### Tamper Detection Guarantees

| Threat | Detectable? | How |
|--------|:-----------:|-----|
| Fragment file modified in S3 | ✅ | `SHA-256(S3) ≠ stored sha256_hash` |
| Fragment deleted from S3 | ✅ | Missing object; chain link broken |
| Fragment record deleted from ClickHouse | ✅ | Gap in `sequence_num` |
| Fragment records reordered in ClickHouse | ✅ | `previous_hash` mismatch |
| New fragment inserted mid-chain | ✅ | All subsequent `sequence_num` would shift; `previous_hash` chain breaks |
| ClickHouse AND Kafka both compromised | ❌ | Defense-in-depth required (HSM, append-only Kafka ACLs) |

The last threat is out of scope for application-layer cryptography. Mitigations are:
Kafka immutable topic ACLs, MinIO Object Lock, and out-of-band audit copies.

---

## Resilience Tier Architecture

The Resilience Engine is not a single component — it is the composition of four layers
that each handle a different failure mode.

```
┌─────────────────────────────────────────────────────────────────┐
│                  Resilience Layers (outermost → innermost)      │
│                                                                 │
│  Layer 4: Nginx rate limiting (5 RPS auth, 100 RPS API)        │
│  Layer 3: HTTP rate limiter (pkg/ratelimiter) — per-IP token bucket│
│  Layer 2: gRPC interceptor (pkg/ratelimiter) — per-session bucket │
│  Layer 1: Circuit breaker (pkg/circuitbreaker) — per downstream  │
│           ├── Kafka circuit breaker                             │
│           └── ClickHouse circuit breaker                        │
└─────────────────────────────────────────────────────────────────┘
```

### Circuit Breaker States

```
CLOSED (normal operation)
  │
  │ 5 consecutive failures within 10s window
  ▼
OPEN (reject all requests immediately → fail-fast)
  │
  │ 30s cooldown timeout expires
  ▼
HALF-OPEN (allow 1 probe request through)
  │
  ├──(probe succeeds)──► CLOSED
  └──(probe fails)────► OPEN (reset timer)
```

When the Kafka circuit is OPEN, events are dropped (counted in Prometheus `events_dropped_total`).
This is the correct behavior: it prevents memory exhaustion from an unbounded queue and allows
the system to remain responsive for health checks and heartbeats.

### Worker Pool Backpressure

```
gRPC Stream Handler
    │
    ▼  Submit(job)
WorkerPool.queue  ────────────────► 512 goroutines process in parallel
    │
    └── (queue full, len = 16,384)
           │
           ▼  return RESOURCE_EXHAUSTED
    gRPC status codes.ResourceExhausted
           │
           ▼  client retries with exponential backoff
    Browser SDK (useResilience.ts + Tier C offline queue)
```

The browser SDK (in `argus-frontend`) handles `RESOURCE_EXHAUSTED` by switching to the
Tier-C offline mode: events are persisted to IndexedDB and retried when backpressure releases.
This is the client-side component of the Resilience Engine.

---

## Concurrency Model

### gRPC Server

- Each gRPC **streaming connection** is handled by a single goroutine (managed by the gRPC framework)
- Events from the stream are **submitted** to the worker pool (non-blocking)
- The worker pool goroutines invoke `IngestUseCase.Ingest()` (blocking, but bounded)

```
10,000 sessions × 1 goroutine/session (gRPC stream reader) = 10,000 goroutines
                 ↓ (non-blocking Submit)
512 worker goroutines × IngestUseCase.Ingest() = 512 goroutines
```

Total goroutines in steady state: ~10,512 + infrastructure goroutines ≈ 11,000.
Each goroutine stack starts at 8KB in Go → ~88MB for stream readers + ~4MB for workers.

### ClickHouse Batching

The ClickHouse writer (`internal/infrastructure/clickhouse/writer.go`) collects events
in an in-memory slice. A **background goroutine** flushes the batch when either:

- `len(batch) >= 5,000` (high-throughput mode)
- `time since last flush >= 3s` (low-throughput / empty queue mode)

This amortizes the per-insert overhead of ClickHouse over many rows, achieving
~100,000 events/second write throughput with the default configuration.

### Kafka Producer

The Kafka producer uses the **async/fire-and-forget** model with `required_acks = -1` (all replicas).
This means:

- Kafka acknowledges only after all in-sync replicas have written the message (durability)
- The application does not block waiting for individual ACKs (throughput)
- Failed messages trigger the circuit breaker after `retry_max = 3` retries

---

## Storage Architecture

### PostgreSQL — Transactional Layer

**What lives here:** Everything that requires ACID transactions — organization configs, user
credentials, API keys, audit logs, review decisions, consent records, appeals, export job queue.

**Schema design principles:**
- UUID primary keys (globally unique across tenants)
- `org_id` VARCHAR as the tenant isolation key (not FK to UUID — intentional for performance)
- Soft deletes via `deleted_at TIMESTAMPTZ` — historical records never physically removed
- Immutable tables: `audit_log`, `consent_records` — no UPDATE operations allowed

**Connection pool:** max 25 open connections, max 5m lifetime (prevents stale connections
behind load balancers).

### ClickHouse — Analytical Layer

**What lives here:** All proctoring events (billions of rows), evidence fragment metadata,
and pre-aggregated materialized views for dashboard queries.

**Schema design principles:**
- `ORDER BY (org_id, exam_id, session_id, server_timestamp)` — compound primary key optimized for
  the most common query patterns (filter by org, then exam, then session, then time range)
- Monthly partitioning (`toYYYYMM(server_timestamp)`) — enables partition pruning for time-range queries
- `LowCardinality(String)` for `event_type`, `severity`, `source` — 4–8x compression
- Data skipping indices (bloom filter on UUIDs, set on enums) — point lookups on sorted tables
- Five materialized views for zero-latency dashboard reads

**Materialized views:**

| View Target | Source | Aggregation | Use Case |
|-------------|--------|-------------|---------|
| `session_event_counts` | `proctoring_events` | Per-session type/severity counts | Session risk score |
| `hourly_event_stats` | `proctoring_events` | Hourly org/exam/type/severity | Executive dashboard |
| `critical_events_recent` | `proctoring_events` | Filter: critical severity, 7-day TTL | Live monitoring feed |
| `student_session_summary` | `proctoring_events` | Per-student per-session | Student risk profile |
| `global_org_stats` | `proctoring_events` | Daily org-level totals | Super admin dashboard |

**Data retention:**
- `proctoring_events`: 90 days (configurable)
- `evidence_fragments`: 730 days / 2 years (legal compliance)
- `critical_events_recent`: 7 days (operational)

### Kafka — Streaming Layer

**What lives here:** Raw event stream (source of truth), forensic evidence chain.

**Topics:**

| Topic | Retention | Purpose |
|-------|-----------|---------|
| `argus.proctoring.events` | 7 days | ClickHouse sink replay buffer |
| `argus.evidence.chain` | Configurable | Full evidence chain with metadata |
| `argus.forensic.ledger` | Permanent | Minimal hash proof for auditors |

**Cluster configuration (3-broker KRaft):**
- 3 brokers with `replication.factor = 3` and `min.insync.replicas = 2`
- KRaft mode (no ZooKeeper dependency) — combined controller + broker nodes
- Survives 1 broker failure with zero data loss

**Producer configuration:**
- `idempotent = true` — exactly-once delivery per partition
- `required_acks = -1` — all in-sync replicas must ACK
- `compression = zstd` — ~70% size reduction for JSON payloads
- `max_message_bytes = 1 MiB` — max single event payload

### MinIO — Object Storage Layer

**What lives here:** Binary evidence files (video clips, JPEG screenshots, audio fragments).

**Design:**
- Content-addressed: `sha256hash/fragment_id` as object key prefix
- Object Lock (WORM): `GOVERNANCE` retention mode with 365-day policy
- Bucket versioning enabled (additional protection against accidental deletion)
- Separate `argus-exports` bucket for bulk export archives (30-day TTL)

---

## Security Architecture

### Authentication Flow

```
Client  ──POST /api/v1/auth/login──►  AdminHandler.handleLogin()
                                          │
                                          ▼  bcrypt.CompareHashAndPassword()
                                      PostgreSQL (users table, password_hash)
                                          │
                                          ▼  auth.GenerateTokenRS256(claims, privateKey)
                                      RSA-SHA256 signed JWT (RS256)
                                          │
                                          ▼  HTTP 200 {"token": "..."}
Client  ──Authorization: Bearer <jwt>──►  Any protected endpoint
                                          │
                                          ▼  auth.VerifyToken(jwt)
                                      RS256 verification (public key only — no DB call)
                                      Algorithm confusion attack prevention
                                      Claims: {sub, org_id, role, exp}
```

**Key Separation (ADR-006):**
- **API Server:** Holds RSA private key (sign) + public key (verify)
- **Workers:** Hold ONLY RSA public key (verify). Cannot forge tokens.
- **Migration:** HMAC-SHA256 (HS256) retained as legacy fallback (set `algorithm: HS256`)

### Defense in Depth

| Layer | Control | Implementation |
|-------|---------|----------------|
| Network | TLS 1.3 only | Nginx (`ssl_protocols TLSv1.2 TLSv1.3`) |
| Network | HSTS preload | Nginx (`Strict-Transport-Security: max-age=63072000`) |
| Network | Rate limiting | Nginx (auth: 5 RPS) + backend (global + per-IP) |
| Transport | CORS | `pkg/cors` (explicit allowlist, no wildcards in production) |
| Application | JWT auth | `pkg/auth` (RS256 asymmetric, 24h expiry, algorithm confusion prevention) |
| Application | Key separation | RSA-4096: private key (API server only), public key (workers) |
| Application | RBAC | Per-handler role checks (`super_admin` > `org_admin` > `proctor` > `viewer`) |
| Application | Security headers | `pkg/securityheaders` (CSP, X-Frame-Options, Referrer-Policy) |
| Data | Tenant isolation | `org_id` in every ClickHouse event and JWT claim |
| Data | Password hashing | `bcrypt` (cost factor 12) |
| Data | Outbox pattern | PostgreSQL `outbox_jobs` ensures task durability before Redis dispatch |
| Evidence | Tamper detection | SHA-256 hash chaining (Forensic Ledger) |
| Evidence | WORM protection | MinIO Object Lock (GOVERNANCE mode, 365-day retention) |
| Evidence | Crypto-erasure | AES-256-GCM envelope encryption, per-student KEK (GDPR Art. 17) |

---

## Observability

### Structured Logging (`go.uber.org/zap`)

All logs are emitted as structured JSON in production:

```json
{
  "level": "info",
  "ts": "2026-01-15T10:23:45.123Z",
  "caller": "http/archive_handler.go:145",
  "msg": "session review submitted",
  "session_id": "sess_abc123",
  "reviewer_id": "usr_def456",
  "decision": "confirmed",
  "org_id": "org-kaznu",
  "duration_ms": 3.2
}
```

Log levels: `debug` (dev only) → `info` (operational) → `warn` (degraded) → `error` (action required)

### Metrics (`/metrics` — Prometheus format)

Key metrics exported by `internal/infrastructure/metrics/`:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `argus_events_ingested_total` | Counter | `org_id`, `severity` | Total events ingested |
| `argus_events_dropped_total` | Counter | `reason` | Events dropped (circuit open, queue full) |
| `argus_kafka_publish_latency_ms` | Histogram | — | Kafka publish round-trip |
| `argus_clickhouse_batch_size` | Histogram | — | ClickHouse batch sizes at flush |
| `argus_worker_pool_queue_depth` | Gauge | — | Current worker pool queue length |
| `argus_evidence_fragments_total` | Counter | `session_id` | Evidence fragments stored |
| `argus_http_request_duration_ms` | Histogram | `method`, `path`, `status` | REST API latency |

### Health Endpoints

```
GET /healthz   → 200 {"status":"ok"}              — liveness: process is alive
GET /readyz    → 200 / 503                         — readiness: Kafka + ClickHouse reachable
```

Kubernetes should use `/readyz` for `readinessProbe` and `/healthz` for `livenessProbe`.

---

## Deployment Architecture

### Binary Architecture (API Server + Worker Split)

The system ships as two binaries (see ADR-006):

- **`cmd/server`** — API server: gRPC event ingestion, REST admin API, evidence recording.
  Holds the RSA private key for token signing.
- **`cmd/worker`** — Background worker: video export, forensic PDF, AI analysis, alerts.
  Holds only the RSA public key for token verification. Cannot forge tokens.

Both binaries share the same Clean Architecture codebase. The split isolates CPU-heavy
background jobs (video encoding, AI inference) from latency-sensitive event ingestion.

### Container Image

The `Dockerfile` uses a multi-stage build:

```dockerfile
Stage 1 (builder): golang:1.24-alpine
    CGO_ENABLED=0 go build -ldflags "-s -w" → statically linked binary (~25MB)

Stage 2 (runtime): alpine:3.20
    Non-root user (argus:argus, uid=1001)
    COPY binary from builder
    EXPOSE 8080 50051
```

The final image is ~30MB total. No shell, no package manager, minimal attack surface.

### Configuration Hierarchy

```
deployments/config.yaml          ← Base configuration (committed to git, no secrets)
    ↑ overridden by
Environment variables             ← Secrets + environment-specific overrides
    ↑ overridden by
CLI flags (-config path)          ← Config file path only (no secret flags)
```

### Rolling Deploy (Zero Downtime)

From `argus-infra/ci/gitlab-ci.yml`:

```bash
docker compose pull backend                    # Pull new image (old container still running)
docker compose up -d --no-deps --wait backend  # Replace container (--wait: wait for healthcheck)
```

The `--wait` flag polls `/healthz` until the new container passes (15s start period).
Old connections drain during the `ShutdownTimeout` (30s). Total downtime: 0.

---

## Decision Log (ADRs)

### ADR-001: gRPC + gRPC-Web over REST for event ingestion

**Decision:** Use gRPC streaming for event ingestion from browser clients via gRPC-Web proxy.

**Rationale:** Browser clients send 5–100 events per second per session. REST (HTTP/1.1) would
require one HTTP request per event, adding ~50ms per event just for TCP handshake overhead.
gRPC streaming multiplexes events over a single long-lived HTTP/2 connection.

**Trade-off:** gRPC-Web requires a proxy layer (the `grpcweb.Handler`). This adds ~200µs overhead
per request but eliminates the per-event HTTP overhead entirely.

---

### ADR-002: ClickHouse over TimescaleDB for event storage

**Decision:** Use ClickHouse for analytical event storage instead of TimescaleDB (PostgreSQL extension).

**Rationale:** Benchmarks showed ClickHouse achieves 5–10x higher write throughput than
TimescaleDB for our schema (wide rows, mostly-append, time-series with high cardinality).
ClickHouse's `LowCardinality` column type and columnar storage result in 70–90% storage
reduction compared to PostgreSQL for the same data.

**Trade-off:** ClickHouse lacks ACID transactions and JOIN support. This is acceptable because
analytics queries never require JOINs (all relevant data is denormalized into each event row)
and writes are fire-and-forget (durability is Kafka's job, not ClickHouse's).

---

### ADR-003: In-memory ring buffer over Redis for evidence capture

**Decision:** Use an in-memory ring buffer (`internal/infrastructure/ringbuffer/`) for the
pre-capture sliding video window instead of Redis.

**Rationale:** Evidence capture requires microsecond access to recent video frames. Redis
introduces ~1ms network RTT. In-memory access is ~100ns. For 10,000 concurrent sessions,
a Redis cluster would require significant overhead and become a potential bottleneck.

**Trade-off:** Ring buffer state is lost if the process crashes. Mitigation: the post-capture
window (configurable, default 10s) is always captured to MinIO synchronously, so the only
data loss risk is the pre-capture window during a crash — an acceptable trade-off for the
performance gain.

---

### ADR-004: Single binary → API/Worker split (superseded by ADR-006)

**Original Decision (v1):** Ship all functionality as a single Go binary.

**Superseded by ADR-006 (v2):** The system has been split into two binaries:
- `cmd/server` — API server (gRPC ingestion + REST admin API)
- `cmd/worker` — Background worker (video export, forensic PDF, AI analysis)

See ADR-006 for the rationale behind this change.

---

### ADR-005: HMAC-SHA256 JWT → RSA-256 JWT (superseded by ADR-006)

**Original Decision (v1):** Use HMAC-SHA256 (symmetric) for JWT signing.

**Superseded (v2):** RSA-256 (asymmetric) is now the default algorithm. The API/Worker
split (ADR-006) introduced a second binary that verifies tokens. With HMAC-SHA256, a
compromised worker could forge admin tokens — this is unacceptable for a proctoring
system handling examination integrity. RS256 ensures workers can verify but CANNOT sign.

HMAC-SHA256 is retained as a legacy fallback (set `algorithm: HS256` in config) for
backward compatibility during migration.

See ADR-006 for the complete security rationale.

---

### ADR-006: Microservice Split & Distributed Inference

**Status:** Accepted
**Date:** 2026-02-22
**Supersedes:** ADR-004 (single binary), ADR-005 (HMAC-SHA256 JWT)

#### Context

The Argus system was initially designed as a single Go binary (ADR-004) with HMAC-SHA256
JWT authentication (ADR-005). Two critical production requirements forced a re-architecture:

1. **Resource starvation:** CPU-heavy background jobs (video export, AI inference with
   ONNX models) running in-process competed with latency-sensitive gRPC event ingestion.
   A single long-running forensic PDF generation could spike p99 latency from 5ms to 200ms+.

2. **Security boundary violation:** HMAC-SHA256 uses a shared secret for both signing and
   verification. In a single binary this is acceptable. With a separate worker binary, the
   worker possesses the signing key — a compromised worker could forge admin-level JWTs,
   granting unrestricted access to modify exam results, delete evidence, or impersonate proctors.

#### Decision

Split the system into two binaries with asymmetric JWT authentication:

| Binary | Role | JWT Key | Background Jobs |
|--------|------|---------|-----------------|
| `cmd/server` | API server: gRPC ingestion, REST admin, evidence recording | RSA private key (sign + verify) | None |
| `cmd/worker` | Background processor: video export, forensic PDF, AI analysis, alerts | RSA public key (verify only) | All asynq queues |

Job dispatch uses the **Transactional Outbox Pattern**:
1. Business logic writes the job to PostgreSQL `outbox_jobs` table within the same DB transaction.
2. A background relay goroutine polls `outbox_jobs` and dispatches to Redis (asynq).
3. If Redis is unavailable, jobs remain safely in PostgreSQL with retry logic.
4. This eliminates the dual-write problem (business data + Redis) and guarantees zero task loss.

#### Infrastructure High Availability

| Component | Configuration | Fault Tolerance |
|-----------|--------------|-----------------|
| Kafka | 3-broker KRaft cluster, replication-factor 3, min.insync.replicas 2 | Survives 1 broker failure with zero data loss |
| ClickHouse | 3-node replicated cluster + 3 ClickHouse Keepers | Survives 1 node failure, automatic failover |
| Redis | 4GB maxmemory, AOF (appendfsync everysec) + RDB snapshots, noeviction | ≤1 second data loss window on crash, OOM returns error (never drops jobs) |
| PostgreSQL | Single node + outbox_jobs table | Transactional outbox guarantees task persistence |

#### JWT Security Model (RS256)

```
┌─────────────────┐     RS256 Private Key     ┌─────────────────┐
│   API Server    │ ─────── (sign) ──────────► │   JWT Token     │
│  (cmd/server)   │                            │  alg: RS256     │
│                 │ ◄────── (verify) ────────── │  sub: user_id   │
│  Private + Pub  │     RS256 Public Key       │  org_id: ...    │
└─────────────────┘                            └────────┬────────┘
                                                        │
                     RS256 Public Key (only)             │
┌─────────────────┐ ◄────── (verify) ──────────────────┘
│    Worker       │
│  (cmd/worker)   │     ❌ CANNOT sign (no private key)
│                 │     ❌ CANNOT forge admin tokens
│  Public Key     │     ✅ CAN verify token authenticity
└─────────────────┘
```

**Algorithm confusion attack prevention:** The verifier reads the JWT header's `alg`
field and rejects tokens whose algorithm does not match the server's configured algorithm.
This prevents an attacker from crafting an HS256 token with the public key as the HMAC secret.

#### Queue Architecture (Dual asynq Servers)

The worker runs two independent asynq server instances to prevent GPU-bound AI tasks
from starving lightweight administrative tasks:

```
┌────────────────────────────────────────────────────────────┐
│                    cmd/worker                              │
│                                                            │
│  ┌─────────────────────────┐  ┌──────────────────────────┐ │
│  │  General Server         │  │  Inference Server        │ │
│  │  Concurrency: 10        │  │  Concurrency: 2          │ │
│  │                         │  │                          │ │
│  │  Queues:                │  │  Queue:                  │ │
│  │   critical  (weight 6)  │  │   inference (weight 10)  │ │
│  │   default   (weight 3)  │  │                          │ │
│  │   low       (weight 1)  │  │  Tasks:                  │ │
│  │                         │  │   AI frame analysis      │ │
│  │  Tasks:                 │  │   ONNX model inference   │ │
│  │   Video export          │  │   Deep scan              │ │
│  │   Forensic PDF          │  │                          │ │
│  │   Telegram alerts       │  │  GPU-bound, isolated     │ │
│  │   Integrity checks      │  │  from general queue      │ │
│  └─────────────────────────┘  └──────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

#### Consequences

**Positive:**
- Event ingestion p99 latency guaranteed < 10ms (no in-process CPU contention)
- Compromised worker cannot escalate privileges (RSA key separation)
- Workers can be scaled independently (horizontal autoscaling on queue depth)
- Zero task loss via transactional outbox pattern
- Infrastructure survives single-node failures across all stateful components

**Negative:**
- Operational complexity: two binaries to deploy, monitor, and version
- Redis becomes a critical dependency (mitigated by outbox pattern + AOF persistence)
- RSA-256 verification is ~50x slower than HMAC-SHA256 (~50µs vs ~1µs per token).
  At 10,000 RPS this adds ~500ms aggregate CPU per second — acceptable on modern hardware.

#### Migration Path

1. Set `algorithm: HS256` in config to use legacy HMAC-SHA256 during transition
2. Generate RSA-4096 key pair: `./scripts/generate_jwt_keys.sh`
3. Set `algorithm: RS256` and provide key paths
4. Rotate: deploy API server with private key, workers with public key only
5. Remove `HS256` fallback after all tokens have expired (24h)
