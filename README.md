# Clock Command Dispatcher (Hexagonal Go Service)

A Go microservice that dispatches smart clock commands using Ports & Adapters (Hexagonal Architecture).

## Security Model

- Commands require bearer auth using credential identities with scoped device access.
- Credential rotation is supported with multiple active credentials.
- Request bodies are size-limited (default 64 KiB).
- Rate limiting is applied to repeated auth failures.
- TLS is enforced by default for inbound API traffic and downstream transports.

Credential format (`API_AUTH_CREDENTIALS`):

```text
id|token|scope1,scope2;id2|token2|*
```

Scope matching:
- `*` => all devices
- `clock-*` => prefix match
- `clock-1` => exact device

Legacy `API_AUTH_TOKEN` is still accepted (mapped to a single wildcard-scoped credential).

## Architecture

- `internal/domain`: Core command model and validation rules
- `internal/application`: Use-case service, output port (`ClockCommandSender`), typed application errors
- `internal/adapters/mqtt`: MQTT adapter implementation (long-lived in-process client)
- `internal/adapters/rest`: REST adapter implementation
- `internal/adapters/composite`: Composite sender that dispatches to multiple adapters
- `internal/api`: REST API handlers, TLS/auth middleware, rate limiting, health/readiness endpoints
- `internal/config`: Environment-based configuration loading/validation
- `internal/bootstrap`: Wiring logic for configured adapters and readiness checks
- `cmd/server`: HTTP service entrypoint
- `cmd/clockctl`: CLI client that sends command requests to `cmd/server`

## API Endpoints

- `GET /health` (liveness)
- `GET /ready` (readiness)
- `POST /commands/alarms`
- `POST /commands/messages`
- `PUT /commands/brightness`

## Configuration

Core server:
- `HTTP_ADDR` (default `:8080`)
- `HTTP_READ_TIMEOUT_MS` (default `10000`)
- `HTTP_WRITE_TIMEOUT_MS` (default `10000`)
- `HTTP_IDLE_TIMEOUT_MS` (default `60000`)
- `HTTP_READ_HEADER_TIMEOUT_MS` (default `5000`)
- `HTTP_SHUTDOWN_TIMEOUT_MS` (default `15000`)
- `TLS_CERT_FILE` (optional)
- `TLS_KEY_FILE` (optional)
- `REQUIRE_TLS` (default `true`)
- `TRUST_PROXY_TLS` (default `false`)
- `READINESS_REQUIRE_AUTH` (default `true`)
- `MAX_BODY_BYTES` (default `65536`)
- `AUTH_FAIL_LIMIT_PER_MIN` (default `60`)

Authentication:
- `API_AUTH_CREDENTIALS` (preferred)
- `API_AUTH_TOKEN` (legacy fallback)

MQTT adapter:
- `MQTT_BROKER_URL` (required if `mqtt` enabled; prefers `mqtts://`)
- `MQTT_CLIENT_ID`
- `MQTT_USERNAME`
- `MQTT_PASSWORD`
- `MQTT_TOPIC_PREFIX` (default `clocks/commands`)
- `MQTT_QOS` (default `1`, valid range `0..2`, adapter supports publish QoS `0..1`)
- `MQTT_RETAINED` (default `false`)
- `MQTT_CONNECT_RETRY` (default `true`)
- `MQTT_TLS_INSECURE_SKIP_VERIFY` (default `false`)
- `ALLOW_INSECURE_TLS_VERIFY` (default `false`)
- `ALLOW_INSECURE_MQTT` (default `false`)

REST adapter:
- `CLOCK_REST_BASE_URL` (required if `rest` enabled, prefers `https://`)
- `CLOCK_REST_TOKEN`
- `CLOCK_REST_TIMEOUT_MS` (default `5000`)
- `CLOCK_REST_HEALTH_PATH` (optional)
- `ALLOW_INSECURE_DOWNSTREAM_HTTP` (default `false`)

Selector:
- `ENABLED_SENDERS` (default `mqtt,rest`)

CLI (`clockctl`):
- `CLOCK_SERVER_BASE_URL` (default `http://localhost:8080`)
- `CLOCK_SERVER_HOST` (docker-style host)
- `CLOCK_SERVER_TOKEN`
- `CLOCKCTL_TIMEOUT_MS` (default `5000`)
- `CLOCKCTL_ALLOW_INSECURE_HTTP` (default `false`)

## Running

```bash
go run ./cmd/server
```

```bash
go run ./cmd/clockctl alarm --device clock-1 --time 2030-01-01T07:00:00Z --label wake
```

## Helm (k3s)

Create a values override, for example `values-k3s.yaml`:

```yaml
image:
  repository: ghcr.io/your-org/clock-server
  tag: "latest"

auth:
  credentials: "ops|replace-me|*"

config:
  enabledSenders: "mqtt"
  mqtt:
    brokerURL: "mqtt://mosquitto.mqtt.svc.cluster.local:1883"
```

Install to k3s:

```bash
helm upgrade --install clock-server ./helm/clock-server \
  --namespace clock-system \
  --create-namespace \
  -f values-k3s.yaml
```

Optional ingress:

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

## Testing

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod-cache go test ./...
```

## BDD Testing (Cucumber for Go)

BDD scenarios are implemented with `godog` (Cucumber for Go) in `internal/api/features`.
They run as part of `go test ./...`.

Run only the BDD suite:

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod-cache go test ./internal/api -run TestFeatures -v
```

## Docker Compose (feature test)

Use Compose to validate `clockctl` connecting to `server` from a different host name via `CLOCK_SERVER_HOST`.

Run the one-shot test:

```bash
docker compose up --build --abort-on-container-exit clockctl
```

This stack includes:
- `server` (API)
- `mosquitto` (MQTT dependency)
- `clockctl` (CLI test container)

The CLI container is configured with:
- `CLOCK_SERVER_HOST=http://server:8080`
- `CLOCK_SERVER_TOKEN=dev-token`
- `CLOCKCTL_ALLOW_INSECURE_HTTP=true` (dev-only)

To clean up:

```bash
docker compose down -v --remove-orphans
```
