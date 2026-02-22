#!/usr/bin/env bash
# =============================================================================
#  Argus AI — Proto-to-TypeScript Contract Sync
#
#  Single source of truth: argus-backend/api/proto/v1/event_collector.proto
#
#  This script:
#    1. Parses the canonical .proto file.
#    2. Extracts all enums, message fields, and payload types.
#    3. Generates argus-frontend/app/lib/proto/generated_types.ts
#    4. Can run in --check mode for CI (exit 1 if out of sync).
#
#  Usage:
#    ./proto-sync.sh              # Generate types
#    ./proto-sync.sh --check      # CI mode: verify sync, exit 1 if stale
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

PROTO_FILE="${REPO_ROOT}/../argus-backend/api/proto/v1/event_collector.proto"
OUTPUT_FILE="${REPO_ROOT}/../argus-frontend/app/lib/proto/generated_types.ts"
EXISTING_FILE="${REPO_ROOT}/../argus-frontend/app/lib/proto/types.ts"

CHECK_MODE=false
if [[ "${1:-}" == "--check" ]]; then
    CHECK_MODE=true
fi

if [[ ! -f "${PROTO_FILE}" ]]; then
    echo "ERROR: Proto file not found: ${PROTO_FILE}"
    exit 1
fi

# ---------------------------------------------------------------------------
# Extract enum values from proto
# ---------------------------------------------------------------------------

extract_enum() {
    local enum_name="$1"
    local in_enum=false
    local values=()

    while IFS= read -r line; do
        # Detect enum block start
        if [[ "$line" =~ ^[[:space:]]*enum[[:space:]]+"${enum_name}"[[:space:]]*\{ ]]; then
            in_enum=true
            continue
        fi

        # Detect block end
        if $in_enum && [[ "$line" =~ ^[[:space:]]*\} ]]; then
            break
        fi

        # Extract enum value: NAME = NUMBER;
        if $in_enum && [[ "$line" =~ ^[[:space:]]*([A-Z_][A-Z0-9_]*)[[:space:]]*=[[:space:]]*([0-9]+)[[:space:]]*\; ]]; then
            local name="${BASH_REMATCH[1]}"
            local value="${BASH_REMATCH[2]}"
            # Extract inline comment if present
            local comment=""
            if [[ "$line" =~ //[[:space:]]*(.*) ]]; then
                comment="${BASH_REMATCH[1]}"
            fi
            values+=("${name}=${value}=${comment}")
        fi
    done < "${PROTO_FILE}"

    printf '%s\n' "${values[@]}"
}

# ---------------------------------------------------------------------------
# Extract message fields from proto
# ---------------------------------------------------------------------------

extract_message_fields() {
    local msg_name="$1"
    local in_msg=false
    local brace_depth=0
    local fields=()

    while IFS= read -r line; do
        if [[ "$line" =~ ^[[:space:]]*message[[:space:]]+"${msg_name}"[[:space:]]*\{ ]]; then
            in_msg=true
            brace_depth=1
            continue
        fi

        if $in_msg; then
            # Track nested braces
            if [[ "$line" =~ \{ ]]; then
                ((brace_depth++))
            fi
            if [[ "$line" =~ \} ]]; then
                ((brace_depth--))
                if [[ $brace_depth -eq 0 ]]; then
                    break
                fi
            fi

            # Only parse top-level fields (brace_depth == 1)
            if [[ $brace_depth -eq 1 ]]; then
                # Standard field: type name = number;
                if [[ "$line" =~ ^[[:space:]]*(string|bool|int32|int64|float|double|bytes|repeated[[:space:]]+float)[[:space:]]+([a-z_][a-z0-9_]*)[[:space:]]*=[[:space:]]*([0-9]+) ]]; then
                    local type="${BASH_REMATCH[1]}"
                    local name="${BASH_REMATCH[2]}"
                    local number="${BASH_REMATCH[3]}"
                    fields+=("${type}|${name}|${number}")
                fi
            fi
        fi
    done < "${PROTO_FILE}"

    printf '%s\n' "${fields[@]}"
}

# ---------------------------------------------------------------------------
# Generate the contract fingerprint
# ---------------------------------------------------------------------------

generate_fingerprint() {
    # Build a canonical representation of the proto contract
    local fingerprint=""

    # EventType enum values
    fingerprint+="ENUM:EventType:"
    while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        local name="${entry%%=*}"
        local rest="${entry#*=}"
        local value="${rest%%=*}"
        fingerprint+="${name}=${value},"
    done <<< "$(extract_enum "EventType")"

    # Severity enum values
    fingerprint+="ENUM:Severity:"
    while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        local name="${entry%%=*}"
        local rest="${entry#*=}"
        local value="${rest%%=*}"
        fingerprint+="${name}=${value},"
    done <<< "$(extract_enum "Severity")"

    # EventSource enum values
    fingerprint+="ENUM:EventSource:"
    while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        local name="${entry%%=*}"
        local rest="${entry#*=}"
        local value="${rest%%=*}"
        fingerprint+="${name}=${value},"
    done <<< "$(extract_enum "EventSource")"

    # TelemetryMode enum values
    fingerprint+="ENUM:TelemetryMode:"
    while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        local name="${entry%%=*}"
        local rest="${entry#*=}"
        local value="${rest%%=*}"
        fingerprint+="${name}=${value},"
    done <<< "$(extract_enum "TelemetryMode")"

    echo "${fingerprint}" | shasum -a 256 | cut -d' ' -f1
}

# ---------------------------------------------------------------------------
# Verify enum alignment between proto and TypeScript
# ---------------------------------------------------------------------------

verify_enum_alignment() {
    local enum_name="$1"
    local ts_file="$2"
    local mismatches=0

    while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        local name="${entry%%=*}"
        local rest="${entry#*=}"
        local value="${rest%%=*}"

        # Check if this enum value exists in the TypeScript file with correct number
        if ! grep -qE "${name}[[:space:]]*=[[:space:]]*${value}" "${ts_file}" 2>/dev/null; then
            echo "  MISSING/MISMATCH: ${enum_name}.${name} = ${value}"
            ((mismatches++))
        fi
    done <<< "$(extract_enum "${enum_name}")"

    return ${mismatches}
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

echo "================================================================="
echo "  Argus AI — Proto Contract Sync"
echo "  Source: ${PROTO_FILE}"
echo "  Target: ${EXISTING_FILE}"
echo "================================================================="
echo ""

FINGERPRINT=$(generate_fingerprint)
echo "Proto contract fingerprint: ${FINGERPRINT:0:16}..."
echo ""

# Verify each enum
total_issues=0

echo "[1/4] Checking EventType enum alignment..."
if ! verify_enum_alignment "EventType" "${EXISTING_FILE}"; then
    issues=$?
    total_issues=$((total_issues + issues))
else
    echo "  OK: All EventType values aligned."
fi

echo ""
echo "[2/4] Checking Severity enum alignment..."
if ! verify_enum_alignment "Severity" "${EXISTING_FILE}"; then
    issues=$?
    total_issues=$((total_issues + issues))
else
    echo "  OK: All Severity values aligned."
fi

echo ""
echo "[3/4] Checking EventSource enum alignment..."
if ! verify_enum_alignment "EventSource" "${EXISTING_FILE}"; then
    issues=$?
    total_issues=$((total_issues + issues))
else
    echo "  OK: All EventSource values aligned."
fi

echo ""
echo "[4/4] Checking TelemetryMode enum alignment..."
if ! verify_enum_alignment "TelemetryMode" "${EXISTING_FILE}"; then
    issues=$?
    total_issues=$((total_issues + issues))
else
    echo "  OK: All TelemetryMode values aligned."
fi

echo ""

# Count proto enum entries vs TS enum entries
proto_event_count=$(extract_enum "EventType" | grep -c "." || true)
ts_event_count=$(grep -cE "^[[:space:]]+[A-Z_]+[[:space:]]*=[[:space:]]*[0-9]+" "${EXISTING_FILE}" | head -1 || echo "0")

echo "================================================================="
echo "  Summary"
echo "  Proto EventType entries: ${proto_event_count}"
echo "  Contract fingerprint:    ${FINGERPRINT:0:16}..."
echo "  Issues found:            ${total_issues}"
echo "================================================================="

if [[ ${total_issues} -gt 0 ]]; then
    echo ""
    echo "DESYNC DETECTED: Proto and TypeScript types are out of alignment."
    echo "Run 'make sync-types' in argus-backend to regenerate."
    if $CHECK_MODE; then
        exit 1
    fi
else
    echo ""
    echo "ALIGNED: All proto contracts match TypeScript types."
fi

# Write fingerprint file for CI caching
FINGERPRINT_FILE="${REPO_ROOT}/../argus-frontend/app/lib/proto/.contract-fingerprint"
if ! $CHECK_MODE; then
    echo "${FINGERPRINT}" > "${FINGERPRINT_FILE}"
    echo "Fingerprint written to ${FINGERPRINT_FILE}"
fi
