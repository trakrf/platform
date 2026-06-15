# TRA-995 — Consolidated fixed-reader antenna & power layout

**Date:** 2026-06-15
**Status:** Approved (design)
**Ticket:** TRA-995 — Improve fixed reader power settings layout
**Surface:** Frontend only (`frontend/src/components/scandevices`)

## Problem

In the fixed-reader edit surface (the "Antennas & Location" section), a CS463
multi-antenna reader renders **two stacked, full-width blocks**:

1. **Scan Points** (`ScanPointsPanel`) — a wide table (Name, Location, Antenna
   Port, Active, actions) whose rows are edited through a **modal dialog**
   (`ScanPointForm`).
2. **Antenna Transmit Power** (`AntennaPowerPanel`) — one **full-width** slider
   per antenna.

Both blocks are keyed by antenna yet presented separately, consuming far more
vertical and horizontal space than the information warrants, and the dialog is
heavyweight for what amounts to picking a location.

## Goals

- **Consolidate** the two blocks into a single per-antenna view.
- **Narrow the slider** to ~40% width (not full width).
- **Click-to-edit** the per-antenna values (location, enable) inline — no dialog.
- **Responsive**: usable down to phone width.
- Mirror how fixed readers present themselves: a row per antenna the reader
  reports, with an **enable/disable checkbox**.

## Design

### One row per reported antenna

The reader self-reports its antenna count via the MQTT-RPC contract
(`capabilities.antennas`). We render **exactly `capabilities.antennas` rows**,
numbered `1..N`:

- **CS463** reports `4`. (A third-party "dumb" RF mux behind a CS463 is invisible
  to the reader, so it still reports 4 — we render 4, matching CS463's own UI.)
- **Impinj R420 / R700 with the official Antenna Hub** report up to `32` logical
  antennas natively over LLRP/REST. To this UI that is simply "a reader that
  reports 32 antennas" — **no special-casing**. (Decision: standardize on the
  official Impinj hubs, not third-party muxes, precisely because the firmware
  exposes logical antennas natively and fits the capabilities-driven model.)

The list is a **flat, scrollable list**. Grouping 32 antennas by physical port
(R700 hub: Port 1 → 1–8, etc.) is **deferred to a future ticket** — the contract
does not yet expose a port→antenna mapping, and CS463 (today's only hardware)
has 4.

### Per-antenna row contents

Each row `n` (1-based) shows, left to right on desktop:

| Element | Control | Bound to |
| --- | --- | --- |
| Enable | checkbox | scan_point `is_active` for the point whose `antenna_port == n` |
| Number | static bold `n` (no "Ant" word) | — |
| Location | click-to-edit select (`InlineEditCell` variant `select`) | scan_point `location_id` |
| Power | range slider (~40% col) + `dBm` value | reader config `tx_power_dbm[n]` (RPC) |

- **No Name, no Description, no Antenna Port** column. The antenna number *is* the
  port; name/description are dropped from this surface as low-value (see Scope).
- **Disabled (unchecked) rows dim** (reduced opacity) but remain editable — you
  can set location/power before enabling.
- The dBm range label (`10–31.5 dBm`) and reader model show once in the section
  header, as today.

### Data model & wiring

Two independent backends are joined per antenna number:

- **Power** comes from the live reader config (`useReaderConfig` →
  `config.tx_power_dbm`) and is pushed back with `useSetReaderConfig`
  (`tx_power_dbm` map). Power exists for **all `N` antennas regardless of scan
  points** — the slider always works.
- **Location + enable** live on the **scan_point** with `antenna_port == n`
  (`useScanPoints` / `useScanPointMutations`). A given antenna may have **no
  scan_point yet**.

**Join rule:** for antenna `n`, find the scan_point with `antenna_port === n`
(first match; the fixed-N model assumes ≤1 scan_point per port). Antennas with no
match render as unchecked, location "— set location —".

**Lazy create / update semantics:**

- **Enable ON** with no scan_point → `create({ antenna_port: n, name:
  "Antenna n", location_id: null, is_active: true })`.
- **Set location** with no scan_point → `create({ antenna_port: n, name:
  "Antenna n", location_id, is_active: true })` (commissioning implies enabled).
- **Enable OFF** on an existing scan_point → `update({ id, { is_active: false }})`.
- **Set location** on an existing scan_point → `update({ id, { location_id }})`.

So **create always sets `is_active: true`**; **disable only ever updates** an
existing point. The synthesized name (`Antenna n`) satisfies the required
`name` field without surfacing it in the UI.

`is_active` is the existing read-attribution gate (a disabled antenna's reads are
not attributed); we reuse it as the "enable" semantic rather than introducing a
reader-hardware antenna-disable (which the RPC contract does not expose).

### Power behaviour (preserved from `AntennaPowerPanel`)

- Local slider state per antenna for smooth dragging; **debounced ~2s** before a
  single `setConfig` push carrying the **full** `tx_power_dbm` map.
- `valuesRef` mirror to avoid stale-closure on the debounced flush.
- Toast on send; the `pending_reload` inline note ("Applies on the next inventory
  cycle — reads briefly pause.") is retained.
- Offline reader (RPC error / no capabilities) shows the existing amber
  "Reader did not respond (offline?)" state — the whole panel degrades, since
  without capabilities we don't know the antenna count.

### Responsive layout

- **Desktop / tablet** (`sm:` and up): one line per antenna — `[✓] n  location`
  on the left (grows), slider + dBm in a ~40% (`sm:w-[44%]`) right column.
- **Phone** (below `sm:`): **two lines** per antenna —
  - Line 1: `[✓] n` … `location` (space-between).
  - Line 2: slider (full width) + dBm.
- Implemented with Tailwind responsive utilities on a single component (no JS
  breakpoint logic).

### Component structure

Replace the two-panel `multi_point` branch of `ReaderPointsSection` with one new
component:

- **`AntennaSettingsPanel({ deviceId })`** — orchestrator. Fetches
  `useReaderConfig`, `useScanPoints`, `useScanPointMutations`, `useLocations`,
  `useSetReaderConfig`. Builds a merged per-antenna view model
  (`{ n, scanPoint?, power, enabled, locationId }[]`) of length
  `capabilities.antennas`. Owns the debounced power flush and the lazy
  create/update handlers. Renders the section header + a list of rows.
- **`AntennaRow`** — presentational row: checkbox, number, location
  `InlineEditCell`, slider + dBm. Receives value + `onToggleEnabled`,
  `onSetLocation`, `onPowerChange` callbacks. Carries the responsive markup.

`ReaderPointsSection` `multi_point` branch renders only
`<AntennaSettingsPanel deviceId={device.id} />` (drops the separate
`ScanPointsPanel` + `AntennaPowerPanel` stack and the "Antenna Transmit Power"
sub-heading). `single_point` and `handheld` branches are unchanged. The outer
section heading stays "Antennas & Location" (in `ScanDeviceFormModal`) — or is
retitled "Antennas & Power" if trivial; not load-bearing.

### Removed / now-dead code

- `AntennaPowerPanel.tsx` (+ test) — logic folded into `AntennaSettingsPanel`.
- `ScanPointsPanel.tsx` (+ test) — replaced; was only used here.
- `ScanPointForm.tsx` — the dialog; only used by `ScanPointsPanel`. Remove with
  it. (Confirm no other importer before deleting.)

## Testing

- **`AntennaSettingsPanel` unit tests** (Vitest + RTL), mocking the hooks:
  - Renders `capabilities.antennas` rows; seeds power from config; defaults
    missing power to capability max.
  - Location edit on an existing scan_point → `update` with `location_id`.
  - Location edit / enable with no scan_point → `create` with `is_active: true`,
    synthesized name, `antenna_port: n`.
  - Disable on existing → `update` `is_active: false`.
  - Slider change debounces and flushes the full `tx_power_dbm` map (carry over
    the relevant existing `AntennaPowerPanel` test cases).
  - Offline (no capabilities / error) → amber offline state.
- **`ReaderPointsSection` test** updated: `multi_point` renders
  `AntennaSettingsPanel`; `single_point` / `handheld` unchanged.
- `pnpm validate` (typecheck + lint + unit) green.
- Manual: verify against `cs463-212` rig (4 antennas) — location click-edit,
  enable toggle (incl. lazy create), slider push; check phone-width reflow.

## Scope / non-goals

- **Drop Name & Description editing** on this surface (deliberate; the antenna
  number is the identity, name is auto-synthesized, description was optional
  metadata with no other editor). Dropping **Name** is confirmed safe: scan-point
  name was once the CS463 read-correlation key but TRA-956 moved matching to
  `antenna_port` (`backend/internal/ingest/parser_cs463.go` — *"AntennaPort
  routes the read to its per-antenna scan_point downstream"*); name now plays no
  role in ingest. **Description still flagged for user review** — if it must
  remain editable somewhere, that's a separate affordance.
- **No backend / API / migration changes.** Pure frontend; reuses existing
  scan_point CRUD + reader-config RPC.
- **Port grouping for 32-antenna hub readers** — future ticket.
- **Retiring `scan_point.name`** — with parsing moved to `antenna_port` (TRA-956)
  and this change removing its last editor, `name` is effectively a dead-end
  field. Making it optional / dropping the column is a backend + migration change
  and is **out of scope here**; candidate follow-up ticket. For now we satisfy
  the still-required `CreateScanPointRequest.name` by synthesizing `"Antenna n"`.
- **Hardware antenna-disable** (vs. read-attribution `is_active`) — out of scope;
  not exposed by the contract.

## Open question for review

- Confirm dropping **Description** editing entirely from the reader surface is
  acceptable (no replacement affordance planned).
