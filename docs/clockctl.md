[← Back to README](../README.md)

# clockctl — CLI Client

`clockctl` is a command-line client for the clock-server API. It dispatches commands to connected clock devices via the server's REST endpoints.

## Installation

```bash
go install ./cmd/clockctl
```

Or build from source:

```bash
go build -o clockctl ./cmd/clockctl
```

## Configuration

All configuration is via environment variables:

| Variable | Description | Default |
|---|---|---|
| `CLOCK_SERVER_BASE_URL` | Full base URL of the clock server | `http://localhost:8080` |
| `CLOCK_SERVER_HOST` | Docker-style host (e.g. `tcp://clock-server:8080`). Ignored if `CLOCK_SERVER_BASE_URL` is set. Automatically selects `http://` for localhost addresses and `https://` otherwise. | — |
| `CLOCK_SERVER_TOKEN` | Optional bearer token for authentication | — |
| `CLOCKCTL_TIMEOUT_MS` | HTTP request timeout in milliseconds | `5000` |
| `CLOCKCTL_ALLOW_INSECURE_HTTP` | Set to `true` to allow sending a bearer token over plain HTTP to non-localhost hosts | `false` |

When both `CLOCK_SERVER_BASE_URL` and `CLOCK_SERVER_HOST` are set, `CLOCK_SERVER_BASE_URL` takes precedence.

## Commands

### alarm

Set an alarm on a clock device.

```
clockctl alarm --device <id> --time <RFC3339> [--label <text>]
```

| Flag | Required | Description |
|---|---|---|
| `--device` | Yes | Clock device ID |
| `--time` | Yes | Alarm time in RFC 3339 format (e.g. `2026-03-01T07:00:00Z`) |
| `--label` | No | Human-readable alarm label |

Sends a `POST /commands/alarms` request to the server.

On success prints:

```
alarm command dispatched
```

### message

Display a text message on a clock device.

```
clockctl message --device <id> --message <text> [--duration <seconds>]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--device` | Yes | — | Clock device ID |
| `--message` | Yes | — | Message text to display |
| `--duration` | No | `10` | Display duration in seconds |

Sends a `POST /commands/messages` request to the server.

On success prints:

```
display-message command dispatched
```

### brightness

Set the brightness level of a clock device.

```
clockctl brightness --device <id> --level <0-100>
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--device` | Yes | — | Clock device ID |
| `--level` | No | `50` | Brightness level (0–100) |

Sends a `PUT /commands/brightness` request to the server.

On success prints:

```
brightness command dispatched
```

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Command dispatched successfully |
| `1` | Runtime error (missing required flag, invalid `--time` value, server returned an error, network failure) |
| `2` | Usage error (missing or unknown subcommand) |

## Examples

Set an alarm for 7 AM:

```bash
clockctl alarm --device clock-01 --time 2026-03-01T07:00:00Z --label "Wake up"
```

Display a message for 30 seconds:

```bash
clockctl message --device clock-01 --message "Meeting in 5 min" --duration 30
```

Set brightness to maximum:

```bash
clockctl brightness --device clock-01 --level 100
```

Use with a remote server and authentication:

```bash
export CLOCK_SERVER_BASE_URL=https://clocks.example.com
export CLOCK_SERVER_TOKEN=my-secret-token
clockctl brightness --device clock-01 --level 75
```

Use with a Docker-style host (e.g. from a linked container):

```bash
export CLOCK_SERVER_HOST=tcp://clock-server:8080
clockctl alarm --device clock-01 --time 2026-03-01T06:30:00Z
```
