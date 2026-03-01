[← Back to README](../README.md)

# Docker — `Dockerfile` & Compose

## Multi-Stage Build

The `Dockerfile` uses a two-stage build:

### Builder stage

- **Base image:** `golang:<GO_VERSION>-alpine` (default Go 1.24.2).
- **Build caches:** Module cache (`/go/pkg/mod`) and build cache (`/root/.cache/go-build`) are mounted for faster rebuilds.
- **Flags:** `CGO_ENABLED=0` produces a static binary. Additional flags:
  - `-trimpath` — removes local filesystem paths from the binary.
  - `-buildvcs=false` — skips embedding VCS info.
  - `-ldflags="-s -w"` — strips debug symbols and DWARF info.
- **Cross-compilation:** Supports `TARGETOS` (default `linux`) and `TARGETARCH` (default `amd64`) via Docker BuildKit.
- **Output:** `/out/server` — the compiled `cmd/server` binary.

### Runtime stage

- **Base image:** `alpine:<ALPINE_VERSION>` (default 3.21).
- **Packages:** `ca-certificates` and `tzdata` are installed.
- **Non-root user:** An `app` user/group is created with configurable UID/GID (default 2000:2000). The container runs as this user.
- **Binary:** Copied from the builder stage to `/usr/local/bin/server` with mode `0555`.
- **Port:** Exposes `8080` (`HTTP_ADDR=:8080`).

## Build Arguments / Makefile Variables

| Variable | Default | Description |
|---|---|---|
| `GO_IMAGE` | `golang:1.24.2-alpine` | Builder stage base image |
| `RUNTIME_IMAGE` | `alpine:3.21` | Runtime stage base image |
| `IMAGE_NAME` | `clock-server` | Docker image name |
| `IMAGE_TAG` | `0.0.1` | Docker image tag |
| `REGISTRY` | `192.168.2.201:32000` | Target container registry |
| `HOST_PORT` | `8080` | Host port for `docker run` |
| `CONTAINER_PORT` | `8080` | Container port for `docker run` |

The Makefile passes `GO_IMAGE` and `RUNTIME_IMAGE` to `docker build` via `--build-arg`.

## Running with Docker

```bash
# Build the image and start a detached container (port 8080)
make docker-run

# Stop the running container
make docker-stop

# Stop the container and remove the image
make docker-clean
```

`make docker-run` builds the image (`make docker-build`), then runs it with `--rm -d` mapping `HOST_PORT:CONTAINER_PORT`.

## Registry Push

```bash
make docker-push
```

This target:

1. Builds the image via `make docker-build`.
2. Tags it as `REGISTRY/IMAGE_NAME:IMAGE_TAG` (e.g. `192.168.2.201:32000/clock-server:0.0.1`).
3. Tags it as `REGISTRY/IMAGE_NAME:latest`.
4. Pushes both tags to the registry.

Use `make docker-login` to authenticate with the registry before pushing.

## Docker Compose Stack

The `docker-compose.yml` defines three services on a shared `clock-server-net` network:

### `mosquitto`

- **Image:** `eclipse-mosquitto:2`
- **Port:** `1883:1883`
- **Config:** Mounts `deploy/mosquitto/mosquitto.conf` read-only.
- **Healthcheck:** Publishes to an MQTT `__health` topic.

### `server`

- **Build:** From the project `Dockerfile`.
- **Depends on:** `mosquitto` (healthy).
- **Port:** `8080:8080`
- **Healthcheck:** `wget` against `http://127.0.0.1:8080/health`.
- **Environment variables:**

| Variable | Value | Purpose |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address |
| `REQUIRE_TLS` | `false` | Disable TLS requirement |
| `TRUST_PROXY_TLS` | `false` | Don't trust proxy TLS headers |
| `READINESS_REQUIRE_AUTH` | `false` | Readiness endpoint skips auth |
| `API_AUTH_CREDENTIALS` | `ops\|dev-token\|*` | Dev credential (user `ops`, token `dev-token`, all scopes) |
| `ENABLED_SENDERS` | `mqtt` | Enable MQTT sender |
| `MQTT_BROKER_URL` | `mqtt://mosquitto:1883` | Broker address (uses compose service name) |
| `ALLOW_INSECURE_MQTT` | `true` | Allow non-TLS MQTT |
| `MQTT_TOPIC_PREFIX` | `clocks/commands` | MQTT topic prefix |

### `clockctl`

- **Image:** `golang:1.24-alpine`
- **Volumes:** Mounts the project root to `/workspace`.
- **Depends on:** `server` (healthy).
- **Entrypoint:** `go run ./cmd/clockctl`
- **Default command:** `message --device clock-1 --message compose-test --duration 10`
- **Environment variables:**

| Variable | Value | Purpose |
|---|---|---|
| `CLOCK_SERVER_HOST` | `http://server:8080` | Server address |
| `CLOCK_SERVER_TOKEN` | `dev-token` | Auth token |
| `CLOCKCTL_ALLOW_INSECURE_HTTP` | `true` | Allow non-TLS HTTP |

### Running an integration test

```bash
# Start mosquitto + server, then run clockctl to send a test message
docker compose up --abort-on-container-exit

# Or run individual services
docker compose up -d mosquitto server
docker compose run --rm clockctl message --device clock-1 --message hello --duration 5
```

## Mosquitto Configuration

`deploy/mosquitto/mosquitto.conf` sets up a minimal MQTT broker for development:

| Directive | Value | Meaning |
|---|---|---|
| `listener` | `1883` | Listen on the standard MQTT port |
| `allow_anonymous` | `true` | No username/password required |
| `persistence` | `false` | Messages are not persisted to disk |
