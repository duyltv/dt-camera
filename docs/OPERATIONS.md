# DT Camera Operations

This document is the operator's playbook for installing, upgrading,
backing up, restoring, troubleshooting, and running DT Camera in production.

## First Install

### Prerequisites

- Docker Engine 24+ and Docker Compose v2
- A Linux host (or VM) with at least 2 vCPU, 4 GB RAM, and enough disk for
  PostgreSQL plus the recording volume
- A DNS name pointed at the host, e.g. `camera.example.com`
- TLS certificates (Let's Encrypt or your PKI) terminated by a reverse proxy

### Steps

```sh
git clone https://github.com/your-org/dt-camera.git /opt/dt-camera
cd /opt/dt-camera
cp .env.example .env

# Edit .env and set:
#   POSTGRES_PASSWORD, COOKIE_SECURE=true,
#   BOOTSTRAP_ADMIN_EMAIL, BOOTSTRAP_ADMIN_PASSWORD (>=12 chars, 3 character classes),
#   SESSION_TTL_HOURS=24, FRONTEND_ORIGIN=https://camera.example.com

# Bring up the stack with the production overlay (adds healthchecks + restart policies)
APP_ENV=production \
  COOKIE_SECURE=true \
  BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
  BOOTSTRAP_ADMIN_PASSWORD='StrongP@ssword-1234' \
  FRONTEND_ORIGIN=https://camera.example.com \
  docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build

# Wait for backend to become healthy
docker compose ps
curl -sf https://camera.example.com/api/auth/login -X POST \
  -H "Content-Type: application/json" \
  -d '{"login":"admin@example.com","password":"StrongP@ssword-1234"}' \
  -c cookies.txt && curl -b cookies.txt https://camera.example.com/api/auth/me
```

The backend refuses to start in production mode if any safety check fails.
See "Production env vars" below for the full list.

## Upgrade

```sh
# Pull new images
cd /opt/dt-camera
git pull
docker compose -f docker-compose.yml -f docker-compose.prod.yml build

# Apply migrations and restart in dependency order
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d postgres
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d backend

# Backend applies embedded SQL migrations on startup. Watch the logs:
docker compose logs -f backend
# Look for "database migrations applied" and any errors.

# Roll recorder and frontend back in
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d recorder frontend
docker compose ps
```

Migrations are forward-only. Never edit a migration that has been applied in
production; add a new migration file (next sequential number) instead.

## Backup

### What is backed up

The script `scripts/backup.sh` produces a single `.tar.gz` containing:

- `pg.dump` — full PostgreSQL logical dump (custom format)
- `pg.dump.sha256` — SHA-256 of the dump, sidecar
- `manifest.json` — tool version, schema_migrations list, dump SHA-256,
  Postgres version, and the `recording_folders` paths that need separate
  filesystem snapshots
- `cameras.json`, `storage_locations.json`, `layouts.json`, `layout_items.json`,
  `recorder_heartbeats.json`, `alerts.json`, `alert_rules.json`,
  `recording_segments.json` — JSON snapshots of metadata tables for
  grep/diff without `pg_restore`

### What is NOT backed up

- Video files. They live in `RECORDINGS_HOST_PATH` (default `./data/recordings`).
  Snapshot that folder separately with your tool of choice (rsync to a backup
  server, btrfs send, EBS snapshot, etc.).
- Docker images and volumes. Pull your image registry or back up the
  `postgres_data` volume with the rest of your Docker infra.
- The `.env` file. Back it up alongside your secrets vault.

### Run

```sh
cd /opt/dt-camera
set -a; source .env; set +a     # load POSTGRES_* into the environment

# Dry run (no writes, prints the plan)
./scripts/backup.sh --dry-run

# Real backup
./scripts/backup.sh

# Custom output path
./scripts/backup.sh --out /var/backups/dt-camera-$(date +%Y%m%d).tar.gz
```

Schedule it via cron or systemd timers. A typical daily schedule:

```cron
# /etc/cron.d/dt-camera-backup
15 2 * * * root cd /opt/dt-camera && /opt/dt-camera/scripts/backup.sh
```

Make sure the script's stdout/stderr is captured to a log file or journald
so failures are visible.

## Restore

### Pre-flight

1. Verify the backup file is intact: `tar -tzf <file> | head`. If that errors,
   the file is corrupt — grab a different backup.
2. Run `./scripts/restore.sh --from <file> --dry-run` to print the manifest
   without making any changes.
3. Make sure the destination database (named in `POSTGRES_DB`) already exists
   on the target Postgres. The script does NOT create the cluster; only the
   schema and data.

### Run

```sh
cd /opt/dt-camera
set -a; source .env; set +a

# Stop the application so no new writes hit the DB
docker compose -f docker-compose.yml -f docker-compose.prod.yml stop backend recorder

# Confirm the destination cluster is reachable
psql "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST:-127.0.0.1}:${POSTGRES_PORT:-5432}/${POSTGRES_DB}" -c '\dt'

# Restore — the script will prompt for "yes"
./scripts/restore.sh --from /var/backups/dt-camera-20260101T020000Z.tar.gz

# Bring the app back up
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d backend recorder
```

### Reattach existing recording folders

Video files are NOT part of the backup tarball. To reattach them:

1. Restore `RECORDINGS_HOST_PATH` from your filesystem snapshot (rsync, etc.)
   onto the new host. The original path is recorded in the backup's
   `manifest.json` under `recording_folders`.
2. Ensure the container user (uid 1000 in the recorder/backend images) can
   read and write the path: `chown -R 1000:1000 /srv/recordings`.
3. Update `RECORDINGS_HOST_PATH` in `.env` to point at the restored path,
   then `docker compose up -d recorder backend` so they pick up the new bind
   mount.
4. Verify with `curl http://localhost:8080/api/storage-locations/STORAGE_ID/health`
   — `exists=true`, `writable=true`, `used_percent` non-null.

## Storage folder permissions

Both the `backend` and `recorder` containers run their main process as a
non-root user (uid 1000). The host directory mounted at
`RECORDINGS_CONTAINER_PATH` (default `/recordings`) must be readable and
writable by that uid.

```sh
mkdir -p /srv/recordings
chown -R 1000:1000 /srv/recordings
chmod 755 /srv/recordings
```

When you back up `/srv/recordings` with rsync/tar, preserve the uid
(`rsync -a`, `tar --preserve`). On restore, `chown -R 1000:1000` again before
starting the recorder.

If you see `latest_validation_error = permission denied` on a storage
location, the container user can't write. Check the host directory's owner and
mode, then restart the recorder container.

## Reverse proxy / HTTPS

DT Camera does not terminate TLS itself; put it behind a reverse proxy. The
proxy must:

- Terminate TLS and forward to backend on `127.0.0.1:8080` (or whatever
  `BACKEND_PORT` is bound to).
- Set `X-Forwarded-For` so the rate limiter can bucket per real client IP.
  The backend reads the first hop.
- Forward `Cookie` and `Authorization` headers verbatim.
- Set a long `Cache-Control: no-store` for `/api/*` and `/hls/*`.
- Issue an HSTS header (`Strict-Transport-Security: max-age=31536000; includeSubDomains`)
  at the edge, not in the backend.

Example nginx snippet:

```nginx
upstream dt_backend {
  server 127.0.0.1:8080;
}

server {
  listen 443 ssl http2;
  server_name camera.example.com;

  ssl_certificate     /etc/letsencrypt/live/camera.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/camera.example.com/privkey.pem;

  add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

  client_max_body_size 32m;

  location / {
    proxy_pass http://dt_backend;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_pass_request_headers on;
    proxy_buffering off;
  }
}
```

Caddy example:

```caddy
camera.example.com {
  reverse_proxy 127.0.0.1:8080 {
    header_up X-Forwarded-For {remote_host}
    header_up X-Forwarded-Proto {scheme}
  }
}
```

## Production env vars

Set `APP_ENV=production` and ensure each of the following is correct.
The backend refuses to start in production if any "error"-level check fails.

| Variable | Required? | Notes |
|---|---|---|
| `APP_ENV` | yes | Set to `production`. Triggers hard safety checks. |
| `COOKIE_SECURE` | yes | Must be `true`. Browsers drop `Secure` cookies over HTTP. |
| `COOKIE_DOMAIN` | optional | Set when backend and frontend are on different subdomains. |
| `SESSION_TTL_HOURS` | yes | Recommended 1-24. The default 168 (1 week) is too long for production. |
| `BOOTSTRAP_ADMIN_EMAIL` | first-time only | Only consumed if no admin exists yet. |
| `BOOTSTRAP_ADMIN_PASSWORD` | first-time only | Must be ≥12 chars and include 3 of: lower/upper/digit/symbol. |
| `POSTGRES_PASSWORD` | yes | Same strength rule. Used in `DATABASE_URL` too. |
| `FRONTEND_ORIGIN` | yes | Public URL of the frontend; CORS will reject mismatches. |
| `DATABASE_URL` | yes | Internal DSN. Use `sslmode=require` if you front Postgres with TLS. |
| `RECORDINGS_PATH` | yes | Container-side path; `RECORDINGS_HOST_PATH` is the host bind. |
| `RECORDINGS_HOST_PATH` | backup-only | Informational; included in the backup manifest. |
| `HLS_ROOT` | yes | Container-side path; usually a named volume. |
| `LOGIN_RATE_LIMIT_PER_KEY` | recommended | Default 10. |
| `LOGIN_RATE_LIMIT_PER_IP` | recommended | Default 50. |
| `ALERT_EVAL_INTERVAL_SECONDS` | recommended | Default 60s. |
| `SESSION_CLEANUP_INTERVAL_SECONDS` | recommended | Default 15m. |

Anything in `.env.example` not listed above can stay at its default.

## Common troubleshooting

### Backend fails to start with `FATAL: refusing to start in production`

Run with `APP_ENV=development` to see which checks fail, fix the offending
variable, and restart. The most common offenders are `COOKIE_SECURE=false`
and `BOOTSTRAP_ADMIN_PASSWORD` being too short.

### Login page just reloads with no error

Check the browser's network panel: if `/api/auth/me` returns CORS, your
`FRONTEND_ORIGIN` doesn't match the actual browser origin. Update `.env` and
restart the backend.

### Recorder heartbeat is stale

Inspect:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "SELECT worker_id, status, last_seen_at, now() - last_seen_at AS age FROM recorder_heartbeats;"
```

If `age` is greater than 90s, the recorder isn't running or can't reach the DB.
Common causes: wrong `DATABASE_URL`, disk full (ffmpeg can't write), or RTSP
URL is unreachable.

### No live tiles in `/live`

Check `/health` (admin) → "Storage" section. If `latest_validation_error`
is set, the backend can't read or write the recordings mount. Fix the host
permissions, then run

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "UPDATE storage_locations SET last_checked_at = NULL;"
```

to force a re-validation, or just call
`curl -X POST http://localhost:8080/api/storage-locations/STORAGE_ID/health` (admin only).

### Backup file is corrupt

`./scripts/restore.sh --from <file>` will abort with `SHA-256 mismatch` if the
file was tampered with or copied incompletely. Verify with
`sha256sum <file>` against an out-of-band record of the expected hash.
Restore from a different backup.

### Restore aborted mid-flight

`pg_restore` was called with `--single-transaction --exit-on-error`, so the
database is rolled back to its pre-restore state. Verify with
`psql ... -c "SELECT count(*) FROM cameras"` — if it matches the pre-restore
count, you can safely retry.

## Quick reference

| Task | Command |
|---|---|
| Tail backend logs | `docker compose logs -f backend` |
| Tail recorder logs | `docker compose logs -f recorder` |
| Open a psql shell | `docker compose exec postgres psql -U dt_camera -d dt_camera` |
| Inspect events | `curl -b cookies.txt "http://localhost:8080/api/events?limit=50"` |
| Inspect alerts | `curl -b cookies.txt "http://localhost:8080/api/alerts?status=open"` |
| Trigger a dry-run backup | `./scripts/backup.sh --dry-run` |
| Run a real backup | `./scripts/backup.sh` |
| Dry-run a restore | `./scripts/restore.sh --from <file> --dry-run` |
| Run a restore | `./scripts/restore.sh --from <file>` |
| Check healthchecks | `docker compose ps` |