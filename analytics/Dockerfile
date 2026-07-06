FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
RUN go build -o /out/analytics-worker ./cmd/analytics-worker

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/analytics-worker /app/analytics-worker

CMD ["/app/analytics-worker"]
