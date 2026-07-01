FROM nginx:1.27-alpine

# Config is mounted from the repo (docker/nginx/conf.d/dt-camera.conf) so
# changes don't require an image rebuild. This makes local iteration fast.
COPY conf.d /etc/nginx/conf.d

EXPOSE 8088

# A simple healthcheck that hits the backend /healthz through the proxy.
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=10s \
  CMD wget -qO- http://127.0.0.1:8088/healthz || exit 1

CMD ["nginx", "-g", "daemon off;"]