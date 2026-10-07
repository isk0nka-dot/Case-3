#!/usr/bin/env bash
# =============================================================================
# Generate RSA-4096 Key Pair for JWT RS256 Authentication
#
# This script generates a 4096-bit RSA key pair for Argus JWT authentication:
#   - private.pem: RSA private key (API server ONLY — signs tokens)
#   - public.pem:  RSA public key  (API server + workers — verifies tokens)
#
# Security model:
#   The API server holds both keys (sign + verify).
#   Workers hold ONLY the public key (verify only).
#   A compromised worker CANNOT forge admin tokens.
#
# Usage:
#   ./scripts/generate_jwt_keys.sh                  # Output to ./docker/keys/
#   ./scripts/generate_jwt_keys.sh /path/to/keys    # Output to custom dir
#
# The generated keys are mounted into containers via docker-compose.yaml:
#   - backend:  ./keys:/app/keys:ro
#
# CRITICAL: In production, use a secrets manager (Vault, AWS KMS, etc.)
#           instead of filesystem-based keys. This script is for development
#           and initial deployment only.
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INFRA_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default output directory is docker/keys (mounted by docker-compose).
OUTPUT_DIR="${1:-$INFRA_DIR/docker/keys}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[JWT-KEYGEN]${NC} $*"; }
warn() { echo -e "${YELLOW}[JWT-KEYGEN]${NC} $*"; }
fail() { echo -e "${RED}[JWT-KEYGEN]${NC} $*"; exit 1; }

# Check prerequisites.
command -v openssl >/dev/null 2>&1 || fail "openssl is not installed"

# Create output directory.
mkdir -p "$OUTPUT_DIR"

PRIVATE_KEY="$OUTPUT_DIR/private.pem"
PUBLIC_KEY="$OUTPUT_DIR/public.pem"

# Check for existing keys.
if [ -f "$PRIVATE_KEY" ] || [ -f "$PUBLIC_KEY" ]; then
    warn "Existing keys found in $OUTPUT_DIR"
    warn "  Private: $([ -f "$PRIVATE_KEY" ] && echo "EXISTS" || echo "missing")"
    warn "  Public:  $([ -f "$PUBLIC_KEY" ] && echo "EXISTS" || echo "missing")"
    read -p "Overwrite? (y/N) " -r
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log "Aborted. Existing keys preserved."
        exit 0
    fi
fi

# Generate RSA-4096 private key.
log "Generating RSA-4096 private key..."
openssl genpkey -algorithm RSA -out "$PRIVATE_KEY" -pkeyopt rsa_keygen_bits:4096 2>/dev/null
chmod 600 "$PRIVATE_KEY"
log "  Private key: $PRIVATE_KEY (600 permissions)"

# Extract public key.
log "Extracting RSA public key..."
openssl rsa -pubout -in "$PRIVATE_KEY" -out "$PUBLIC_KEY" 2>/dev/null
chmod 644 "$PUBLIC_KEY"
log "  Public key:  $PUBLIC_KEY (644 permissions)"

# Verify key pair.
log "Verifying key pair..."
TEST_DATA="argus-jwt-keypair-verification-$(date +%s)"
SIGNATURE=$(echo -n "$TEST_DATA" | openssl dgst -sha256 -sign "$PRIVATE_KEY" | base64)
echo -n "$TEST_DATA" | openssl dgst -sha256 -verify "$PUBLIC_KEY" -signature <(echo "$SIGNATURE" | base64 -d) >/dev/null 2>&1 \
    || fail "Key pair verification FAILED — keys are corrupted"
log "  Key pair verification: PASSED"

# Display key info.
KEY_BITS=$(openssl rsa -in "$PRIVATE_KEY" -text -noout 2>/dev/null | head -1 | grep -oE '[0-9]+')

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  ✅ RSA JWT Key Pair Generated Successfully${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "  Key size:      ${KEY_BITS:-4096} bits"
echo "  Algorithm:     RS256 (RSA-SHA256)"
echo "  Private key:   $PRIVATE_KEY"
echo "  Public key:    $PUBLIC_KEY"
echo ""
echo "  Docker Compose env vars:"
echo "    EVENT_COLLECTOR_JWT_PRIVATE_KEY_PATH=/app/keys/private.pem"
echo "    EVENT_COLLECTOR_JWT_PUBLIC_KEY_PATH=/app/keys/public.pem"
echo ""
echo -e "  ${YELLOW}⚠ SECURITY: Never commit private.pem to version control.${NC}"
echo -e "  ${YELLOW}  Add 'docker/keys/' to .gitignore.${NC}"
echo ""
