# Getting Started

This guide gets DT Camera running locally with Docker Compose.

## Requirements

- Docker
- Docker Compose
- A browser
- At least one RTSP or ONVIF-capable camera
- A host folder for recordings, defaulting to `./data/recordings`

## 1. Clone The Repository

```sh
git clone https://github.com/duyltv/dt-camera.git
cd dt-camera
```

## 2. Create `.env`

```sh
cp .env.example .env
```

Edit `.env` and set the first admin user:

```env
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=change-this-long-password
```

DT Camera does not create insecure default credentials. If the bootstrap admin
values are empty and no admin exists, you will not be able to sign in.

## 3. Start The Stack

```sh
docker compose up -d --build
```

Open:

```text
http://localhost:8088
```

## 4. First Setup In The UI

After signing in:

1. Create a storage location using container path `/recordings`.
2. Add a camera manually or scan ONVIF cameras from the Cameras page.
3. Create or update the default layout.
4. Grant live/playback permissions to normal users.
5. Configure notifications if you want Telegram alerts.

## 5. Verify Services

```sh
docker compose ps -a
curl http://localhost:8088/healthz
docker compose logs recorder
```

The app should show healthy backend/frontend/nginx containers, and recorder
logs should show that the recorder is running.

## Stop Or Reset

Stop containers:

```sh
docker compose down
```

Stop containers and remove persisted PostgreSQL data:

```sh
docker compose down -v
```

Recorded video files are stored in the host folder configured by
`RECORDINGS_HOST_PATH`; removing Docker volumes does not delete that host folder.
