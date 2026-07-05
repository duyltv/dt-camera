# Domain And HTTPS Setup

This guide explains how to put DT Camera behind a real domain such as
`camera.example.com`.

## Recommended Shape

Run DT Camera internally with Docker Compose, then put a TLS reverse proxy in
front of it.

```text
Internet
  |
  v
https://camera.example.com
  |
  v
TLS reverse proxy
  |
  v
http://127.0.0.1:8088
  |
  v
DT Camera nginx -> frontend/backend/HLS
```

The app already exposes one public HTTP entrypoint through its bundled nginx:

```text
http://localhost:8088
```

Your external proxy should forward HTTPS traffic to that port.

## DNS

Create a DNS record for your domain:

```text
camera.example.com -> your server public IP
```

Use an `A` record for IPv4, or an `AAAA` record for IPv6.

## Environment

In `.env`, set:

```env
APP_ENV=production
APP_PORT=8088
FRONTEND_ORIGIN=https://camera.example.com
COOKIE_SECURE=true
```

Use strong values for:

```env
POSTGRES_PASSWORD=change-this-to-a-strong-password
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=change-this-to-a-strong-password
```

Start with the production overlay:

```sh
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build
```

## Caddy Example

Caddy is the simplest option for automatic HTTPS.

```caddyfile
camera.example.com {
  reverse_proxy 127.0.0.1:8088 {
    header_up X-Forwarded-Proto https
    header_up X-Forwarded-Host {host}
  }
}
```

Then run or reload Caddy.

## Nginx Example

If you already use host-level nginx:

```nginx
server {
    listen 443 ssl http2;
    server_name camera.example.com;

    ssl_certificate     /etc/letsencrypt/live/camera.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/camera.example.com/privkey.pem;

    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:8088;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        proxy_buffering off;
        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
    }
}

server {
    listen 80;
    server_name camera.example.com;
    return 301 https://$host$request_uri;
}
```

## Important Notes

- Keep the app, API, and HLS paths on the same origin.
- Do not expose PostgreSQL to the internet.
- Do not expose the backend or frontend direct service ports unless debugging.
- Use `COOKIE_SECURE=true` when serving over HTTPS.
- Make sure your firewall only exposes the public proxy ports, usually `80` and
  `443`.
- HLS live video needs proxy timeouts long enough for playlist and segment
  delivery.

## Quick Verification

Check health through the domain:

```sh
curl https://camera.example.com/healthz
```

Check the web app:

```text
https://camera.example.com
```

If login succeeds and live video loads, the reverse proxy is usually configured
correctly.
