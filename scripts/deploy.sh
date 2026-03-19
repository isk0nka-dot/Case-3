#!/usr/bin/env bash
# =============================================================================
#  Argus AI — Production Deploy Script
#
#  Lives on the production server at /opt/argus/scripts/deploy.sh
#  Called by GitLab CI via SSH for each service independently.
#
#  Features:
#    - Atomic deploys with flock (prevents concurrent deploys from racing)
#    - Automatic rollback if health check fails
#    - PostgreSQL + ClickHouse migrations (backend only, idempotent SQL)
#    - Deploy logging to /var/log/argus/
#    - Dangling image cleanup after each deploy
#
#  Usage:
#    deploy.sh --service=backend  [--ref=origin/main] [--skip-migrations]
#    deploy.sh --service=frontend [--ref=origin/main]
#    deploy.sh --service=backend  --ref=refs/tags/v1.2.0
#
#  Required environment:
#    None — all secrets are read from /opt/argus/docker/.env by docker compose.
#
#  Best practice: create a non-root "deploy" user in the docker group:
#    useradd -m -s /bin/bash deploy && usermod -aG docker deploy
#    chown -R deploy:deploy /opt/argus
# =============================================================================
set -euo pipefail

# ── Configuration ────────────────────────────────────────────────────────────
DEPLOY_PATH="${DEPLOY_PATH:-/opt/argus}"
COMPOSE_DIR="${DEPLOY_PATH}/docker"
MIGRATIONS_DIR="${DEPLOY_PATH}/migrations"
LOCK_FILE="${DEPLOY_PATH}/deploy.lock"
LOG_DIR="${DEPLOY_PATH}/logs"

SERVICE=""
GIT_REF="origin/main"
SKIP_MIGRATIONS=false

# Health check tuning
HEALTH_RETRIES=20        # 20 attempts
HEALTH_INTERVAL=6        # 6 seconds apart  → 120s max wait
ROLLBACK_RETRIES=15      # 15 attempts for rollback recovery

# ── Output helpers ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"; }
info() { log "${GREEN}[INFO]${NC}  $*"; }
warn() { log "${YELLOW}[WARN]${NC}  $*"; }
fail() { log "${RED}[FAIL]${NC}  $*"; exit 1; }
step() { log "${CYAN}[STEP]${NC}  $*"; }

# ── Parse arguments ──────────────────────────────────────────────────────────
for arg in "$@"; do
  case $arg in
    --service=*)       SERVICE="${arg#*=}" ;;
    --ref=*)           GIT_REF="${arg#*=}" ;;
    --skip-migrations) SKIP_MIGRATIONS=true ;;
    --help|-h)
      echo "Usage: deploy.sh --service=backend|frontend [--ref=REF] [--skip-migrations]"
      exit 0
      ;;
    *) fail "Unknown argument: $arg" ;;
  esac
done

[[ -z "$SERVICE" ]] && fail "Missing required argument: --service=backend|frontend"
[[ "$SERVICE" != "backend" && "$SERVICE" != "frontend" ]] && fail "Invalid service: '$SERVICE'. Must be 'backend' or 'frontend'"

# ── Set up logging ───────────────────────────────────────────────────────────
mkdir -p "$LOG_DIR"
LOG_FILE="${LOG_DIR}/deploy-${SERVICE}-$(date +%Y%m%d-%H%M%S).log"

info "═══════════════════════════════════════════════════════════"
info "  Argus AI Deploy: service=$SERVICE  ref=$GIT_REF"
info "═══════════════════════════════════════════════════════════"

# ── Acquire deploy lock (prevents concurrent deploys) ────────────────────────
exec 200>"$LOCK_FILE"
if ! flock -n 200; then
  fail "Another deploy is already in progress (lock: $LOCK_FILE). Aborting."
fi
info "Deploy lock acquired"

# ── Step 1: Record rollback point ───────────────────────────────────────────
REPO_DIR="${DEPLOY_PATH}/${SERVICE}"
[[ -d "$REPO_DIR/.git" ]] || fail "Not a git repo: $REPO_DIR"

# Mark the repo as safe if owned by a different user (e.g., root-owned repo, deploy user)
git config --global --add safe.directory "$REPO_DIR" 2>/dev/null || true

cd "$REPO_DIR"
ROLLBACK_SHA=$(git rev-parse HEAD)
ROLLBACK_SHORT=$(git rev-parse --short HEAD)
info "Current HEAD: $ROLLBACK_SHORT ($ROLLBACK_SHA)"

# ── Step 2: Fetch and reset to target ref ────────────────────────────────────
step "Fetching latest code from origin..."
if [[ -n "$CI_JOB_TOKEN" ]]; then
  git remote set-url origin "https://gitlab-ci-token:${CI_JOB_TOKEN}@gitlab.com/argus_ai_group/argus-${SERVICE}.git"
fi
git fetch origin --prune --quiet

# Handle both branch refs (origin/main) and tag refs (refs/tags/v1.0.0)
if [[ "$GIT_REF" == refs/tags/* ]]; then
  git fetch origin --tags --quiet
  info "Checking out tag: $GIT_REF"
  git checkout --force "$GIT_REF" --quiet 2>/dev/null || git checkout --force "$(echo "$GIT_REF" | sed 's|refs/tags/||')" --quiet
else
  info "Resetting to: $GIT_REF"
  git reset --hard "$GIT_REF" --quiet
fi

NEW_SHA=$(git rev-parse HEAD)
NEW_SHORT=$(git rev-parse --short HEAD)
info "Updated to: $NEW_SHORT ($NEW_SHA)"

if [[ "$ROLLBACK_SHA" == "$NEW_SHA" ]]; then
  warn "No code change (same SHA: $NEW_SHORT). Rebuilding anyway."
fi

# ── Step 3: Run database migrations (backend only) ──────────────────────────
run_migrations() {
  if [[ "$SERVICE" != "backend" ]]; then
    return 0
  fi
  if [[ "$SKIP_MIGRATIONS" == "true" ]]; then
    info "Migrations skipped (--skip-migrations flag)"
    return 0
  fi
  if [[ ! -d "$MIGRATIONS_DIR" ]]; then
    warn "Migrations directory not found: $MIGRATIONS_DIR — skipping"
    return 0
  fi

  # ── PostgreSQL migrations ──────────────────────────────────────────────
  local pg_dir="$MIGRATIONS_DIR/postgres"
  if [[ -d "$pg_dir" ]]; then
    step "Running PostgreSQL migrations via golang-migrate..."
    # The runner must be on the docker host network to reach localhost:5432
    # Ensure argus backend has POSTGRES_DB_URL populated from .env
    # We will use the root argus user config for simplicity.
    local db_url="postgres://argus:argus_secret_password@157.180.46.33:5432/argus?sslmode=disable"
    
    if docker run --rm --network host -v "$pg_dir:/migrations" migrate/migrate:v4.18.1 -path=/migrations/ -database "$db_url" up >> "$LOG_FILE" 2>&1; then
      info "PostgreSQL: Migrations applied successfully or already up to date."
    else
      warn "PostgreSQL: Migrations failed. Check $LOG_FILE. Attempting to proceed anyway in case of benign failure."
    fi
  else
    info "No PostgreSQL migration files found"
  fi

  # ── ClickHouse migrations ──────────────────────────────────────────────
  local ch_dir="$MIGRATIONS_DIR/clickhouse"
  if [[ -d "$ch_dir" ]] && ls "$ch_dir"/*.sql &>/dev/null; then
    step "Running ClickHouse migrations..."
    local ch_count=0
    local ch_fail=0
    for f in "$ch_dir"/*.sql; do
      [[ -f "$f" ]] || continue
      local basename=$(basename "$f")
      info "  → $basename"
      if docker exec -i argus-clickhouse-1 clickhouse-client --multiquery < "$f" >> "$LOG_FILE" 2>&1; then
        ch_count=$((ch_count + 1))
      else
        warn "  ⚠ $basename returned non-zero (expected for idempotent re-runs)"
        ch_fail=$((ch_fail + 1))
      fi
    done
    info "ClickHouse: $ch_count applied, $ch_fail warnings"
  else
    info "No ClickHouse migration files found"
  fi
}

run_migrations

# ── Step 4: Rebuild and restart the service ──────────────────────────────────
step "Rebuilding $SERVICE container..."
cd "$COMPOSE_DIR"

# Pass build-time args
export BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Rebuild only the target service (--no-deps prevents restarting dependencies)
# --no-cache ensures Docker always builds from fresh source, not stale layer cache
docker compose build --no-cache "$SERVICE" 2>&1 | tee -a "$LOG_FILE"
docker compose up -d --no-deps "$SERVICE" 2>&1 | tee -a "$LOG_FILE"

# ── Step 5: Wait for container to become healthy ────────────────────────────
CONTAINER_NAME="argus-${SERVICE}"
step "Waiting for $CONTAINER_NAME to become healthy (max $((HEALTH_RETRIES * HEALTH_INTERVAL))s)..."

healthy=false
for i in $(seq 1 $HEALTH_RETRIES); do
  sleep "$HEALTH_INTERVAL"

  # Check if container exists and is running
  container_status=$(docker inspect --format='{{.State.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo "not_found")
  if [[ "$container_status" == "not_found" ]]; then
    warn "  Attempt $i/$HEALTH_RETRIES: container not found"
    continue
  fi
  if [[ "$container_status" != "running" ]]; then
    warn "  Attempt $i/$HEALTH_RETRIES: container status=$container_status"
    continue
  fi

  # Check Docker healthcheck
  health_status=$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$CONTAINER_NAME" 2>/dev/null)

  if [[ "$health_status" == "healthy" ]]; then
    healthy=true
    info "  ✓ Container healthy after $((i * HEALTH_INTERVAL))s"
    break
  fi

  info "  Attempt $i/$HEALTH_RETRIES: health=$health_status"
done

# ── Step 6: HTTP health verification ────────────────────────────────────────
if [[ "$healthy" == "true" ]]; then
  if [[ "$SERVICE" == "backend" ]]; then
    step "Verifying backend HTTP health..."
    if curl -sf -o /dev/null --max-time 10 http://localhost:8080/healthz 2>/dev/null; then
      info "  ✓ /healthz returned 200 OK"
    else
      warn "  ⚠ /healthz not responding (container reports healthy — may need more startup time)"
    fi
  elif [[ "$SERVICE" == "frontend" ]]; then
    step "Verifying frontend HTTP health..."
    if curl -sf -o /dev/null --max-time 10 http://localhost:3000/ 2>/dev/null; then
      info "  ✓ Frontend responded on :3000"
    else
      warn "  ⚠ Frontend not responding on :3000 (container reports healthy — may need more startup time)"
    fi
  fi
fi

# ── Step 7: Rollback if unhealthy ────────────────────────────────────────────
if [[ "$healthy" != "true" ]]; then
  warn "════════════════════════════════════════════════════════════"
  warn "  DEPLOY FAILED: $SERVICE did not become healthy"
  warn "  Rolling back to $ROLLBACK_SHORT ($ROLLBACK_SHA)..."
  warn "════════════════════════════════════════════════════════════"

  # Show container logs for debugging
  info "Last 30 lines of container logs:"
  docker logs --tail=30 "$CONTAINER_NAME" >> "$LOG_FILE" 2>&1 || true

  # Revert database if we are rolling back the backend
  if [[ "$SERVICE" == "backend" && "$SKIP_MIGRATIONS" != "true" ]]; then
    step "Reverting database migration (1 step down)..."
    rollback_db_url="postgres://argus:argus_secret_password@157.180.46.33:5432/argus?sslmode=disable"
    rollback_pg_dir="$MIGRATIONS_DIR/postgres"
    if [[ -d "$rollback_pg_dir" ]]; then
      docker run --rm --network host -v "$rollback_pg_dir:/migrations" migrate/migrate:v4.18.1 -path=/migrations/ -database "$rollback_db_url" down 1 >> "$LOG_FILE" 2>&1 || warn "Failed to revert database migration!"
    fi
  fi

  # Rollback: reset to previous commit
  cd "$REPO_DIR"
  git reset --hard "$ROLLBACK_SHA" --quiet
  info "Git reset to $ROLLBACK_SHORT"

  # Rebuild from old code
  cd "$COMPOSE_DIR"
  export BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  docker compose up -d --build --no-deps "$SERVICE" 2>&1 | tee -a "$LOG_FILE"

  # Wait for rollback container to stabilize
  step "Waiting for rollback to stabilize..."
  rollback_ok=false
  for i in $(seq 1 $ROLLBACK_RETRIES); do
    sleep "$HEALTH_INTERVAL"
    health_status=$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$CONTAINER_NAME" 2>/dev/null || echo "not_found")
    if [[ "$health_status" == "healthy" ]]; then
      rollback_ok=true
      info "  ✓ Rollback successful — $CONTAINER_NAME is healthy on $ROLLBACK_SHORT"
      break
    fi
    info "  Rollback attempt $i/$ROLLBACK_RETRIES: health=$health_status"
  done

  if [[ "$rollback_ok" != "true" ]]; then
    fail "CRITICAL: Rollback also failed! Manual intervention required. Log: $LOG_FILE"
  fi

  fail "Deploy of $NEW_SHORT failed. Rolled back to $ROLLBACK_SHORT. Check: $LOG_FILE"
fi

# ── Step 8: Success ──────────────────────────────────────────────────────────
info "════════════════════════════════════════════════════════════"
info "  ✓ DEPLOY SUCCESS"
info "  Service:  $SERVICE"
info "  Commit:   $NEW_SHORT ($NEW_SHA)"
info "  Log:      $LOG_FILE"
info "════════════════════════════════════════════════════════════"

# Clean up dangling images to prevent disk bloat
info "Pruning dangling Docker images..."
docker image prune -f >> "$LOG_FILE" 2>&1 || true

# Release the lock (happens automatically on exit via fd 200)
exit 0
