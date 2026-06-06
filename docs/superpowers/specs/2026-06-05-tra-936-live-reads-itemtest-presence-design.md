# TRA-936 — Live Reads: ItemTest-style tag presence (server-authoritative deltas + RSSI/count aggregation)

**Status:** Design · **Ticket:** [TRA-936](https://linear.app/trakrf/issue/TRA-936) · **Parent epic:** TRA-897 (fixed-reader) · **Date:** 2026-06-05

## 1. Summary

Rework the **Live Reads** display to behave like KEYPR and Impinj **ItemTest** — the two tools Tim already uses. Today the feed is a raw-read firehose rendered as a flat, per-EPC, 15s client-side dedup table. Target: a **tag-presence inventory** with per-tag read counts, RSSI aggregates (last/avg/min/max), first/last-seen timestamps, a smooth staleness gradient, and ENTER/UPDATE/LEAVE semantics — sourced from a **server-authoritative presence set**.

References: KEYPR handoff (`/home/mike/keypr/keypr-tag-presence-and-sse-handoff.md`), live KEYPR (`http://192.168.50.231/`), ItemTest 2.14 User Guide §7.1 (Inventory Showcase, Fig 44, pp.46–47).

## 2. Current state (what we replace)

**Backend**
- `backend/internal/services/readstream/broadcaster.go` — in-process per-org pub/sub; fans out **every parsed raw read** (`ReadEvent{EPC, ReaderKey, CapturePointName, AntennaPort, RSSI, ReaderTimestampMs}`). No presence, no aggregation, no expiry. 256-event per-subscriber buffer, drop-on-full.
- `backend/internal/handlers/readstream/readstream.go` — `GET /api/v1/reads/stream`, JWT+org, SSE; writes bare `data: {json}\n\n` frames + `: ping` heartbeat; clears `WriteTimeout`.
- `backend/internal/ingest/subscriber.go` — step **3b** taps parsed reads **pre-membership** (`s.feed.Publish(orgID, topic, reads)`) so unknown EPCs surface. **Keep this tap point.**

**Frontend** (`frontend/src/.../readerfeed/`)
- `lib/readerfeed/stream.ts` — SSE-over-fetch (bearer; native `EventSource` can't set headers), reconnect w/ backoff, parses bare `data:` frames into `ParsedRead`.
- `lib/readerfeed/store.ts` — `Map<epc, LiveRead>`, last-write-wins, `READ_TTL_SECONDS=15` client expiry, `ageBandClass()` green/white/gray bands.
- `hooks/readerfeed/useReaderFeed.ts` — wires stream → store; 1s expiry tick; optional client-side `filterReaderKey`.
- `components/readerfeed/LiveReadsFeed.tsx` — shared by global `LiveReadsScreen` and the scoped `ScanDeviceFormModal` panel. Columns: EPC, Reader, Capture Point, Antenna, RSSI, Age.
- `types/readerfeed/index.ts` — `ParsedRead`, `LiveRead`, `ReaderFeedStatus`.

This is precisely the "push raw reads + client TTL" model the KEYPR handoff §5 rejects: bandwidth ∝ read-rate, LEAVE can't be derived from absence-of-reads, counts can't be derived at all.

## 3. Decision: backend-authoritative presence + aggregation

**The new requirement is decisive: read count and avg/min/max RSSI are aggregates that require seeing *every* read.** You cannot reconstruct a count or an average from coalesced/throttled updates. So either the **server** aggregates and pushes compact per-tag *state*, or **every raw read** is shipped to **every client** to aggregate in the browser. The latter is the current firehose.

| Axis | Server aggregation (chosen) | Client aggregation (firehose) |
|---|---|---|
| Bandwidth | ∝ tag population × coalesce rate (~50 msg/s) | ∝ raw read rate (500–2000 msg/s for a 50-tag portal) |
| Browser cost | thin reducer; survives ItemTest-scale | parse+reduce 1000s/s/tab; degrades |
| Count/avg correctness | exact (server sees every read) | undercounts on any dropped read |
| Drop recovery | idempotent state + keyframe self-heals | silent data loss |
| Single-replica | unchanged (broadcaster already in-process) | unchanged |
| Alarm coupling | none — alarms already server-side (TRA-901/903/929) | none |

Today's firehose only "works" because fixed-reader/MQTT ingestion is demo-track (low volume); it will not survive a real portal — exactly when Tim evaluates it. **Single-replica is not a regression** — the broadcaster is already in-process per-org (TRA-907 owns multi-replica). This matches the KEYPR handoff's recommended target.

## 4. ItemTest column parity

ItemTest Inventory **List View** → our feed (all derivable since the backend sees every read):

| ItemTest column | Source |
|---|---|
| EPC (white when <1s, darkens with age) | presence key + client-computed gradient |
| **Read Count** | server counter |
| **Last RSSI / RSSI Avg / RSSI Max / RSSI Min** | server aggregates |
| Last Read From (reader) | last `readerKey` |
| Antenna | last `antennaPort` |
| First Seen / Last Seen / Time Since Last Seen | server timestamps; "time since" client-computed |
| Footer: Unique Tags + Read Rate (reads/s) | server-side |

**Omitted — not derivable from CS463/GL-S10 ingest:** TID / Tag Model (needs Impinj FastID), Power, Phase Angle, Doppler, PC/XPC words, CR Handle. Documented so the gap is intentional, not an oversight.

## 5. Backend design

New package `backend/internal/services/presence/` (name TBD during build), inserted between the ingest tap and SSE fan-out. The broadcaster keeps doing SSE transport; presence owns state + event generation.

### 5.1 State

Per org, keyed `(readerKey, epc)`:

```go
type TagState struct {
    ReaderKey        string
    EPC              string
    Alias            string    // optional; empty until alias follow-up
    CapturePointName string
    AntennaPort      int       // most recent
    FirstSeen        time.Time // server ingest time, set once
    LastSeen         time.Time // server ingest time, refreshed each sight
    ReadCount        int64
    LastRSSI         int
    rssiSum          int64     // unexported; for avg
    RSSIMin          int
    RSSIMax          int
}
```

`RSSIAvg` is derived (`rssiSum / ReadCount`) at emit time. Use **server-side timestamps** for first/last-seen and expiry; the reader clock is informational only (handoff §1.1).

### 5.2 Transitions (handoff §1.2)

- **First sight** — insert, `FirstSeen=LastSeen=now`, init aggregates. Emit an **upsert** promptly (don't wait for the coalesce tick).
- **Re-sight** — `ReadCount++`, update `LastRSSI/rssiSum/RSSIMin/RSSIMax/AntennaPort`, `LastSeen=now`; mark dirty. Emit a **coalesced ≤1/s** upsert per tag (flush dirty set on a ticker). Render rate ≠ network rate (handoff §5).
- **Expiry** — remove key, emit one **leave** **per evicted tag** (handoff §3.3).

ENTER and UPDATE are collapsed into a single **`upsert`** event: both carry the full `TagState` and the client reducer handles them identically (`map.set`), so a separate event type is redundant — the client derives "new vs seen" from map membership if it ever needs an enter animation. They differ only in timing (first sight immediate, re-sight coalesced), which is a backend concern, not a wire distinction.

### 5.3 Expiration

- **30s sliding window**: stale when `now - LastSeen >= 30s`. No fixed/max-dwell toggle (YAGNI; `expireFirstSeen` is a later knob).
- Sweep at a **fraction of the threshold** (e.g. 1s ticker), never `== threshold` (handoff §3.2 — that bug doubles LEAVE latency). Per-tag deadline timers are an acceptable alternative; a 1s sweep is simplest and adequate at our population.

### 5.3a Per-session lazy tracking (count semantics)

Presence is tracked **per browser session, not per org**: each subscriber owns
its own store, created empty at connect and discarded on disconnect. `Publish`
folds reads into every *watching* session's store independently, so a read count
means "reads since **you** started watching" and two operators tuning the same
readers never share counts (Mike's and Tim's sessions are independent). Reads for
an org with no watchers are dropped (lazy — nothing tracked). Clear (TRA-937) ≈
reconnect, which starts a fresh empty store. This is a setup/tuning activity, not
steady state, so the per-session memory is immaterial.

### 5.4 Concurrency (handoff §3.1)

State is touched by ingest publishes, the coalesce ticker, the sweep ticker, and new-subscriber snapshots. Own it from a **single goroutine via channels** or guard with a **mutex**. No unsynchronized shared map.

### 5.5 SSE contract (named events)

Replaces the current bare `data:` raw-read frames:

```
event: snapshot  data: { "tags": [ TagStateWire, ... ], "uniqueTags": N, "readRate": R }   // on connect + periodic keyframe
event: upsert    data: TagStateWire                                                        // first sight (immediate) or re-sight (coalesced ≤1/s)
event: leave     data: { readerKey, epc }
```

`TagStateWire = { readerKey, epc, alias, antennaPort, firstSeen, lastSeen, readCount, lastRssi, rssiAvg, rssiMin, rssiMax }` (capturePointName dropped — see §4 note). Timestamps as epoch-millis (consistent with current `ReaderTimestampMs`). Keep the `: ping` heartbeat.

- **Snapshot on connect** seeds client state (per-org filtered).
- **Periodic keyframe** every 30–60s so any missed delta self-heals (I-frame/P-frame, handoff §5).
- Optional later: SSE `id:` + `Last-Event-ID` resume. Out of scope for v1.

### 5.6 Footer metrics

This is the part Tim actually watches (ItemTest Fig 40/41, p.44 — the Inventory Showcase): a header **session timer** plus a footer stat strip.

- **Unique Tags** = current presence-set size (org-filtered).
- **Read Rate (reads/s)** = reads counted over a trailing window ÷ window seconds; emit on the snapshot/keyframe (or a dedicated low-rate `stats` event). Keep simple: piggyback on snapshot.
- **Session timer** = elapsed wall-time since the stream connected; computed **client-side** from connect time (no backend involvement). Mirrors ItemTest's top-center run stopwatch.

## 6. Frontend design

Transport and pure modules stay; semantics change.

- **`types/readerfeed/index.ts`** — replace `ParsedRead`/`LiveRead` with `TagState` (mirror wire) + delta payload types + event-union type.
- **`lib/readerfeed/stream.ts`** — parse the SSE **event type** (currently only reads `data:`), emit typed events to callbacks (`onSnapshot/onEnter/onUpdate/onLeave`). Reconnect/backoff/401-refresh unchanged.
- **`lib/readerfeed/store.ts`** — reducer over `Map<"readerKey\x00epc", TagState>`:
  - snapshot → replace/reconcile; upsert → set; leave → delete.
  - Client TTL = **backstop only** (`now - lastSeen > 30s + grace`) for a LEAVE missed during a reconnect blip (handoff §5). Not primary.
  - `gradient(ageSeconds)` — smooth, dark-theme-adapted. KEYPR formula is `max(255 - age*4, 192)` grayscale (white→#c0c0c0 by ~16s on a light table). Our table is dark, so invert: fresh = bright/accent, fading to the base row bg by ~16s. Pure function of `now - lastSeen`; recomputed locally each tick — **zero network to animate** (handoff §5).
- **`hooks/readerfeed/useReaderFeed.ts`** — dispatch typed events into the reducer; keep the 1s render tick (drives gradient + "time since" recompute) and the optional `filterReaderKey` (now filters by `readerKey` on `TagState`). Remove the client-as-primary 15s expiry (server owns LEAVE; keep only the backstop).
- **`components/readerfeed/LiveReadsFeed.tsx`** — render the ItemTest **Inventory** column set in this order (matching Fig 40): `# · Tag (alias || epc) · Read Count · Reader (Last Read From) · First Seen · Last Seen · Time Since Last Seen · Last RSSI · RSSI Avg · RSSI Max · RSSI Min · Antenna`; row bg = `gradient(age)`. **Time Since Last Seen** ticks live (ItemTest renders these as relative seconds, e.g. `0.071`, not wall-clock — recomputed client-side from `lastSeen` each tick). Header shows the **session timer**; footer shows **Unique Tags** + **Read Rate (reads/s)**. Keep the scoped/global reuse intact — scoped (single reader) naturally renders one row per tag (= pure ItemTest); global shows (reader,epc) rows.

## 7. Decisions / defaults (overridable at review)

- **Key = (reader, epc)** — coverage overlap visible on the global page; collapses to one-row-per-tag in the scoped panel.
- **30s sliding window**, no toggle.
- **Alias deferred** — optional `alias` in the contract now; asset-name resolution is a follow-up (keep it out of the ingest hot path).
- **Dark-theme gradient**, not KEYPR's literal white→gray.

## 8. Non-goals

Multi-replica fan-out (TRA-907); alarm/geofence changes; FastID/TID/Power/Doppler columns; `Last-Event-ID` resume; alias resolution.

## 9. Test plan

- **Backend:** table-driven unit tests for the presence reducer — ENTER on first sight; UPDATE aggregates (count, avg/min/max, last) over a read sequence; coalescing (N sights in <1s → ≤1 UPDATE); sliding expiry → exactly one LEAVE per evicted tag; sweep period ≪ TTL; per-org isolation; snapshot reflects current set. Concurrency: race detector over concurrent publish+sweep+snapshot.
- **Frontend:** pure-reducer tests for snapshot/upsert/leave; gradient monotonicity over age; backstop TTL only fires when no LEAVE arrives; stream parser dispatches named events and ignores heartbeats/malformed frames.
- **Manual:** preview against live MQTT (or replay), compared side-by-side with live KEYPR for parity feel.

## 10. Risks

- **Coalescing vs freshness** — too-aggressive UPDATE coalescing makes "time since last seen" lag; cap at 1s and recompute age client-side from `lastSeen` so the gradient stays smooth regardless.
- **Snapshot size** — a large presence set makes the connect snapshot big; bounded by population, acceptable; keyframe interval tunable.
- **Contract break** — the SSE payload changes shape (raw reads → named delta events). Backend + frontend ship together in one PR; no external consumers (internal-only surface).
