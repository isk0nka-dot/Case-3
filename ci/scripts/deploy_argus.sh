#!/bin/sh
set -eu

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

safe_abs_path() {
  name="$1"
  value="$2"

  [ -n "$value" ] || fail "$name is empty"
  case "$value" in
    /*) ;;
    *) fail "$name must be an absolute path: $value" ;;
  esac
  case "$value" in
    "/"|"/opt"|"/home"|"/home/deploy"|"/tmp"|"/var"|"/usr"|"/etc")
      fail "$name is too broad: $value"
      ;;
  esac
  case "$value" in
    *".."*|*" "*|*"'"*|*'"'*|*";"*|*"|"*|*"&"*|*'$'*|*"("*|*")"*|*"["*|*"]"*|*"{"*|*"}"*|*"!"*|*"*"*|*"?"*)
      fail "$name contains unsafe characters: $value"
      ;;
  esac
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

materialize_ssh_key() {
  raw_key="${DEPLOY_SSH_KEY:-${ARGUS_DEPLOY_SSH_KEY:-${SSH_PRIVATE_KEY:-}}}"
  [ -n "$raw_key" ] || fail "SSH key is required. Set DEPLOY_SSH_KEY, ARGUS_DEPLOY_SSH_KEY, or SSH_PRIVATE_KEY"

  if [ -r "$raw_key" ]; then
    printf '%s\n' "$raw_key"
    return 0
  fi

  key_file="$(mktemp)"
  case "$raw_key" in
    *"\\n"*) printf '%b' "$raw_key" ;;
    *) printf '%s\n' "$raw_key" ;;
  esac | sed 's/\r$//' > "$key_file"
  chmod 600 "$key_file"
  printf '%s\n' "$key_file"
}

ARGUS_DEPLOY_HOST="${ARGUS_DEPLOY_HOST:-${DEPLOY_HOST:-}}"
ARGUS_DEPLOY_USER="${ARGUS_DEPLOY_USER:-${DEPLOY_USER:-}}"
ARGUS_PROJECT_ROOT="${ARGUS_PROJECT_ROOT:-/opt/argus-ai}"
ARGUS_BACKEND_DEPLOY_PATH="${ARGUS_BACKEND_DEPLOY_PATH:-$ARGUS_PROJECT_ROOT/argus-backend}"

[ -n "$ARGUS_DEPLOY_HOST" ] || fail "DEPLOY_HOST or ARGUS_DEPLOY_HOST is required"
[ -n "$ARGUS_DEPLOY_USER" ] || fail "DEPLOY_USER or ARGUS_DEPLOY_USER is required"

safe_abs_path "ARGUS_PROJECT_ROOT" "$ARGUS_PROJECT_ROOT"
safe_abs_path "ARGUS_BACKEND_DEPLOY_PATH" "$ARGUS_BACKEND_DEPLOY_PATH"

require_cmd ssh
require_cmd ssh-agent
require_cmd ssh-add
require_cmd ssh-keygen
require_cmd ssh-keyscan

SSH_KEY_FILE="$(materialize_ssh_key)"
REMOVE_SSH_KEY_FILE="false"
if [ -r "$SSH_KEY_FILE" ] && [ "${SSH_KEY_FILE#/tmp/}" != "$SSH_KEY_FILE" ]; then
  REMOVE_SSH_KEY_FILE="true"
fi

eval "$(ssh-agent -s)"
trap 'ssh-agent -k >/dev/null 2>&1 || true; if [ "${REMOVE_SSH_KEY_FILE:-false}" = "true" ]; then rm -f "${SSH_KEY_FILE:-}" 2>/dev/null || true; fi' EXIT

chmod 600 "$SSH_KEY_FILE"
ssh-keygen -lf "$SSH_KEY_FILE"
ssh-add "$SSH_KEY_FILE"
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
ssh-keyscan -H "$ARGUS_DEPLOY_HOST" >> "$HOME/.ssh/known_hosts" 2>/dev/null

echo "Checking SSH access to $ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST..."
ssh -o BatchMode=yes -o StrictHostKeyChecking=yes "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST" "printf '%s\n' ssh-ok"

echo "============================================================"
echo "Deploying Argus stack on $ARGUS_DEPLOY_HOST"
echo "Path: $ARGUS_BACKEND_DEPLOY_PATH"
echo "Commit: ${CI_COMMIT_SHORT_SHA:-local}"
echo "============================================================"

ssh "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST" \
  "ARGUS_BACKEND_DEPLOY_PATH='$ARGUS_BACKEND_DEPLOY_PATH' sh -s" <<'REMOTE_SCRIPT'
set -eu

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  elif command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
  else
    echo "ERROR: docker compose is not installed" >&2
    exit 1
  fi
}

cd "$ARGUS_BACKEND_DEPLOY_PATH"

echo "Restarting Argus app services (no rebuild)..."
compose up -d --no-build --remove-orphans app worker inference ai-sidecar

echo "Reloading nginx config..."
sudo nginx -s reload 2>/dev/null || true

echo "Waiting for backend health..."
for i in $(seq 1 20); do
  sleep 5
  health="$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' argus-backend-app 2>/dev/null || echo 'missing')"
  if [ "$health" = "healthy" ] || [ "$health" = "no-healthcheck" ]; then
    echo "Backend health: $health"
    exit 0
  fi
  echo "Attempt $i/20: $health"
done

echo "DEPLOY FAILED: argus-backend-app did not become healthy"
docker logs --tail=50 argus-backend-app 2>&1 || true
exit 1
REMOTE_SCRIPT

echo "============================================================"
echo "ARGUS DEPLOY SUCCESS (${CI_COMMIT_SHORT_SHA:-local})"
echo "============================================================"
