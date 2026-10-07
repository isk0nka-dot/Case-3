#!/bin/sh
# =============================================================================
#  MinIO Bucket Initialization Script
#  Creates the evidence bucket with S3 Object Lock (WORM) for legal compliance.
#
#  This script runs as a one-shot init container (minio-init) after MinIO
#  starts. It is idempotent — safe to run multiple times.
#
#  Object Lock (WORM) guarantees:
#  - Evidence fragments cannot be deleted or overwritten during retention
#  - GOVERNANCE mode: only root credentials can override (audit-logged)
#  - 365-day retention: meets Tier-1 enterprise legal compliance
# =============================================================================

set -e

MINIO_ALIAS="argus"
ENDPOINT="http://${MINIO_HOST:-minio}:${MINIO_PORT:-9000}"

echo "================================================================="
echo "  MinIO Bucket Initialization"
echo "  Endpoint: ${ENDPOINT}"
echo "  Bucket:   ${BUCKET_NAME:-argus-evidence}"
echo "================================================================="

# ── Step 1: Wait for MinIO to be fully ready ────────────────────────────────
echo "[1/4] Waiting for MinIO to be ready..."
until mc alias set "${MINIO_ALIAS}" "${ENDPOINT}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" 2>/dev/null; do
    echo "  MinIO not ready yet, retrying in 2s..."
    sleep 2
done
echo "  MinIO connection established."

# ── Step 2: Create evidence bucket with Object Lock ─────────────────────────
BUCKET="${BUCKET_NAME:-argus-evidence}"
if mc ls "${MINIO_ALIAS}/${BUCKET}" > /dev/null 2>&1; then
    echo "[2/4] Bucket '${BUCKET}' already exists. Skipping creation."
else
    echo "[2/4] Creating bucket '${BUCKET}' with Object Lock enabled..."
    mc mb "${MINIO_ALIAS}/${BUCKET}" --with-lock
    echo "  Bucket created with S3 Object Lock (WORM) support."
fi

# ── Step 3: Set GOVERNANCE retention policy (365 days) ──────────────────────
echo "[3/4] Setting GOVERNANCE retention policy (365 days)..."
mc retention set --default GOVERNANCE 365d "${MINIO_ALIAS}/${BUCKET}" || \
    echo "  Warning: retention policy may already be set or Object Lock not supported in dev mode."

# ── Step 4: Set bucket versioning (required for Object Lock) ────────────────
echo "[4/4] Ensuring bucket versioning is enabled..."
mc version enable "${MINIO_ALIAS}/${BUCKET}" 2>/dev/null || \
    echo "  Versioning already enabled or not required."

# ── Step 5: Create exports bucket (for bulk evidence archives) ────────────
EXPORTS_BUCKET="argus-exports"
if mc ls "${MINIO_ALIAS}/${EXPORTS_BUCKET}" > /dev/null 2>&1; then
    echo "[5/5] Bucket '${EXPORTS_BUCKET}' already exists. Skipping creation."
else
    echo "[5/5] Creating exports bucket '${EXPORTS_BUCKET}' with Object Lock enabled..."
    mc mb "${MINIO_ALIAS}/${EXPORTS_BUCKET}" --with-lock
    mc retention set --default GOVERNANCE 30d "${MINIO_ALIAS}/${EXPORTS_BUCKET}" || \
        echo "  Warning: retention policy may already be set."
    mc version enable "${MINIO_ALIAS}/${EXPORTS_BUCKET}" 2>/dev/null || \
        echo "  Versioning already enabled."
    echo "  Exports bucket created."
fi

# ── Step 6: Set 30-day ILM expiration lifecycle on exports bucket ───────────
echo "[6/6] Setting 30-day ILM expiration lifecycle on exports bucket..."
mc ilm rule add --expiry-days 30 "${MINIO_ALIAS}/${EXPORTS_BUCKET}" || \
    echo "  Warning: ILM lifecycle rule may already exist or is not supported."

echo ""
echo "================================================================="
echo "  MinIO initialization complete."
echo "  Bucket: ${BUCKET}"
echo "  Exports: ${EXPORTS_BUCKET}"
echo "  Object Lock: GOVERNANCE mode"
echo "  Exports lifecycle: 30-day auto-expiration"
echo "  S3 endpoint: ${ENDPOINT}"
echo "================================================================="
