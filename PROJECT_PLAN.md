# DT Camera Project Plan

## Architecture Baseline

The whole system must run with Docker Compose from the start. Local development,
testing, and deployment should use the same service boundaries so that storage,
networking, and runtime permissions are exercised early.

## Stack

- Backend API: Golang
- Frontend UI: ReactJS
- Database: PostgreSQL
- Recorder worker: separate Golang process that runs `ffmpeg`
- Stream proxy: HLS first, with WebRTC deferred until later
- Storage: video folders mounted into containers as Docker volumes or bind mounts

## Docker Compose Services

- `postgres`
  - Stores camera configuration, recording metadata, users, and system settings.
  - Persists database files with a named Docker volume.

- `backend`
  - Exposes the public API consumed by the frontend.
  - Owns camera CRUD, storage configuration, authentication, and playback APIs.
  - Validates that configured storage folders exist and are writable.
  - Knows RTSP URLs, but never returns them to the frontend.

- `frontend`
  - ReactJS application served as its own container.
  - Talks only to the backend API.
  - Never receives RTSP URLs or direct camera credentials.

- `recorder`
  - Separate Golang worker process, sharing a Go module with the backend if useful.
  - Reads enabled camera configuration from PostgreSQL.
  - Periodically reconciles recording jobs.
  - Runs `ffmpeg` for RTSP recording and segment generation.
  - Writes recording segment metadata back to PostgreSQL.
  - Mounts the same configured video storage folders used by the backend.

- `stream-proxy` optional initially
  - Converts RTSP to HLS for live viewing.
  - May be implemented as a small Go service wrapping `ffmpeg`, or a stable
    off-the-shelf HLS proxy if it keeps RTSP URLs server-side.
  - WebRTC can be added later without exposing RTSP URLs to the frontend.

## Storage Requirements

- PostgreSQL data must be persisted with a Docker named volume.
- Video recordings must be persisted using mounted host folders.
- Camera recording paths must be configurable as mounted paths inside the
  containers, not hard-coded host paths.
- Backend startup and storage-setting updates must validate that each configured
  folder:
  - exists inside the backend container,
  - is a directory,
  - is writable by the backend container user.
- Recorder must mount every active recording storage folder at the same container
  path or through an explicit path mapping stored in configuration.
- Missing or unwritable storage must fail clearly before a camera is marked
  recordable.

## Security Boundaries

- RTSP URLs and camera credentials are only available to:
  - `backend`,
  - `recorder`,
  - `stream-proxy` if enabled.
- Frontend receives backend-generated IDs, status, thumbnails, HLS URLs, and
  recording playback URLs only.
- Backend API must redact RTSP fields in all frontend-facing responses.
- Docker Compose environment variables and secrets should keep database and
  camera-sensitive values out of frontend build/runtime configuration.

## Initial Data Model

- `cameras`
  - name, enabled flag, RTSP URL, location/group fields, recording policy,
    storage target, timestamps.

- `storage_locations`
  - logical name, container path, enabled flag, capacity metadata, health status,
    timestamps.

- `recording_segments`
  - camera ID, storage location ID, file path, start time, end time, duration,
    size, format, created timestamp.

- `recorder_heartbeats`
  - worker ID, last seen time, version, active job count, status.

## Backend Responsibilities

- Provide camera and storage configuration APIs.
- Validate storage folders on startup and whenever configuration changes.
- Redact RTSP URLs from frontend responses.
- Provide recording search and playback APIs.
- Provide live-view endpoints that return HLS playback information without
  exposing RTSP source URLs.
- Run database migrations on startup or via a dedicated migration command.

## Recorder Responsibilities

- Poll PostgreSQL for enabled cameras and recording policies.
- Start, stop, and restart `ffmpeg` processes as camera configuration changes.
- Write segments to the configured mounted storage path.
- Record segment metadata in PostgreSQL.
- Report health and active jobs through database heartbeat rows.
- Handle transient RTSP and `ffmpeg` failures with bounded retry/backoff.

## Frontend Responsibilities

- Provide camera list, camera detail, storage health, live view, and recordings
  search screens.
- Use backend APIs only.
- Treat live playback as HLS initially.
- Display storage validation and recorder health states clearly.

## Implementation Phases

1. Compose foundation
   - Add `docker-compose.yml`.
   - Add backend, frontend, recorder, and PostgreSQL service definitions.
   - Add named volume for PostgreSQL.
   - Add example bind-mounted host folders for recordings.

2. Go module and database
   - Create shared Go module with separate backend and recorder commands.
   - Add PostgreSQL connection handling and migrations.
   - Create initial schema for cameras, storage locations, recording segments,
     and recorder heartbeats.

3. Backend API
   - Implement storage validation.
   - Implement camera and storage CRUD APIs.
   - Ensure RTSP redaction in all frontend-facing responses.
   - Add recording listing and playback file endpoints.

4. Recorder worker
   - Poll enabled cameras from PostgreSQL.
   - Start `ffmpeg` recording jobs.
   - Persist recording segment metadata.
   - Add heartbeat and retry behavior.

5. Frontend UI
   - Build React screens for cameras, storage, recorder status, live HLS view,
     and recording search/playback.

6. HLS stream proxy
   - Add simple HLS live-view support.
   - Keep RTSP URLs server-side.
   - Mount temporary or persistent HLS output paths as needed.

7. Hardening
   - Add authentication and authorization.
   - Add retention cleanup policies.
   - Add health checks for all Compose services.
   - Add integration tests around storage validation, RTSP redaction, and recorder
     metadata writes.

## Deferred Work

- WebRTC live streaming.
- Event queue between backend and recorder.
- Multi-recorder scheduling and load balancing.
- Object storage support.
- Advanced motion detection or analytics.
