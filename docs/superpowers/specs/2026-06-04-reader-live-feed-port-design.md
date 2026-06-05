# TRA-902 — Reader Live-Feed Port (reader view + coverage diagnostic)

**Status:** design
**Ticket:** TRA-902 (parent TRA-897; related TRA-141, TRA-915)
**Date:** 2026-06-04

## Goal

Port Power Mixer's live tag-read feed into the TrakRF React frontend as a
reader live-view and coverage diagnostic. It shows the raw firehose of tag
reads off a fixed reader (CS463) — **every** read, registered as an asset or
not — so an operator can tune antenna placement and the RSSI threshold. This
is the human-facing counterpart to the backend ingest path; it reads the same
MQTT feed the backend ingester consumes, but in the browser.

## Source being ported

`/home/mike/power-mixer` (Next.js + Ant Design):

- `helpers/mqtt.ts` — `MqttOperations` singleton (mqtt.js over WebSocket)
- `hooks_and_commons/useTagManagement.tsx` — `Map<epc, Tag>` dedup, ~15s
  age-based expiry, 2s expiry tick
- `helpers/tagColorChange.ts` — age-band row coloring
- `components/TagDisplayTable.tsx` — Ant Design `<Table>` of current reads

The ~15s aging is also the exit-detection primitive the backend rules engine
(TRA-901) reuses, so the window matters and is preserved.

## Target

`/home/mike/platform/frontend` — React 18 + Vite + TypeScript, **Tailwind**
(no component library), Zustand for UI/router state, tab-based hash router.
No MQTT dependency exists yet.

## Message format (authoritative)

The CS463 publishes to topic `trakrf.id/{key}/reads`. Payload shape is
verified against live preview traffic 2026-06-04 (mirrors the backend
`parseCS463` in `internal/ingest/parser_cs463.go`):

```json
{ "tags": [
  { "epc": "...", "timeStampOfRead": 1717500000000000,
    "antennaPort": 1, "capturePointName": "...", "rssi": "-56" } ] }
```

- `timeStampOfRead` is **microseconds** since epoch.
- `rssi` is a **quoted string** ("-56"); parse leniently (float/blank → 0).
- The `{key}` segment of the topic identifies the reader; surface it as a
  column so the diagnostic works across multiple readers.

## Architecture

Direct **browser → MQTT broker over WebSocket** (the ticket's "port the MQTT
subscription into the frontend, single origin"). Three isolated units:

1. **`lib/readerfeed/parse.ts`** (pure) — `parseReaderPayload(topic, raw)` →
   `ParsedRead[]`. Tolerant of malformed JSON/rssi (returns `[]` / rssi 0,
   never throws). Extracts `{key}` from the topic. Pure → unit tested.

2. **`lib/readerfeed/store.ts`** (pure) — the dedup/expiry reducer extracted
   from `useTagManagement` as pure functions over a `Map`:
   `mergeReads(map, reads, receivedAt)` and `expireReads(map, now, ttlSec)`.
   Age and expiry are computed from **browser receive time**, not the reader
   clock — a deliberate improvement over Power Mixer (the backend already
   declares reader time "informational only"; this immunizes the
   expiry/coloring primitive against reader clock skew). The reader timestamp
   is still displayed as "last read". Pure → unit tested.

3. **`hooks/readerfeed/useReaderFeed.ts`** — owns the mqtt.js client
   lifecycle: connect on mount from env config, subscribe the topic, feed
   messages through `mergeReads`, run a 1s `expireReads` tick (ttl 15s),
   disconnect on unmount. Returns `{ reads, status, error, readerCount }`.
   Connection is **lazy** (on mount) — no module-load side effect, unlike the
   Power Mixer singleton that connects at import.

UI: **`components/LiveReadsScreen.tsx`** — re-skinned off Ant Design onto the
existing Tailwind table tokens (sticky `bg-gray-50 dark:bg-gray-700` header,
uppercase column labels, light/dark row variants — matching
`shared/DataTable.tsx`). Age-band row coloring adapted to light/dark. A
purpose-built table rather than `DataTable` because that component is built
around server pagination/sort; a 15s live window wants neither.

Columns: **EPC**, **Reader** ({key}), **Capture Point**, **Antenna**,
**RSSI**, **Age** (s, live). Header strip shows connection status, live read
count, and the RSSI range in view (the coverage-tuning numbers).

Age bands (TTL 15s): ≤1s fresh (green tint) → ≤3s → ≤6s → >6s fading/muted.

## Configuration (runtime, not build-time)

Browser-side MQTT config is injected at **runtime** via
`window.__APP_CONFIG__.readerFeed`, reusing the TRA-853 mechanism: the backend
frontend handler replaces a placeholder in `index.html` at serve time with an
inline `<script>` carrying config from pod env. One immutable bundle connects
to whatever broker the pod points at — infra flips the feed on by setting env
vars, **no frontend rebuild**. (We deliberately avoid build-time `VITE_*` vars;
project preference is runtime config wherever possible.)

Backend pod env (sourced at the composition root, `cmd/serve`):

- `READER_FEED_MQTT_URL` — `wss://host:port/path`; **empty disables the feed**
  (the default everywhere — builds/tests/preview stay inert until infra exposes
  WS, so the PR is safe to land first).
- `READER_FEED_MQTT_USERNAME` / `_PASSWORD`
- `READER_FEED_MQTT_TOPIC` — frontend defaults to `trakrf.id/+/reads` when blank.

These land in pre-auth `index.html`, so they are **public** — the broker user
MUST be least-privilege, subscribe-only (same exposure as a baked bundle, but
runtime-toggleable). When the URL is empty the page renders a clear "live feed
not configured" empty state rather than erroring.

### Infra prerequisite (out of this PR)

Browser MQTT needs a **WebSocket listener with a browser-trusted TLS cert** on
the broker, plus a read-only subscribe-only user for `trakrf.id/+/reads`.
Confirming/provisioning this is infra work (asked over cc2cc). The frontend
ships disabled-by-default so it does not block on it.

## Navigation / registration

New tab `live-reads`, gated to owner/admin (`canManageScanDevices`), grouped
with Scan Devices / Alarm Devices — it's an operator diagnostic. Wire into
`uiStore` `TabType`, `App.tsx` (`VALID_TABS` + `tabComponents` +
`loadingScreens`, lazy-loaded), and `TabNavigation.tsx` (`NavItem`,
`Radio` icon).

## Out of scope

- The Power Mixer `reader-api` branch (HTTP reader config: login, operational
  profile, antenna settings) — broken scaffolding; the CS463 is configured
  once by hand for the demo.
- Cross-referencing reads against provisioned assets to label
  registered/unregistered. The diagnostic intentionally shows the raw feed;
  "all reads" is satisfied by not filtering.
- Backend changes. This is frontend-only and parallelizable with the backend
  tickets.

## Testing

- `parse.test.ts` — valid payload, microsecond→ms, string rssi (neg/float/
  blank), malformed JSON, topic-key extraction, multi-tag.
- `store.test.ts` — dedup by EPC (last write wins), merge keeps latest,
  receive-time age, expiry past TTL, age-band classification boundaries.
- MQTT client lifecycle (connect/subscribe/teardown) is thin glue over
  mqtt.js; covered by manual/preview verification, not unit tests.
- `just frontend validate` (lint + typecheck + test + build) green, incl. the
  new `mqtt` dep building under Vite.
