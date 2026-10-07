#!/usr/bin/env bash
# =============================================================================
#  Argus AI — Database Backup Script
#
#  Backs up:
#    1. PostgreSQL (pg_dump → gzip)
#    2. ClickHouse (clickhouse-backup or native clickhouse-client export)
#
#  Usage:
#    ./backup.sh [--dev|--prod] [--dest /path/to/backups]
#
#  Cron example (daily at 02:00):
#    0 2 * * * /opt/argus/infra/scripts/backup.sh --prod >> /var/log/argus-backup.log 2>&1
#
#  Restore PostgreSQL:
#    gunzip -c backup.sql.gz | docker exec -i argus-postgres psql -U argus argus
#
#  Restore ClickHouse:
#    gunzip -c ch-argus_analytics-YYYYMMDD.sql.gz | \
#      docker exec -i argus-clickhouse clickhouse-client --database argus_analytics
# =============================================================================

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
PROFILE="dev"
BACKUP_DIR="${BACKUP_DIR:-/tmp/argus-backups}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"

PG_CONTAINER="${PG_CONTAINER:-argus-dev-postgres}"
PG_USER="${POSTGRES_USER:-argus}"
PG_DB="${POSTGRES_DB:-argus}"

CH_CONTAINER="${CH_CONTAINER:-argus-dev-clickhouse}"
CH_DB="${CLICKHOUSE_DB:-argus_analytics}"

# Parse args
for arg in "$@"; do
  case $arg in
    --prod)    PROFILE="prod"; PG_CONTAINER="argus-postgres"; CH_CONTAINER="argus-clickhouse" ;;
    --dev)     PROFILE="dev" ;;
    --dest=*)  BACKUP_DIR="${arg#*=}" ;;
  esac
done

TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
mkdir -p "$BACKUP_DIR"

echo "========================================"
echo " Argus Backup — $(date)"
echo " Profile : $PROFILE"
echo " Dest    : $BACKUP_DIR"
echo "========================================"

# ── PostgreSQL backup ─────────────────────────────────────────────────────────
PG_FILE="$BACKUP_DIR/postgres-${PG_DB}-${TIMESTAMP}.sql.gz"

echo "[1/2] PostgreSQL → $PG_FILE"

if docker ps --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
  docker exec "$PG_CONTAINER" \
    pg_dump -U "$PG_USER" -d "$PG_DB" --format=plain --no-owner --no-acl \
    | gzip > "$PG_FILE"
  echo "  ✓ PostgreSQL backup: $(du -sh "$PG_FILE" | cut -f1)"
else
  echo "  ✗ Container '$PG_CONTAINER' not running — skipping PostgreSQL backup"
fi

# ── ClickHouse backup ─────────────────────────────────────────────────────────
CH_FILE="$BACKUP_DIR/clickhouse-${CH_DB}-${TIMESTAMP}.sql.gz"

echo "[2/2] ClickHouse → $CH_FILE"

if docker ps --format '{{.Names}}' | grep -q "^${CH_CONTAINER}$"; then
  # Export all tables as CREATE + INSERT statements using clickhouse-client
  docker exec "$CH_CONTAINER" \
    clickhouse-client \
      --database "$CH_DB" \
      --query "SHOW TABLES" \
    | while read -r table; do
        echo "-- Table: $table"
        docker exec "$CH_CONTAINER" \
          clickhouse-client --database "$CH_DB" \
          --query "SHOW CREATE TABLE $table"
      done \
    | gzip > "$CH_FILE"
  echo "  ✓ ClickHouse DDL backup: $(du -sh "$CH_FILE" | cut -f1)"
  echo "  ℹ  Note: ClickHouse data is in MergeTree — for full data backup"
  echo "     use 'BACKUP TABLE ... TO ...' or clickhouse-backup tool."
else
  echo "  ✗ Container '$CH_CONTAINER' not running — skipping ClickHouse backup"
fi

# ── MinIO backup (metadata only) ──────────────────────────────────────────────
echo "  ℹ  MinIO: use 'mc mirror' for evidence data backup to S3-compatible storage."

# ── Prune old backups ─────────────────────────────────────────────────────────
echo ""
echo "Pruning backups older than $RETENTION_DAYS days..."
find "$BACKUP_DIR" -name "*.sql.gz" -mtime "+$RETENTION_DAYS" -delete -print \
  | sed 's/^/  deleted: /'

echo ""
echo "Backup complete. Files in $BACKUP_DIR:"
ls -lh "$BACKUP_DIR"/*.sql.gz 2>/dev/null || echo "  (none)"
echo "========================================"
