# API Reference

This page summarizes the backend API groups. For expanded curl examples, see
[Technical Reference](TECHNICAL_REFERENCE.md).

All authenticated routes use the browser session cookie created by login.

## Public

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Basic backend/database health. |
| `POST` | `/api/auth/login` | Create a session. |

## Authentication

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/auth/me` | Return current user. |
| `POST` | `/api/auth/logout` | Revoke current session. |

## Admin: Users And Permissions

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/users` | List users. |
| `POST` | `/api/users` | Create user. |
| `PUT` | `/api/users/{user_id}/camera-permissions/{camera_id}` | Grant/update camera permission. |
| `DELETE` | `/api/users/{user_id}/camera-permissions/{camera_id}` | Revoke camera permission. |

## Admin: Storage

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/storage-locations` | List storage locations. |
| `POST` | `/api/storage-locations` | Create storage location. |
| `GET` | `/api/storage-locations/{id}` | Get storage location. |
| `PATCH` | `/api/storage-locations/{id}` | Update storage location. |
| `PATCH` | `/api/storage-locations/{id}/enabled` | Enable/disable storage. |
| `GET` | `/api/storage-locations/{id}/health` | Storage health and capacity. |
| `DELETE` | `/api/storage-locations/{id}` | Delete when safe. |

## Admin: Cameras

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/cameras` | List cameras without RTSP URLs. |
| `POST` | `/api/cameras` | Create camera. |
| `GET` | `/api/cameras/{id}` | Get camera without RTSP URL. |
| `PATCH` | `/api/cameras/{id}` | Update camera. |
| `PATCH` | `/api/cameras/{id}/enabled` | Enable/disable camera. |
| `DELETE` | `/api/cameras/{id}` | Delete when safe or soft-disable. |
| `POST` | `/api/cameras/scan` | Scan IP range for ONVIF devices. |
| `POST` | `/api/cameras/onvif/test` | Test ONVIF credentials/stream discovery. |
| `POST` | `/api/cameras/onvif/import` | Import an ONVIF-discovered camera. |
| `POST` | `/api/cameras/onvif/preview` | Generate preview for an ONVIF device. |

## Layouts

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/layouts` | List layouts. |
| `POST` | `/api/layouts` | Create layout. |
| `GET` | `/api/layouts/{id}` | Get layout with items. |
| `PATCH` | `/api/layouts/{id}` | Update layout settings. |
| `POST` | `/api/layouts/{id}/default` | Mark default layout. |
| `DELETE` | `/api/layouts/{id}` | Delete layout. |

## Live

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/live/cameras/{camera_id}` | Return HLS URL for one camera. |
| `GET` | `/api/live/layouts/{layout_id}` | Return HLS URLs by layout. |
| `GET` | `/hls/{camera_id}/index.m3u8` | Serve playlist with permission checks. |

## Playback

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/recordings/search` | Search recording segments by cameras/time. |
| `GET` | `/api/recordings/timeline` | Return availability ranges and gaps. |
| `GET` | `/api/recordings/{segment_id}/file` | Serve recorded segment safely. |
| `POST` | `/api/playback/prepare` | Prepare layout/camera playback at a timestamp. |

## Alerts And Events

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/alerts` | List alerts. |
| `GET` | `/api/alert-rules` | List alert rules. |
| `POST` | `/api/alert-rules` | Create alert rule. |
| `PATCH` | `/api/alert-rules/{id}` | Update alert rule. |
| `DELETE` | `/api/alert-rules/{id}` | Delete alert rule. |
| `GET` | `/api/system-events` | Admin event log. |

## Safety Rules

- API responses must not expose RTSP URLs.
- Playback file serving uses segment IDs, not raw file paths.
- Normal users only see cameras they have permission to view.
- Admins bypass camera permission checks for management and review.
