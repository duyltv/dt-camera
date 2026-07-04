# DT Camera

Docker Compose foundation for a camera recording system.

## Stack

- Backend API: Go
- Recorder worker: Go plus `ffmpeg`
- Frontend: React with Vite
- Database: PostgreSQL
- Runtime: Docker Compose

## Local Setup

DT Camera is fronted by a single nginx reverse proxy. **The only port you need
to remember is `APP_PORT`, default `8088`.** Hitting `http://127.0.0.1:8088` (or
`http://localhost:8088` — pick one and stick with it, browsers treat them as
different origins) gives you the SPA, the `/api/*` endpoints, and the `/hls/*`
playlists all on the same origin, so cookies work and there is no CORS.

> The original CORS problem was caused by opening the frontend on
> `127.0.0.1:5173` while the API client hard-coded `http://localhost:8080`.
> Routing everything through nginx on `127.0.0.1:8088` puts the SPA and the
> API on the same origin.

Create a local environment file:

```sh
cp .env.example .env
```

Before the first run, open `.env` and set `BOOTSTRAP_ADMIN_EMAIL` and
`BOOTSTRAP_ADMIN_PASSWORD` to create the first admin user. Without these values
there are no default credentials, so you will not be able to sign in to the UI.
After the stack starts, use that email/password at `http://127.0.0.1:8088/login`.

The default recording folder is `./data/recordings`. It is mounted into the
backend and recorder containers at `/recordings`.

Start the stack:

```sh
docker compose up --build
```

Open the app:

```text
http://127.0.0.1:8088
```

The login form posts to `http://127.0.0.1:8088/api/auth/login` and the
session cookie is set for `127.0.0.1:8088`. Subsequent `/api/auth/me` and
all other `/api/*` calls succeed without CORS because the SPA and the backend
share the origin.

Run in the background:

```sh
docker compose up --build -d
```

Stop the stack:

```sh
docker compose down
```

Stop the stack and remove PostgreSQL data:

```sh
docker compose down -v
```

### Direct service ports (dev only)

The backend (`8080`) and frontend (`5173`) ports are commented out in
`docker-compose.yml` by default. Uncomment the relevant `ports:` block if you
want to attach a debugger to one service in isolation. Cookies set through
nginx will **not** be visible to the direct service port because of the
different origin, so use this only for offline API testing.

### Production env

For a real deployment behind TLS, the nginx container should be replaced
(or fronted) by a TLS-terminating reverse proxy. The included nginx is fine
for direct LAN/internet exposure only when wrapped by an external proxy that
sets `X-Forwarded-Proto https`. Set `COOKIE_SECURE=true` in that case.


## Verify

Backend health endpoint (through nginx):

```sh
curl http://127.0.0.1:8088/healthz
```

Expected response:

```json
{
  "database": "ok",
  "recordings_path": "/recordings",
  "service": "backend",
  "started_at": "2026-07-01T00:00:00Z",
  "status": "ok"
}
```

Recorder logs:

```sh
docker compose logs recorder
```

Expected log lines include:

```text
recorder running
recordings path mounted at /recordings
```

Frontend:

```text
http://localhost:5173
```

PostgreSQL persistence uses the `postgres_data` Docker volume. Video recordings
are intended to persist on the host through `./data/recordings`.

## Database Migrations

Backend startup automatically opens PostgreSQL, pings it, and applies embedded
SQL migrations before serving HTTP traffic.

You can also run migrations explicitly:

```sh
docker compose run --rm backend /app/migrate
```

Verify tables exist:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera -c "\\dt"
```

Verify applied migrations:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera -c "SELECT * FROM schema_migrations ORDER BY version;"
```

Verify playback indexes:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera -c "\\di recording_segments*"
```

Reset the database and remove persisted PostgreSQL data:

```sh
docker compose down -v
docker compose up --build
```

## Authentication

Create the first admin by setting explicit bootstrap environment variables before
starting the backend. No default credentials are created automatically. For the
normal Docker Compose flow, put these values in `.env` before `docker compose up
--build`, then log in through the UI at `http://127.0.0.1:8088/login`.

```sh
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD='change-this-long-password'
docker compose up -d --build backend
```

Log in and store the session cookie:

```sh
curl -i -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"login":"admin@example.com","password":"change-this-long-password"}'
```

Get the current user:

```sh
curl -b cookies.txt http://localhost:8080/api/auth/me
```

Log out:

```sh
curl -b cookies.txt -c cookies.txt -X POST http://localhost:8080/api/auth/logout
```

Admin creates a user:

```sh
curl -b cookies.txt -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"email":"viewer@example.com","username":"viewer","display_name":"Viewer","password":"viewer-password","role":"user"}'
```

Admin grants camera permissions:

```sh
curl -b cookies.txt -X PUT http://localhost:8080/api/users/USER_ID/camera-permissions/CAMERA_ID \
  -H "Content-Type: application/json" \
  -d '{"can_view_live":true,"can_view_playback":true}'
```

## Motion Detection And Telegram Notifications

Motion detection runs live inside each recorder camera job. The recorder keeps
a low-FPS rolling frame buffer, detects motion as frames arrive, waits for the
configured post-event window, then creates evidence next to the camera
recordings and uses reusable notification rules to deliver the event.

Evidence files are stored under:

```text
/recordings/{camera_id}/events/YYYY/MM/DD/
```

Recording segment length is controlled separately by
`RECORDER_SEGMENT_SECONDS` and defaults to 60 seconds. Motion alerts do not wait
for a recording segment to finish.

For Telegram, create a bot with BotFather and get the target chat ID. Then log
in as admin and create one notification channel:

```sh
curl -b cookies.txt -X POST http://127.0.0.1:8088/api/notification-channels \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Home Telegram",
    "method": "telegram",
    "enabled": true,
    "config": {
      "bot_token": "123456:telegram-bot-token",
      "chat_id": "123456789"
    }
  }'
```

API responses redact `config.bot_token`; the recorder reads the raw token
directly from PostgreSQL.

Create a reusable rule for motion events. Set `camera_id` to limit it to one
camera, or omit `camera_id` to apply it to every camera.

```sh
curl -b cookies.txt -X POST http://127.0.0.1:8088/api/notification-rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Motion to Telegram",
    "event_type": "motion_detected",
    "notification_channel_id": "NOTIFICATION_CHANNEL_ID",
    "cooldown_seconds": 300,
    "attach_image": true,
    "attach_video": true,
    "pre_event_seconds": 7,
    "post_event_seconds": 3,
    "video_fps": 4
  }'
```

Enable motion detection on a camera:

```sh
curl -b cookies.txt -X PATCH http://127.0.0.1:8088/api/cameras/CAMERA_ID \
  -H "Content-Type: application/json" \
  -d '{
    "motion_detection_enabled": true,
    "motion_sensitivity": 0.35,
    "motion_min_duration_seconds": 1
  }'
```

Useful verification commands:

```sh
docker compose logs -f recorder
docker compose exec postgres psql -U dt_camera -d dt_camera -c "SELECT camera_id, occurred_at, score, status, image_path, video_path FROM motion_events ORDER BY occurred_at DESC LIMIT 10;"
docker compose exec postgres psql -U dt_camera -d dt_camera -c "SELECT event_type, status, camera_id, sent_at, error FROM notification_deliveries ORDER BY created_at DESC LIMIT 10;"
```

Notification rules are intentionally generic: the same table supports
`motion_detected`, `human_detected`, and `classification`, so later detector
types can reuse Telegram or any future notification method.

Admin revokes camera permissions:

```sh
curl -b cookies.txt -X DELETE http://localhost:8080/api/users/USER_ID/camera-permissions/CAMERA_ID
```

Protected API summary:

- `GET /healthz` remains public and does not expose storage paths.
- Storage and camera APIs require admin.
- Layout create/update/delete/default/item modification requires admin.
- Layout read APIs require a logged-in user.
- Recording search, timeline, file serving, and playback preparation require a
  logged-in user and `can_view_playback` for each camera, unless the user is an
  admin.

## Authentication Hardening

The login flow is rate-limited in-memory and every failed attempt is logged
without leaking credentials. Backend configuration (see `.env.example`):

| Variable | Default | Purpose |
|---|---|---|
| `SESSION_TTL_HOURS` | `2160` | Session lifetime in hours (90 days from login). |
| `SESSION_CLEANUP_INTERVAL_SECONDS` | `15m` | How often the backend deletes expired sessions and emits `auth.session_cleanup`. |
| `COOKIE_SECURE` | `false` | Set to `true` when serving over HTTPS so the session cookie is only sent on TLS. |
| `COOKIE_DOMAIN` | _empty_ | Optional cookie domain for cross-subdomain deployments. |
| `LOGIN_RATE_LIMIT_PER_KEY` | `10` | Failed attempts allowed per `(client IP, login)` within the window before blocking. |
| `LOGIN_RATE_LIMIT_PER_IP` | `50` | Per-IP ceiling across all login keys within the window. |
| `LOGIN_RATE_LIMIT_WINDOW` | `1m` | Sliding window for the counters above. |
| `LOGIN_RATE_LIMIT_BLOCK_FOR` | `5m` | How long the (IP, login) pair stays blocked after the limit is hit. |

Behavior:

- The pre-check happens before `findUserByLogin`, so failed lookups cannot be
  used to enumerate valid accounts. Rate-limit responses always look identical
  to the regular "invalid login or password" error.
- A successful login clears that key's counter; a fresh attacker starts back at
  zero after the window slides.
- Each failed attempt emits an `auth.login_failed` event with metadata
  `{reason, ip, login_fingerprint}` (`login_fingerprint` is a short sha256 of
  the attempted login so events can be grouped without storing the raw value).
  Passwords and session tokens are never written to `system_events`.
- Every `SessionCleanupIntervalSeconds` the backend `DELETE`s expired sessions
  and writes `auth.session_cleanup` with `deleted` count.

### Production Cookie Notes

For production deployments served behind HTTPS:

```sh
COOKIE_SECURE=true
COOKIE_DOMAIN=your.domain.example
SESSION_TTL_HOURS=2160
```

The session cookie is always `HttpOnly`, `SameSite=Lax`, `Path=/`, with
`Expires` set to the configured TTL. RTSP URLs are never written to events,
logs, or cookies; the recorder already scrubs them via `sanitizeLog` before
inserting `recorder.job_failure` metadata.

## Live HLS API Examples

Request live stream info for a camera:

```sh
curl -b cookies.txt -sS http://localhost:8080/api/live/cameras/CAMERA_ID
```

Example response:

```json
{
  "camera_id": "018f5d67-89ab-4def-8123-456789abcdef",
  "status": "ok",
  "hls_url": "/hls/018f5d67-89ab-4def-8123-456789abcdef/index.m3u8"
}
```

Fetch the playlist with the same session cookie:

```sh
curl -b cookies.txt -sS http://localhost:8080/hls/CAMERA_ID/index.m3u8
```

Request live stream info for a layout:

```sh
curl -b cookies.txt -sS http://localhost:8080/api/live/layouts/LAYOUT_ID
```

Layout live responses include each tile position plus one status per camera:

```json
{
  "layout_id": "018f5d67-89ab-4def-8123-456789abcdea",
  "cameras": [
    {
      "camera_id": "018f5d67-89ab-4def-8123-456789abcdef",
      "status": "ok",
      "hls_url": "/hls/018f5d67-89ab-4def-8123-456789abcdef/index.m3u8",
      "layout_item": {
        "x": 0,
        "y": 0,
        "width": 2,
        "height": 2,
        "tile_type": "large"
      }
    }
  ]
}
```

Live status values are:

- `ok`
- `no_permission`
- `camera_disabled`
- `stream_unavailable`

HLS is generated by backend-owned `ffmpeg` processes. Output is stored under the
Docker-mounted `HLS_ROOT`, defaulting to `/tmp/dt-camera-hls`, and served as
`/hls/{camera_id}/index.m3u8`. By default, `HLS_WARM_ENABLED=true` keeps a
rolling HLS buffer active for enabled cameras so the Live view can reuse an
already-ready playlist instead of starting cold. Set `HLS_WARM_ENABLED=false` to
return to on-demand streams, where inactive streams stop after
`HLS_INACTIVITY_SECONDS`. `HLS_WARM_INTERVAL_SECONDS` controls how often the
backend reconciles enabled cameras with warm HLS processes.

Camera management previews are captured by the backend and cached on disk under
`PREVIEW_CACHE_ROOT`, defaulting to `/var/lib/dt-camera/previews` in the backend
container. Docker Compose persists this directory with the `preview_cache`
volume. When a camera stream is offline, the backend serves the last cached JPEG
if one exists, and marks the response with `X-Preview-Source: cache`.

To test with one real camera:

```sh
RTSP_URL='rtsp://user:password@camera-host:554/path'

curl -b cookies.txt -sS -X POST http://localhost:8080/api/cameras \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Live test\",\"rtsp_url\":\"${RTSP_URL}\",\"enabled\":true}"

curl -b cookies.txt -sS http://localhost:8080/api/live/cameras/CAMERA_ID
docker compose logs -f backend
```

RTSP URLs are never returned by the live APIs.

## Frontend Flow

Start the full stack:

```sh
docker compose up --build
```

Open the frontend:

```text
http://localhost:5173
```

Suggested admin verification flow:

1. Bootstrap an admin with `BOOTSTRAP_ADMIN_EMAIL` and
   `BOOTSTRAP_ADMIN_PASSWORD`, then rebuild the backend.
2. Log in through `/login`.
3. Create storage at `/storage` using `/recordings`.
4. Create a camera at `/cameras`.
5. Create a normal user at `/users`.
6. Grant that user live/playback permissions at `/permissions`.
7. Create a layout and add cameras at `/layouts`.
8. Open `/live`, select the layout, and start live view.
9. Open `/playback`, select the layout, choose a review window, and load the
   timeline.
10. Drag the shared timeline to an incident timestamp and use `Play all` /
    `Pause all` to review the layout in sync.
11. Open `/health` to inspect backend, recorder, storage, camera status, and
    latest events.

Suggested normal-user verification flow:

1. Log in as the normal user created by the admin.
2. Open `/layouts` to view available layouts.
3. Open `/live` and select a layout. Cameras without `can_view_live` show a
   permission/status message.
4. Open `/playback`, load a review window, and choose one timestamp from the
   shared timeline. Cameras without recordings show `no_recording`; cameras
   without playback permission show a permission message.

The playback screen calls `/api/recordings/timeline` for the layout cameras and
draws available ranges in green. Gaps between ranges are shown with an amber
striped marker. Selecting a timestamp calls `/api/playback/prepare`, renders
the returned videos in the layout geometry, and seeks each available segment to
the returned `offset_seconds`.

Current sync limitation: the UI starts, pauses, and initially seeks all videos
together, but browser buffering and independent video decoders can still cause
minor drift across tiles. Exact frame-level synchronization is a later
improvement.

The frontend uses cookie-based sessions with `credentials: include`. It only
uses backend-provided HLS and playback URLs; RTSP URLs are never rendered.

## Alerts

The alerts subsystem lets admins define rules that turn existing health and
event data into actionable alerts. Four rule types are supported:

| Type | Trigger |
|---|---|
| `recorder_stale` | A recorder heartbeat is older than `max_age_seconds` (default 60). |
| `camera_recording_failed` | At least `min_failures` (default 3) `recorder.job_failure` events for the same camera within `window_seconds` (default 900). |
| `storage_low_disk` | An enabled storage location's `used_percent` ≥ `min_used_percent` (default 90). |
| `live_stream_failed` | At least `min_failures` (default 3) `live.failure` events for the same camera within `window_seconds` (default 900). |

All thresholds live inside the rule's JSONB `threshold` field, e.g.
`{"min_used_percent": 85, "window_seconds": 600}`. Rules also carry
`cooldown_seconds` (default 300) to suppress duplicate alerts for the same
`(rule, entity)` pair.

The backend evaluator runs every `ALERT_EVAL_INTERVAL_SECONDS` (default 60s) on
the same goroutine model as the session cleanup loop. It only opens an alert
when none is already open for the same `(rule, entity)` pair, and never
re-opens within the cooldown window. Alert metadata is sanitized before insert:
RTSP URLs, bearer tokens, and password-shaped fields are stripped to
`[redacted]`.

### Alert API Examples

All endpoints below require an admin session.

List rules:

```sh
curl -b cookies.txt -sS http://localhost:8080/api/alert-rules
```

Create a rule:

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/alert-rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Storage 90%",
    "type": "storage_low_disk",
    "severity": "warning",
    "cooldown_seconds": 1800,
    "threshold": {"min_used_percent": 90}
  }'
```

Patch a rule (disable without deleting):

```sh
curl -b cookies.txt -sS -X PATCH http://localhost:8080/api/alert-rules/RULE_ID \
  -H "Content-Type: application/json" \
  -d '{"enabled": false, "cooldown_seconds": 600}'
```

Delete a rule:

```sh
curl -b cookies.txt -sS -X DELETE http://localhost:8080/api/alert-rules/RULE_ID
```

List alerts (filterable by `status`, `severity`, `entity_type`, `entity_id`, `limit`):

```sh
curl -b cookies.txt -sS "http://localhost:8080/api/alerts?status=open&severity=error&limit=25"
```

Acknowledge an alert (open → acknowledged):

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/alerts/ALERT_ID/acknowledge \
  -H "Content-Type: application/json" -d '{}'
```

Resolve an alert (open or acknowledged → resolved):

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/alerts/ALERT_ID/resolve \
  -H "Content-Type: application/json" -d '{}'
```

Triggering sample events for testing:

```sh
# Force a "storage low disk" finding by lowering the threshold temporarily
curl -b cookies.txt -sS -X POST http://localhost:8080/api/alert-rules \
  -H "Content-Type: application/json" \
  -d '{"name":"Always-firing storage test","type":"storage_low_disk","cooldown_seconds":0,"threshold":{"min_used_percent":0}}'

# Wait one ALERT_EVAL_INTERVAL_SECONDS, then list open alerts
sleep 70
curl -b cookies.txt -sS "http://localhost:8080/api/alerts?status=open"
```

## Backend API Examples

Admin observability endpoints:

```sh
curl -b cookies.txt -sS http://localhost:8080/api/events
curl -b cookies.txt -sS "http://localhost:8080/api/events?event_type=camera.create&severity=info&limit=25"
curl -b cookies.txt -sS http://localhost:8080/api/recorder/status
```

Event filters support `event_type`, `entity_type`, `entity_id`, `start_time`,
`end_time`, `severity`, and `limit`. Results are newest first.

Useful debug workflow:

1. Log in as admin.
2. Create or update a storage location/camera/layout.
3. Open `/health` or call `/api/events?limit=20`.
4. Check `/api/recorder/status` for heartbeat freshness, active jobs, last
   segments, and latest recorder errors.

Create a storage location using the mounted recordings folder:

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/storage-locations \
  -H "Content-Type: application/json" \
  -d '{"name":"Default recordings","container_path":"/recordings","enabled":true}'
```

List storage locations:

```sh
curl -b cookies.txt -sS http://localhost:8080/api/storage-locations
```

Refresh and read storage health:

```sh
curl -b cookies.txt -sS http://localhost:8080/api/storage-locations/STORAGE_LOCATION_ID/health
```

Storage health includes `exists`, `writable`, `total_bytes`, `free_bytes`,
`used_bytes`, `used_percent`, and `latest_validation_error`.

Get one storage location:

```sh
curl -b cookies.txt -sS http://localhost:8080/api/storage-locations/STORAGE_LOCATION_ID
```

Update a storage location:

```sh
curl -b cookies.txt -sS -X PATCH http://localhost:8080/api/storage-locations/STORAGE_LOCATION_ID \
  -H "Content-Type: application/json" \
  -d '{"name":"Primary recordings"}'
```

Enable or disable a storage location:

```sh
curl -b cookies.txt -sS -X PATCH http://localhost:8080/api/storage-locations/STORAGE_LOCATION_ID/enabled \
  -H "Content-Type: application/json" \
  -d '{"enabled":false}'
```

Delete a storage location:

```sh
curl -b cookies.txt -sS -X DELETE http://localhost:8080/api/storage-locations/STORAGE_LOCATION_ID
```

Test storage validation with an invalid path:

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/storage-locations \
  -H "Content-Type: application/json" \
  -d '{"name":"Broken path","container_path":"/does-not-exist"}'
```

Create a camera. The backend stores `rtsp_url`, but API responses do not return
it:

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/cameras \
  -H "Content-Type: application/json" \
  -d '{"name":"Front Gate","rtsp_url":"rtsp://user:pass@camera.local/stream1","storage_location_id":"STORAGE_LOCATION_ID","enabled":true,"retention_days":30}'
```

Example camera response:

```json
{
  "id": "018f5d67-89ab-7def-8123-456789abcdea",
  "storage_location_id": "018f5d67-89ab-7def-8123-456789abcdef",
  "name": "Front Gate",
  "enabled": true,
  "recording_enabled": false,
  "retention_days": 30,
  "created_at": "2026-07-01T00:00:00Z",
  "updated_at": "2026-07-01T00:00:00Z"
}
```

List cameras:

```sh
curl -sS http://localhost:8080/api/cameras
```

Get one camera:

```sh
curl -sS http://localhost:8080/api/cameras/CAMERA_ID
```

Update a camera:

```sh
curl -sS -X PATCH http://localhost:8080/api/cameras/CAMERA_ID \
  -H "Content-Type: application/json" \
  -d '{"name":"Front Gate Updated","retention_days":14,"max_storage_bytes":10737418240}'
```

Enable or disable a camera:

```sh
curl -sS -X PATCH http://localhost:8080/api/cameras/CAMERA_ID/enabled \
  -H "Content-Type: application/json" \
  -d '{"enabled":false}'
```

Delete a camera:

```sh
curl -sS -X DELETE http://localhost:8080/api/cameras/CAMERA_ID
```

## Layout API Examples

Create a layout with items:

```sh
curl -sS -X POST http://localhost:8080/api/layouts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Main wall",
    "settings": {"columns": 4},
    "is_default": true,
    "layout_items": [
      {
        "camera_id": "CAMERA_ID",
        "x": 0,
        "y": 0,
        "width": 2,
        "height": 2,
        "display_order": 0,
        "tile_type": "large"
      }
    ]
  }'
```

List layouts:

```sh
curl -sS http://localhost:8080/api/layouts
```

Get one layout:

```sh
curl -sS http://localhost:8080/api/layouts/LAYOUT_ID
```

Update layout name or settings:

```sh
curl -sS -X PATCH http://localhost:8080/api/layouts/LAYOUT_ID \
  -H "Content-Type: application/json" \
  -d '{"name":"Updated wall","settings":{"columns":3}}'
```

Mark a layout as the global default:

```sh
curl -sS -X PATCH http://localhost:8080/api/layouts/LAYOUT_ID/default
```

Add a layout item:

```sh
curl -sS -X POST http://localhost:8080/api/layouts/LAYOUT_ID/items \
  -H "Content-Type: application/json" \
  -d '{"camera_id":"CAMERA_ID","x":2,"y":0,"width":1,"height":1,"display_order":1,"tile_type":"small"}'
```

Update or delete a layout item:

```sh
curl -sS -X PATCH http://localhost:8080/api/layouts/LAYOUT_ID/items/ITEM_ID \
  -H "Content-Type: application/json" \
  -d '{"camera_id":"CAMERA_ID","x":0,"y":0,"width":2,"height":1,"display_order":0,"tile_type":"landscape"}'

curl -sS -X DELETE http://localhost:8080/api/layouts/LAYOUT_ID/items/ITEM_ID
```

Delete a layout:

```sh
curl -sS -X DELETE http://localhost:8080/api/layouts/LAYOUT_ID
```

Example layout response:

```json
{
  "id": "018f5d67-89ab-4def-8123-456789abcdea",
  "name": "Main wall",
  "settings": {"columns": 4},
  "is_default": true,
  "layout_items": [
    {
      "id": "018f5d67-89ab-4def-8123-456789abcdef",
      "layout_id": "018f5d67-89ab-4def-8123-456789abcdea",
      "camera_id": "018f5d67-89ab-4def-8123-456789abcdeb",
      "x": 0,
      "y": 0,
      "width": 2,
      "height": 2,
      "display_order": 0,
      "tile_type": "large"
    }
  ]
}
```

## Recorder Worker

The recorder connects to PostgreSQL, polls for enabled cameras with assigned
enabled storage locations, and starts one `ffmpeg` process per camera. RTSP URLs
are read from the database but are not printed in logs.

Recorder settings are configured in `.env`:

```sh
RECORDER_WORKER_ID=recorder-1
RECORDER_POLL_INTERVAL_SECONDS=5
RECORDER_SEGMENT_SECONDS=60
RECORDER_MAX_BACKOFF_SECONDS=60
RECORDER_CLEANUP_INTERVAL_SECONDS=60
RECORDER_LOW_DISK_FREE_PERCENT=10
RECORDER_LOW_DISK_MIN_FILE_AGE_SECONDS=3600
```

Restart the recorder after changing code or environment:

```sh
docker compose up -d --build recorder
```

Create a storage location for recorder output:

```sh
curl -sS -X POST http://localhost:8080/api/storage-locations \
  -H "Content-Type: application/json" \
  -d '{"name":"Default recordings","container_path":"/recordings","enabled":true}'
```

Create a camera using a real RTSP URL. Keep the URL in a shell variable so it is
not copied into docs or logs:

```sh
RTSP_URL='rtsp://user:password@camera-host:554/path'

curl -sS -X POST http://localhost:8080/api/cameras \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Front Gate\",\"rtsp_url\":\"${RTSP_URL}\",\"storage_location_id\":\"STORAGE_LOCATION_ID\",\"enabled\":true}"
```

Inspect recorder logs:

```sh
docker compose logs -f recorder
```

Expected log lines include camera ID and name, but not the full RTSP URL:

```text
recorder running worker_id=recorder-1
ffmpeg starting camera_id=... camera_name="Front Gate"
segment metadata inserted camera_id=...
```

Check generated files on the host:

```sh
find ./data/recordings -type f | sort
```

Recorder files use this structure:

```text
./data/recordings/{camera_id}/YYYY/MM/DD/{camera_id}_YYYYMMDDTHHMMSS.mp4
```

Check heartbeat rows:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "SELECT worker_id, status, active_job_count, last_seen_at FROM recorder_heartbeats ORDER BY worker_id;"
```

Check inserted segment metadata:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "SELECT camera_id, file_path, start_time, end_time, duration_seconds, size_bytes, format, status FROM recording_segments ORDER BY created_at DESC LIMIT 10;"
```

## Retention Cleanup And Storage Health

Each camera has simple retention settings:

- `retention_days`: completed segments older than this are eligible for cleanup.
- `max_storage_bytes`: optional per-camera cap; oldest completed segments are
  deleted first until the camera is back under the cap.

Cleanup runs inside the recorder container on
`RECORDER_CLEANUP_INTERVAL_SECONDS`. It only deletes files referenced by
`recording_segments`, verifies each file path is inside the configured storage
location container path, deletes the video file, then deletes the database row.
Unrelated files in the recordings folder are never scanned for deletion.

Low disk cleanup uses `RECORDER_LOW_DISK_FREE_PERCENT`. If a storage location
falls below that free-space percentage, the recorder deletes the oldest
completed segments for that storage location first, but only after they are at
least `RECORDER_LOW_DISK_MIN_FILE_AGE_SECONDS` old. If free space is still low,
storage health is marked `warning`; invalid or unwritable storage is marked
`error`.

Synthetic cleanup test:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "SELECT id, storage_location_id FROM cameras LIMIT 1;"

docker compose exec recorder sh -lc 'mkdir -p /recordings/synthetic && printf video > /recordings/synthetic/old.mp4'

docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "UPDATE cameras SET retention_days = 1 WHERE id = 'CAMERA_ID';"

docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "INSERT INTO recording_segments (camera_id, storage_location_id, start_time, end_time, duration_seconds, file_path, size_bytes, format, status)
      VALUES ('CAMERA_ID', 'STORAGE_LOCATION_ID', now() - interval '2 days', now() - interval '2 days' + interval '60 seconds', 60, '/recordings/synthetic/old.mp4', 5, 'mp4', 'completed');"

docker compose logs -f recorder
```

After the next cleanup interval, verify the file and row are gone:

```sh
docker compose exec recorder test ! -f /recordings/synthetic/old.mp4
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "SELECT id, file_path FROM recording_segments WHERE file_path = '/recordings/synthetic/old.mp4';"
```

Optional fake stream testing can be done by running a local RTSP test source in
another container or machine, then creating a camera with that RTSP URL. The
recorder only requires that the URL is reachable from inside the `recorder`
container.

## Playback API Examples

Search recording segments by camera and time range:

```sh
curl -sS "http://localhost:8080/api/recordings/search?camera_id=CAMERA_ID&start_time=2026-07-01T00:00:00Z&end_time=2026-07-01T01:00:00Z"
```

Search multiple cameras for layout playback preparation:

```sh
curl -sS "http://localhost:8080/api/recordings/search?camera_id=CAMERA_ID_1,CAMERA_ID_2&start_time=2026-07-01T00:00:00Z&end_time=2026-07-01T01:00:00Z"
```

Example segment response. The API returns a playback URL, not the raw file path:

```json
{
  "segments": [
    {
      "id": "018f5d67-89ab-4def-8123-456789abcdea",
      "camera_id": "018f5d67-89ab-4def-8123-456789abcdef",
      "storage_location_id": "018f5d67-89ab-4def-8123-456789abcdeb",
      "start_time": "2026-07-01T00:00:00Z",
      "end_time": "2026-07-01T00:01:00Z",
      "duration_seconds": 60,
      "size_bytes": 1048576,
      "format": "mp4",
      "status": "completed",
      "playback_url": "/api/recordings/018f5d67-89ab-4def-8123-456789abcdea/file"
    }
  ]
}
```

Get timeline availability ranges and gaps:

```sh
curl -b cookies.txt -sS "http://localhost:8080/api/recordings/timeline?camera_id=CAMERA_ID&start_time=2026-07-01T00:00:00Z&end_time=2026-07-01T01:00:00Z&gap_threshold_seconds=3"
```

Serve a recorded video file by segment ID. Browser seeking is supported through
normal HTTP range requests handled by the backend:

```sh
curl -b cookies.txt -I http://localhost:8080/api/recordings/RECORDING_SEGMENT_ID/file
curl -b cookies.txt -H "Range: bytes=0-1023" http://localhost:8080/api/recordings/RECORDING_SEGMENT_ID/file -o sample.mp4
```

Prepare synchronized playback for a layout or camera list:

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/playback/prepare \
  -H "Content-Type: application/json" \
  -d '{"camera_ids":["CAMERA_ID_1","CAMERA_ID_2"],"selected_timestamp":"2026-07-01T00:00:30Z"}'
```

Layout-based preparation uses cameras from `layout_items`:

```sh
curl -b cookies.txt -sS -X POST http://localhost:8080/api/playback/prepare \
  -H "Content-Type: application/json" \
  -d '{"layout_id":"LAYOUT_ID","selected_timestamp":"2026-07-01T00:00:30Z"}'
```

Layout playback responses include each camera's layout position. Cameras without
matching recordings still return their position with `status: "no_recording"`.
Matching cameras include `segment_start_time` and `offset_seconds` so the
frontend can seek into the segment around the selected timestamp.

Real playback UI test flow:

1. Record at least one completed segment with the recorder, or insert synthetic
   `recording_segments` rows that point to video files under `/recordings`.
2. Log in at `http://localhost:5173`.
3. Open `/playback`, choose the layout containing those cameras, and set a
   review window that includes the segment times.
4. Click `Load timeline`; available camera ranges appear in green and gaps are
   striped.
5. Drag the shared timeline. The app reloads playback around that timestamp and
   seeks each available video to its segment offset.

Inspect segment rows directly:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "SELECT id, camera_id, file_path, start_time, end_time, status FROM recording_segments ORDER BY start_time DESC LIMIT 10;"
```

## Project Structure

```text
.
├── backend
│   ├── cmd/backend
│   ├── cmd/migrate
│   ├── internal/database
│   └── internal/httpapi
├── recorder
│   ├── cmd/recorder
│   └── internal/worker
├── frontend
│   └── src
├── data/recordings
├── docker-compose.yml
└── PROJECT_PLAN.md
```

## Current Scope

This project ships Compose services, the Go backend, the recorder worker, the
React frontend, PostgreSQL schema migrations, storage and camera CRUD, HLS
live streaming, recording playback, layout management, cookie-based
authentication with login rate limiting, role and per-camera permissions,
system event logging, cached camera previews, a recorder status API, retention
cleanup, and a basic admin dashboard.

Deferred items: WebRTC, multi-recorder scheduling/load balancing, object
storage, motion detection, alert/notification channels.
