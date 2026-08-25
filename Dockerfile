# Build the server as a static binary, then ship it on a minimal runtime image.
FROM golang:1.23-alpine AS build

WORKDIR /src

# Dependencies first so the module cache survives source-only changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 claims

WORKDIR /app

COPY --from=build /out/server /app/server
# The service reads its seed catalogue and serves product images from disk.
COPY seed ./seed
COPY assets ./assets

USER claims

EXPOSE 8080

ENV PORT=:8080 \
    SEED_FILE=/app/seed/product.json \
    MEDIA_DIR=/app/assets/images

ENTRYPOINT ["/app/server"]
