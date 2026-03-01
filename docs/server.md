[Back to README](../README.md)

# Server -- `cmd/server`

The `cmd/server` binary is the main HTTP entrypoint for the Clock Command Dispatcher. It:

1. Loads configuration from environment variables (`config.LoadFromEnv`)
2. Builds and connects the enabled sender adapters (MQTT, REST, or both) via the bootstrap package
3. Wires the `CommandDispatcher` application service and the HTTP `Handler`
4. Starts an `http.Server` with configurable timeouts, optional native TLS, and graceful shutdown on `SIGINT`/`SIGTERM`

```bash
# Minimal local run (no TLS, MQTT only)
REQUIRE_TLS=false \
API_AUTH_TOKEN=dev-token \
ENABLED_SENDERS=mqtt \
MQTT_BROKER_URL=mqtt://localhost:1883 \
ALLOW_INSECURE_MQTT=true \
go run ./cmd/server
```

When `TLS_CERT_FILE` and `TLS_KEY_FILE` are both set, the server calls `ListenAndServeTLS`; otherwise it falls back to plain `ListenAndServe`. Graceful shutdown is controlled by `HTTP_SHUTDOWN_TIMEOUT_MS` (default 15 s).

---

## Internal Packages

### `internal/domain`

Pure domain layer -- no external dependencies.

**Key types:**

| Type | Description |
|---|---|
| `ClockCommand` (interface) | Contract every command must implement: `Execute(ctx)`, `TargetDeviceID()`, `CommandType()`, `Validate()` |
| `SetAlarmCommand` | Sets an alarm on a device. Validates that `DeviceID` is non-empty, `AlarmTime` is non-zero and not more than 1 minute in the past. |
| `DisplayMessageCommand` | Displays a message on a device. Validates `DeviceID`, non-empty `Message`, and `DurationSeconds` in the range 1--3600. |
| `SetBrightnessCommand` | Sets screen brightness. Validates `DeviceID` and `Level` in 0--100. |
| `ValidationError` | Typed error for domain invariant violations, used for error classification upstream. |

`Execute()` on each command currently delegates to `Validate()`. The `CommandType()` methods return stable strings: `set_alarm`, `display_message`, `set_brightness`.

---

### `internal/application`

Application service layer (use-case orchestration). Defines the **output port** and wires validation to sending.

**Key types:**

| Type | Description |
|---|---|
| `ClockCommandSender` (interface) | Output port: `Send(ctx, cmd) error`. Adapters implement this. |
| `ReadinessChecker` (interface) | Dependency health check: `Check(ctx) error`. Used by the `/ready` probe. |
| `CommandDispatcher` | Validates a command via `cmd.Execute()`, then forwards it through the configured `ClockCommandSender`. |

**Sentinel errors:**

| Error | Meaning |
|---|---|
| `ErrValidation` | Client-side validation problem (maps to HTTP 400) |
| `ErrDownstream` | Transport/integration failure (maps to HTTP 502) |

The dispatcher wraps domain `ValidationError` as `ErrValidation` and all other errors as `ErrDownstream`.

---

### `internal/adapters/mqtt`

MQTT adapter -- a long-lived, in-process MQTT 3.1.1 client built entirely on the standard library (no third-party MQTT dependency). Implements both `ClockCommandSender` and `ReadinessChecker`.

**Key behaviours:**

- Connects via raw TCP (or TLS) to the broker and performs MQTT CONNECT/CONNACK handshake
- Supports QoS 0 (fire-and-forget) and QoS 1 (with PUBACK)
- Topic format: `{TopicPrefix}/{deviceId}/{commandType}` (e.g. `clocks/commands/clock-1/set_alarm`)
- JSON payload includes `deviceId`, `type`, and command-specific fields
- Connection retry: when `ConnectRetry` is enabled, `Send()` retries up to 3 times on connection loss
- TLS is required by default (`mqtts://`); plain `mqtt://` requires `ALLOW_INSECURE_MQTT=true`
- `TLS_INSECURE_SKIP_VERIFY` requires explicit opt-in via `ALLOW_INSECURE_TLS_VERIFY=true`
- `Check()` returns an error if the connection is nil (used by `/ready`)
- `Close()` cleanly closes the TCP connection

**Config struct fields:** `BrokerURL`, `ClientID`, `Username`, `Password`, `TopicPrefix`, `QoS`, `Retained`, `ConnectRetry`, `TLSInsecureSkipVerify`, `AllowInsecureTLS`, `AllowInsecureTransport`.

---

### `internal/adapters/rest`

REST adapter -- forwards commands as JSON over HTTP to a downstream service. Implements both `ClockCommandSender` and `ReadinessChecker`.

**Request mapping:**

| Command | Method | Path |
|---|---|---|
| `SetAlarmCommand` | `POST` | `/clocks/{deviceId}/alarms` |
| `DisplayMessageCommand` | `POST` | `/clocks/{deviceId}/messages` |
| `SetBrightnessCommand` | `PUT` | `/clocks/{deviceId}/brightness` |

**Key behaviours:**

- Sends a `Bearer` token in the `Authorization` header when configured
- HTTPS is required by default; plain HTTP requires `ALLOW_INSECURE_DOWNSTREAM_HTTP=true`
- Default request timeout: 5 s (configurable via `CLOCK_REST_TIMEOUT_MS`)
- `Check()` performs a `GET` to `HealthPath` on the downstream service (no-op if path is empty)

**Config struct fields:** `BaseURL`, `AuthToken`, `Timeout`, `HealthPath`, `AllowInsecureHTTP`.

---

### `internal/adapters/composite`

Fan-out adapter -- dispatches a command through multiple `ClockCommandSender` implementations in sequence.

**Behaviour:**

- Calls each sender's `Send()` in order
- Collects all errors; returns them joined via `errors.Join`
- Returns an error if no senders are configured
- Skips nil senders (records an error for each)

This is the sender returned by the bootstrap package when multiple senders are enabled.

---

### `internal/api`

HTTP handlers and middleware. This is the **inbound adapter** (driving side).

**Routes:**

| Method | Path | Handler | Auth required |
|---|---|---|---|
| `GET` | `/health` | Liveness probe, always 200 | No |
| `GET` | `/ready` | Readiness probe, calls all `ReadinessChecker`s | Configurable (`READINESS_REQUIRE_AUTH`) |
| `POST` | `/commands/alarms` | Set alarm | Yes |
| `POST` | `/commands/messages` | Display message | Yes |
| `PUT` | `/commands/brightness` | Set brightness | Yes |

**Middleware chain (applied to all routes):**

1. **Request ID** -- reads `X-Request-Id` header or generates `req-{N}`
2. **TLS enforcement** -- rejects non-TLS requests when `REQUIRE_TLS=true` (426 Upgrade Required); trusts `X-Forwarded-Proto: https` when `TRUST_PROXY_TLS=true`
3. **Auth failure rate limiting** -- per-IP sliding window, configurable via `AUTH_FAIL_LIMIT_PER_MIN`
4. **Bearer token authentication** -- constant-time token comparison via `crypto/subtle`
5. **Device authorization** -- checks that the authenticated credential's scope covers the target device

**Security headers** on every response: `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, `Pragma: no-cache`.

**Audit logging** -- every command dispatch (accepted or failed) is logged with principal, remote IP, method, path, device, command type, result, and request ID.

**Body limiting** -- `http.MaxBytesReader` enforces `MAX_BODY_BYTES`; `json.Decoder.DisallowUnknownFields()` rejects unexpected JSON keys.

---

### `internal/config`

Loads all runtime configuration from environment variables (12-factor). Returns a validated `Config` struct or a descriptive error.

**Validation rules applied at load time:**

- `API_AUTH_CREDENTIALS` or `API_AUTH_TOKEN` must be set (at least one credential required)
- `TLS_CERT_FILE` and `TLS_KEY_FILE` must both be set or both be empty
- If `REQUIRE_TLS=true`, either TLS cert/key or `TRUST_PROXY_TLS=true` must be configured
- `ENABLED_SENDERS` entries must be `mqtt` or `rest`
- `CLOCK_REST_BASE_URL` must be a valid URL if set
- Numeric values are range-checked and fall back to defaults on parse errors

---

### `internal/bootstrap`

Wires sender adapters based on `ENABLED_SENDERS`. The single public function:

```go
func BuildCompositeSender(cfg config.Config) (
    application.ClockCommandSender,
    []application.ReadinessChecker,
    func(),   // cleanup function
    error,
)
```

For each enabled sender (`mqtt`, `rest`), it:

1. Creates the adapter via `mqtt.NewSender` or `rest.NewSender`
2. Registers it as both a sender and a readiness checker
3. Chains cleanup functions (e.g. `mqtt.Close()`)
4. Wraps all senders in a `composite.Sender`

---

### `internal/security`

Credential parsing and device-scope authorization.

**`Credential` struct:**

```go
type Credential struct {
    ID      string
    Token   string
    Devices []string   // scope list
}
```

**`ParseCredentials(raw string)`** parses the `API_AUTH_CREDENTIALS` format:

```
id|token|scope1,scope2;id2|token2|*
```

Semicolons separate credentials; pipes separate fields; commas separate scopes.

**`Allows(deviceID string) bool`** -- scope matching rules:

| Scope pattern | Matches |
|---|---|
| `*` | All devices |
| `clock-*` | Any device ID starting with `clock-` |
| `clock-1` | Exactly `clock-1` |

---

## Hexagonal Architecture

```
                        Inbound (Driving)
                              |
                    +---------v----------+
                    |    internal/api     |
                    | HTTP handlers/auth  |
                    +---------+----------+
                              |
                    +---------v----------+
                    | internal/application|
                    | CommandDispatcher   |
                    |   (use-case svc)    |
                    +---------+----------+
                              |
                   ClockCommandSender port
                              |
           +------------------+------------------+
           |                  |                  |
  +--------v-------+  +------v-------+  +-------v--------+
  | adapters/mqtt  |  | adapters/rest|  |adapters/composite|
  | MQTT 3.1.1     |  | HTTP client  |  | fan-out sender   |
  | sender         |  | sender       |  |                  |
  +----------------+  +--------------+  +------------------+
                        Outbound (Driven)

  Supporting:
    internal/config     - environment-variable configuration
    internal/bootstrap  - adapter wiring and cleanup
    internal/security   - credential parsing and device scoping
    internal/domain     - command models and validation
```

---

## Environment Variables

A consolidated reference of all environment variables read by the server.

### Server / HTTP

| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address |
| `HTTP_READ_TIMEOUT_MS` | `10000` | Read timeout (ms) |
| `HTTP_WRITE_TIMEOUT_MS` | `10000` | Write timeout (ms) |
| `HTTP_IDLE_TIMEOUT_MS` | `60000` | Idle connection timeout (ms) |
| `HTTP_READ_HEADER_TIMEOUT_MS` | `5000` | Header read timeout (ms) |
| `HTTP_SHUTDOWN_TIMEOUT_MS` | `15000` | Graceful shutdown period (ms) |
| `TLS_CERT_FILE` | -- | Path to TLS certificate (PEM) |
| `TLS_KEY_FILE` | -- | Path to TLS private key (PEM) |
| `REQUIRE_TLS` | `true` | Reject non-TLS requests (426) |
| `TRUST_PROXY_TLS` | `false` | Trust `X-Forwarded-Proto: https` from reverse proxy |
| `MAX_BODY_BYTES` | `65536` | Max request body size (1024--10485760) |
| `AUTH_FAIL_LIMIT_PER_MIN` | `60` | Per-IP auth failure rate limit (1--10000) |
| `READINESS_REQUIRE_AUTH` | `true` | Require bearer token for `/ready` |

### Authentication

| Variable | Default | Description |
|---|---|---|
| `API_AUTH_CREDENTIALS` | -- | Multi-credential string: `id\|token\|scope;...` |
| `API_AUTH_TOKEN` | -- | Legacy single-token (wildcard scope) |

### Sender Selection

| Variable | Default | Description |
|---|---|---|
| `ENABLED_SENDERS` | `mqtt,rest` | Comma-separated list: `mqtt`, `rest` |

### MQTT Adapter

| Variable | Default | Description |
|---|---|---|
| `MQTT_BROKER_URL` | -- | Broker URL (`mqtts://` or `mqtt://`) |
| `MQTT_CLIENT_ID` | auto-generated | MQTT client ID |
| `MQTT_USERNAME` | -- | MQTT username |
| `MQTT_PASSWORD` | -- | MQTT password |
| `MQTT_TOPIC_PREFIX` | `clocks/commands` | Topic prefix |
| `MQTT_QOS` | `1` | Publish QoS (0 or 1) |
| `MQTT_RETAINED` | `false` | MQTT retained flag |
| `MQTT_CONNECT_RETRY` | `true` | Retry on connection failure |
| `MQTT_TLS_INSECURE_SKIP_VERIFY` | `false` | Skip broker TLS cert verification |
| `ALLOW_INSECURE_TLS_VERIFY` | `false` | Required to enable `MQTT_TLS_INSECURE_SKIP_VERIFY` |
| `ALLOW_INSECURE_MQTT` | `false` | Allow plaintext `mqtt://` |

### REST Adapter

| Variable | Default | Description |
|---|---|---|
| `CLOCK_REST_BASE_URL` | -- | Downstream REST base URL |
| `CLOCK_REST_TOKEN` | -- | Bearer token for downstream |
| `CLOCK_REST_TIMEOUT_MS` | `5000` | Per-request timeout (ms) |
| `CLOCK_REST_HEALTH_PATH` | -- | Health-check path on downstream |
| `ALLOW_INSECURE_DOWNSTREAM_HTTP` | `false` | Allow plaintext `http://` downstream |

---

## Running the Server

```bash
# 1. With TLS and MQTT (production-like)
TLS_CERT_FILE=/path/to/cert.pem \
TLS_KEY_FILE=/path/to/key.pem \
API_AUTH_CREDENTIALS="ops|s3cr3t-token|*" \
ENABLED_SENDERS=mqtt \
MQTT_BROKER_URL=mqtts://broker.example.com:8883 \
go run ./cmd/server

# 2. Behind a reverse proxy (Traefik, nginx)
REQUIRE_TLS=true \
TRUST_PROXY_TLS=true \
API_AUTH_CREDENTIALS="ops|s3cr3t-token|*" \
ENABLED_SENDERS=mqtt,rest \
MQTT_BROKER_URL=mqtts://broker.example.com:8883 \
CLOCK_REST_BASE_URL=https://downstream.example.com \
go run ./cmd/server

# 3. Local development (no TLS, single legacy token)
REQUIRE_TLS=false \
API_AUTH_TOKEN=dev-token \
ENABLED_SENDERS=mqtt \
MQTT_BROKER_URL=mqtt://localhost:1883 \
ALLOW_INSECURE_MQTT=true \
go run ./cmd/server
```
