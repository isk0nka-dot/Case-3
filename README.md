# argus-infra — The Ecosystem of Argus AI

> This repository owns everything that **orchestrates and operates** the Argus AI platform —
> but contains no application source code. It is the ops layer: infrastructure as code,
> deployment pipelines, database schemas, and the security perimeter.

---

## Table of Contents

1. [The Ecosystem](#the-ecosystem)
2. [Orchestration](#orchestration)
3. [Security Layer](#security-layer)
4. [Operations](#operations)
   - [First-Time Provisioning](#first-time-provisioning)
   - [Zero-Downtime Deployment](#zero-downtime-deployment)
   - [Backup Strategy](#backup-strategy)
5. [Environment Variables](#environment-variables)
6. [Database Migrations](#database-migrations)
7. [CI/CD Pipeline](#cicd-pipeline)
8. [Repository Structure](#repository-structure)
9. [Related Repositories](#related-repositories)

---

## The Ecosystem

The Argus AI platform is composed of nine Dockerized services, all running on a single
private bridge network (`argus-network`). Services communicate by container name —
no service is directly exposed to the internet except through Nginx.

| Service | Image | Ports (Internal → Host) | Role |
|---------|-------|------------------------|------|
| **backend** | `registry.argus.ai/argus/backend:${BACKEND_TAG}` | 8080, 50051 | Go event-collector: gRPC ingestion + REST admin API |
| **frontend** | `registry.argus.ai/argus/frontend:${FRONTEND_TAG}` | 3000 | Nuxt 4 SPA: admin dashboard |
| **nginx** | (production: host nginx or containerized) | 80 → 443 | TLS termination, reverse proxy, rate limiting, security headers |
| **postgres** | `postgres:16-alpine` | 5432 | Primary transactional database (orgs, users, sessions, audit) |
| **clickhouse** | `clickhouse/clickhouse-server:latest` | 9000 (native), 8123 (HTTP) | Analytical database (billions of proctoring events) |
| **kafka** | `apache/kafka:latest` (KRaft mode) | 9092 (internal), 9094 (host) | Event streaming backbone — source of truth for all events |
| **redis** | `redis:7-alpine` | 6379 (internal only) | LiveKit session state store |
| **minio** | `minio/minio:latest` | 9001 (console), 9002 (S3 API) | S3-compatible binary evidence storage (video clips, screenshots) |
| **livekit** | `livekit/livekit-server:latest` | 7880, 7881, 7882/udp | WebRTC media server for real-time proctoring video |

**Service health dependencies:**

```
zookeeper (none)
  kafka (none — KRaft mode, no Zookeeper needed)
    └── backend (depends_on: kafka healthy)

clickhouse (none)
    └── backend (depends_on: clickhouse healthy)

postgres (none)
    └── backend (depends_on: postgres healthy)

redis (none)
    └── livekit (depends_on: redis healthy)

minio (none)
    └── minio-init (one-shot, depends_on: minio healthy)
    └── backend (depends_on: minio healthy)

backend (all above healthy)
    └── frontend (depends_on: backend healthy)
```

---

## Orchestration

### Docker Compose

**File:** `docker/docker-compose.yaml`

The compose file uses a **polyrepo model**: it references pre-built Docker images from the
registry rather than building from source. The backend and frontend teams manage their
own build pipelines independently. This repo only manages deployment.

```yaml
# Images are versioned via environment variables
services:
  backend:
    image: registry.argus.ai/argus/backend:${BACKEND_TAG:-latest}
  frontend:
    image: registry.argus.ai/argus/frontend:${FRONTEND_TAG:-latest}
```

### Private Network

All inter-service communication happens on the `argus-network` bridge network.
No service exposes ports to the host except for development convenience.
In production, only Nginx exposes ports 80 and 443 to the host.

```
argus-network (bridge)
  ├── backend     → kafka:9092, clickhouse:9000, postgres:5432, minio:9000
  ├── frontend    → (no direct service calls — all via browser → Nginx → backend)
  ├── livekit     → redis:6379
  └── minio-init  → minio:9000
```

### Data Volumes

All persistent data lives in named Docker volumes (not bind mounts):

| Volume | Service | Contents |
|--------|---------|---------|
| `postgres-data` | PostgreSQL | Organizations, users, API keys, audit log, export jobs |
| `clickhouse-data` | ClickHouse | Proctoring events (billions of rows), materialized views |
| `clickhouse-logs` | ClickHouse | ClickHouse server logs |
| `kafka-data` | Kafka | Event stream (7-day retention), forensic ledger |
| `redis-data` | Redis | LiveKit session state |
| `minio-data` | MinIO | Binary evidence fragments (video, screenshots) — 365-day WORM retention |

**Named volumes survive container restarts and upgrades.** `docker compose down` does NOT
delete volumes. To wipe all data (development reset only):

```bash
docker compose down -v   # WARNING: destroys all persistent data
```

### Starting the Full Stack

```bash
cd docker
cp .env.example .env
# Edit .env — fill in the REQUIRED variables (see Environment Variables section)

docker compose pull          # Pull all images
docker compose up -d         # Start all services in background
docker compose ps            # Verify all services are healthy
docker compose logs -f       # Follow all logs
docker compose logs backend  # Follow backend logs only
```

---

## Security Layer

### Nginx: The Security Perimeter

**File:** `nginx/nginx.conf`

Nginx is the only internet-facing entry point. It terminates TLS, enforces security headers,
rate-limits sensitive endpoints, and routes traffic to the correct upstream service.

#### TLS Configuration

```nginx
ssl_protocols       TLSv1.2 TLSv1.3;
ssl_ciphers         ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:...;
ssl_prefer_server_ciphers off;
ssl_session_cache   shared:SSL:10m;
ssl_session_timeout 1d;
ssl_stapling        on;    # OCSP stapling — reduces TLS handshake latency
```

TLS 1.1 and older are explicitly disabled. TLS 1.3 is preferred for new connections.

#### Security Headers

Every response from Nginx includes these security headers:

| Header | Value | Protection |
|--------|-------|-----------|
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains; preload` | Forces HTTPS for 2 years; enables HSTS preload list submission |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; connect-src 'self' wss://livekit.argus.ai https://api.argus.ai` | Prevents XSS, restricts resource origins |
| `X-Frame-Options` | `DENY` | Prevents clickjacking — dashboard cannot be embedded in iframes |
| `X-Content-Type-Options` | `nosniff` | Prevents MIME-type sniffing attacks |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Limits referrer leakage to same-origin only |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | Restricts browser API access (admin dashboard doesn't need camera) |
| `X-XSS-Protection` | `0` | Disables the broken legacy XSS auditor (CSP is the modern replacement) |

#### Rate Limiting Zones

Two rate-limiting zones protect against brute-force and DoS attacks:

```nginx
limit_req_zone $binary_remote_addr zone=api:10m  rate=100r/s;   # General API
limit_req_zone $binary_remote_addr zone=auth:10m rate=5r/s;     # Login endpoint only
```

Applied per-location:

```nginx
location /api/v1/auth/login {
  limit_req zone=auth burst=10 nodelay;   # 5 RPS, burst of 10 — brute-force protection
  proxy_pass http://backend:8080;
}

location /api/ {
  limit_req zone=api burst=200 nodelay;   # 100 RPS, burst of 200 — normal API traffic
  proxy_pass http://backend:8080;
}
```

When a rate limit is exceeded, Nginx returns HTTP 429 with a `Retry-After` header.

#### Traffic Routing

```nginx
# gRPC-Web (browser event streaming) → backend gRPC-Web proxy
location ~ ^/argus\.proctoring\. {
  grpc_pass grpc://backend:50051;
}

# REST Admin API → backend HTTP
location /api/ {
  proxy_pass http://backend:8080;
}

# Frontend SPA (catch-all) → Nuxt server
location / {
  proxy_pass http://frontend:3000;
  try_files $uri $uri/ /index.html;   # SPA fallback for client-side routing
}
```

---

## Operations

### First-Time Provisioning

Run these steps on a **fresh Ubuntu 22.04 LTS** server:

#### 1. Install Docker

```bash
# Install Docker Engine
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER

# Install Docker Compose v2
sudo apt-get install -y docker-compose-v2
docker compose version   # verify: Docker Compose version v2.x.x
```

#### 2. Create the Deploy Directory

```bash
sudo mkdir -p /opt/argus
sudo chown $USER:$USER /opt/argus
cd /opt/argus
```

#### 3. Copy Configuration

```bash
# Copy files from this repository
cp docker/docker-compose.yaml /opt/argus/docker-compose.yaml
cp docker/.env.example /opt/argus/.env

# Edit .env with production values
nano /opt/argus/.env
```

Fill in all `REQUIRED` variables (see [Environment Variables](#environment-variables)).

#### 4. Initialize Buckets (MinIO)

The `minio-init` service runs as a one-shot container and initializes the MinIO buckets
with Object Lock (WORM) retention policies:

```bash
docker compose up minio minio-init
# Wait for: "Bucket argus-evidence created with Object Lock enabled"
```

MinIO bucket configuration (performed by `scripts/minio-init.sh`):
- `argus-evidence` — Object Lock (GOVERNANCE mode, 365-day retention), versioning enabled
- `argus-exports` — Standard bucket, 30-day lifecycle policy (export archives expire)

#### 5. Run Database Migrations

```bash
# PostgreSQL: Run all migrations in order
for f in migrations/postgres/*.sql; do
  echo "Running $f..."
  docker compose exec postgres psql -U argus -d argus -f /dev/stdin < "$f"
done

# ClickHouse: Run analytical schema
docker compose exec clickhouse clickhouse-client \
  --multiquery < migrations/clickhouse/init.sql

# ClickHouse: Forensic Ledger extension (v2.1)
docker compose exec clickhouse clickhouse-client \
  --multiquery < migrations/clickhouse/002_forensic_ledger.sql
```

#### 6. Start All Services

```bash
cd /opt/argus
docker compose up -d
docker compose ps      # All services should show "healthy" within ~60 seconds
```

#### 7. Verify the Stack

```bash
# Backend health
curl http://localhost:8080/healthz   # → {"status":"ok"}
curl http://localhost:8080/readyz    # → {"status":"ok"} (all deps connected)

# Frontend
curl http://localhost:3000           # → HTML response

# Admin login (seed credentials from init.sql)
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+77077469966","password":"Admin1234!"}' | python3 -m json.tool
```

#### 8. Configure TLS (Production)

Option A — **Let's Encrypt with Certbot** (recommended):

```bash
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d api.argus.ai -d app.argus.ai
# Certbot automatically updates nginx.conf and sets up auto-renewal
```

Option B — **Bring your own certificates**:

```bash
mkdir -p /opt/argus/certs
# Copy your .crt and .key files:
cp your-cert.crt /opt/argus/certs/argus.crt
cp your-key.key /opt/argus/certs/argus.key

# Update nginx/nginx.conf:
# ssl_certificate     /opt/argus/certs/argus.crt;
# ssl_certificate_key /opt/argus/certs/argus.key;
```

---

### Zero-Downtime Deployment

Deployments are performed by the CI/CD pipeline in `ci/gitlab-ci.yml`.
Manual deployments follow the same steps.

#### Automated (CI/CD)

The deployment is triggered when either `argus-backend` or `argus-frontend`
publishes a new image tag. The infra pipeline:

1. Validates the Docker Compose configuration (`docker compose config --quiet`)
2. Optionally runs database migrations (manual approval required)
3. Deploys with rolling restart

#### Manual Deployment

```bash
# On the production server as the deploy user
cd /opt/argus

# 1. Update image versions in .env (or export as environment variables)
export BACKEND_TAG=v2.1.1
export FRONTEND_TAG=v2.1.1

# 2. Pull new images while old containers are still running
docker compose pull backend frontend

# 3. Rolling restart — backend first (--wait polls /healthz until healthy)
docker compose up -d --no-deps --wait backend

# 4. Restart frontend (--wait polls GET / until 200)
docker compose up -d --no-deps --wait frontend
```

**Why this is zero-downtime:**

- `docker compose pull` pre-downloads the new images while the old containers serve traffic
- `up -d --no-deps --wait` starts a new container and only stops the old one after the
  new one passes its health check (15s start period, 3 retries)
- Existing gRPC streaming connections drain within the 30-second `ShutdownTimeout`

**Rollback** (if the new image is broken):

```bash
export BACKEND_TAG=v2.1.0   # previous known-good tag
docker compose up -d --no-deps --wait backend
```

---

### Backup Strategy

#### PostgreSQL — Transactional Data

PostgreSQL contains the authoritative records for organizations, users, review decisions,
consent records, and appeals. These must never be lost.

**Automated daily backup (recommended):**

```bash
# Add to crontab: 0 2 * * * (runs at 02:00 UTC daily)
docker compose exec -T postgres pg_dump \
  -U argus -d argus --no-password \
  | gzip > /backup/postgres/argus-$(date +%Y%m%d).sql.gz

# Prune backups older than 30 days
find /backup/postgres/ -name "*.sql.gz" -mtime +30 -delete
```

**Point-in-time recovery:** Enable PostgreSQL WAL archiving for continuous backup.
The `pg_basebackup` tool creates a full base backup; WAL files allow recovery to any
point in time since the last base backup.

**Restore from backup:**

```bash
# Stop the backend (stop writes)
docker compose stop backend

# Restore
gunzip -c /backup/postgres/argus-20260115.sql.gz | \
  docker compose exec -T postgres psql -U argus -d argus

# Restart
docker compose start backend
```

#### ClickHouse — Analytical Data

ClickHouse contains billions of proctoring events. Full table backups are impractical
due to size. Use partition-level backup instead.

**Partition-level cold archive:**

```bash
# Archive last month's events partition (e.g., January 2026 = 202601)
PARTITION=$(date -d "last month" +%Y%m)

docker compose exec clickhouse clickhouse-client --query \
  "ALTER TABLE argus_analytics.proctoring_events FREEZE PARTITION '${PARTITION}'"
# Frozen partition is in: /var/lib/clickhouse/shadow/

# Copy to backup storage
docker compose exec clickhouse tar -czf - \
  /var/lib/clickhouse/shadow/ > /backup/clickhouse/events-${PARTITION}.tar.gz
```

**Kafka as replay buffer:** Kafka retains all events for **7 days**. If ClickHouse data
is corrupted, events can be replayed from Kafka to rebuild the last 7 days of analytics.
For longer recovery windows, the partition archive is authoritative.

#### MinIO — Evidence Storage

MinIO stores binary evidence files (video clips, screenshots). MinIO Object Lock
(GOVERNANCE mode) makes objects **immutable for 365 days** — they cannot be deleted
or overwritten within the retention period, even by administrators.

**Active protection:** Object Lock is the primary protection. No backup is required
during the retention period because the objects cannot be deleted.

**Cross-region replication (production recommendation):**

```bash
# Replicate argus-evidence bucket to a second MinIO instance or AWS S3
mc mirror minio-local/argus-evidence minio-backup/argus-evidence --watch
```

**Export archives** (`argus-exports` bucket) have a 30-day lifecycle policy and do not
require backup — they can be regenerated from the source evidence fragments.

#### Redis — Session State

Redis contains only transient LiveKit session state. This data is ephemeral and can be
rebuilt from active gRPC connections. No backup required.

**If Redis data is lost:** LiveKit will recreate room state on next connection. Proctoring
sessions may briefly show as "reconnecting" in the dashboard but continue uninterrupted.

---

## Environment Variables

**File:** `docker/.env.example`

Copy to `docker/.env` and fill in production values. The `.env` file is never committed to git
(enforced by `.gitignore`).

### Required Variables (must be set for the system to function)

| Variable | Format | Production Requirement |
|----------|--------|----------------------|
| `JWT_SIGNING_KEY` | Minimum 32 characters | **CRITICAL** — signs all admin JWTs. Rotate requires all users to re-login. Generate with: `openssl rand -hex 32` |
| `POSTGRES_PASSWORD` | Strong password | **CRITICAL** — PostgreSQL `argus` user password. Set once during `initdb`. Changing requires DB restart. |
| `MINIO_ROOT_PASSWORD` | Minimum 8 characters | **CRITICAL** — MinIO admin password. Loss means loss of evidence access. |
| `LIVEKIT_API_SECRET` | Minimum 32 characters | Required for WebRTC video tokens. Generate with: `openssl rand -hex 32` |

### Image Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_TAG` | `latest` | Backend image tag — set to a specific version (e.g., `v2.1.0`) in production |
| `FRONTEND_TAG` | `latest` | Frontend image tag — set to a specific version |

**Best practice:** Never use `latest` in production. Pin to a specific semantic version
(`v2.1.0`) so deployments are reproducible and rollbacks are deterministic.

### Network Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_PUBLIC_URL` | `https://api.argus.ai` | Public URL for the backend API (used by frontend as `NUXT_PUBLIC_API_BASE_URL`) |
| `FRONTEND_ORIGIN` | `https://app.argus.ai` | Frontend origin — injected into backend's CORS allowlist via `EVENT_COLLECTOR_CORS_ORIGINS` |
| `BACKEND_GRPC_PORT` | `50051` | gRPC listener port |
| `BACKEND_HTTP_PORT` | `8080` | HTTP listener port |
| `FRONTEND_PORT` | `3000` | Nuxt server port |

### Database Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_DB` | `argus` | PostgreSQL database name |
| `POSTGRES_USER` | `argus` | PostgreSQL username |
| `CH_DATABASE` | `argus_analytics` | ClickHouse database name |
| `CH_USERNAME` | `default` | ClickHouse username |
| `CH_PASSWORD` | *(empty)* | ClickHouse password (set in production) |

### Storage Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIO_ROOT_USER` | `argus-minio-admin` | MinIO admin username |
| `MINIO_BUCKET` | `argus-evidence` | Primary evidence bucket name |

### Media Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LIVEKIT_API_KEY` | `argus-dev-api-key` | LiveKit API key (public identifier) |
| `LIVEKIT_API_SECRET` | — | **Required** — LiveKit signing secret |

### Full Production `.env` Example

```env
# Image versions — ALWAYS pin to specific versions in production
BACKEND_TAG=v2.1.0
FRONTEND_TAG=v2.1.0

# ── CRITICAL SECRETS ──────────────────────────────────────────────────────────
JWT_SIGNING_KEY=a3f8b2c1d9e4f7a6b5c8d2e1f9a4b7c3d6e2f8a1b4c7d9e3f6a2b5c8d1e4f7
POSTGRES_PASSWORD=pR0d-P@ssw0rd-2026-s3cur3
MINIO_ROOT_PASSWORD=M1n10-S3cur3-P@ss-2026
LIVEKIT_API_SECRET=lk-s3cr3t-k3y-minimum-32-chars-prod-2026

# ── Network ───────────────────────────────────────────────────────────────────
BACKEND_PUBLIC_URL=https://api.argus.ai
FRONTEND_ORIGIN=https://app.argus.ai

# ── Databases ─────────────────────────────────────────────────────────────────
POSTGRES_DB=argus
POSTGRES_USER=argus
CH_DATABASE=argus_analytics
CH_USERNAME=default
CH_PASSWORD=ch-password-2026

# ── MinIO ─────────────────────────────────────────────────────────────────────
MINIO_ROOT_USER=argus-minio-admin
MINIO_BUCKET=argus-evidence
LIVEKIT_API_KEY=argus-prod-livekit

# ── Logging ───────────────────────────────────────────────────────────────────
LOG_LEVEL=info
```

---

## Database Migrations

Migrations are plain SQL files. They are **idempotent** — safe to re-run
(`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, `ON CONFLICT DO NOTHING`).

### PostgreSQL Migrations

| File | Description |
|------|-------------|
| `migrations/postgres/init.sql` | Core schema: organizations, users, api_keys, audit_log, review_decisions + seed data (super admin + 4 test orgs) |
| `migrations/postgres/002_export_jobs.sql` | Bulk export job queue (pending → processing → completed/failed) |
| `migrations/postgres/003_consent.sql` | GDPR-compliant immutable consent records (append-only) |
| `migrations/postgres/004_appeals.sql` | Student appeals state machine (submitted → under_review → upheld/overturned) |

### ClickHouse Migrations

| File | Description |
|------|-------------|
| `migrations/clickhouse/init.sql` | Core tables: `proctoring_events`, `evidence_fragments` + 5 materialized views |
| `migrations/clickhouse/002_forensic_ledger.sql` | Adds `sequence_num`, `previous_hash`, `record_hash` columns to `evidence_fragments` — the Forensic Ledger |

### Adding a New Migration

1. Create the next file: `migrations/postgres/NNN_description.sql`
2. Write idempotent SQL only (no `DROP TABLE`, no `TRUNCATE`, no destructive operations)
3. Test locally against the Docker stack
4. The CI pipeline will prompt for manual approval before running in production

---

## CI/CD Pipeline

**File:** `ci/gitlab-ci.yml`

The infra pipeline is **triggered downstream** — it runs when `argus-backend` or
`argus-frontend` publish a new image to the registry:

```
argus-backend  ──triggers──┐
                            ├──► argus-infra pipeline
argus-frontend ──triggers──┘
```

### Pipeline Stages

```
validate ──► migrate (manual) ──► deploy (manual on main)
```

| Stage | Job | Trigger | Description |
|-------|-----|---------|-------------|
| `validate` | `validate-compose` | Auto | Runs `docker compose config --quiet` — catches YAML syntax errors |
| `migrate` | `run-pg-migrations` | **Manual** | SSHes to deploy host, runs PostgreSQL SQL files in order |
| `migrate` | `run-ch-migrations` | **Manual** | SSHes to deploy host, runs ClickHouse SQL files in order |
| `deploy` | `deploy-production` | **Manual** (main branch) | Rolling restart via SSH |

### Required CI/CD Variables

Configure in **GitLab → Settings → CI/CD → Variables** (protected + masked):

| Variable | Description |
|----------|-------------|
| `DEPLOY_HOST` | Production server IP or hostname |
| `DEPLOY_USER` | SSH user with Docker access |
| `DEPLOY_SSH_KEY` | SSH private key (raw PEM or base64-encoded) |
| `JWT_SIGNING_KEY` | JWT signing secret (same as `.env`) |
| `POSTGRES_PASSWORD` | PostgreSQL password |
| `MINIO_ROOT_PASSWORD` | MinIO root password |
| `LIVEKIT_API_SECRET` | LiveKit API secret |
| `REGISTRY_HOST` | Container registry hostname (e.g., `registry.argus.ai`) |
| `BACKEND_PUBLIC_URL` | Public backend URL (optional, defaults to `https://api.argus.ai`) |
| `FRONTEND_ORIGIN` | Frontend origin (optional) |

---

## Repository Structure

```
argus-infra/
│
├── docker/
│   ├── docker-compose.yaml     ← Full 9-service stack (polyrepo model — uses pre-built images)
│   └── .env.example            ← All environment variables with descriptions
│
├── nginx/
│   └── nginx.conf              ← Reverse proxy: TLS, CORS, gRPC-Web, rate limiting, CSP
│
├── migrations/
│   ├── postgres/               ← PostgreSQL DDL migrations (ordered by prefix)
│   │   ├── init.sql            ← Core schema + seed data
│   │   ├── 002_export_jobs.sql
│   │   ├── 003_consent.sql
│   │   └── 004_appeals.sql
│   └── clickhouse/             ← ClickHouse analytical schema
│       ├── init.sql            ← Core tables + 5 materialized views
│       └── 002_forensic_ledger.sql ← SHA-256 hash chain columns
│
├── scripts/
│   └── minio-init.sh           ← Idempotent MinIO bucket initialization (Object Lock, WORM)
│
├── ci/
│   └── gitlab-ci.yml           ← GitLab CI/CD pipeline (validate → migrate → deploy)
│
├── README.md                   ← This file
└── .gitignore                  ← Excludes *.env, *.pem, *.key, nginx/certs/
```

---

## Related Repositories

| Repository | Path | Purpose |
|------------|------|---------|
| argus-backend | `~/Desktop/argus_ai/argus-backend` | Go event-collector: gRPC + REST API + Forensic Ledger |
| argus-frontend | `~/Desktop/argus_ai/argus-frontend` | Nuxt 4 admin dashboard SPA |
| **argus-infra** | `~/Desktop/argus_ai/argus-infra` | **This repo** — Orchestration, operations, security |

**API Contract:** `argus-backend/api/openapi.yaml` — the single source of truth for all REST endpoints.
