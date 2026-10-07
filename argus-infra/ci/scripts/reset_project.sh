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
ARGUS_RESET_DATA="${ARGUS_RESET_DATA:-false}"
ARGUS_RESET_CONFIG="${ARGUS_RESET_CONFIG:-false}"

[ -n "$ARGUS_DEPLOY_HOST" ] || fail "ARGUS_DEPLOY_HOST is required"
[ -n "$ARGUS_DEPLOY_USER" ] || fail "ARGUS_DEPLOY_USER is required"
[ "${ARGUS_CONFIRM_RESET:-}" = "DELETE_ARGUS_PROJECT" ] || fail "set ARGUS_CONFIRM_RESET=DELETE_ARGUS_PROJECT to run this destructive job"

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

echo "============================================================"
echo "Resetting Argus project namespace on $ARGUS_DEPLOY_HOST"
echo "Project root: $ARGUS_PROJECT_ROOT"
echo "Reset data volumes: $ARGUS_RESET_DATA"
echo "Reset config files: $ARGUS_RESET_CONFIG"
echo "============================================================"

ssh "$ARGUS_DEPLOY_USER@$ARGUS_DEPLOY_HOST" \
  "ARGUS_PROJECT_ROOT='$ARGUS_PROJECT_ROOT' ARGUS_INFRA_DEPLOY_PATH='$ARGUS_INFRA_DEPLOY_PATH' ARGUS_RESET_DATA='$ARGUS_RESET_DATA' ARGUS_RESET_CONFIG='$ARGUS_RESET_CONFIG' sh -s" <<'REMOTE_SCRIPT'
set -eu

case "$ARGUS_PROJECT_ROOT" in
  ""|"/"|"/opt"|"/home"|"/home/deploy"|"/tmp"|"/var"|"/usr"|"/etc")
    echo "ERROR: unsafe ARGUS_PROJECT_ROOT: $ARGUS_PROJECT_ROOT" >&2
    exit 1
    ;;
esac

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  elif command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
  else
    return 127
  fi
}

if [ -d "$ARGUS_INFRA_DEPLOY_PATH/docker" ]; then
  cd "$ARGUS_INFRA_DEPLOY_PATH/docker"
  compose -f docker-compose.yaml -f docker-compose.prod.yml down --remove-orphans >/dev/null 2>&1 || true
fi

for id in $(docker ps -aq --filter "name=^/argus-" 2>/dev/null || true); do
  docker rm -f "$id" >/dev/null 2>&1 || true
done

if [ "$ARGUS_RESET_DATA" = "true" ]; then
  for volume in $(docker volume ls -q --filter "name=argus-" 2>/dev/null || true); do
    docker volume rm "$volume" >/dev/null 2>&1 || true
  done
fi

preserve="$ARGUS_PROJECT_ROOT/.ci-preserve"
rm -rf "$preserve"
mkdir -p "$preserve"

if [ "$ARGUS_RESET_CONFIG" != "true" ]; then
  if [ -f "$ARGUS_INFRA_DEPLOY_PATH/docker/.env" ]; then
    cp "$ARGUS_INFRA_DEPLOY_PATH/docker/.env" "$preserve/env"
  fi
  if [ -d "$ARGUS_INFRA_DEPLOY_PATH/docker/keys" ]; then
    mkdir -p "$preserve/keys"
    cp -a "$ARGUS_INFRA_DEPLOY_PATH/docker/keys/." "$preserve/keys/"
  fi
fi

rm -rf \
  "$ARGUS_PROJECT_ROOT/argus-backend" \
  "$ARGUS_PROJECT_ROOT/argus-frontend" \
  "$ARGUS_PROJECT_ROOT/argus-infra"

mkdir -p \
  "$ARGUS_PROJECT_ROOT/argus-backend" \
  "$ARGUS_PROJECT_ROOT/argus-frontend" \
  "$ARGUS_INFRA_DEPLOY_PATH/docker" \
  "$ARGUS_INFRA_DEPLOY_PATH/nginx" \
  "$ARGUS_INFRA_DEPLOY_PATH/migrations" \
  "$ARGUS_INFRA_DEPLOY_PATH/scripts"

if [ -f "$preserve/env" ]; then
  mv "$preserve/env" "$ARGUS_INFRA_DEPLOY_PATH/docker/.env"
fi
if [ -d "$preserve/keys" ]; then
  mkdir -p "$ARGUS_INFRA_DEPLOY_PATH/docker/keys"
  cp -a "$preserve/keys/." "$ARGUS_INFRA_DEPLOY_PATH/docker/keys/"
fi
rm -rf "$preserve"

echo "Argus project namespace reset complete."
REMOTE_SCRIPT

echo "============================================================"
echo "RESET SUCCESS"
echo "============================================================"
