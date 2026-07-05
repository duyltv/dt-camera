# Development Guide

This guide is for contributors working on DT Camera locally.

## Repository Layout

```text
backend/        Go backend API and migrations
recorder/       Go recorder worker
frontend/       React frontend
docker/nginx/   Single-origin reverse proxy
docs/           Documentation
scripts/        Backup and restore helpers
```

## Run The Full Stack

```sh
docker compose up -d --build
```

The app is served at:

```text
http://localhost:8088
```

## Backend Tests

```sh
cd backend
GOCACHE="$(pwd)/../.gocache" go test ./...
```

## Recorder Tests

```sh
cd recorder
GOCACHE="$(pwd)/../.gocache" go test ./...
```

## Frontend Build

```sh
cd frontend
npm run build
```

## Rebuild Only Frontend

```sh
docker compose up -d --build frontend nginx
```

## Rebuild Backend

```sh
docker compose up -d --build backend
```

## Database Migrations

Migrations live in:

```text
backend/internal/database/migrations/
```

Backend startup runs embedded migrations automatically. You can also run:

```sh
docker compose run --rm backend /app/migrate
```

## Documentation Rules

- Keep `README.md` short and friendly.
- Put API commands, troubleshooting, architecture, and legal notes in `/docs`.
- Update `docs/README.md` when adding a new documentation page.
- Keep legal text clear that it is not legal advice.
