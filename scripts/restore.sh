#!/usr/bin/env bash
# scripts/restore.sh — Restore a dt-camera backup tar.gz.
#
# Usage:
#   ./scripts/restore.sh --from /var/backups/dt-camera-20260101T000000Z.tar.gz
#   ./scripts/restore.sh --from ./backups/foo.tar.gz --yes       # skip confirmations
#   ./scripts/restore.sh --from ./backups/foo.tar.gz --dry-run  # verify integrity only
#
# Safety:
#   - The script REFUSES to run unless you pass --yes (or pipe YES below)
#   - The destination database must already exist; the script will NOT
#     recreate the cluster
#   - The script applies pg_restore with --clean --if-exists so any
#     conflicting tables in the destination are dropped first
#   - The script verifies the SHA-256 of the embedded pg.dump against the
#     manifest's pg_dump.sha256 sidecar
#   - The script always restores into a transaction and rolls back on error
#   - The script compares the backup's schema_version against the running
#     app's latest migration and WARNS (but does not refuse) if the
#     backup is older
#
# What is NOT restored:
#   - Video files. Use your regular backup tool (rsync, btrfs send, EBS
#     snapshot, ...) to restore recording_folders before pointing the new
#     deployment at them.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

FROM=""
ASSUME_YES=0
DRY_RUN=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --from) FROM="$2"; shift 2 ;;
    --yes|-y) ASSUME_YES=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help)
      sed -n '2,30p' "$0"
      exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

require_var() {
  local name="$1"
  local value="${!name:-}"
  if [[ -z "$value" ]]; then
    echo "error: required env var $name is not set" >&2
    exit 2
  fi
}

require_var POSTGRES_DB
require_var POSTGRES_USER
require_var POSTGRES_PASSWORD

if [[ -z "$FROM" ]]; then
  echo "error: --from <path-to-tar.gz> is required" >&2
  exit 2
fi
if [[ ! -f "$FROM" ]]; then
  echo "error: backup file not found: $FROM" >&2
  exit 2
fi

PG_HOST="${POSTGRES_HOST:-127.0.0.1}"
PG_PORT="${POSTGRES_PORT:-5432}"

log() { echo "[restore] $*"; }

# --- Pre-flight integrity check --------------------------------------------------

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

log "verifying backup: $FROM"
if ! tar -tzf "$FROM" >/dev/null 2>&1; then
  echo "error: not a valid tar.gz or file is corrupt" >&2
  exit 2
fi

tar -xzf "$FROM" -C "$TMP_DIR"

if [[ ! -s "${TMP_DIR}/manifest.json" ]]; then
  echo "error: backup is missing manifest.json; refusing to restore" >&2
  exit 2
fi
if [[ ! -s "${TMP_DIR}/pg.dump" ]]; then
  echo "error: backup is missing pg.dump; refusing to restore" >&2
  exit 2
fi
if [[ ! -s "${TMP_DIR}/pg.dump.sha256" ]]; then
  echo "error: backup is missing pg.dump.sha256 sidecar; refusing to restore" >&2
  exit 2
fi

EXPECTED_SHA="$(awk '{print $1}' "${TMP_DIR}/pg.dump.sha256")"
ACTUAL_SHA="$(sha256sum "${TMP_DIR}/pg.dump" | awk '{print $1}')"
if [[ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]]; then
  echo "error: SHA-256 mismatch; backup is corrupt" >&2
  echo "  expected: $EXPECTED_SHA" >&2
  echo "  actual:   $ACTUAL_SHA" >&2
  exit 2
fi

log "manifest:"
# Strip the leading bytes so cat doesn't print binary; rely on `jq` if
# available, otherwise fall back to plain cat.
if command -v jq >/dev/null 2>&1; then
  jq . "${TMP_DIR}/manifest.json"
else
  cat "${TMP_DIR}/manifest.json"
fi

# Migration freshness check. Compare the backup's last applied migration
# to whatever migrations the running app's embedded migration list has. The
# comparison is informational: a backup from an older app is valid data,
# but the running app may add new columns/tables after the restore that
# won't be present.
BACKUP_MAX_MIG="$(jq -r '.schema_version' "${TMP_DIR}/manifest.json" 2>/dev/null | tr ',' '\n' | tail -n 1 | tr -d ' ')"
APP_MAX_MIG="$(ls "${PROJECT_DIR}/backend/internal/database/migrations/" 2>/dev/null | awk -F_ '/^[0-9]+/ {print $1}' | sort -n | tail -n 1)"
if [[ -n "${BACKUP_MAX_MIG}" && -n "${APP_MAX_MIG}" && "${BACKUP_MAX_MIG}" -lt "${APP_MAX_MIG}" ]]; then
  echo ""
  echo "WARNING: backup migration ${BACKUP_MAX_MIG} is OLDER than the running app's migration ${APP_MAX_MIG}."
  echo "         After restore, the new app's startup will auto-apply the missing"
  echo "         migrations. Verify the data shape matches expectations before opening"
  echo "         the service to users. See docs/OPERATIONS.md (Upgrade section)."
elif [[ -n "${BACKUP_MAX_MIG}" && -n "${APP_MAX_MIG}" && "${BACKUP_MAX_MIG}" -gt "${APP_MAX_MIG}" ]]; then
  echo ""
  echo "WARNING: backup migration ${BACKUP_MAX_MIG} is NEWER than the running app's migration ${APP_MAX_MIG}."
  echo "         You are restoring a newer database into an older binary. This usually"
  echo "         means a downgrade. Most schema changes are not reversible; if the"
  echo "         older app references dropped columns, you will see errors. Stop and"
  echo "         upgrade the binary to match or newer than the backup."
fi

if [[ $DRY_RUN -eq 1 ]]; then
  log "DRY RUN — integrity OK; no writes performed"
  exit 0
fi

# --- Confirmation guard --------------------------------------------------------

DEST="${POSTGRES_DB}@${PG_HOST}:${PG_PORT}"
echo ""
echo "WARNING: this will DROP and RECREATE all data in the destination database:"
echo "  destination: ${DEST}"
echo "  source:      ${FROM}"
echo "  sha256:      ${EXPECTED_SHA}"
if [[ -n "${BACKUP_MAX_MIG}" ]]; then
  echo "  backup migrations: ${BACKUP_MAX_MIG}"
fi
if [[ -n "${APP_MAX_MIG}" ]]; then
  echo "  app migrations:    ${APP_MAX_MIG}"
fi
echo ""
if [[ $ASSUME_YES -ne 1 ]]; then
  read -r -p "Type 'yes' to continue: " CONFIRM
  if [[ "$CONFIRM" != "yes" ]]; then
    echo "aborted"
    exit 1
  fi
fi

# --- Restore -------------------------------------------------------------------

log "stopping backend/recorder containers (if docker compose is available)"
if command -v docker >/dev/null 2>&1 && [[ -f "${PROJECT_DIR}/docker-compose.yml" ]]; then
  docker compose -f "${PROJECT_DIR}/docker-compose.yml" stop backend recorder || true
fi

log "restoring pg.dump into ${DEST} (--clean --if-exists --single-transaction)"
PGPASSWORD="${POSTGRES_PASSWORD}" pg_restore \
  -h "${PG_HOST}" -p "${PG_PORT}" -U "${POSTGRES_USER}" \
  -d "${POSTGRES_DB}" \
  --clean --if-exists --single-transaction --no-owner --no-privileges \
  --exit-on-error \
  "${TMP_DIR}/pg.dump"

log "verifying schema_migrations is reachable"
APPLIED="$(PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tA -c 'SELECT string_agg(version::text, '','' ORDER BY version) FROM schema_migrations;')"
if [[ -z "$APPLIED" ]]; then
  echo "error: schema_migrations table is empty after restore" >&2
  echo "hint: did you run the latest migrations? See docs/OPERATIONS.md" >&2
  exit 2
fi
log "applied migrations: ${APPLIED}"

echo ""
echo "Restore complete."
echo ""
echo "Next steps:"
echo "  1. Restart the backend and recorder: docker compose start backend recorder"
echo "  2. Reattach existing recording folders under \${RECORDINGS_HOST_PATH}"
echo "     (the manifest lists the original paths in recording_folders)"
echo "  3. Verify with: curl http://localhost:8080/healthz"
echo ""