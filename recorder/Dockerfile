FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/recorder ./cmd/recorder

FROM alpine:3.20

RUN apk add --no-cache ca-certificates ffmpeg
WORKDIR /app
COPY --from=build /out/recorder /app/recorder

CMD ["/app/recorder"]
