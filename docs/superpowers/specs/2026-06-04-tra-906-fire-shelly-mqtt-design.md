# TRA-906 — Fire Shelly alarm over MQTT (firewall-friendly path)

**Status:** Design (approved: 1A/2A, namespace (b))
**Date:** 2026-06-04
**Ticket:** [TRA-906](https://linear.app/trakrf/issue/TRA-906) (epic TRA-897; builds on [[TRA-903]]/PR #450)
**Branch:** `feat/tra-906-fire-shelly-mqtt` (stacked on `feat/tra-903-alarm-device-shelly`)

## Why

TRA-903 fires the Shelly over local HTTP. That only works when the firing
backend is on the **same network** as the relay. From cloud preview/prod the
backend cannot reach a LAN/firewalled device (confirmed live: Test-fire → 502
"no route"). The firewall-friendly path: the Shelly connects **outbound** to our
shared MQTT broker and subscribes to a command topic; the cloud backend
**publishes** the trigger. Nothing inbound to the device.

## Decisions

- **1A — Shelly native MQTT control**: publish `on`/`off` to
  `<command_topic>/command/switch:<switch_id>` (Shelly Gen2+ MQTT control). Not
  RPC-over-MQTT.
- **2A — publish-and-trust**: MQTT publish is fire-and-forget; Test-fire returns
  200 if the broker accepts the publish (we do NOT wait for device status). HTTP
  keeps its real 502-on-unreachable semantics.
- **Namespace (b)** (infra): keep one per-device namespace
  `trakrf.id/{key}/...` (reads `/reads`, commands `/command`, status `/status`).
  Infra narrows the ingest subscription `trakrf.id/#` → `trakrf.id/+/reads` so
  alarm commands aren't delivered to the read parser. (infra one-line chart PR.)
- **HTTP stays** the edge/on-LAN fast path; transport is per-device.

## Broker facts (infra)

Self-hosted Mosquitto. Backend publishes on its existing `MQTT_URL` connection
(shared `trakrf-mqtt` user, TLS 1.2, no per-topic ACL yet → TRA-857). Hosts:
`mqtt.{preview,prod}.gke.trakrf.id:8883`. Shelly connects outbound mqtts and
must trust the LE chain (same CA gotcha as the GL-S10 readers,
project_mqtt_reader_cert_trust) — a runbook (TRA-904) item, not code.

## Architecture

### Model — migration `000015`
Add to `alarm_devices`:
- `transport` enum `alarm_transport ('http','mqtt')` NOT NULL DEFAULT `'http'`
- `command_topic` VARCHAR(255) NULL — the Shelly's MQTT topic **prefix**
  (operator sets it to match the device's configured prefix; backend publishes
  to `<command_topic>/command/switch:<switch_id>`)

`base_url`/`switch_id` stay (http path + the switch channel, shared by both).

### Dispatch — `internal/alarm`
A `Dispatcher` chooses the transport per device:
```go
type httpSetter   interface { Set(ctx, baseURL string, switchID int, on bool) error } // *shelly.Client
type mqttPublisher interface { Publish(ctx, commandTopic string, switchID int, on bool) error }

type Dispatcher struct { http httpSetter; mqtt mqttPublisher }
func (d Dispatcher) Set(ctx, dev alarmdevice.AlarmDevice, on bool) error {
    if dev.Transport == alarmdevice.TransportMQTT {
        return d.mqtt.Publish(ctx, dev.CommandTopic, dev.SwitchID, on)
    }
    return d.http.Set(ctx, dev.BaseURL, dev.SwitchID, on)
}
```
- `Firer` (TRA-903) is refactored to hold a device-aware setter (the Dispatcher)
  instead of the bare http driver: it calls `disp.Set(ctx, dev, true)` per bound
  device. Same membership/location/best-effort behaviour.
- The alarmdevices handler's Test/Reset also take the Dispatcher, so test-fire
  /reset respect per-device transport.

### MQTT publisher — `internal/alarm/mqttpub` (or `internal/ingest` publisher)
Wraps a paho client connected to `MQTT_URL` (reuse `ingest.Config`). Publishes
`on`/`off` (QoS 1) to `<commandTopic>/command/switch:<switchID>`. Unit-tested via
an injected publish func (assert topic + payload); the paho wiring is the thin
real impl. Only constructed when MQTT is enabled (same gate as the subscriber).

### Wiring — `serve.go`
When `mqttCfg.Enabled()`: build the publisher, build
`alarm.Dispatcher{http: shellyClient, mqtt: publisher}`; inject the Dispatcher
into both the geofence firer and the alarmdevices handler. When MQTT disabled:
Dispatcher with a nil/no-op mqtt (mqtt-transport devices error clearly "MQTT
disabled"), http path unaffected.

### Frontend
Alarm device form: a `transport` select (HTTP / MQTT). Show `base_url` for HTTP,
`command_topic` for MQTT (switch_id shared). List shows transport. Test-fire/
Reset buttons unchanged (server dispatches).

## Test-fire UX caveat (2A)
For an MQTT device, a green Test-fire means "command published to broker," not
"relay confirmed on." Surface that in the UI helper text so Tim isn't misled.

## Testing (TDD)
- Dispatcher: http device → http.Set with base_url; mqtt device → mqtt.Publish
  with `<command_topic>/command/switch:<id>` + correct payload; mqtt device with
  nil publisher → clear error.
- MQTT publisher: injected publish func asserts topic/payload/QoS; on/off.
- Firer: unchanged behaviour through the Dispatcher (re-point existing tests).
- Storage/handler/frontend: transport + command_topic round-trip; mqtt test-fire
  publishes (fake).
- `just validate` green.

## Out of scope
- Per-topic ACLs / per-device broker creds → TRA-857.
- Device provisioning (Shelly MQTT config + CA trust) → manual/on-LAN, TRA-904
  runbook. The cloud backend cannot provision the device (same firewall reason).
- Status-confirmed test-fire (2B) — deferred.
