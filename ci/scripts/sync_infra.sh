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
ARGUS_PROJECT_ROOT="${ARGUS_PROJECT_ROOT:-${DEPLOY_PATH:-/opt/argus-ai}}"
ARGUS_INFRA_DEPLOY_PATH="${ARGUS_INFRA_DEPLOY_PATH:-$ARGUS_PROJECT_ROOT/argus-infra}"

[ -n "$ARGUS_DEPLOY_HOST" ] || fail "ARGUS_DEPLOY_HOST is required"
[ -n "$ARGUS_DEPLOY_USER" ] || fail "ARGUS_DEPLOY_USER is required"

safe_abs_path "ARGUS_PROJECT_ROOT" "$ARGUS_PROJECT_ROOT"
safe_abs_path "ARGUS_INFRA_DEPLOY_PATH" "$ARGUS_INFRA_DEPLOY_PATH"

require_cmd ssh
require_cmd ssh-agent
require_cmd ssh-add
require_cmd ssh-keygen
require_cmd ssh-keyscan
require_cmd rsync

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
echo "Syncing Argus infrastructure to $ARGUS_DEPLOY_HOST:$ARGUS_INFRA_DEPLOY_PATH"
echo "Commit: ${CI_COMMIT_SHORT_SHA:-local}"
echo "============================================================"

ssh "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST" "mkdir -p '$ARGUS_INFRA_DEPLOY_PATH/docker/clickhouse' '$ARGUS_INFRA_DEPLOY_PATH/nginx' '$ARGUS_INFRA_DEPLOY_PATH/migrations' '$ARGUS_INFRA_DEPLOY_PATH/scripts'"

echo "Syncing docker compose and configs..."
rsync -azO --exclude='.env' --exclude='keys/' \
  docker/docker-compose.yaml \
  docker/docker-compose.prod.yml \
  docker/docker-compose.argus-webizon.prod.yml \
  docker/.env.argus-webizon.example \
  docker/redis.conf \
  "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST:$ARGUS_INFRA_DEPLOY_PATH/docker/"

rsync -azO \
  docker/clickhouse/ \
  "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST:$ARGUS_INFRA_DEPLOY_PATH/docker/clickhouse/"

echo "Syncing nginx..."
rsync -azO \
  nginx/ \
  "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST:$ARGUS_INFRA_DEPLOY_PATH/nginx/"

echo "Syncing migrations..."
rsync -azO \
  migrations/ \
  "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST:$ARGUS_INFRA_DEPLOY_PATH/migrations/"

echo "Syncing scripts..."
rsync -azO \
  scripts/ \
  "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST:$ARGUS_INFRA_DEPLOY_PATH/scripts/"

ssh "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST" "chmod +x '$ARGUS_INFRA_DEPLOY_PATH'/scripts/*.sh 2>/dev/null || true"

echo "============================================================"
echo "INFRA SYNC SUCCESS"
echo "============================================================"
