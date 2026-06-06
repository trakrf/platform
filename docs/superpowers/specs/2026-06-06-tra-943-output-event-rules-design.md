# Output Event Rules — egress/presence modes + visible per-output config

- **Tickets:** TRA-943 (geofence boundary granularity / rule behavior — backend), TRA-935 (output device auto-off UI field — frontend)
- **Date:** 2026-06-06
- **Status:** Design approved, ready for implementation plan
- **Scope decision:** Build the **minimal demo slice (a)** now; capture the **full rule object (b)** as documented future direction (see §8).

## 1. Problem

After output devices were mapped to **location** (not scan point), the geofence fire path mixes two granularities that no longer line up (TRA-943):

- Trigger gate is **per scan point** (`geofence/engine.go` `is_boundary` suppression).
- Output firing is **per location** (`alarm/firer.go` `ListOutputDevicesForLocation`).

Net effect: a read at any boundary-flagged scan point fires all outputs at the location, and the `is_boundary` toggle has degraded from "this is a portal" into a confusing location-wide arming switch. Separately, the only output behavior today is **egress** (fire ON on a boundary crossing, latch). There is no **presence** behavior ("light on while an asset is in the doorway, off when it clears"), which is the more compelling demo and the primitive muster mode (TRA-914) will need. Finally, two rule knobs are currently opaque/hidden (`scan_points.metadata` `is_boundary` and `rssi_threshold`) and TRA-935's `auto_off_seconds` has no UI at all.

## 2. Scope

We are building **(a) the minimal demo slice**:

- Add **egress** vs **presence** rule modes, configurable per output.
- Add a **configurable age-out** per output (replaces reliance on the global `GEOFENCE_LATCH_TTL` alone).
- Make **all rule config visible** on the output device form: mode, age-out, auto-off (TRA-935), RSSI threshold.
- **Remove** the misaligned `is_boundary` gate and column, and the hidden per-scan-point `rssi_threshold` knob.

We are **not** building the input→output decoupling (explicit trigger-set ↔ action-set). That is the rules-engine proper, deferred to (b) and documented in §8 so the metadata keys chosen here map forward cleanly.

This is greenfield: the fixed-reader/geofence stack was built within the last few days and has **not** been released, so there is no production data to migrate — destructive schema removals are safe.

## 3. Data model

All rule config lives on `output_device.metadata` (jsonb — already round-trips through PATCH/POST, no new columns, no output_devices migration):

| key | type | meaning | default / empty |
| --- | --- | --- | --- |
| `mode` | `"egress"` \| `"presence"` | rule behavior (see §4) | `"egress"` (today's behavior) |
| `age_out_seconds` | int ≥ 0 | egress: re-arm window; presence: departure window | empty/0 → fall back to global `GEOFENCE_LATCH_TTL` (60s) |
| `auto_off_seconds` | int ≥ 0 | TRA-935: device-side Shelly `toggle_after` momentary off | empty/0 → latch until manual reset; **ignored in presence mode** |
| `rssi_threshold` | int | minimum read RSSI for this output to react | empty → fall back to global `RSSIThreshold` |

**Removed from `scan_points`:**

- `is_boundary` — column dropped (migration), removed from the `PersistReads` query, the `ResolvedRead` struct, the engine gate, and the scan-point edit form.
- `rssi_threshold` (metadata key) — no longer read; the visible output-level field replaces it. Drop from the `PersistReads` query and `ResolvedRead`.

Result: scan points hold only physical wiring (device, location, external_key, antenna_port); **all rule behavior is visible on the output.**

## 4. Backend behavior

### 4.1 Engine restructure (resolve outputs up front)

Because mode / age-out / rssi_threshold now live on the output, the engine must resolve `location → outputs` (with their metadata) at evaluate time. This moves the `ListOutputDevicesForLocation` lookup **from the firer into the engine**; the firer/dispatcher becomes a thin ON/OFF executor (`Set(ctx, dev, on, autoOff)`). Dedup and presence tracking are then keyed per **(output, epc)** instead of (scan_point, epc) — which is what makes per-output age-out and per-output RSSI actually work.

Per read in `Engine.Evaluate`:

1. Resolve member/asset (unchanged — non-members already dropped upstream in `PersistReads`).
2. Resolve `location → active outputs`.
3. For each output:
   a. **RSSI gate** using that output's `rssi_threshold` (fallback global). 0 RSSI sentinel = "no usable RSSI" → still skipped as today.
   b. Apply the output's **mode** logic (§4.2 / §4.3), keyed (output, epc).

The `is_boundary` gate is **deleted**. A member read at any scan point of a location now reaches that location's outputs. (Tradeoff: on a multi-reader location, egress now fires from interior readers too, not just the portal — see §8; for the single-portal demo location, behavior is identical.)

### 4.2 Egress mode (existing semantics, re-keyed)

- Qualifying read → fire **ON**.
- Per-(output, epc) **re-arm dedup**: suppress repeat fires until the tag has been absent longer than the output's `age_out_seconds` (fallback global TTL). This is the existing latch logic, re-keyed from (scan_point, epc) to (output, epc).
- Momentary off via TRA-934 device-side `auto_off_seconds` (Shelly `toggle_after`); manual reset path unchanged.
- Semantics: "something crossed / left — alert." Momentary.

### 4.3 Presence mode (new)

- Per-(output, epc) **last-seen tracker**.
- First member present for an output (set empty → non-empty) → fire **ON**.
- Subsequent member reads refresh last-seen; no re-fire while already ON.
- The existing **sweep goroutine** (`geofence/latch.go`) ages out entries; when the last member for an output ages out past `age_out_seconds` (set non-empty → empty), fire **OFF** (`dispatcher.Set(ctx, dev, false, 0)` — the bool path already exists, we just start calling it).
- `auto_off_seconds` is **ignored** in presence mode (the engine owns the OFF edge; a device timer would fight it).
- Semantics: "something is here / must stay." Level, not edge. Age-out *is* the presence primitive — RFID emits no "tag left" event, so "off when it leaves" is *defined* by the age-out.

### 4.4 Shared

- RSSI gate applies in **both** modes (only strong reads count as crossing/present).
- The sweep cadence (`GEOFENCE_SWEEP_INTERVAL`) and global TTL/RSSI env defaults stay as fallbacks.

## 5. Frontend

### 5.1 Output device form (`OutputDeviceForm.tsx`)

Add four `metadata`-backed fields (the form does not touch `metadata` today — add extraction in edit-init and assembly in submit):

- **Mode** — select: Egress / Presence, with help text describing each.
- **Auto-off (seconds)** — numeric ≥ 0 (TRA-935); placeholder/help: "0 or empty = stay on until manual reset." **Disabled** with explanatory help when mode = Presence ("auto-off is managed by presence detection").
- **Age-out (seconds)** — numeric ≥ 0; help text adapts to mode ("re-arm window before the same tag can re-fire" for egress / "how long after the last read before the output clears" for presence); empty = system default.
- **RSSI threshold** — numeric; help: "minimum signal strength for this output to react; empty = system default."

Validation: each numeric field, if non-empty, must parse as a (non-negative, for seconds) integer; show inline field errors matching the existing pattern (`ScanPointForm` antenna_port style).

### 5.2 Scan point form (`ScanPointForm.tsx`)

- **Remove** the `is_boundary` toggle (column is being dropped).

### 5.3 Types / API

- `OutputDevice`, `CreateOutputDeviceRequest`, `UpdateOutputDeviceRequest` already carry `metadata?: Record<string, unknown>`; optionally tighten with a documented sub-shape. API client + mutation hooks need no change (metadata round-trips).

## 6. Migrations

- New migration: `DROP COLUMN is_boundary` from `trakrf.scan_points` (with `.down.sql` per 011+ convention re-adding it `DEFAULT false`).
- No output_devices migration (jsonb).
- No data backfill needed (unreleased, no prod data).

## 7. Testing

- **Backend (Go):** engine unit tests for egress re-arm keyed per-output; presence ON-on-first / OFF-on-last-age-out via the sweep; per-output RSSI gate (override + fallback); `is_boundary` removal doesn't break the read path. Storage test that `PersistReads` no longer selects the dropped columns.
- **Frontend (Vitest):** `OutputDeviceForm` renders the four fields, auto-off disabled in presence mode, validation rejects negatives/non-integers, metadata round-trips on submit; `ScanPointForm` no longer renders the is_boundary toggle. Update `OutputDeviceFormModal.test.tsx` (inline + modal variants).
- Live validation on preview deferred until the TRA-934 Shelly image is present (per TRA-935 acceptance criteria).

## 8. Captured for later — (b) full rule object

The expensive, deferred half of TRA-943 is the **wiring decoupling**: an explicit rule object

```
geofence_rule { trigger: [scan_points], mode, age_out_seconds, rssi_threshold, action: [output_devices] }
```

many-to-many on both ends, replacing the implicit `output_device.location_id` routing and restoring per-portal granularity ("fire output B for door 2 only"). That entails a new table + migration + RLS, net-new internal CRUD + storage + OpenAPI, an engine refactor from location-routing to rule-matching, a net-new many-to-many rule-builder UI, and migrating existing location bindings into rule rows — i.e. the rules-engine proper (cf. TRA-901 follow-on, TRA-914 muster). The metadata keys chosen in §3 (`mode`, `age_out_seconds`, `rssi_threshold`) map directly to rule columns, so (a) does not paint (b) into a corner.

**Known tradeoff accepted for (a):** removing the `is_boundary` gate means egress fires from any scan point at a location; per-portal selectivity returns only with (b).

## 9. Handoff

One spec → one implementation plan → one branch → one PR covering both tickets (shared form + metadata model). Commits stay separable: (1) drop `is_boundary` + scan-point form/query/struct cleanup; (2) output metadata fields + engine restructure + presence/egress modes; (3) frontend output-form fields incl. TRA-935 auto-off. Output devices are internal-only → no docs PR.
