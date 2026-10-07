#!/usr/bin/env bash
# =============================================================================
#  Argus AI — Master Reset Script
#  Full system recovery via admin API. Resets circuit breakers, flushes
#  overflow queues, and optionally clears the DLQ.
#
#  Usage:
#    ./scripts/master-reset.sh
#    ./scripts/master-reset.sh --clear-dlq
#    BACKEND_URL=http://prod:8080 ./scripts/master-reset.sh
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Colors & Helpers
# ---------------------------------------------------------------------------

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

ok()   { echo -e "  ${GREEN}✓${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; }
info() { echo -e "  ${CYAN}ℹ${NC} $1"; }

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

BACKEND="${BACKEND_URL:-http://localhost:8080}"
CLEAR_DLQ=false

for arg in "$@"; do
  case "$arg" in
    --clear-dlq) CLEAR_DLQ=true ;;
    --help|-h)
      echo "Usage: $0 [--clear-dlq]"
      echo ""
      echo "Options:"
      echo "  --clear-dlq    Also clear the dead-letter queue (requires confirmation)"
      echo ""
      echo "Environment:"
      echo "  BACKEND_URL    Backend URL (default: http://localhost:8080)"
      exit 0
      ;;
  esac
done

echo -e "\n${BOLD}${CYAN}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${CYAN}║         ARGUS AI — MASTER RESET SCRIPT          ║${NC}"
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════╝${NC}\n"

info "Backend: ${BACKEND}"

# ---------------------------------------------------------------------------
# Step 1: Authenticate
# ---------------------------------------------------------------------------

echo -e "\n${BOLD}Step 1/5: Authentication${NC}"

login_resp=$(curl -sf --max-time 10 \
  -X POST "${BACKEND}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"phone":"+77077469966","password":"Astana01+"}' 2>/dev/null || echo "FAIL")

if [[ "$login_resp" == "FAIL" ]]; then
  fail "Cannot reach backend at ${BACKEND}"
  exit 1
fi

JWT=$(echo "$login_resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || echo "")

if [[ -z "$JWT" ]]; then
  fail "Authentication failed — no token returned"
  exit 1
fi

ok "Authenticated (token: ${JWT:0:20}...)"

AUTH_HEADER="Authorization: Bearer ${JWT}"

# ---------------------------------------------------------------------------
# Step 2: Pre-Reset Health Snapshot
# ---------------------------------------------------------------------------

echo -e "\n${BOLD}Step 2/5: Pre-Reset Health Snapshot${NC}"

pre_health=$(curl -sf --max-time 10 \
  -H "${AUTH_HEADER}" \
  "${BACKEND}/api/v1/admin/system-health" 2>/dev/null || echo "FAIL")

if [[ "$pre_health" == "FAIL" ]]; then
  fail "Cannot fetch system health"
  exit 1
fi

kafka_state=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('kafka_breaker_state','unknown'))" 2>/dev/null)
ch_state=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('ch_breaker_state','unknown'))" 2>/dev/null)
dlq_size=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('kafka_dlq_size',0))" 2>/dev/null)
overflow=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('ch_overflow_size',0))" 2>/dev/null)
backpressure=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('ch_backpressure_active',False))" 2>/dev/null)
degraded=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('kafka_is_degraded',False))" 2>/dev/null)
goroutines=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('goroutine_count',0))" 2>/dev/null)
mem_mb=$(echo "$pre_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('mem_alloc_mb',0))" 2>/dev/null)

echo -e "  ${CYAN}Kafka Breaker:${NC}       ${kafka_state}"
echo -e "  ${CYAN}CH Breaker:${NC}          ${ch_state}"
echo -e "  ${CYAN}DLQ Size:${NC}            ${dlq_size}"
echo -e "  ${CYAN}CH Overflow:${NC}         ${overflow}"
echo -e "  ${CYAN}Backpressure:${NC}        ${backpressure}"
echo -e "  ${CYAN}Kafka Degraded:${NC}      ${degraded}"
echo -e "  ${CYAN}Goroutines:${NC}          ${goroutines}"
echo -e "  ${CYAN}Memory (alloc):${NC}      ${mem_mb} MB"

# Check if system is already healthy
if [[ "$kafka_state" == "closed" && "$ch_state" == "closed" && "$overflow" == "0" && "$dlq_size" == "0" ]]; then
  ok "System is already healthy — no reset needed"
  exit 0
fi

warn "System needs recovery"

# ---------------------------------------------------------------------------
# Step 3: Reset Circuit Breakers + Flush Overflow
# ---------------------------------------------------------------------------

echo -e "\n${BOLD}Step 3/5: Reset Circuit Breakers & Flush Overflow${NC}"

reset_resp=$(curl -sf --max-time 15 \
  -X POST "${BACKEND}/api/v1/admin/system-reset" \
  -H "${AUTH_HEADER}" \
  -H "Content-Type: application/json" \
  -d '{"actions":["reset_kafka_breaker","reset_ch_breaker","flush_overflow"]}' 2>/dev/null || echo "FAIL")

if [[ "$reset_resp" == "FAIL" ]]; then
  fail "Reset request failed"
else
  ok "Reset command sent: reset_kafka_breaker, reset_ch_breaker, flush_overflow"
  info "Response: ${reset_resp}"
fi

# ---------------------------------------------------------------------------
# Step 4: Optional DLQ Clear
# ---------------------------------------------------------------------------

echo -e "\n${BOLD}Step 4/5: DLQ Management${NC}"

if [[ "$CLEAR_DLQ" == "true" ]]; then
  if [[ "$dlq_size" == "0" ]]; then
    info "DLQ is empty — nothing to clear"
  else
    echo -e "  ${YELLOW}⚠  WARNING: This will discard ${dlq_size} undelivered events!${NC}"
    read -p "  Clear DLQ? [y/N]: " confirm
    if [[ "$confirm" == "y" || "$confirm" == "Y" ]]; then
      dlq_resp=$(curl -sf --max-time 15 \
        -X POST "${BACKEND}/api/v1/admin/system-reset" \
        -H "${AUTH_HEADER}" \
        -H "Content-Type: application/json" \
        -d '{"actions":["clear_dlq"]}' 2>/dev/null || echo "FAIL")

      if [[ "$dlq_resp" == "FAIL" ]]; then
        fail "DLQ clear failed"
      else
        ok "DLQ cleared"
      fi
    else
      info "DLQ clear skipped by user"
    fi
  fi
else
  info "DLQ clear not requested (use --clear-dlq flag)"
fi

# ---------------------------------------------------------------------------
# Step 5: Post-Reset Verification
# ---------------------------------------------------------------------------

echo -e "\n${BOLD}Step 5/5: Post-Reset Verification${NC}"

MAX_ATTEMPTS=10
SLEEP_BETWEEN=3

for i in $(seq 1 $MAX_ATTEMPTS); do
  post_health=$(curl -sf --max-time 10 \
    -H "${AUTH_HEADER}" \
    "${BACKEND}/api/v1/admin/system-health" 2>/dev/null || echo "FAIL")

  if [[ "$post_health" == "FAIL" ]]; then
    warn "Attempt ${i}/${MAX_ATTEMPTS}: Cannot fetch health"
    sleep $SLEEP_BETWEEN
    continue
  fi

  p_kafka=$(echo "$post_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('kafka_breaker_state','unknown'))" 2>/dev/null)
  p_ch=$(echo "$post_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('ch_breaker_state','unknown'))" 2>/dev/null)
  p_overflow=$(echo "$post_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('ch_overflow_size',-1))" 2>/dev/null)
  p_dlq=$(echo "$post_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('kafka_dlq_size',-1))" 2>/dev/null)

  echo -e "  Attempt ${i}/${MAX_ATTEMPTS}: kafka=${p_kafka} ch=${p_ch} overflow=${p_overflow} dlq=${p_dlq}"

  if [[ "$p_kafka" == "closed" && "$p_ch" == "closed" && "$p_overflow" == "0" ]]; then
    echo ""
    ok "System fully recovered!"
    echo ""

    # Print final health
    echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${GREEN}║            RECOVERY COMPLETE                    ║${NC}"
    echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  ${CYAN}Kafka Breaker:${NC}   ${GREEN}${p_kafka}${NC}"
    echo -e "  ${CYAN}CH Breaker:${NC}      ${GREEN}${p_ch}${NC}"
    echo -e "  ${CYAN}CH Overflow:${NC}     ${GREEN}${p_overflow}${NC}"
    echo -e "  ${CYAN}DLQ Size:${NC}        ${p_dlq}"

    p_goroutines=$(echo "$post_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('goroutine_count',0))" 2>/dev/null)
    p_mem=$(echo "$post_health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('mem_alloc_mb',0))" 2>/dev/null)
    echo -e "  ${CYAN}Goroutines:${NC}      ${p_goroutines}"
    echo -e "  ${CYAN}Memory:${NC}          ${p_mem} MB"
    echo ""
    exit 0
  fi

  sleep $SLEEP_BETWEEN
done

echo ""
fail "Recovery incomplete after $((MAX_ATTEMPTS * SLEEP_BETWEEN)) seconds"
echo -e "  Manual intervention may be required."
echo -e "  Check backend logs: ${CYAN}docker logs argus-backend --tail 100${NC}"
exit 1
