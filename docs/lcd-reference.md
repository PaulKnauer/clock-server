[Back to README](../README.md)

# LCD Clock Device Reference

This document defines the integration contract for an LCD-based clock device that will be driven by `clock-server`.

It has two purposes:

1. Capture the MQTT command contract that `clock-server` already publishes today.
2. Define the additional device-side feature set and MQTT events needed to complete the end-to-end clock project.

## Scope

The LCD device is assumed to be a dedicated smart clock endpoint with a unique `device_id` such as `clock-kitchen` or `clock-bedroom`.

This reference is split into:

- `Implemented now`: behavior already supported by this repository.
- `Required for LCD project`: behavior the LCD device should publish or support so the overall system is complete.

## Device Identity Rules

`device_id` values must match the current server validation rules:

- 1 to 64 characters
- allowed characters: `a-z`, `A-Z`, `0-9`, `_`, `-`
- not allowed: `/`, `+`, `#`, spaces

Examples:

- valid: `clock-1`
- valid: `kitchen_clock`
- invalid: `clock/1`
- invalid: `clock+1`

## Implemented Now: Commands Published By `clock-server`

The MQTT adapter currently publishes commands to:

```text
{topic_prefix}/{device_id}/{command_type}
```

Default `topic_prefix`:

```text
clocks/commands
```

So the concrete topics are:

- `clocks/commands/{device_id}/set_alarm`
- `clocks/commands/{device_id}/display_message`
- `clocks/commands/{device_id}/set_brightness`

### Transport Expectations

- MQTT 3.1.1
- QoS: `0` or `1` are supported by the server adapter
- Retained: configurable; defaults to `false`
- Broker URL should normally use `mqtts://`
- Plain `mqtt://` is only for explicitly allowed local/insecure setups

### `set_alarm`

Topic:

```text
clocks/commands/{device_id}/set_alarm
```

Payload:

```json
{
  "deviceId": "clock-1",
  "type": "set_alarm",
  "alarmTime": "2030-01-01T06:00:00Z",
  "label": "Wake up"
}
```

Device behavior:

- create or replace a scheduled alarm entry
- store `alarmTime` as an absolute RFC 3339 timestamp
- show `label` when useful in the local UI

### `display_message`

Topic:

```text
clocks/commands/{device_id}/display_message
```

Payload:

```json
{
  "deviceId": "clock-1",
  "type": "display_message",
  "message": "Meeting in 5 minutes",
  "durationSeconds": 30
}
```

Device behavior:

- immediately show the message on the LCD
- keep it visible for `durationSeconds`
- then return to the normal clock screen

### `set_brightness`

Topic:

```text
clocks/commands/{device_id}/set_brightness
```

Payload:

```json
{
  "deviceId": "clock-1",
  "type": "set_brightness",
  "level": 75
}
```

Device behavior:

- set LCD/backlight brightness on a `0-100` scale
- persist the latest brightness locally so it survives reboot if possible

## Required For LCD Project: Device Feature Set

The LCD device should support these capabilities so the current command set is actually useful:

### Core Display Features

- always-on time display in local timezone
- date display
- visible connectivity indicator for Wi-Fi/MQTT health
- brightness control from MQTT command
- temporary message overlay from MQTT command
- alarm scheduled indicator
- alarm active/ringing state

### Alarm UX Features

- local buzzer or other alarm indicator when an alarm becomes active
- local dismiss action
- optional snooze action
- visible next-alarm summary on screen

### Reliability Features

- reconnect to MQTT automatically
- resubscribe after reconnect
- restore last known brightness after restart
- recover cleanly after power loss
- sync time via NTP or equivalent before evaluating alarms

## Required For LCD Project: MQTT Topics The Device Should Publish

The server currently publishes commands, but it does not yet consume device-originated MQTT events. The following event contract is recommended so the LCD project has a stable target and the backend can be extended to consume it later.

Recommended namespaces:

- commands from server to device: `clocks/commands/{device_id}/...`
- events from device to backend/observers: `clocks/events/{device_id}/...`
- state snapshots from device: `clocks/state/{device_id}/...`

### 1. Online / Presence

Topic:

```text
clocks/state/{device_id}/presence
```

Payload:

```json
{
  "deviceId": "clock-1",
  "state": "online",
  "at": "2030-01-01T05:55:00Z",
  "firmwareVersion": "1.0.0",
  "ip": "192.168.1.42"
}
```

Requirements:

- publish `online` on boot and reconnect
- publish `offline` via MQTT last will if supported
- retain the latest presence message

### 2. Heartbeat

Topic:

```text
clocks/events/{device_id}/heartbeat
```

Payload:

```json
{
  "deviceId": "clock-1",
  "at": "2030-01-01T05:56:00Z",
  "uptimeSeconds": 120,
  "wifiRssi": -58,
  "mqttConnected": true,
  "ntpSynced": true
}
```

Requirements:

- publish every 30 to 60 seconds
- use QoS `0`
- do not retain heartbeat events

### 3. Display State

Topic:

```text
clocks/state/{device_id}/display
```

Payload:

```json
{
  "deviceId": "clock-1",
  "mode": "clock",
  "brightness": 75,
  "messageActive": false,
  "lastMessage": "Meeting in 5 minutes",
  "updatedAt": "2030-01-01T05:56:10Z"
}
```

Requirements:

- publish on brightness changes
- publish when a message overlay starts and ends
- retain the latest display state snapshot

### 4. Alarm State Snapshot

Topic:

```text
clocks/state/{device_id}/alarm
```

Payload:

```json
{
  "deviceId": "clock-1",
  "enabled": true,
  "nextAlarmTime": "2030-01-01T06:00:00Z",
  "label": "Wake up",
  "updatedAt": "2030-01-01T05:56:15Z"
}
```

Requirements:

- publish whenever the device accepts or changes its current scheduled alarm
- retain the latest alarm snapshot

### 5. Alarm Triggered

Topic:

```text
clocks/events/{device_id}/alarm_triggered
```

Payload:

```json
{
  "deviceId": "clock-1",
  "alarmTime": "2030-01-01T06:00:00Z",
  "label": "Wake up",
  "triggeredAt": "2030-01-01T06:00:00Z"
}
```

Requirements:

- publish exactly once when an alarm starts ringing
- use QoS `1`
- do not retain

### 6. Alarm Acknowledged

Topic:

```text
clocks/events/{device_id}/alarm_acknowledged
```

Payload:

```json
{
  "deviceId": "clock-1",
  "action": "dismiss",
  "at": "2030-01-01T06:00:08Z",
  "source": "button"
}
```

Allowed `action` values:

- `dismiss`
- `snooze`

Requirements:

- publish when the user dismisses or snoozes locally
- use QoS `1`

### 7. Command Processing Result

Topic:

```text
clocks/events/{device_id}/command_result
```

Payload:

```json
{
  "deviceId": "clock-1",
  "commandType": "set_brightness",
  "status": "applied",
  "at": "2030-01-01T05:56:20Z",
  "detail": ""
}
```

Allowed `status` values:

- `received`
- `applied`
- `rejected`
- `failed`

Requirements:

- publish for each consumed command
- use this to diagnose invalid payloads, unsupported commands, or hardware failures

## Recommended MQTT Delivery Rules

Use these defaults unless there is a device-level reason not to:

| Topic type | QoS | Retained |
|---|---:|---:|
| `clocks/commands/...` | `1` | `false` |
| `clocks/events/.../heartbeat` | `0` | `false` |
| `clocks/events/.../alarm_triggered` | `1` | `false` |
| `clocks/events/.../alarm_acknowledged` | `1` | `false` |
| `clocks/events/.../command_result` | `0` or `1` | `false` |
| `clocks/state/...` | `1` | `true` |

## LCD Device State Machine

Recommended top-level UI modes:

- `clock`: default time/date view
- `message`: transient text overlay
- `alarm_ringing`: alarm active locally
- `settings`: optional local config mode
- `error`: clock cannot keep correct time or has lost required services

Priority order:

1. `alarm_ringing`
2. `message`
3. `clock`

This keeps transient messages from hiding an active alarm.

## Implementation Notes For The LCD Project

- treat MQTT command payloads as authoritative instructions from the backend
- validate `deviceId` matches the local configured device identity before applying a command
- parse all timestamps as UTC-capable RFC 3339 values
- store enough local state to redraw the LCD after reconnect or reboot
- prefer idempotent handling so duplicate MQTT deliveries do not create duplicate alarms
- if local buttons exist, publish user actions as events rather than mutating hidden state only on-device

## Gap Between Current Backend And Full Device Workflow

What exists today in `clock-server`:

- HTTP API for dispatching commands
- MQTT publishing for the three command types

What is not implemented yet in this repository:

- subscription/ingestion of device-originated MQTT events
- persistence of device state
- APIs for reading back display state, presence, or alarm acknowledgements

That means the LCD project can implement the event topics above now, but additional backend work will still be needed before those events are consumed centrally.
