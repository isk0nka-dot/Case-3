#!/usr/bin/env bash
# =============================================================================
# T4: Redis Durability Kill Test — Integration Script
#
# Full integration test for Redis AOF+RDB persistence:
#   1. Start Redis via docker-compose
#   2. Write 100 test keys (simulating in-flight asynq jobs)
#   3. docker stop redis (simulating crash)
#   4. docker start redis (AOF replay)
#   5. Verify all 100 keys are recovered (zero data loss)
#
# Prerequisites:
#   - Docker and docker-compose installed
#   - Run from argus-infra directory: ./scripts/test_redis_durability.sh
#
# Exit codes:
#   0 — all checks passed
#   1 — test failure (data loss or misconfiguration)
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INFRA_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_FILE="$INFRA_DIR/docker/docker-compose.yaml"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[T4]${NC} $*"; }
warn() { echo -e "${YELLOW}[T4]${NC} $*"; }
fail() { echo -e "${RED}[T4-FAIL]${NC} $*"; exit 1; }

# ── Phase 0: Check prerequisites ─────────────────────────────────────────
command -v docker >/dev/null 2>&1 || fail "docker is not installed"
command -v docker compose >/dev/null 2>&1 && COMPOSE_CMD="docker compose" || {
    command -v docker-compose >/dev/null 2>&1 && COMPOSE_CMD="docker-compose" || fail "docker compose is not installed"
}

log "Phase 0: Prerequisites OK"
log "  Compose file: $COMPOSE_FILE"
log "  Compose command: $COMPOSE_CMD"

# ── Phase 1: Start Redis ─────────────────────────────────────────────────
log "Phase 1: Starting Redis..."
$COMPOSE_CMD -f "$COMPOSE_FILE" up -d redis 2>/dev/null

# Wait for Redis to be healthy.
for i in $(seq 1 30); do
    if docker exec argus-redis redis-cli ping 2>/dev/null | grep -q PONG; then
        break
    fi
    if [ "$i" -eq 30 ]; then
        fail "Redis did not become healthy within 30 seconds"
    fi
    sleep 1
done
log "  Redis is healthy (PONG received)"

# Verify AOF is enabled.
AOF_STATUS=$(docker exec argus-redis redis-cli CONFIG GET appendonly 2>/dev/null | tail -1)
if [ "$AOF_STATUS" != "yes" ]; then
    fail "AOF is NOT enabled: appendonly=$AOF_STATUS"
fi
log "  AOF enabled: $AOF_STATUS"

# Verify noeviction policy.
EVICTION=$(docker exec argus-redis redis-cli CONFIG GET maxmemory-policy 2>/dev/null | tail -1)
if [ "$EVICTION" != "noeviction" ]; then
    fail "Eviction policy is NOT noeviction: $EVICTION"
fi
log "  Eviction policy: $EVICTION"

# ── Phase 2: Write test data ─────────────────────────────────────────────
NUM_KEYS=100
log "Phase 2: Writing $NUM_KEYS test keys..."

for i in $(seq 1 $NUM_KEYS); do
    PAYLOAD="{\"task_id\":\"test-$i\",\"type\":\"durability_test\",\"data\":\"payload-$i\"}"
    docker exec argus-redis redis-cli SET "asynq:t4:task:$i" "$PAYLOAD" >/dev/null 2>&1
done

# Verify all keys were written.
WRITTEN=$(docker exec argus-redis redis-cli KEYS "asynq:t4:task:*" 2>/dev/null | wc -l | tr -d ' ')
if [ "$WRITTEN" -ne "$NUM_KEYS" ]; then
    fail "Only $WRITTEN/$NUM_KEYS keys written"
fi
log "  Written: $WRITTEN/$NUM_KEYS keys"

# Force AOF rewrite to ensure all data is persisted.
docker exec argus-redis redis-cli BGREWRITEAOF >/dev/null 2>&1
sleep 2
log "  BGREWRITEAOF triggered"

# ── Phase 3: Kill Redis (simulate crash) ──────────────────────────────────
log "Phase 3: Killing Redis (docker stop)..."
docker stop argus-redis >/dev/null 2>&1
sleep 2
log "  Redis stopped"

# ── Phase 4: Restart Redis (AOF replay) ───────────────────────────────────
log "Phase 4: Restarting Redis (docker start)..."
docker start argus-redis >/dev/null 2>&1

# Wait for Redis to be healthy again.
for i in $(seq 1 30); do
    if docker exec argus-redis redis-cli ping 2>/dev/null | grep -q PONG; then
        break
    fi
    if [ "$i" -eq 30 ]; then
        fail "Redis did not recover within 30 seconds"
    fi
    sleep 1
done
log "  Redis restarted and healthy"

# ── Phase 5: Verify data recovery ────────────────────────────────────────
log "Phase 5: Verifying data recovery..."

RECOVERED=$(docker exec argus-redis redis-cli KEYS "asynq:t4:task:*" 2>/dev/null | wc -l | tr -d ' ')

if [ "$RECOVERED" -ne "$NUM_KEYS" ]; then
    LOST=$((NUM_KEYS - RECOVERED))
    fail "DATA LOSS: $RECOVERED/$NUM_KEYS keys recovered ($LOST keys LOST)"
fi

# Verify payload integrity for a sample of keys.
INTEGRITY_FAILURES=0
for i in 1 25 50 75 100; do
    EXPECTED="{\"task_id\":\"test-$i\",\"type\":\"durability_test\",\"data\":\"payload-$i\"}"
    ACTUAL=$(docker exec argus-redis redis-cli GET "asynq:t4:task:$i" 2>/dev/null)
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        warn "  Payload mismatch for key $i: expected='$EXPECTED', got='$ACTUAL'"
        INTEGRITY_FAILURES=$((INTEGRITY_FAILURES + 1))
    fi
done

if [ "$INTEGRITY_FAILURES" -gt 0 ]; then
    fail "INTEGRITY FAILURE: $INTEGRITY_FAILURES payload mismatches"
fi

log "  Recovered: $RECOVERED/$NUM_KEYS keys (ZERO data loss)"
log "  Payload integrity: 5/5 sampled keys match"

# ── Phase 6: Cleanup ─────────────────────────────────────────────────────
log "Phase 6: Cleaning up test keys..."
for i in $(seq 1 $NUM_KEYS); do
    docker exec argus-redis redis-cli DEL "asynq:t4:task:$i" >/dev/null 2>&1
done
log "  Cleanup complete"

# ── Summary ───────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  ✅ T4-PASS: Redis Durability Kill Test PASSED${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "  Keys written before crash:     $NUM_KEYS"
echo "  Keys recovered after restart:  $RECOVERED"
echo "  Data loss:                     0"
echo "  AOF status:                    $AOF_STATUS"
echo "  Eviction policy:               $EVICTION"
echo "  Payload integrity:             5/5 verified"
echo ""
