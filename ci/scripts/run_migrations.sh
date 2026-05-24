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

ssh "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST" "ARGUS_INFRA_DEPLOY_PATH='$ARGUS_INFRA_DEPLOY_PATH' sh -s" <<'REMOTE_SCRIPT'
set -eu

MIGRATIONS="$ARGUS_INFRA_DEPLOY_PATH/migrations"

container_exists() {
  docker ps --format '{{.Names}}' | grep -qx "$1"
}

echo "=== PostgreSQL Migrations ==="
if container_exists argus-postgres; then
  pg_container=argus-postgres
elif container_exists argus-db; then
  pg_container=argus-db
else
  echo "ERROR: no supported PostgreSQL container found" >&2
  exit 1
fi

pg_user="$(docker exec "$pg_container" sh -c 'echo ${POSTGRES_USER:-argus}')"
pg_db="$(docker exec "$pg_container" sh -c 'echo ${POSTGRES_DB:-argus_db}')"

set -- "$MIGRATIONS"/postgres/*.up.sql
if [ ! -e "$1" ]; then
  set -- "$MIGRATIONS"/postgres/*.sql
fi
if [ -e "$1" ]; then
  for f do
    echo "  -> $(basename "$f")"
    docker exec -i "$pg_container" psql -U "$pg_user" -d "$pg_db" < "$f"
  done
else
  echo "  No PostgreSQL migration files found"
fi

echo ""
echo "=== ClickHouse Migrations ==="
if container_exists argus-clickhouse-1; then
  ch_container=argus-clickhouse-1
elif container_exists argus-clickhouse; then
  ch_container=argus-clickhouse
else
  echo "ERROR: no supported ClickHouse container found" >&2
  exit 1
fi

set -- "$MIGRATIONS"/clickhouse/*.up.sql
if [ ! -e "$1" ]; then
  set -- "$MIGRATIONS"/clickhouse/*.sql
fi
if [ -e "$1" ]; then
  for f do
    echo "  -> $(basename "$f")"
    docker exec -i "$ch_container" clickhouse-client --database argus_analytics --multiquery < "$f"
  done
else
  echo "  No ClickHouse migration files found"
fi

echo ""
echo "Migrations complete."
REMOTE_SCRIPT