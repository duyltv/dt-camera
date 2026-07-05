# Architecture

DT Camera runs as a Docker Compose system.

## Services

| Service | Purpose |
| --- | --- |
| `postgres` | Stores users, sessions, cameras, layouts, recording metadata, alerts, and events. |
| `backend` | Go API server, auth, layouts, live HLS management, playback, storage health, admin APIs. |
| `recorder` | Go worker that runs ffmpeg for background recording and cleanup. |
| `frontend` | React UI served by a static server. |
| `nginx` | Single public entrypoint for the SPA, API, health, and HLS paths. |

## High-Level Flow

```text
Browser
  |
  v
nginx :8088
  |-- /           -> frontend
  |-- /api/*      -> backend
  |-- /hls/*      -> backend HLS files
  |-- /healthz    -> backend health

backend <-> PostgreSQL
recorder <-> PostgreSQL
recorder -> ffmpeg -> /recordings
backend  -> ffmpeg -> /tmp/dt-camera-hls
```

## Recording Flow

1. Recorder polls PostgreSQL for enabled cameras.
2. For each enabled recording camera with valid storage, recorder starts one
   ffmpeg process.
3. ffmpeg writes segments into:

```text
/recordings/{camera_id}/YYYY/MM/DD/
```

4. Recorder inserts `recording_segments` metadata after files are stable.
5. Retention cleanup deletes old eligible segment rows and files.

## Live HLS Flow

1. Backend reads enabled streamable cameras.
2. If warm streaming is enabled, backend starts HLS ffmpeg processes in the
   background.
3. Live view requests return backend-generated HLS URLs, never RTSP URLs.
4. Backend serves:

```text
/hls/{camera_id}/index.m3u8
```

5. If the newest HLS segment is older than `HLS_MAX_LAG_SECONDS`, backend
   treats the stream as stale and restarts that camera's HLS process.

## Playback Flow

1. Frontend requests timeline availability.
2. User selects a timestamp.
3. Frontend calls playback preparation for a layout or cameras.
4. Backend returns the best segment per camera and safe playback URLs.
5. Files are served by segment ID, not by raw filesystem path.

## Security Boundaries

- RTSP URLs are known only to backend, recorder, and stream handling code.
- The frontend receives safe camera fields and backend-generated URLs.
- Users need permissions for live and playback access.
- Admins can manage cameras, storage, users, layouts, and system settings.
- Recording file serving validates paths against configured storage roots.
