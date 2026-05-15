# argus-backend — The Brain of Argus AI

> [!IMPORTANT]  
> **�������� ����� ���������!** ������ �������� �� 4 ����������� (backend, frontend, infra, sdk). ������ ��� ������� ��� ��� ������ ����������, �� **�������** ��������� ����������� [**DEVELOPER_GUIDE.md**](https://gitlab.com/argus_ai_group/argus-infra/-/blob/main/DEVELOPER_GUIDE.md) � ����������� argus-infra. ����� ������������� ������������� ���� ��� ����������� ����������� ��-�� ����������� ����� �������� � ���������� PR.


> **The event-collector is the core intelligence engine of the Argus AI proctoring platform.**
> It ingests millions of proctoring events per second, enforces cryptographic evidence integrity
> via a tamper-proof Forensic Ledger, orchestrates real-time risk scoring, and exposes a hardened
> REST + gRPC API surface to the rest of the system.

---

## Table of Contents

1. [Overview](#overview)
2. [Tech Stack](#tech-stack)
3. [Core Concepts](#core-concepts)
   - [Forensic Ledger](#forensic-ledger)
   - [Resilience Engine](#resilience-engine)
4. [Internal Structure](#internal-structure)
5. [Getting Started](#getting-started)
6. [Running the Server](#running-the-server)
7. [API Reference](#api-reference)
8. [Onboarding: Adding a New Endpoint](#onboarding-adding-a-new-endpoint)
9. [Load Testing](#load-testing)
10. [Environment Variables](#environment-variables)
11. [Related Repositories](#related-repositories)

---

## Overview

`argus-backend` is a single Go binary that exposes two network interfaces simultaneously:

| Interface | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| gRPC server | `50051` | H2C (native gRPC) | Event ingestion from browser SDK; internal service calls |
| HTTP server | `8080` | HTTP/1.1 + H2C | gRPC-Web proxy, REST admin API, health checks, metrics |

The HTTP listener on `:8080` is a **unified handler** — it inspects the `Content-Type` header to route
gRPC-Web requests (from the Nuxt 3 SPA) through to the gRPC server, serves health endpoints (`/healthz`,
`/readyz`), exposes Prometheus metrics (`/metrics`), and routes all `/api/v1/*` paths to the
appropriate REST handler.

In production, TLS termination and Nginx-level rate limiting are handled by `argus-infra`.
The backend binary never terminates TLS directly.

---

## Tech Stack

| Technology | Version | Role |
|------------|---------|------|
| **Go** | 1.24 | Service runtime — single statically-linked binary |
| **PostgreSQL** | 16 | Primary transactional database — organizations, users, API keys, audit log, review decisions, consent records, appeals, export job queue |
| **ClickHouse** | 24 | Telemetry and analytical storage — billions of proctoring events, pre-aggregated materialized views, Forensic Ledger (`evidence_fragments` table), hourly stats |
| **Apache Kafka** | Latest (KRaft) | Event streaming backbone — immutable source of truth for all ingested events; forensic ledger published to `argus.evidence.chain` and `argus.forensic.ledger` topics |
| **MinIO** | Latest | S3-compatible object storage for binary evidence fragments (video clips, screenshots, audio) with WORM Object Lock for legal compliance |
| **LiveKit** | Latest | WebRTC media server for real-time proctoring video streams (token generation in `pkg/media`) |
| **Prometheus** | Client v1.x | Metrics exposition at `GET /metrics` — consumed by Grafana |
| **go.uber.org/zap** | v1.x | Structured JSON logging |
| **sony/gobreaker** | v0.x | Circuit breaker pattern for downstream resilience |

> **Redis in the stack:** Redis is present in `argus-infra` as a session store exclusively for
> LiveKit. The backend Go service itself uses in-memory structures (ring buffers, session cache,
> hash-chain state) — there are **zero Redis imports** in `go.mod`. This is an intentional
> design choice to minimize network dependencies in the hot ingestion path.

---

## Core Concepts

### Forensic Ledger

The **Forensic Ledger** is the tamper-proof evidence integrity system, introduced in v2.1.
It is the cryptographic backbone that makes Argus AI evidence legally defensible in court
and compliant with data protection regulations.

#### The Problem

Binary evidence fragments (video clips, screenshots) are uploaded to MinIO (S3-compatible
object storage). An object in S3 can theoretically be replaced, reordered, or deleted after
the fact without detection — unless its existence is anchored cryptographically and sequenced.
Courts require a demonstrable, unbroken chain of custody.

#### Design: SHA-256 Hash Chaining

Every evidence fragment recorded during a proctoring session is linked into an
immutable, append-only chain. Each record's hash includes the previous record's hash:

```
Genesis
  │
  ▼  record_hash₁ = SHA-256( 1 ‖ "GENESIS" ‖ fragment_id₁ ‖ sha256(file₁) ‖ uploaded_at₁ )
  │
  ▼  record_hash₂ = SHA-256( 2 ‖ record_hash₁ ‖ fragment_id₂ ‖ sha256(file₂) ‖ uploaded_at₂ )
  │
  ▼  record_hashₙ = SHA-256( n ‖ record_hashₙ₋₁ ‖ fragment_idₙ ‖ sha256(fileₙ) ‖ uploaded_atₙ )
```

Each record in the ClickHouse `evidence_fragments` table carries three forensic columns
(added by `migrations/clickhouse/002_forensic_ledger.sql`):

| Column | Type | Description |
|--------|------|-------------|
| `sequence_num` | `UInt64` | Monotonically increasing per-session (1-based) |
| `previous_hash` | `String` | `record_hash` of the previous record (`"GENESIS"` for the first) |
| `record_hash` | `String` | SHA-256 of `(seq_num ‖ prev_hash ‖ fragment_id ‖ sha256_hash ‖ uploaded_at)` |

#### Write Path

The chain is written **atomically and in order** by `internal/infrastructure/evidence/chain_writer.go`:

1. **Kafka `argus.evidence.chain`** — Full evidence metadata with chain fields (primary, replay-safe)
2. **Kafka `argus.forensic.ledger`** — Minimal chain proof only (secondary, lightweight verification)
3. **ClickHouse `evidence_fragments`** — Queryable analytical store (2-year retention)

Kafka is the immutable source of truth. ClickHouse enables fast integrity reports and dashboard queries.

#### Integrity Verification

The `internal/infrastructure/integrity/verifier.go` recomputes the entire hash chain
for a session and cross-references each fragment's stored SHA-256 against the live S3 object.

It detects four classes of attack:

| Attack | Detection Method |
|--------|-----------------|
| **Insertion** | Gap in `sequence_num` |
| **Deletion** | Missing chain link / broken `previous_hash` |
| **Reordering** | `previous_hash` mismatch at any position |
| **Substitution** | `SHA-256(S3 object) ≠ stored sha256_hash` |

REST endpoints:
```
POST /api/v1/integrity/verify-session        — Full chain verification for a session
POST /api/v1/integrity/verify-fragment       — Single fragment spot-check
GET  /api/v1/integrity/session/{id}/report   — Pre-computed integrity report
```

---

### Resilience Engine

The **Resilience Engine** ensures the backend never becomes a single point of failure,
even under extreme load (10,000 concurrent proctoring sessions) or partial infrastructure outages.
It is composed of four cooperating systems.

#### 1. Circuit Breaker (`pkg/circuitbreaker`)

Wraps all downstream calls (Kafka producer, ClickHouse writer) with the `sony/gobreaker`
circuit breaker pattern. When failures exceed the threshold, the circuit opens and write
attempts fail-fast — preventing memory exhaustion and cascade failures.

```
CLOSED ──(5 failures in 10s)──► OPEN ──(30s timeout)──► HALF-OPEN ──(probe success)──► CLOSED
```

#### 2. Rate Limiting (`pkg/ratelimiter`)

Token-bucket rate limiting at three levels:

| Level | Default Limit | Default Burst | Applied At |
|-------|-------------|--------------|-----------|
| gRPC global | 50,000 RPS | 100,000 | All gRPC connections combined |
| gRPC per-session | 100 RPS | 200 | Per `session_id` JWT claim |
| HTTP admin global | 5,000 RPS | 10,000 | All `/api/v1/*` requests |
| HTTP per-IP | Configurable | Configurable | Per client IP address |
| Auth endpoint (Nginx) | 5 RPS | — | `/api/v1/auth/login` only |

#### 3. Worker Pool (`internal/transport/grpc/worker_pool.go`)

gRPC stream events are dispatched to a bounded pool of **512 goroutines** (self-optimized during
load testing, up from 256) with a queue depth of **16,384** (power-of-2 for memory-alignment).
When the queue is full, callers receive a `RESOURCE_EXHAUSTED` gRPC status and must retry with
exponential backoff. This prevents the service from accepting more work than it can process.

#### 4. Graceful Shutdown (`pkg/shutdown`)

The 18-step shutdown sequence in `cmd/server/main.go` guarantees zero event data loss.
Shutdown is triggered by `SIGINT` or `SIGTERM` (Kubernetes sends `SIGTERM` before killing the pod):

| Phase | Action | Description |
|-------|--------|-------------|
| 1a | `grpcServer.GracefulStop()` | Drain in-flight gRPC RPCs |
| 1b | `workerPool.Close()` | Drain pending jobs from the queue |
| 2 | `httpServer.Shutdown()` | Drain in-flight REST requests |
| 3 | Stop session validator, PostgreSQL, evidence recorder, chunk assembler, MinIO | Release resources |
| 4 | `ingestUC.Close()` | Kafka `AsyncClose()` + final ClickHouse batch flush |

**Total shutdown budget: 30 seconds** — aligned with Kubernetes `terminationGracePeriodSeconds`.

---

## Internal Structure

The codebase follows **Clean Architecture** (Hexagonal / Ports and Adapters).
The single inviolable rule: **inner layers never import outer layers.**

```
argus-backend/
│
├── cmd/                            ← Binary entry points (no business logic)
│   ├── server/
│   │   └── main.go                 ← 18-step initialization sequence
│   └── loadtest/                   ← 10,000-user concurrent load simulator
│       ├── main.go
│       ├── generator.go            ← Event generation (honest / suspicious / cheater profiles)
│       ├── metrics.go              ← Load test metrics aggregation
│       ├── optimizer.go            ← Self-optimizing config (pool size, batch sizes)
│       ├── profiles.go             ← Virtual user behavioral profiles
│       └── verifier.go             ← Result validation
│
├── internal/                       ← All application code (unexported from module)
│   │
│   ├── domain/                     ← Layer 1: Pure business logic — ZERO infrastructure imports
│   │   ├── entity/                 ← Aggregate roots and domain objects
│   │   │   ├── event.go            ← ProctoringEvent (the core domain object)
│   │   │   ├── evidence.go         ← EvidenceFragment with SHA-256 + chain fields
│   │   │   ├── organization.go     ← Multi-tenant org with plan + RBAC roles
│   │   │   ├── user.go             ← User with role (super_admin, org_admin, proctor, viewer)
│   │   │   ├── appeal.go           ← Student appeal state machine
│   │   │   ├── audit.go            ← Immutable audit trail record
│   │   │   ├── consent.go          ← GDPR consent record (append-only)
│   │   │   ├── review.go           ← Proctor review decision
│   │   │   └── export.go           ← Bulk export job entity
│   │   └── valueobject/
│   │       └── types.go            ← EventType, Severity, EventSource enumerations
│   │
│   ├── application/                ← Layer 2: Use cases + port interfaces
│   │   ├── port/                   ← Interfaces (contracts) that infrastructure must implement
│   │   │   ├── event_writer.go     ← Write events to Kafka + ClickHouse
│   │   │   ├── evidence_chain.go   ← Write forensic hash-chain records
│   │   │   ├── evidence_store.go   ← Store/retrieve binary evidence in S3
│   │   │   ├── integrity.go        ← Verify forensic hash chain
│   │   │   ├── chunk_store.go      ← Assemble chunked binary uploads
│   │   │   ├── org_repository.go   ← Organization/user/key CRUD
│   │   │   └── session_validator.go ← Validate JWT + session against Eduser
│   │   └── usecase/
│   │       └── ingest.go           ← IngestUseCase — core event ingestion orchestrator
│   │
│   ├── infrastructure/             ← Layer 3: Concrete implementations of application ports
│   │   ├── clickhouse/             ← Analytical writer (batch size 5,000 / flush 3s)
│   │   ├── kafka/                  ← Idempotent producer (zstd, required_acks=-1, 3 retries)
│   │   ├── postgres/               ← PostgreSQL repository (SaaS admin layer, max 25 conns)
│   │   ├── minio/                  ← S3 evidence store + Object Lock + presigned URLs
│   │   ├── evidence/
│   │   │   └── chain_writer.go     ← Forensic Ledger: SHA-256 hash chaining implementation
│   │   ├── integrity/
│   │   │   └── verifier.go         ← Full chain verification + S3 cross-reference
│   │   ├── recorder/               ← Evidence capture trigger (violation event → ring buffer → S3)
│   │   ├── ringbuffer/             ← In-memory circular buffer for sliding video window
│   │   ├── chunk/                  ← Chunked binary evidence assembler (Tier-C uploads)
│   │   ├── export/                 ← Async TAR.GZ + forensic manifest export worker
│   │   ├── session/                ← JWT + Eduser cached session validator (5m TTL)
│   │   ├── config/                 ← YAML loader + environment variable override mapping
│   │   ├── health/                 ← /healthz (process alive) + /readyz (deps healthy)
│   │   ├── metrics/                ← Prometheus metrics collector + registration
│   │   └── monitoring/             ← System resource stats (CPU, memory, goroutines)
│   │
│   └── transport/                  ← Layer 4: Protocol adapters — thin, zero business logic
│       ├── grpc/
│       │   ├── server.go           ← gRPC server constructor
│       │   ├── interceptors.go     ← Chain: Recovery → RateLimit → Auth (JWT+Session) → Logging
│       │   ├── mapper.go           ← Protobuf ↔ domain entity mapping
│       │   └── worker_pool.go      ← Bounded concurrency: 512 workers, 16,384 queue
│       ├── grpcweb/
│       │   └── proxy.go            ← gRPC-Web proxy (Content-Type routing for browser clients)
│       └── http/                   ← REST admin API (11 handler files, 54 endpoints)
│           ├── admin_handler.go    ← Auth, organizations, users, API keys, stats
│           ├── analytics_handler.go ← Executive dashboard analytics
│           ├── monitoring_handler.go ← Live session monitoring
│           ├── archive_handler.go  ← Session history, events, review workflow
│           ├── evidence_handler.go ← Evidence fragments + presigned URLs
│           ├── export_handler.go   ← Bulk async export jobs
│           ├── appeals_handler.go  ← Student appeals workflow
│           ├── consent_handler.go  ← GDPR consent recording
│           ├── integrity_handler.go ← Forensic verification endpoints
│           ├── media_handler.go    ← LiveKit WebRTC token generation
│           └── chunk_handler.go    ← Chunked binary evidence upload
│
├── pkg/                            ← Reusable packages (zero domain imports)
│   ├── auth/                       ← Stateless JWT creation + HMAC-SHA256 verification
│   │   ├── jwt.go                  ← Token encode/decode
│   │   └── crypto.go               ← Key derivation + HMAC utilities
│   ├── circuitbreaker/
│   │   └── breaker.go              ← sony/gobreaker wrapper (Closed → Open → Half-Open)
│   ├── cors/
│   │   └── middleware.go           ← CORS for gRPC-Web + HTTP (configurable origins)
│   ├── logger/
│   │   └── logger.go               ← zap.Logger factory (dev console / prod JSON)
│   ├── ratelimiter/
│   │   ├── limiter.go              ← Token-bucket rate limiting (global + per-IP)
│   │   └── http_middleware.go      ← HTTP middleware wrapping the limiter
│   ├── securityheaders/
│   │   └── middleware.go           ← HSTS, X-Frame-Options, CSP, Permissions-Policy
│   └── shutdown/
│       └── graceful.go             ← SIGINT/SIGTERM handler with drain coordination
│
├── api/
│   ├── openapi.yaml                ← OpenAPI 3.1 spec — the shared contract (54 endpoints)
│   └── proto/                      ← Protobuf definitions (gRPC event ingestion)
│
├── deployments/
│   └── config.yaml                 ← Production configuration (all fields overridable by env)
│
├── Dockerfile                      ← Multi-stage: golang:1.24-alpine builder → scratch/alpine runtime
├── Makefile                        ← build, test, lint, vet, docker, run targets
├── go.mod                          ← Module: github.com/argus-ai/event-collector
└── go.sum
```

### Dependency Rule (Clean Architecture)

```
cmd  ──imports──►  transport  ──imports──►  application/usecase
                       │                          │
                       │                    application/port
                       │                          ▲
                       └──imports──►  infrastructure  (implements ports)
                                           │
                                     domain/entity  (no outward imports)

pkg/*  ◄──── imported by any layer (zero domain/application imports)
```

---

## Getting Started

### Prerequisites

| Tool | Minimum Version | Install |
|------|----------------|---------|
| Go | 1.24 | https://go.dev/dl/ |
| Docker Desktop | 4.x | https://docker.com |
| GNU Make | Any | OS package manager |

### 1. Clone and Install Dependencies

```bash
git clone git@gitlab.argus.ai:argus/argus-backend.git
cd argus-backend

# Download all Go module dependencies into the module cache
go mod download

# Verify integrity of the dependency graph
go mod verify
```

### 2. Start Infrastructure Services

The backend requires Kafka, ClickHouse, PostgreSQL, and MinIO. Use the `argus-infra` stack:

```bash
cd ../argus-infra
cp docker/.env.example docker/.env
# Open docker/.env and fill in:
#   JWT_SIGNING_KEY=<at-least-32-random-chars>
#   POSTGRES_PASSWORD=<strong-password>
#   MINIO_ROOT_PASSWORD=<strong-password>
#   LIVEKIT_API_SECRET=<at-least-32-random-chars>

# Start the full infrastructure
docker compose up -d kafka clickhouse postgres minio minio-init
docker compose ps       # wait until all are "healthy"
```

**Minimal set for event ingestion only (no admin API):**
```bash
docker compose up -d kafka clickhouse
```

### 3. Run Database Migrations

PostgreSQL migrations are idempotent SQL files run in sequence:

```bash
# From argus-infra/ — PostgreSQL schema + seed data
docker compose exec postgres psql -U argus -d argus \
  -f /docker-entrypoint-initdb.d/init.sql

# ClickHouse analytical schema (run the files in order)
docker compose exec clickhouse clickhouse-client \
  --multiquery < ../argus-infra/migrations/clickhouse/init.sql

# ClickHouse Forensic Ledger extension (v2.1)
docker compose exec clickhouse clickhouse-client \
  --multiquery < ../argus-infra/migrations/clickhouse/002_forensic_ledger.sql
```

> **Tip:** The PostgreSQL container auto-runs `init.sql` on first start via
> `docker-entrypoint-initdb.d/`. You only need to run it manually if the
> container already existed before you pulled the migration.

### 4. Configure the Backend

The server reads `deployments/config.yaml` and applies environment variable overrides.
For local development, use env vars to avoid editing the shared config file:

```bash
export EVENT_COLLECTOR_AUTH_ENABLED=false           # Skip JWT verification locally
export EVENT_COLLECTOR_POSTGRES_HOST=localhost
export EVENT_COLLECTOR_POSTGRES_PASSWORD=change-me-in-production
export EVENT_COLLECTOR_CLICKHOUSE_ADDRS=localhost:9000
export EVENT_COLLECTOR_KAFKA_BROKERS=localhost:9092
export EVENT_COLLECTOR_MINIO_ENDPOINT=localhost:9002
export EVENT_COLLECTOR_CORS_ORIGINS=http://localhost:3000
```

---

## Running the Server

```bash
# Development mode (hot-reload not included — restart manually after changes)
go run ./cmd/server/... -config deployments/config.yaml

# Or use the Makefile
make run

# Build a production binary
make build
# Output: bin/event-collector

# With build-time version injection
go build \
  -ldflags "-X main.version=2.1.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/event-collector \
  ./cmd/server/...

# Run the pre-built binary
./bin/event-collector -config deployments/config.yaml

# Print version and exit
./bin/event-collector -version
```

**Expected startup output (all 18 steps):**
```
{"level":"info", "msg":"starting event-collector", "version":"2.1.0"}
{"level":"info", "msg":"kafka producer initialised", "brokers":["localhost:9092"]}
{"level":"info", "msg":"clickhouse writer initialised", "database":"argus_analytics"}
{"level":"info", "msg":"postgresql repository initialised", "host":"localhost"}
{"level":"info", "msg":"evidence recorder initialised"}
{"level":"info", "msg":"ingest use-case initialised", "evidence_capture_enabled":true}
{"level":"info", "msg":"worker pool initialised", "workers":512, "queue_size":16384}
{"level":"info", "msg":"admin api registered", "base_path":"/api/v1", "endpoints":31}
{"level":"info", "msg":"grpc server listening", "addr":":50051"}
{"level":"info", "msg":"http server listening", "addr":":8080"}
```

**Verify the server is running:**
```bash
# Liveness probe (always 200 if the process is alive)
curl http://localhost:8080/healthz

# Readiness probe (200 only when Kafka + ClickHouse are reachable)
curl http://localhost:8080/readyz

# Smoke test: login with seed credentials
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+77077469966","password":"Admin1234!"}' | jq .token
```

**Run the test suite:**
```bash
make test               # Unit + integration tests
make test-race          # With Go race detector
make lint               # golangci-lint
make vet                # go vet
```

---

## API Reference

The complete API contract lives in **`api/openapi.yaml`** — an OpenAPI 3.1 specification
covering all **54 endpoints**. This file is the **single source of truth** shared across
the entire Argus AI platform.

> **Frontend developers:** You never need to read Go source code. The OpenAPI spec contains
> every request schema, response schema, status code, and authentication requirement.

### Endpoint Summary

| Group | Base Path | # Endpoints | Min Role |
|-------|-----------|:-----------:|---------|
| Auth | `/api/v1/auth` | 2 | — / Any |
| Admin — Organizations | `/api/v1/admin/organizations` | 7 | `super_admin` |
| Admin — Users | `/api/v1/admin/organizations/{id}/users` | 2 | `org_admin` |
| Admin — API Keys | `/api/v1/admin/.../keys` | 3 | `org_admin` |
| Admin — Stats & Audit | `/api/v1/admin/stats`, `/audit-logs` | 2 | `org_admin` |
| Analytics | `/api/v1/analytics` | 7 | `proctor` |
| Monitoring | `/api/v1/monitoring` | 3 | `proctor` |
| Archive | `/api/v1/archive` | 7 | `proctor` |
| Evidence | `/api/v1/evidence` | 3 | `proctor` |
| Export (Async) | `/api/v1/export` | 4 | `org_admin` |
| Appeals | `/api/v1/appeals` | 4 | `proctor` |
| Consent | `/api/v1/consent` | 2 | `proctor` |
| Integrity | `/api/v1/integrity` | 3 | `org_admin` |
| Media (LiveKit) | `/api/v1/media` | 2 | `proctor` |
| Ingest (Chunk Upload) | `/api/v1/ingest/chunk` | 2 | Any (API key) |
| Health | `/healthz`, `/readyz` | 2 | None |

### Authentication

All endpoints except login and health require a Bearer JWT in the `Authorization` header:

```
Authorization: Bearer <token>
```

Tokens are obtained from `POST /api/v1/auth/login` (valid 24 hours).
The JWT payload carries three claims: `sub` (user ID), `org_id`, `role`.

Roles and their access scope:

| Role | Scope | Access |
|------|-------|--------|
| `super_admin` | All organizations (`orgId = "*"`) | Full access to all endpoints |
| `org_admin` | Own organization | Manage users, API keys, export, integrity |
| `proctor` | Own organization | Monitor sessions, review, archive |
| `viewer` | Own organization | Read-only access to analytics + archive |

---

## Onboarding: Adding a New Endpoint

Follow these **8 steps** in order. Do not skip to the handler — the contract comes first.

### Step 1 — Define the OpenAPI contract

Open `api/openapi.yaml`. Add the path, request body, response schema, and any new
`components/schemas` entries. Use `$ref` to reuse existing schemas.

```yaml
# api/openapi.yaml

paths:
  /api/v1/exams/{examId}/summary:
    parameters:
      - name: examId
        in: path
        required: true
        schema:
          type: string
    get:
      tags: [Archive]
      summary: Get exam summary
      operationId: getExamSummary
      responses:
        '200':
          description: Exam summary
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ExamSummary'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'

components:
  schemas:
    ExamSummary:
      type: object
      properties:
        examId:
          type: string
        orgId:
          type: string
        sessionCount:
          type: integer
```

### Step 2 — Add a domain entity (if introducing a new concept)

```go
// internal/domain/entity/exam.go
package entity

// ExamSummary is an aggregate of session statistics for a single exam.
// No JSON tags here — JSON marshalling belongs in the transport layer.
type ExamSummary struct {
    ExamID       string
    OrgID        string
    SessionCount int
}
```

### Step 3 — Define a port interface (if a new query is needed)

```go
// internal/application/port/exam_repository.go
package port

import (
    "context"
    "github.com/argus-ai/event-collector/internal/domain/entity"
)

type ExamRepository interface {
    GetExamSummary(ctx context.Context, orgID, examID string) (*entity.ExamSummary, error)
}
```

### Step 4 — Implement the infrastructure query

Add a method to the appropriate infrastructure struct. For PostgreSQL:

```go
// internal/infrastructure/postgres/repository.go

func (r *Repository) GetExamSummary(ctx context.Context, orgID, examID string) (*entity.ExamSummary, error) {
    var e entity.ExamSummary
    err := r.db.QueryRowContext(ctx,
        `SELECT exam_id, org_id, COUNT(*) AS session_count
         FROM review_decisions
         WHERE org_id = $1 AND exam_id = $2
         GROUP BY exam_id, org_id`,
        orgID, examID,
    ).Scan(&e.ExamID, &e.OrgID, &e.SessionCount)
    if err != nil {
        return nil, fmt.Errorf("GetExamSummary: %w", err)
    }
    return &e, nil
}
```

### Step 5 — Write the HTTP handler

Follow the exact pattern used in the codebase. Add to the most appropriate existing handler
file (or create a new `*_handler.go` file for a new domain):

```go
// internal/transport/http/archive_handler.go

// examSummaryResponse mirrors the OpenAPI ExamSummary schema exactly.
// JSON field names MUST match the openapi.yaml property names.
type examSummaryResponse struct {
    ExamID       string `json:"examId"`
    OrgID        string `json:"orgId"`
    SessionCount int    `json:"sessionCount"`
}

func (h *ArchiveHandler) handleGetExamSummary(w http.ResponseWriter, r *http.Request) {
    examID := r.PathValue("examId")             // Go 1.22+ path values
    claims := claimsFromContext(r.Context())    // JWT claims injected by auth middleware

    // RBAC guard
    if !claims.HasOrgAccess(claims.OrgID) {
        writeError(w, http.StatusForbidden, "forbidden")
        return
    }

    summary, err := h.repo.GetExamSummary(r.Context(), claims.OrgID, examID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeError(w, http.StatusNotFound, "exam not found")
            return
        }
        h.logger.Error("GetExamSummary", zap.Error(err), zap.String("exam_id", examID))
        writeError(w, http.StatusInternalServerError, "internal error")
        return
    }

    writeJSON(w, http.StatusOK, examSummaryResponse{
        ExamID:       summary.ExamID,
        OrgID:        summary.OrgID,
        SessionCount: summary.SessionCount,
    })
}
```

### Step 6 — Register the route

In the handler's `RegisterRoutes` method, use Go 1.22 pattern syntax (`METHOD path`):

```go
func (h *ArchiveHandler) RegisterRoutes(mux *http.ServeMux) {
    // existing routes ...
    mux.HandleFunc("GET /api/v1/exams/{examId}/summary", h.withAuth(h.handleGetExamSummary))
}
```

### Step 7 — Write tests

```bash
# Run the specific test
go test ./internal/transport/http/... -run TestArchiveHandler_GetExamSummary -v

# Run with race detector
go test -race ./internal/... -run TestArchiveHandler_GetExamSummary

# Full test suite
make test
```

### Step 8 — Update endpoint count in main.go

Find the `admin api registered` log line and increment the counter:

```go
logger.Info("admin api registered",
    zap.String("base_path", "/api/v1"),
    zap.Int("endpoints", 32),   // was 31
)
```

---

## Load Testing

The `cmd/loadtest` binary simulates 10,000 concurrent proctoring sessions against a live server.
It was used to tune the worker pool size (512) and queue depth (16,384).

```bash
# Build the load test binary
go build -o bin/loadtest ./cmd/loadtest/...

# Simulate 1,000 concurrent sessions for 60 seconds
./bin/loadtest -sessions 1000 -duration 60s -target http://localhost:8080

# Full 10K session stress test (requires production-class hardware + full infra stack)
./bin/loadtest -sessions 10000 -duration 300s -target http://localhost:8080
```

**Behavioral profiles for virtual users:**

| Profile | Proportion | Description |
|---------|-----------|-------------|
| Honest | 72% | Normal event patterns; no critical events |
| Suspicious | 21% | Moderate violation rate; warning-level events |
| Cheater | 7% | High-frequency critical events; coordinated patterns |

---

## Environment Variables

All configuration values in `deployments/config.yaml` can be overridden by environment variables
using the convention `EVENT_COLLECTOR_<SECTION>_<KEY>` (uppercase, underscores).

| Variable | Default | Required in Prod | Description |
|----------|---------|:----------------:|-------------|
| `EVENT_COLLECTOR_JWT_SIGNING_KEY` | — | ✅ | HMAC-SHA256 signing key (min 32 chars) |
| `EVENT_COLLECTOR_AUTH_ENABLED` | `true` | — | Set `false` to skip JWT in local dev |
| `EVENT_COLLECTOR_GRPC_PORT` | `50051` | — | gRPC listener port |
| `EVENT_COLLECTOR_HTTP_PORT` | `8080` | — | HTTP listener port |
| `EVENT_COLLECTOR_KAFKA_BROKERS` | `localhost:9092` | — | Comma-separated Kafka broker list |
| `EVENT_COLLECTOR_CLICKHOUSE_ADDRS` | `localhost:9000` | — | Comma-separated ClickHouse addrs |
| `EVENT_COLLECTOR_POSTGRES_HOST` | `localhost` | — | PostgreSQL hostname |
| `EVENT_COLLECTOR_POSTGRES_PASSWORD` | — | ✅ | PostgreSQL password |
| `EVENT_COLLECTOR_MINIO_ENDPOINT` | `localhost:9002` | — | MinIO S3 endpoint |
| `EVENT_COLLECTOR_MINIO_ACCESS_KEY` | — | ✅ | MinIO access key |
| `EVENT_COLLECTOR_MINIO_SECRET_KEY` | — | ✅ | MinIO secret key |
| `EVENT_COLLECTOR_CORS_ORIGINS` | `http://localhost:3000` | ✅ | Comma-separated CORS allowed origins |
| `EVENT_COLLECTOR_LOG_LEVEL` | `info` | — | `debug` / `info` / `warn` / `error` |
| `EVENT_COLLECTOR_WORKER_POOL_SIZE` | `512` | — | gRPC worker pool goroutine count |
| `EVENT_COLLECTOR_SERVER_TLS` | `false` | — | Enable HSTS header (only if terminating TLS here) |

See `.env.example` for the complete list and `deployments/config.yaml` for all defaults and comments.

---

## Related Repositories

| Repository | Path | Purpose |
|------------|------|---------|
| **argus-backend** | `~/Desktop/argus_ai/argus-backend` | **This repo** — The Brain |
| argus-frontend | `~/Desktop/argus_ai/argus-frontend` | Nuxt 3 admin dashboard SPA |
| argus-infra | `~/Desktop/argus_ai/argus-infra` | Docker Compose, Nginx, CI/CD, migrations |

**API Contract:** `api/openapi.yaml` — the single source of truth for all 54 REST endpoints.
Frontend developers work exclusively from this spec; no Go source access required.
