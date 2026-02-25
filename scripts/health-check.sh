#!/usr/bin/env bash
# =============================================================================
#  Argus AI — Local Stack Health Check
#  Verifies all 9 Docker containers + Backend API + Frontend are operational.
# =============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

PASS=0
FAIL=0
WARN=0

pass() { echo -e "  ${GREEN}✓${NC} $1"; ((PASS++)); }
fail() { echo -e "  ${RED}✗${NC} $1"; ((FAIL++)); }
warn() { echo -e "  ${YELLOW}!${NC} $1"; ((WARN++)); }

# ─── 1. Docker Containers ────────────────────────────────────────────────────
echo -e "\n${BOLD}${CYAN}═══ Argus AI — Health Check ═══${NC}\n"
echo -e "${BOLD}[1/3] Docker Containers${NC}"

EXPECTED_CONTAINERS=(
  "argus-kafka"
  "argus-clickhouse"
  "argus-postgres"
  "argus-redis"
  "argus-minio"
  "argus-livekit"
)

# minio-init is a one-shot; we check it completed rather than running
ONESHOT_CONTAINERS=(
  "argus-minio-init"
)

for cname in "${EXPECTED_CONTAINERS[@]}"; do
  status=$(docker inspect --format='{{.State.Status}}' "$cname" 2>/dev/null || echo "not_found")
  health=$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$cname" 2>/dev/null || echo "unknown")

  if [[ "$status" == "running" && "$health" == "healthy" ]]; then
    pass "$cname — running (healthy)"
  elif [[ "$status" == "running" ]]; then
    warn "$cname — running (health: $health)"
  else
    fail "$cname — $status"
  fi
done

for cname in "${ONESHOT_CONTAINERS[@]}"; do
  status=$(docker inspect --format='{{.State.Status}}' "$cname" 2>/dev/null || echo "not_found")
  exit_code=$(docker inspect --format='{{.State.ExitCode}}' "$cname" 2>/dev/null || echo "unknown")

  if [[ "$status" == "exited" && "$exit_code" == "0" ]]; then
    pass "$cname — completed (exit 0)"
  elif [[ "$status" == "not_found" ]]; then
    warn "$cname — not found (may not have run yet)"
  else
    fail "$cname — status=$status exit=$exit_code"
  fi
done

# Also check for optional app containers (if running via docker-compose full stack)
for cname in "argus-backend" "argus-frontend"; do
  status=$(docker inspect --format='{{.State.Status}}' "$cname" 2>/dev/null || echo "not_found")
  if [[ "$status" == "running" ]]; then
    pass "$cname — running (docker)"
  fi
done

# ─── 2. Backend API ──────────────────────────────────────────────────────────
echo -e "\n${BOLD}[2/3] Backend API (localhost:8080)${NC}"

if curl -sf -o /dev/null --max-time 5 http://localhost:8080/healthz 2>/dev/null; then
  pass "/healthz — OK"
else
  fail "/healthz — not responding"
fi

if curl -sf -o /dev/null --max-time 5 http://localhost:8080/readyz 2>/dev/null; then
  pass "/readyz — OK (all dependencies connected)"
else
  warn "/readyz — not responding (dependencies may be initializing)"
fi

# Quick check: login endpoint exists (should return 4xx without body, not 404)
login_status=$(curl -sf -o /dev/null -w "%{http_code}" --max-time 5 \
  -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{}' 2>/dev/null || echo "000")

if [[ "$login_status" != "000" && "$login_status" != "404" ]]; then
  pass "/api/v1/auth/login — reachable (HTTP $login_status)"
else
  fail "/api/v1/auth/login — not reachable (HTTP $login_status)"
fi

# ─── 3. Frontend ──────────────────────────────────────────────────────────────
echo -e "\n${BOLD}[3/3] Frontend (localhost:3000)${NC}"

if curl -sf -o /dev/null --max-time 5 http://localhost:3000 2>/dev/null; then
  pass "http://localhost:3000 — accessible"
else
  fail "http://localhost:3000 — not responding"
fi

# ─── 4. Service Connectivity Probes ──────────────────────────────────────────
echo -e "\n${BOLD}[Bonus] Direct Service Probes${NC}"

# PostgreSQL
if docker exec argus-postgres pg_isready -U argus -d argus -q 2>/dev/null; then
  pass "PostgreSQL — accepting connections"
else
  fail "PostgreSQL — not ready"
fi

# ClickHouse
if docker exec argus-clickhouse clickhouse-client --query "SELECT 1" >/dev/null 2>&1; then
  pass "ClickHouse — query OK"
else
  fail "ClickHouse — not responding"
fi

# Kafka
if docker exec argus-kafka /opt/kafka/bin/kafka-broker-api-versions.sh \
  --bootstrap-server localhost:9092 >/dev/null 2>&1; then
  pass "Kafka — broker responsive"
else
  fail "Kafka — broker not responding"
fi

# Redis
if docker exec argus-redis redis-cli ping 2>/dev/null | grep -q PONG; then
  pass "Redis — PONG"
else
  fail "Redis — not responding"
fi

# MinIO
if curl -sf -o /dev/null --max-time 5 http://localhost:9001 2>/dev/null; then
  pass "MinIO Console — accessible at :9001"
else
  warn "MinIO Console — not reachable on host (may be internal only)"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}${CYAN}─── Summary ───${NC}"
echo -e "  ${GREEN}Passed: $PASS${NC}  ${RED}Failed: $FAIL${NC}  ${YELLOW}Warnings: $WARN${NC}"

if [[ $FAIL -eq 0 ]]; then
  echo -e "\n  ${GREEN}${BOLD}🟢 Argus AI stack is operational!${NC}\n"
  exit 0
else
  echo -e "\n  ${RED}${BOLD}🔴 $FAIL check(s) failed. See above for details.${NC}\n"
  exit 1
fi
