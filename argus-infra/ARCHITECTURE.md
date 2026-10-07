# Argus AI — Technical Architecture Specification

**Version:** 1.0.0
**Classification:** Internal Engineering Reference
**Audience:** Senior engineers, DevOps, and platform architects

---

## Table of Contents

1. [System Blueprint](#1-system-blueprint)
2. [Event Pipeline](#2-event-pipeline)
3. [BadgerDB Dead Letter Queue (DLQ)](#3-badgerdb-dead-letter-queue-dlq)
4. [ReplacingMergeTree Idempotency](#4-replacingmergetree-idempotency)
5. [Passive Health Governor](#5-passive-health-governor)
6. [Data Retention (TTL)](#6-data-retention-ttl)
7. [Shared Proto Registry](#7-shared-proto-registry)
8. [Bilingual Telegram Alerting](#8-bilingual-telegram-alerting)
9. [Backend Architecture](#9-backend-architecture)
10. [Frontend Architecture](#10-frontend-architecture)

---

## 1. System Blueprint

Argus AI is a polyrepo ecosystem. Three repositories, three teams, one event pipeline.

```
+---------------------------------------------------------------------+
|                         argus-infra                                   |
|  Docker Compose . Nginx . Migrations . CI/CD                         |
|                                                                       |
|  +------------------------+          +------------------------+       |
|  |   argus-frontend       |  gRPC-Web |   argus-backend        |       |
|  |                        |<-------->|                        |       |
|  |   Nuxt 4 / Vue 3       |  JSON/HTTP|   Go 1.24              |       |
|  |   TypeScript            |          |   gRPC + REST           |       |
|  +----------+-------------+          +----------+-------------+       |
|             |                                    |                    |
|             | SPA served via Nginx               | Writes to:        |
|             |                                    +-> Kafka (primary)  |
|             |                                    +-> ClickHouse       |
|             |                                    +-> MinIO (S3)       |
|             |                                    +-> PostgreSQL       |
+-------------+------------------------------------+--------------------+
```

**Repo boundaries are strict.** Frontend never talks to databases. Backend never serves HTML. Infra never contains application logic. Each repo has its own CI pipeline; infra orchestrates deployment via triggered downstream pipelines and image tags (`BACKEND_TAG`, `FRONTEND_TAG`).

**Communication contract:** The frontend speaks to the backend exclusively through gRPC-Web (JSON over HTTP, proxied by Nginx) and REST endpoints under `/api/v1/`. The proto definition at `argus-backend/api/proto/v1/event_collector.proto` is the single source of truth for the wire format.

---

## 2. Event Pipeline

Every proctoring event from every browser session follows this path:

```
Browser (Vue composable)
  |
  +-- CRITICAL events -------> IngestEvent (unary gRPC, immediate)
  |
  +-- WARNING/INFO/TELEMETRY -> Client batch buffer
                                    |
                                    v
                              IngestBatch (gRPC, tier-adaptive interval)
                                    |
                           +--------+------------+
                           v        v             v
                        Kafka   ClickHouse   Evidence Recorder
                      (primary) (secondary)    (tertiary)
                         |          |              |
                         |          |         Ring Buffer -> MinIO
                         |          |              |
                         |          |         Chain Writer
                         |          |         (Forensic Ledger)
                         |          |
                    7-day retain  90-day TTL
                    Source of     Analytical
                    truth         queries
```

**Failure semantics are asymmetric by design:**

| Downstream | On Failure | Rationale |
|------------|-----------|-----------|
| Kafka | Event **rejected** (client retries) | Kafka is the source of truth |
| ClickHouse | Event **accepted** | Kafka retains; ClickHouse rebuilds from Kafka |
| Evidence capture | Event **accepted** | Async, never blocks ingestion |

**Never change this hierarchy.** Kafka is the append-only log. ClickHouse is the query engine. They are not interchangeable.

---

## 3. BadgerDB Dead Letter Queue (DLQ)

### Resilience Model

The DLQ provides fault tolerance when Kafka is unavailable. A circuit breaker wraps the Kafka producer (`port.EventWriter`). When Kafka fails, events persist to BadgerDB (embedded KV store, pure Go, zero CGO). A background reclamation goroutine drains them back to Kafka when the circuit breaker closes.

### Write Path

```
IngestUseCase -> ResilientWriter.Write()
                       |
                  Circuit Breaker
                  /            \
           [CLOSED]          [OPEN]
              |                 |
         Kafka Producer    BadgerDB Store.Save()
              |                 |
          return nil         return nil (deferred delivery)
                                |
                          markDegraded() -> Telegram heartbeat
```

- **Kafka succeeds** -> event delivered, `markHealthy()` if recovering
- **Kafka fails / breaker OPEN** -> BadgerDB stores event, returns nil (deferred)
- **BadgerDB fails** -> return error (CRITICAL Telegram alert fired)

### Background Goroutines

| Goroutine | Interval | Behavior |
|-----------|----------|----------|
| **Reclamation** | `ReclamationInterval` (30s) | Polls BadgerDB; when breaker CLOSED, drains events to Kafka in batches. Idempotent — ClickHouse `ReplacingMergeTree` deduplicates replays. |
| **Heartbeat** | `HeartbeatInterval` (5m) | While breaker is not CLOSED, sends periodic bilingual Telegram alerts with DLQ size, breaker state, and degraded duration. |

### Recovery Flow

When the circuit breaker transitions from OPEN -> HALF-OPEN -> CLOSED:

1. Reclamation goroutine detects breaker CLOSED
2. Drains BadgerDB in batches of `ReclamationBatchSize`
3. On full drain: `markHealthy()` fires `MsgKafkaRecovered()` Telegram notification
4. System exits Buffered Mode transparently

### Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ReclamationInterval` | 30s | DLQ drain poll frequency |
| `ReclamationBatchSize` | 100 | Max events per drain cycle |
| `HeartbeatInterval` | 5m | Telegram heartbeat during degraded mode |
| `DataDir` | `/app/data/dlq` | BadgerDB storage path |
| `MaxEntries` | 1,000,000 | Safety cap on DLQ size |
| `SyncWrites` | true | fsync on every write (crash safety) |
| `CircuitBreakerFailureThreshold` | 5 | Consecutive Kafka failures to trip breaker |
| `CircuitBreakerTimeout` | 30s | Duration breaker stays OPEN before probe |

### Key Files

| File | Role |
|------|------|
| `argus-backend/internal/infrastructure/dlq/resilient_writer.go` | ResilientWriter decorator (heartbeat, reclamation, state tracking) |
| `argus-backend/internal/infrastructure/dlq/store.go` | BadgerDB Store (Save, List, Delete, Size) |
| `argus-backend/pkg/circuitbreaker/breaker.go` | Circuit breaker (gobreaker-compatible, callback hooks) |

---

## 4. ReplacingMergeTree Idempotency

### Problem

DLQ recovery replays events that may already exist in ClickHouse, creating duplicates.

### Solution

```sql
ENGINE = ReplacingMergeTree(server_timestamp)
ORDER BY (org_id, exam_id, session_id, event_id)
```

- `event_id` (UUIDv7) in `ORDER BY` ensures replayed events share the same sort key as originals
- `server_timestamp` as the version column: the row with the highest timestamp survives
- Background merges automatically collapse duplicates — no application-level dedup required

### Query Guidance

| Scenario | Approach |
|----------|----------|
| Dashboard queries (tolerant of brief duplicates) | Standard SELECT — merges happen asynchronously |
| Exact-count analytics | Use `SELECT ... FINAL` or `argMax(col, server_timestamp)` |
| Forensic audit | Always use `FINAL` to guarantee deduplication |

### Migration

Migration `004_reliability_hardening.sql` performs a zero-downtime schema evolution:

1. `RENAME TABLE proctoring_events TO proctoring_events_legacy`
2. `CREATE TABLE proctoring_events` with `ReplacingMergeTree(server_timestamp)`
3. `INSERT INTO proctoring_events SELECT * FROM proctoring_events_legacy`
4. `DROP TABLE proctoring_events_legacy`
5. Recreate all 6 materialized views

---

## 5. Passive Health Governor

### Design Philosophy: Zero-CPU

No background polling. No goroutines. No timers. Health checks execute **on-demand** when Kubernetes probes hit the endpoints. Between probes, the health system consumes zero resources.

### Endpoints

| Endpoint | Type | HTTP Status | Purpose |
|----------|------|-------------|---------|
| `GET /healthz` | Liveness | Always 200 | Process is alive. Kubernetes restarts on failure. |
| `GET /readyz` | Readiness | 200 or 503 | All dependencies reachable. Kubernetes removes from LB on failure. |

### Criticality Tagging

```go
healthHandler := health.NewHandler(logger,
    health.Critical(kafkaProducer),    // Failure -> 503 (remove from LB)
    health.NonCritical(chWriter),      // Failure -> degraded (200, still serving)
)
```

| Tag | On Failure | HTTP Status | Effect |
|-----|-----------|-------------|--------|
| `Critical` | `StatusUnhealthy` | 503 | Service removed from load balancer |
| `NonCritical` | `StatusDegraded` | 200 | Service continues, analytics delayed |

### DLQStatusProvider Interface

When the DLQ is enabled, the health handler exposes pipeline metrics through `/readyz`:

```json
{
  "status": "degraded",
  "pipeline": {
    "breaker_state": "open",
    "dlq_size": 1247,
    "is_degraded": true,
    "degraded_for": "4m32s"
  }
}
```

### Concurrency Model

Readiness checks run in parallel goroutines with `sync.WaitGroup`. Each checker has a 5-second context timeout. Results are collected and the worst status wins (unhealthy > degraded > ok).

---

## 6. Data Retention (TTL)

| Store | Data | Retention | Mechanism |
|-------|------|-----------|-----------|
| Kafka | All events | 7 days | `KAFKA_LOG_RETENTION_HOURS=168` |
| ClickHouse | Raw events (`proctoring_events`) | 90 days | `TTL server_timestamp + INTERVAL 90 DAY DELETE` |
| ClickHouse | Hourly aggregates (`hourly_event_stats`) | 180 days | `TTL hour + INTERVAL 180 DAY DELETE` |
| ClickHouse | Org statistics (`global_org_stats`) | 365 days | `TTL day + INTERVAL 365 DAY DELETE` |
| ClickHouse | Evidence metadata (`evidence_fragments`) | 730 days | `TTL uploaded_at + INTERVAL 730 DAY DELETE` |
| ClickHouse | Critical alerts (`critical_events_recent`) | 7 days | `TTL server_timestamp + INTERVAL 7 DAY DELETE` |
| ClickHouse | Session summaries (`student_session_summary`) | 90 days | `TTL last_event_time + INTERVAL 90 DAY DELETE` |
| MinIO | Video evidence (Object Lock GOVERNANCE) | 365 days | S3 WORM retention policy |
| MinIO | Export archives (`argus-exports`) | 30 days | ILM lifecycle expiration |

**Retention is enforced automatically.** ClickHouse TTL triggers on background merges. Kafka retention is log-segment based. MinIO GOVERNANCE mode prevents deletion by non-admin users for the retention period.

---

## 7. Shared Proto Registry

### Single Source of Truth

```
argus-backend/api/proto/v1/event_collector.proto
```

This file defines all wire formats: 4 gRPC methods, 59 EventType values, 4 Severity levels, 8 EventSource values, 3 TelemetryMode values, and 13 payload message types.

### Contract Verification Pipeline

```
Proto (.proto)                TypeScript (types.ts)
      |                              |
      +------ contractcheck ---------+
                   |
              CI exit code
              0 = aligned
              1 = desync
              2 = parse error
```

| Tool | Location | Purpose |
|------|----------|---------|
| `contractcheck` | `argus-backend/cmd/contractcheck/main.go` | Go-native enum parser. Cross-references proto and TS enums. CI-safe (exit codes). |
| `proto-sync.sh` | `argus-infra/scripts/proto-sync.sh` | Bash-based TS type generator with `--check` mode for CI verification. |

### Makefile Targets

```bash
make sync-types           # Verify TS<->proto alignment (CI-safe, exit 1 on desync)
make sync-types-generate  # Regenerate TypeScript types from proto definitions
make proto                # Regenerate Go protobuf stubs (protoc)
```

### Checked Enums

| Enum | Values | Scope |
|------|--------|-------|
| `EventType` | 59 | All proctoring event categories (face, gaze, audio, screen, sidecam, backend AI) |
| `Severity` | 4 | CRITICAL, WARNING, INFO, TELEMETRY |
| `EventSource` | 8 | BROWSER, EXTENSION, MOBILE, DESKTOP, SIDECAM, BACKEND, ADMIN_MANUAL, BACKEND_AI |
| `TelemetryMode` | 3 | REALTIME, BATCH, OFFLINE_SYNC |

---

## 8. Bilingual Telegram Alerting

### Architecture

All system alerts are delivered in **dual-language format: Kazakh (KZ) + English (EN)**. This supports the platform's deployment context where stakeholders include both Kazakh-speaking administrators and international technical staff.

### Message Format

```
[KZ flag] [KZ Title]
---separator---
[KZ body with HTML formatting]
===========================
[EN flag] [EN Title]
---separator---
[EN body with HTML formatting]
```

### Alert Types

| Function | Trigger | Severity |
|----------|---------|----------|
| `MsgServerStartup` | Service boot | INFO |
| `MsgCircuitBreakerOpen` | Downstream failure threshold reached | CRITICAL |
| `MsgBufferedModeHeartbeat` | Periodic pulse during Kafka outage | WARNING |
| `MsgKafkaRecovered` | Circuit breaker closes, DLQ drained | INFO |
| `MsgDLQStoreFailed` | BadgerDB write failure | CRITICAL |
| `MsgHealthCheck` | Manual or scheduled health report | INFO |

### Rate Limiting

- Max 1 alert per component per minute (`rateLimitPerComp = 60s`)
- Non-blocking async queue (100 alert buffer)
- 10-second HTTP timeout per Telegram API call

---

## 9. Backend Architecture

### Hexagonal Architecture (Ports & Adapters)

```
cmd/server/main.go              <- 18-step bootstrap, wires everything
internal/
  domain/
    entity/                     <- ProctoringEvent, EvidenceFragment, Appeal, Consent
    valueobject/                <- EventType, Severity, EventSource (typed enums)
  application/
    port/                       <- Interfaces: EventWriter, EvidenceStore, ChainWriter
    usecase/                    <- IngestUseCase (the single entry point)
  infrastructure/
    kafka/                      <- AsyncProducer (sarama), idempotent, zstd
    clickhouse/                 <- Native protocol, two-stage buffer swap
    dlq/                        <- BadgerDB dead-letter queue + ResilientWriter
    alerting/                   <- Telegram bilingual alert provider
    health/                     <- Passive health governor
    minio/                      <- S3 upload with single-pass SHA-256
    postgres/                   <- Admin/SaaS layer (orgs, users, audit, exports)
    evidence/                   <- Forensic Ledger chain writer (SHA-256 hash chaining)
    recorder/                   <- Orchestrates: snapshot -> concat -> upload -> chain
    inference/                  <- AI model integration (vision, audio analysis)
  transport/
    grpc/                       <- gRPC server, worker pool, interceptor chain
    grpcweb/                    <- improbable-eng/grpc-web proxy
    http/                       <- REST admin handlers (analytics, monitoring, exports)
pkg/
  auth/                         <- JWT verification, HMAC crypto
  circuitbreaker/               <- Fail-fast on downstream outages
  ratelimiter/                  <- Token bucket (global + per-session + per-IP)
  cors/                         <- CORS middleware
  securityheaders/              <- HSTS, CSP, X-Frame-Options
```

### Design Rules

- **Constructor injection only.** No `init()`, no globals, no service locators.
- **Domain has zero infrastructure imports.** Dependency arrows point inward.
- **IngestUseCase is the single entry point.** 3-stage fan-out: Kafka -> ClickHouse -> Evidence.
- **Graceful shutdown:** 30-second budget. Order: gRPC -> WorkerPool -> HTTP -> Session -> UseCase.

### Forensic Ledger (Hash Chain)

```
record_hash = SHA-256( sequence_num | previous_hash | fragment_id | sha256_hash | uploaded_at )
```

- First record uses `previous_hash = "GENESIS"`
- Sequence numbers are monotonic per session (no gaps)
- Tampering detection: insertion breaks continuity, deletion breaks linkage, reordering breaks hashes

### Concurrency Model

```
10K+ gRPC streams --> Worker Pool (512 goroutines) --> IngestUseCase
                          |
                     Bounded channel (8192 jobs)
                     Submit() blocks when full (backpressure)
```

---

## 10. Frontend Architecture

### Stack

Nuxt 4 / Vue 3 / TypeScript / Pinia / gRPC-Web

### Tier-Adaptive Resilience

The frontend implements a 3-tier state machine that adapts behavior based on network and device health:

| Tier | Condition | Batch Interval | Evidence |
|------|-----------|---------------|----------|
| **A** (Optimal) | Network stable, device healthy | 2s | Full capture |
| **B** (Degraded) | Intermittent issues | 5s | Reduced capture |
| **C** (Critical) | Offline or severe degradation | 30s | Minimal, queued to IndexedDB |

### Offline Queue

IndexedDB write-ahead log (500MB default) ensures zero event loss during network outages. Events are automatically replayed when connectivity restores, with exponential backoff.

### RBAC Model

| Role | Scope | Capabilities |
|------|-------|-------------|
| Super Admin | Platform-wide | All operations, org management |
| Org Admin | Organization | User management, exam config, analytics |
| Proctor | Exam session | Live monitoring, incident flagging |
| Viewer | Read-only | Dashboard access, report viewing |
