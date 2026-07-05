# Troubleshooting

## The Logo Or Favicon Does Not Update

Browsers aggressively cache favicons.

Try:

```text
Cmd + Shift + R
```

Or open this URL once, then refresh the app:

```text
http://localhost:8088/favicon.png
```

If running through Docker, rebuild the frontend:

```sh
docker compose up -d --build frontend nginx
```

## Cannot Log In

Check that the first admin was configured before first startup:

```env
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=change-this-long-password
```

Then inspect backend logs:

```sh
docker compose logs backend
```

If you need to fully reset local data:

```sh
docker compose down -v
docker compose up -d --build
```

## Storage Validation Fails

Use container paths, not host paths.

Good:

```text
/recordings
```

Usually wrong inside Docker:

```text
/Users/name/videos
```

Check mount configuration:

```env
RECORDINGS_HOST_PATH=./data/recordings
RECORDINGS_CONTAINER_PATH=/recordings
```

## Live View Is Stuck Or Delayed

DT Camera uses HLS. A small delay is normal, but stale streams should be
restarted automatically when they exceed `HLS_MAX_LAG_SECONDS`.

Check:

```sh
docker compose logs backend
docker compose logs recorder
```

Useful settings:

```env
HLS_WARM_ENABLED=true
HLS_WARM_INTERVAL_SECONDS=15s
HLS_MAX_LAG_SECONDS=15s
```

If only one camera fails, verify the camera RTSP URL, credentials, network, and
whether the camera allows multiple simultaneous RTSP connections.

## Recording Does Not Happen In Background

Recording should be handled by the `recorder` container, independent of user UI
activity.

Check:

```sh
docker compose ps -a
docker compose logs recorder
```

In the UI, confirm the camera is:

- enabled
- recording enabled
- assigned to an enabled storage location
- using a reachable RTSP stream

## Playback Has No Data

Verify files exist:

```sh
find ./data/recordings -type f | head
```

Verify segment metadata exists:

```sh
docker compose exec postgres psql -U dt_camera -d dt_camera \
  -c "SELECT camera_id, start_time, end_time, file_path, status FROM recording_segments ORDER BY start_time DESC LIMIT 10;"
```

## ONVIF Scan Finds Nothing

Try:

- scanning a smaller IP range
- adding common ONVIF ports: `80`, `8000`, `8080`, `8899`
- confirming cameras and server are on the same network
- checking camera settings for ONVIF enablement
- entering ONVIF credentials if the camera requires authentication

## Docker Is Healthy But Browser Fails

Use the bundled nginx URL:

```text
http://localhost:8088
```

Avoid mixing direct frontend/backend ports unless debugging. Cookies depend on
browser origin, and `localhost:8088`, `localhost:5173`, and `localhost:8080` are
different origins.
