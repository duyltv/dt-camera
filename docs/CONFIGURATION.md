# Configuration

DT Camera is configured mainly through `.env` and then through the admin UI.

## Environment Files

Start from:

```sh
cp .env.example .env
```

The `.env.example` file is the canonical list of supported variables and
includes comments for each setting.

## Most Important Variables

| Variable | Meaning |
| --- | --- |
| `APP_PORT` | Public port served by bundled nginx. Default: `8088`. |
| `APP_ENV` | `development`, `staging`, or `production`. Production enables stricter safety checks. |
| `FRONTEND_ORIGIN` | Public browser origin, for example `http://localhost:8088` or `https://camera.example.com`. |
| `POSTGRES_PASSWORD` | PostgreSQL password. Change this before production. |
| `BOOTSTRAP_ADMIN_EMAIL` | First admin email. Used only when no admin exists. |
| `BOOTSTRAP_ADMIN_PASSWORD` | First admin password. Must be strong in production. |
| `RECORDINGS_HOST_PATH` | Host folder where recording files persist. |
| `RECORDINGS_CONTAINER_PATH` | Container path for recording storage, normally `/recordings`. |
| `HLS_WARM_ENABLED` | Keeps live streams warm before users open Live view. |
| `HLS_MAX_LAG_SECONDS` | Maximum acceptable live lag before backend restarts a stale HLS stream. |
| `RECORDER_SEGMENT_SECONDS` | Length of recording segments created by ffmpeg. |
| `RECORDER_LOW_DISK_FREE_PERCENT` | Free-space threshold for low-disk cleanup. |

## Camera Settings

Each camera can be configured with:

- Name
- RTSP URL, or ONVIF-discovered stream
- Storage location
- Camera enabled/disabled
- Recording enabled/disabled
- Streaming enabled/disabled
- Audio recording/streaming flags when supported
- Retention days
- Optional max storage bytes
- Motion detector settings
- Notification cooldown period

RTSP URLs are stored in PostgreSQL but are never returned to the frontend in
normal API responses.

## Storage Settings

Storage locations use container paths, not host paths.

Example:

```text
/recordings
```

The backend validates that storage folders:

- exist inside the backend container
- are directories
- are writable

For Docker Compose, `RECORDINGS_HOST_PATH` is mounted into
`RECORDINGS_CONTAINER_PATH`, so the recorder and backend see the same files.

## Production Settings

For a real deployment:

```env
APP_ENV=production
COOKIE_SECURE=true
FRONTEND_ORIGIN=https://camera.example.com
POSTGRES_PASSWORD=use-a-strong-password
```

Then start with:

```sh
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build
```
