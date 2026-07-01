FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/backend ./cmd/backend
RUN go build -o /out/migrate ./cmd/migrate

FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget ffmpeg
WORKDIR /app
COPY --from=build /out/backend /app/backend
COPY --from=build /out/migrate /app/migrate

EXPOSE 8080
CMD ["/app/backend"]
