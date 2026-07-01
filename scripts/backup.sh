#!/usr/bin/env bash
# scripts/backup.sh — PostgreSQL + recording-metadata backup.
#
# What gets backed up:
#   - A PostgreSQL logical dump (pg_dump custom format, .dump inside the tar)
#   - Recording-metadata rows exported as JSON via the backend's psql session
#   - A manifest.json describing the backup (tool version, schema version,
#     row counts, recording-folder snapshot hint, SHA-256 of the dump, and
#     the build identity of the app that produced it)
#
# What is NOT included:
#   - Video files. Those live on the host under RECORDINGS_HOST_PATH and
#     must be snapshotted separately (rsync, btrfs send, EBS snapshot, ...).
#     The manifest's recording_folders field tells you which directories to
#     snapshot to obtain a complete restore.
#
# Usage:
#   ./scripts/backup.sh                        # writes ./backups/dt-camera-<ts>.tar.gz
#   BACKUP_DIR=/var/backups ./scripts/backup.sh
#   ./scripts/backup.sh --dry-run               # print plan, do not write
#   ./scripts/backup.sh --out /tmp/foo.tar.gz   # explicit output path

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

DRY_RUN=0
OUT_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --out) OUT_FILE="$2"; shift 2 ;;
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
    echo "hint: copy .env.example to .env or export it manually" >&2
    exit 2
  fi
}

BACKUP_DIR="${BACKUP_DIR:-${PROJECT_DIR}/backups}"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="${OUT_FILE:-${BACKUP_DIR}/dt-camera-${TS}.tar.gz}"

mkdir -p "${BACKUP_DIR}"

log() { echo "[backup] $*"; }

PG_HOST="${POSTGRES_HOST:-127.0.0.1}"
PG_PORT="${POSTGRES_PORT:-5432}"
PG_DUMP_FORMAT="${PG_DUMP_FORMAT:-custom}"
DOCKER_PG_SERVICE="${DOCKER_PG_SERVICE:-postgres}"

sanity_check_psql() {
  if ! command -v psql >/dev/null 2>&1; then
    echo "error: psql not found in PATH" >&2
    echo "hint: install postgresql-client or run this from inside the postgres container" >&2
    exit 2
  fi
  if ! command -v pg_dump >/dev/null 2>&1; then
    echo "error: pg_dump not found in PATH" >&2
    exit 2
  fi
}

# Capture the backend's build identity (set via -ldflags when the image was
# built) so the manifest records which app version produced the backup. We
# don't fail if the binary can't be invoked: this is informational only.
APP_VERSION="$( (timeout 5 docker compose -f "${PROJECT_DIR}/docker-compose.yml" exec -T backend /app/backend -version 2>/dev/null || true) | tr -d '\n' )"
APP_GIT_COMMIT="${GIT_COMMIT:-}"
APP_BUILD_TIME="${BUILD_TIME:-}"

dump_pg() {
  log "dumping PostgreSQL database ${POSTGRES_DB} on ${PG_HOST}:${PG_PORT} (format=${PG_DUMP_FORMAT})"
  PGPASSWORD="${POSTGRES_PASSWORD}" pg_dump \
    -h "${PG_HOST}" -p "${PG_PORT}" -U "${POSTGRES_USER}" \
    -F "${PG_DUMP_FORMAT}" -d "${POSTGRES_DB}" --no-owner --no-privileges
}

snapshot_metadata() {
  log "snapshotting recording metadata + config rows"
  PGPASSWORD="${POSTGRES_PASSWORD}" psql \
    -h "${PG_HOST}" -p "${PG_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tA <<'SQL'
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM cameras ORDER BY created_at) t) TO '/tmp/cameras.json' WITH (FORMAT text)
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM storage_locations ORDER BY created_at) t) TO '/tmp/storage_locations.json' WITH (FORMAT text)
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM layouts ORDER BY created_at) t) TO '/tmp/layouts.json' WITH (FORMAT text)
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM layout_items ORDER BY layout_id, display_order) t) TO '/tmp/layout_items.json' WITH (FORMAT text)
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM recorder_heartbeats ORDER BY worker_id) t) TO '/tmp/recorder_heartbeats.json' WITH (FORMAT text)
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT id, alert_rule_id, event_type, entity_type, entity_id, severity, status, message, opened_at, acknowledged_at, resolved_at FROM alerts ORDER BY opened_at DESC LIMIT 1000) t) TO '/tmp/alerts.json' WITH (FORMAT text)
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM alert_rules ORDER BY lower(name)) t) TO '/tmp/alert_rules.json' WITH (FORMAT text)
\copy (SELECT json_agg(row_to_json(t)) FROM (SELECT id, camera_id, storage_location_id, start_time, end_time, duration_seconds, size_bytes, format, status FROM recording_segments WHERE status = 'completed' ORDER BY start_time DESC LIMIT 5000) t) TO '/tmp/recording_segments.json' WITH (FORMAT text)
SQL
  for f in cameras storage_locations layouts layout_items recorder_heartbeats alerts alert_rules recording_segments; do
    if [[ ! -s "/tmp/${f}.json" ]]; then
      echo "[]" > "/tmp/${f}.json"
    fi
  done
}

build_manifest() {
  local dump_sha="$1"
  local dump_size="$2"
  local pg_version
  pg_version="$(PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tA -c 'SHOW server_version;')"
  local applied_migrations
  applied_migrations="$(PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tA -c 'SELECT string_agg(version::text, '','' ORDER BY version) FROM schema_migrations;')"
  cat > "${TMP_DIR}/manifest.json" <<EOF
{
  "tool": "dt-camera/scripts/backup.sh",
  "app": {
    "app_version": "${APP_VERSION:-unknown}",
    "git_commit":  "${APP_GIT_COMMIT:-}",
    "build_time":  "${APP_BUILD_TIME:-}"
  },
  "schema_version": "${applied_migrations}",
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "postgres": { "host": "${PG_HOST}", "port": ${PG_PORT}, "database": "${POSTGRES_DB}", "version": "${pg_version}" },
  "artifacts": {
    "pg_dump": { "path": "pg.dump", "format": "${PG_DUMP_FORMAT}", "size_bytes": ${dump_size}, "sha256": "${dump_sha}" }
  },
  "recording_folders": ["${RECORDINGS_HOST_PATH:-./data/recordings}"],
  "notes": [
    "Video files are NOT included. Snapshot the recording_folders paths separately.",
    "Restore with scripts/restore.sh against the same Postgres major version."
  ]
}
EOF
}

do_dry_run() {
  local db_label="${POSTGRES_DB:-<unset, set POSTGRES_DB>}"
  log "DRY RUN — would write to: ${OUT_FILE}"
  log "would dump PostgreSQL database: ${db_label}"
  log "would snapshot metadata tables: cameras, storage_locations, layouts, layout_items, recorder_heartbeats, alerts, alert_rules, recording_segments (completed only)"
  log "would write manifest.json with schema_migrations versions, app_version, and a SHA-256 of the dump"
  log "no writes performed"
  exit 0
}

[[ $DRY_RUN -eq 1 ]] && do_dry_run

require_var POSTGRES_DB
require_var POSTGRES_USER
require_var POSTGRES_PASSWORD

sanity_check_psql

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

DUMP_FILE="${TMP_DIR}/pg.dump"
DUMP_SHA_FILE="${TMP_DIR}/pg.dump.sha256"

dump_pg > "${DUMP_FILE}"

DUMP_SHA="$(sha256sum "${DUMP_FILE}" | awk '{print $1}')"
DUMP_SIZE="$(stat -c %s "${DUMP_FILE}" 2>/dev/null || stat -f %z "${DUMP_FILE}")"
echo "${DUMP_SHA}  pg.dump" > "${DUMP_SHA_FILE}"

snapshot_metadata

build_manifest "${DUMP_SHA}" "${DUMP_SIZE}"

# Bundle everything into a tar.gz.
log "writing ${OUT_FILE}"
tar -czf "${OUT_FILE}" \
  -C "${TMP_DIR}" manifest.json pg.dump pg.dump.sha256 \
  -C /tmp cameras.json storage_locations.json layouts.json layout_items.json recorder_heartbeats.json alerts.json alert_rules.json recording_segments.json

log "done: ${OUT_FILE} ($(stat -c %s "${OUT_FILE}" 2>/dev/null || stat -f %z "${OUT_FILE}") bytes)"
log "manifest: schema_migrations=${applied_migrations:-unknown}, app_version=${APP_VERSION:-unknown}, dump_sha256=${DUMP_SHA:0:16}..."