# Clock Command Dispatcher

A Go microservice that acts as a secure HTTP command gateway for smart clock devices. It validates and routes commands (set alarm, display message, set brightness) to MQTT and/or REST backends. Built with hexagonal architecture, scoped Bearer auth, TLS-by-default, rate limiting, and a companion CLI.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [API Reference](#api-reference)
- [Configuration](#configuration)
- [Security Model](#security-model)
- [CLI: clockctl](#cli-clockctl)
- [Development](#development)
- [Testing](#testing)
- [Docker](#docker)
- [Kubernetes / Helm](#kubernetes--helm)
- [Makefile Reference](#makefile-reference)

## Subproject Documentation

Detailed per-subproject references live in the `docs/` directory:

| Document | Description |
|---|---|
| [docs/server.md](docs/server.md) | `cmd/server` entrypoint and all `internal/` packages (domain, application, adapters, API, config, bootstrap, security) |
| [docs/clockctl.md](docs/clockctl.md) | `cmd/clockctl` CLI client — commands, flags, env vars, and examples |
| [docs/helm.md](docs/helm.md) | Helm chart (`helm/clock-server`) — values reference, templates, security posture, ingress, and secrets |
| [docs/docker.md](docs/docker.md) | Dockerfile multi-stage build, Docker Compose integration test stack, and Mosquitto sidecar config |
| [docs/dagger.md](docs/dagger.md) | Dagger CI/CD pipeline (`.dagger/`) — module overview and pipeline functions |

---

## Overview

`clock-server` is an HTTP command dispatcher for smart clock devices. Clients send commands (set alarm, display message, set brightness) to the API; the server validates and routes each command to one or more downstream transports — MQTT and/or a REST backend — based on configuration.

**Key properties:**
- Hexagonal (Ports & Adapters) architecture — transport is pluggable
- Bearer auth with per-credential device scoping
- TLS enforced by default on both inbound and outbound connections
- Rate limiting on auth failures, request body size cap
- 12-factor configuration (environment variables only)
- Minimal dependencies — only `godog` for BDD tests; all transports implemented in stdlib

---

## Quick Start

**Run locally (no TLS, single token):**

```bash
REQUIRE_TLS=false \
API_AUTH_TOKEN=dev-token \
ENABLED_SENDERS=mqtt \
MQTT_BROKER_URL=mqtt://localhost:1883 \
ALLOW_INSECURE_MQTT=true \
go run ./cmd/server
```

**Send a command:**

```bash
CLOCK_SERVER_BASE_URL=http://localhost:8080 \
CLOCK_SERVER_TOKEN=dev-token \
CLOCKCTL_ALLOW_INSECURE_HTTP=true \
go run ./cmd/clockctl message --device clock-1 --message "Hello" --duration 10
```

**Full local stack with Docker Compose:**

```bash
docker compose up --build --abort-on-container-exit clockctl
```

---

## Architecture

The project follows Hexagonal Architecture (Ports & Adapters), isolating domain logic from transport concerns.

```
┌─────────────────────────────────────────────────────────┐
│                        cmd/server                       │
│                 (entrypoint / wiring)                   │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                     internal/api                        │
│        (HTTP handlers, TLS/auth middleware)             │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                 internal/application                    │
│         CommandDispatcher  ←→  ClockCommandSender       │
│                 (use-case service / port)               │
└────────────────────────┬────────────────────────────────┘
                         │
          ┌──────────────┼──────────────┐
          │              │              │
┌─────────▼──────┐ ┌─────▼──────┐ ┌───▼────────────┐
│ adapters/mqtt  │ │ adapters/  │ │ adapters/      │
│ (MQTT sender)  │ │ rest       │ │ composite      │
│                │ │ (REST)     │ │ (fan-out)      │
└────────────────┘ └────────────┘ └────────────────┘
```

### Package Map

| Package | Purpose |
|---|---|
| `cmd/server` | HTTP server entrypoint, wiring, graceful shutdown |
| `cmd/clockctl` | CLI client for sending commands |
| `internal/domain` | Core command models and validation rules |
| `internal/application` | `CommandDispatcher` service and `ClockCommandSender` output port |
| `internal/adapters/mqtt` | MQTT adapter — long-lived in-process client |
| `internal/adapters/rest` | REST adapter — forwards commands over HTTP |
| `internal/adapters/composite` | Fan-out adapter dispatching to multiple senders |
| `internal/api` | HTTP handlers, auth middleware, rate limiting, probes |
| `internal/config` | Environment-based configuration loading and validation |
| `internal/bootstrap` | Adapter wiring and readiness check assembly |
| `internal/security` | Credential parsing, token matching, device scope enforcement |

---

## API Reference

All command endpoints require a `Authorization: Bearer <token>` header. The token must have scope for the target device.

### Health & Readiness

#### `GET /health`

Liveness probe. Always returns `200 OK` when the server is running.

```
HTTP/1.1 200 OK
```

#### `GET /ready`

Readiness probe. Returns `200 OK` when all configured adapters are reachable. Returns `503 Service Unavailable` when not ready.

When `READINESS_REQUIRE_AUTH=true` (default `false` in Helm), this endpoint also requires authentication.

---

### Commands

#### `POST /commands/alarms`

Set an alarm on a device.

**Request:**

```json
{
  "device_id": "clock-1",
  "alarm_time": "2030-06-01T07:00:00Z",
  "label": "Wake up"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `device_id` | string | yes | Target device identifier |
| `alarm_time` | string (RFC 3339) | yes | Time to trigger the alarm; must not be more than 1 minute in the past |
| `label` | string | no | Human-readable label |

**Responses:**

| Status | Meaning |
|---|---|
| `202 Accepted` | Command dispatched |
| `400 Bad Request` | Validation failure (body contains error detail) |
| `401 Unauthorized` | Missing or invalid bearer token |
| `403 Forbidden` | Token does not have scope for the target device |
| `413 Request Entity Too Large` | Body exceeds `MAX_BODY_BYTES` |
| `502 Bad Gateway` | Downstream adapter error |

---

#### `POST /commands/messages`

Display a message on a device.

**Request:**

```json
{
  "device_id": "clock-1",
  "message": "Meeting in 5 minutes",
  "duration_seconds": 30
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `device_id` | string | yes | Target device identifier |
| `message` | string | yes | Text to display |
| `duration_seconds` | integer | yes | How long to show the message (1–3600) |

**Responses:** same codes as `/commands/alarms`

---

#### `PUT /commands/brightness`

Set display brightness on a device.

**Request:**

```json
{
  "device_id": "clock-1",
  "level": 75
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `device_id` | string | yes | Target device identifier |
| `level` | integer | yes | Brightness level (0–100) |

**Responses:** same codes as `/commands/alarms`

---

## Configuration

All configuration is via environment variables (12-factor).

### Server

| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address |
| `HTTP_READ_TIMEOUT_MS` | `10000` | Read timeout in milliseconds |
| `HTTP_WRITE_TIMEOUT_MS` | `10000` | Write timeout in milliseconds |
| `HTTP_IDLE_TIMEOUT_MS` | `60000` | Idle connection timeout in milliseconds |
| `HTTP_READ_HEADER_TIMEOUT_MS` | `5000` | Header read timeout in milliseconds |
| `HTTP_SHUTDOWN_TIMEOUT_MS` | `15000` | Graceful shutdown timeout in milliseconds |
| `TLS_CERT_FILE` | — | Path to TLS certificate (PEM) |
| `TLS_KEY_FILE` | — | Path to TLS private key (PEM) |
| `REQUIRE_TLS` | `true` | Reject non-TLS requests |
| `TRUST_PROXY_TLS` | `false` | Trust that a reverse proxy terminated TLS; disables direct TLS |
| `MAX_BODY_BYTES` | `65536` | Maximum request body size (1 KiB – 10 MiB) |
| `AUTH_FAIL_LIMIT_PER_MIN` | `60` | Rate limit for auth failures per minute |
| `READINESS_REQUIRE_AUTH` | `true` | Whether `/ready` requires a valid bearer token |

### Authentication

| Variable | Description |
|---|---|
| `API_AUTH_CREDENTIALS` | Preferred. Multi-credential format: `id\|token\|scope1,scope2;id2\|token2\|*` |
| `API_AUTH_TOKEN` | Legacy. Single token with wildcard (`*`) scope |

### MQTT Adapter

| Variable | Default | Description |
|---|---|---|
| `MQTT_BROKER_URL` | — | Required when `mqtt` enabled. Prefer `mqtts://` |
| `MQTT_CLIENT_ID` | — | MQTT client identifier |
| `MQTT_USERNAME` | — | MQTT username |
| `MQTT_PASSWORD` | — | MQTT password |
| `MQTT_TOPIC_PREFIX` | `clocks/commands` | Topic prefix; commands publish to `{prefix}/{device-id}/{command-type}` |
| `MQTT_QOS` | `1` | Publish QoS level (0–2; adapter supports 0–1) |
| `MQTT_RETAINED` | `false` | Set the MQTT retained flag on published messages |
| `MQTT_CONNECT_RETRY` | `true` | Retry broker connection on failure |
| `MQTT_TLS_INSECURE_SKIP_VERIFY` | `false` | Skip TLS certificate verification for broker |
| `ALLOW_INSECURE_TLS_VERIFY` | `false` | Alias for `MQTT_TLS_INSECURE_SKIP_VERIFY` |
| `ALLOW_INSECURE_MQTT` | `false` | Allow plaintext `mqtt://` connections |

### REST Adapter

| Variable | Default | Description |
|---|---|---|
| `CLOCK_REST_BASE_URL` | — | Required when `rest` enabled. Prefer `https://` |
| `CLOCK_REST_TOKEN` | — | Bearer token for downstream REST service |
| `CLOCK_REST_TIMEOUT_MS` | `5000` | Per-request timeout in milliseconds |
| `CLOCK_REST_HEALTH_PATH` | — | Optional path to poll for readiness check |
| `ALLOW_INSECURE_DOWNSTREAM_HTTP` | `false` | Allow plaintext `http://` downstream connections |

### Sender Selection

| Variable | Default | Description |
|---|---|---|
| `ENABLED_SENDERS` | `mqtt,rest` | Comma-separated list of active senders (`mqtt`, `rest`) |

---

## Security Model

### Authentication

Commands require a `Authorization: Bearer <token>` header. The server supports multiple simultaneous credentials to enable rotation without downtime.

**Credential format** (`API_AUTH_CREDENTIALS`):

```
id|token|scope;id2|token2|scope2,scope3
```

**Examples:**

```bash
# Single wildcard credential
API_AUTH_CREDENTIALS="ops|s3cr3t|*"

# Multiple credentials with device scoping
API_AUTH_CREDENTIALS="ops|s3cr3t|*;dev|dev-token|clock-1,clock-2"

# With prefix matching
API_AUTH_CREDENTIALS="prod|prod-token|clock-*;monitor|mon-token|clock-monitor"
```

**Scope matching rules:**

| Scope | Matches |
|---|---|
| `*` | All devices |
| `clock-*` | Any device ID starting with `clock-` |
| `clock-1` | Exactly the device `clock-1` |

**Legacy:** `API_AUTH_TOKEN=<token>` is still accepted and behaves as a single wildcard-scoped credential.

### TLS

- `REQUIRE_TLS=true` (default): all inbound HTTP is rejected unless over TLS
- `TRUST_PROXY_TLS=true`: disables direct TLS; suitable when a reverse proxy (e.g. Traefik, nginx) terminates TLS upstream
- MQTT and REST adapters default to secure transports (`mqtts://`, `https://`); insecure variants require explicit opt-in flags

### Rate Limiting

Auth failures are rate-limited per source IP. The limit is configurable via `AUTH_FAIL_LIMIT_PER_MIN` (default: 60 per minute).

### Request Body Size

Requests are capped at `MAX_BODY_BYTES` (default: 64 KiB). Requests exceeding this limit are rejected with `413`.

---

## CLI: clockctl

`clockctl` is a command-line client for sending commands to the server.

### Installation

```bash
make build-clockctl
# binary written to bin/clockctl

# or run directly:
go run ./cmd/clockctl
```

### Configuration

| Variable | Default | Description |
|---|---|---|
| `CLOCK_SERVER_BASE_URL` | `http://localhost:8080` | Server base URL |
| `CLOCK_SERVER_HOST` | — | Overrides host in base URL (docker-style) |
| `CLOCK_SERVER_TOKEN` | — | Bearer token |
| `CLOCKCTL_TIMEOUT_MS` | `5000` | Request timeout in milliseconds |
| `CLOCKCTL_ALLOW_INSECURE_HTTP` | `false` | Allow plaintext HTTP connections |

### Commands

**Set alarm:**

```bash
go run ./cmd/clockctl alarm \
  --device clock-1 \
  --time 2030-06-01T07:00:00Z \
  --label "Wake up"
```

**Display message:**

```bash
go run ./cmd/clockctl message \
  --device clock-1 \
  --message "Meeting in 5 minutes" \
  --duration 30
```

**Set brightness:**

```bash
go run ./cmd/clockctl brightness \
  --device clock-1 \
  --level 75
```

---

## Development

### Prerequisites

- Go 1.22+
- Docker (for container builds and Compose tests)
- Helm (for Kubernetes deployment)
- An MQTT broker (e.g. Mosquitto) if using the MQTT adapter

### Build

```bash
# Format, tidy, test, build
make all

# Just build packages
make build

# Build clockctl binary
make build-clockctl   # output: bin/clockctl

# Clean build artifacts
make clean
```

### Code Quality

```bash
# Format code
make fmt

# Tidy go.mod/go.sum
make mod
```

---

## Testing

### Unit Tests

```bash
make unit-test
# or
go test ./...
```

### BDD Tests (Cucumber / godog)

BDD scenarios live in `internal/api/features/` and run as part of `go test ./...`.

Run the BDD suite alone:

```bash
make cucumber-test
# or
go test ./internal/api -run TestFeatures -v
```

### Integration Test (Docker Compose)

The Compose stack spins up Mosquitto + server + a one-shot `clockctl` container to validate end-to-end command flow:

```bash
# Run integration test
docker compose up --build --abort-on-container-exit clockctl

# Clean up
docker compose down -v --remove-orphans
```

The `clockctl` container sends a `message` command to `server` via `CLOCK_SERVER_HOST=http://server:8080`. Exit code 0 = pass.

### Race Detector

```bash
go test -race ./...
```

---

## Docker

### Build

```bash
make docker-build
# or with custom images:
make docker-build GO_IMAGE=golang:1.24.2-alpine RUNTIME_IMAGE=alpine:3.21
```

The Dockerfile uses a multi-stage build:
1. **Builder** (`golang:1.24.2-alpine`): compiles a fully static binary (`CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`)
2. **Runtime** (`alpine:3.21`): minimal image, non-root user (`uid:2000`), read-only binary, CA certs + timezone data

### Run Locally

```bash
make docker-run    # starts container on port 8080
make docker-stop   # stops the container
make docker-clean  # removes the image
```

### Push to Registry

```bash
# Default registry: 192.168.2.201:32000
make docker-push

# Override registry/tag:
make docker-push REGISTRY=ghcr.io/your-org IMAGE_TAG=1.2.3
```

This builds, then pushes both `:<tag>` and `:latest` tags.

---

## Kubernetes / Helm

The Helm chart is at `helm/clock-server/`. It targets k3s but works with any Kubernetes cluster.

### Default Security Posture

| Control | Setting |
|---|---|
| Run as non-root | `runAsNonRoot: true` |
| Read-only root filesystem | `readOnlyRootFilesystem: true` |
| Privilege escalation | `allowPrivilegeEscalation: false` |
| Capabilities | `drop: [ALL]` |
| Seccomp | `RuntimeDefault` |
| Service account automount | `false` |
| `/tmp` volume | `emptyDir` (required for any runtime temp use) |

### Minimal Install

Create a values override (e.g. `values-k3s.yaml`):

```yaml
image:
  repository: 192.168.2.201:32000/clock-server
  tag: "0.0.1"

auth:
  credentials: "ops|replace-me|*"

config:
  enabledSenders: "mqtt"
  mqtt:
    brokerURL: "mqtt://mosquitto.mqtt.svc.cluster.local:1883"
    allowInsecureMQTT: true
```

Install:

```bash
make helm-install HELM_VALUES=values-k3s.yaml
# or directly:
helm upgrade --install clock-server ./helm/clock-server \
  --namespace clock-system \
  --create-namespace \
  -f values-k3s.yaml
```

### With Ingress (Traefik)

```yaml
ingress:
  enabled: true
  className: traefik
  hosts:
    - host: clock.example.local
      paths:
        - path: /
          pathType: Prefix
```

### Using an Existing Secret

```yaml
auth:
  existingSecret: my-clock-server-secret
  secretKeys:
    credentials: API_AUTH_CREDENTIALS
    token: API_AUTH_TOKEN
```

### Uninstall

```bash
make helm-uninstall
```

### Chart Validation

The chart enforces at install time:
- At least one auth source (`credentials`, `legacyToken`, or `existingSecret`) must be provided
- If `mqtt` is in `enabledSenders`, `mqtt.brokerURL` must be set
- If `rest` is in `enabledSenders`, `rest.baseURL` must be set

---

## Makefile Reference

| Target | Description |
|---|---|
| `make all` | fmt + mod + test + build |
| `make fmt` | Format Go source |
| `make mod` | Tidy go.mod and go.sum |
| `make test` | Run unit and BDD tests |
| `make unit-test` | Run Go unit tests |
| `make cucumber-test` | Run BDD scenarios only |
| `make build` | Compile all packages |
| `make build-clockctl` | Build clockctl binary to `bin/clockctl` |
| `make clean` | Remove build artifacts |
| `make docker-build` | Build Docker image |
| `make docker-run` | Build and run container on port 8080 |
| `make docker-stop` | Stop the running container |
| `make docker-clean` | Remove the local image |
| `make docker-login` | Authenticate with the registry |
| `make docker-push` | Build and push `:<tag>` and `:latest` to registry |
| `make helm-install` | Install/upgrade Helm chart to k3s |
| `make helm-uninstall` | Remove Helm release |

**Key Makefile variables** (override on the command line):

| Variable | Default | Description |
|---|---|---|
| `IMAGE_NAME` | `clock-server` | Docker image name |
| `IMAGE_TAG` | `0.0.1` | Docker image tag |
| `REGISTRY` | `192.168.2.201:32000` | Docker registry |
| `HELM_NAMESPACE` | `clock-system` | Kubernetes namespace |
| `HELM_VALUES` | — | Path to Helm values override file |
| `GO_IMAGE` | `golang:1.24.2-alpine` | Go builder image |
| `RUNTIME_IMAGE` | `alpine:3.21` | Runtime base image |
