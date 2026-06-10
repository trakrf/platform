# TRA-922 — MQTT topic routing: org-slug-prefixed `publish_topic` + data-driven subscriptions

**Date:** 2026-06-09
**Ticket:** [TRA-922](https://linear.app/trakrf/issue/TRA-922) (High, In Progress)
**Branch:** `feat/tra-922-mqtt-topic-org-slug-prefix`
**Related:** TRA-900 (ingest subscriber + `resolve_scan_topic`), TRA-956 (match by `(device, antenna_port)`, dropped `external_key`), TRA-857 (per-topic ACLs, out of scope), TRA-907 (multi-replica `$share`, deferred)

## Problem

Read ingestion routes each MQTT message via `resolve_scan_topic` — a DB lookup of `scan_devices.publish_topic`. The broker-side subscription is a single static filter (`MQTT_TOPIC`, deployed as `trakrf.id/+/reads` via infra #147), a leftover from when the middle topic segment was the org slug used to route by org. That org-slug routing is gone. Two problems fall out:

1. **The coarse filter silently constrains a "free-form" field.** Any `publish_topic` not matching `trakrf.id/{one-segment}/reads` is never delivered and is silently dropped.
2. **Cross-org collision is latent.** The unique index is `(org_id, publish_topic)` — only per-org. Two orgs could both register `trakrf.id/dock-1/reads`, making topic→device routing ambiguous.

## Decisions (resolved with Mike, 2026-06-09)

- **D1 — Topic scheme: slug-as-root.** New `publish_topic` values are forced to start with `{org_slug}/` where `org_slug = organizations.identifier` (globally unique, treated as immutable — no rename path exists in code). Drop the `trakrf.id/` literal root. Example: `organized-chaos/dock-1/reads`. The globally-unique slug prefix makes the existing per-org `(org_id, publish_topic)` index effectively global for conforming rows — cross-org collision becomes structurally impossible. `{org_slug}/#` is a ready-made per-tenant ACL scope for TRA-857.
- **D2 — Org-slug mutability: non-issue.** `organizations.identifier` is `UNIQUE` and has no rename path in the codebase; treated as immutable. No cascade logic needed.
- **D3 — Existing rows: grandfather.** No data migration. Prefix is enforced only on create and on edit-that-changes-the-topic. Existing demo rows (`trakrf.id/C4DEE229A176/reads`, MK107, etc.) are left untouched and keep working.
- **D4 — Broker filtering: data-driven per-topic subscriptions (not a firehose).** Instead of subscribing to `#` and re-filtering in-app, the backend subscribes to exactly the set of registered `publish_topic`s. The broker delivers only registered reads topics — no Shelly command/status noise, no firehose, no negative cache. The static `MQTT_TOPIC` filter retires entirely (data-driven, not pattern-driven), so there is **no cloud infra hand-off** for a chart `mqtt.topic` value. This keeps `publish_topic` truly free-form (any depth/shape) and aligns 1:1 with per-topic ACLs (TRA-857).

### Approaches considered for D4

| Approach | Subscription | Broker delivers | Verdict |
|---|---|---|---|
| A. `#` firehose + in-app negative cache | one static `#` | all broker traffic | Rejected — re-implements broker filtering; needs negative cache to survive junk; dirtiest |
| B. One structural filter `+/+/reads` | one static filter | 3-segment `.../reads` only | Rejected — re-imposes fixed structure (no `{org}/bldg-a/dock-1/reads`); kills free-form |
| **C. Per-topic exact (chosen)** | one exact sub per device, managed dynamically | only registered reads topics | **Chosen** — broker filters precisely; free-form preserved; subscription set *is* the routing map; retires `MQTT_TOPIC`; ACL-aligned |

## Architecture

### New component: `internal/services/topicroute` registry

A process-wide registry that owns (a) the in-memory `publish_topic → Route` map used to route incoming messages and (b) the broker subscription set. One structure, two jobs — there is no separate cache.

```
type Route struct { OrgID int; ScanDeviceID int64; DeviceType string }

type SubscriptionManager interface {  // implemented by *ingest.Subscriber
    Subscribe(topic string)
    Unsubscribe(topic string)
}

type Registry struct {
    store   *storage.Storage
    mu      sync.RWMutex
    routes  map[string]Route          // publish_topic -> route
    mgr     SubscriptionManager       // nil until a subscriber attaches (MQTT off => nil)
}

func NewRegistry(store *storage.Storage) *Registry
func (r *Registry) SetManager(m SubscriptionManager)            // wired after subscriber built
func (r *Registry) Load(ctx context.Context) error              // bulk-populate from DB
func (r *Registry) Lookup(topic string) (Route, bool)           // message-path routing
func (r *Registry) Topics() []string                            // snapshot for OnConnect bulk-subscribe
func (r *Registry) Add(route Route, topic string)               // CRUD create/update-new: map + mgr.Subscribe
func (r *Registry) Remove(topic string)                         // CRUD delete/update-old: map + mgr.Unsubscribe
func (r *Registry) Reconcile(ctx context.Context) error         // periodic: re-Load, diff, sub/unsub deltas
```

- `Add`/`Remove` mutate the map under the lock and, if a manager is attached, call `mgr.Subscribe`/`Unsubscribe`. With MQTT off (no manager) they keep the map current and are subscription no-ops.
- Boundaries: `topicroute` imports only `storage`. `ingest` and `scandevices` import `topicroute`. The `SubscriptionManager` interface is defined in `topicroute` and implemented by `*ingest.Subscriber` — no import cycle (no `topicroute → ingest` edge).

### New DB function (migration `000021`): `trakrf.list_active_scan_topics()`

The subscriber runs as the RLS-enforced `trakrf-app` role with no org context, so it cannot list every org's devices directly. A `SECURITY DEFINER` function returns the full active set for bootstrap + reconcile, mirroring `resolve_scan_topic`:

```sql
CREATE OR REPLACE FUNCTION trakrf.list_active_scan_topics()
RETURNS TABLE (org_id bigint, scan_device_id bigint, device_type trakrf.scan_device_type, publish_topic text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = trakrf, public
AS $$
    SELECT d.org_id, d.id, d.type, d.publish_topic
    FROM trakrf.scan_devices d
    WHERE d.deleted_at IS NULL
      AND d.transport = 'mqtt'
      AND d.publish_topic IS NOT NULL;
$$;
```

`resolve_scan_topic` is **kept** as the message-path fallback (registry `Lookup` miss → `resolve_scan_topic`), a defensive backstop for any race between Subscribe and map population. `.down.sql` drops `list_active_scan_topics`.

### Subscriber changes (`internal/ingest`)

- `NewSubscriber` gains a `*topicroute.Registry` param; the subscriber registers itself via `registry.SetManager(s)` and implements `Subscribe`/`Unsubscribe` against the live `mqtt.Client` (best-effort: a Subscribe while disconnected just logs; `OnConnect` reconciles).
- `OnConnect` no longer subscribes a static `s.cfg.Topic`. It bulk-subscribes `registry.Topics()` via `client.SubscribeMultiple` (fires on initial connect and every reconnect — handles resubscribe for free).
- `handleMessage` routes via `registry.Lookup(topic)`; on miss, falls back to `store.ResolveScanTopic`. Everything downstream (Parse → PersistReads → geofence → feed) is unchanged.
- A periodic `Reconcile` (ticker, ~5 min) is started alongside the subscriber as a safety net for missed CRUD events / direct DB edits / future multi-replica drift.
- `MQTT_TOPIC` retired: remove `Topic` from the subscribe path in `Config`/`ConfigFromEnv`; keep `MQTT_URL`, `MQTT_CLIENT_ID`. Update `config_test.go`. Remove the `MQTT_TOPIC` line from `deploy/edge/.env.example`.

### CRUD changes (`internal/handlers/scandevices` + `internal/storage/scan_devices`)

- **Prefix validation (D1).** On create and on update-that-changes `publish_topic` (mqtt transport): fetch the caller's org via `storage.GetOrganizationByID(GetRequestOrgID(r)).Identifier` and require `publish_topic` to start with `{identifier}/`. Reject with **422** + a clear message otherwise. Org with NULL `identifier` → 422 (can't form a valid topic). Update with an **unchanged** `publish_topic` is left alone (grandfather, D3).
- **Registry wiring.** Handler gains a `*topicroute.Registry`. After a successful create → `registry.Add`; successful delete → `registry.Remove`; update with topic change → `registry.Remove(old)` + `registry.Add(new)`. Non-mqtt (web_ble) devices have no topic → skipped.

### Frontend (`frontend/src/components/scandevices` + `lib/scandevices`)

- **Prefill (D1).** On create, prefill the `publish_topic` input with `{org_slug}/` from the existing frontend org context; update placeholder/help text to the new scheme.
- **`readerKeyFromTopic` (`deviceProfile.ts`) + backend `broadcaster.go`.** Relax the hardcoded `trakrf.id/` first segment to a wildcard so `{seg}/{key}/reads` extracts the key for both grandfathered and new topics; fall back to the whole topic for non-conforming shapes. (Live Reads correlation only; org-scoped SSE already isolates cross-org key collisions.)

### Composition (`internal/cmd/serve/serve.go`)

- Construct `registry := topicroute.NewRegistry(store)` **unconditionally** (the handler always needs it) and call `registry.Load(ctx)` before the MQTT block.
- Inside `if mqttCfg.Enabled()`: pass `registry` to `NewSubscriber` (which calls `registry.SetManager(self)`); start the reconcile ticker; `defer` its stop.
- Pass `registry` to `scandeviceshandler.NewHandler(store, registry)`.

## Testing (TDD)

- `topicroute` unit tests: `Load` populates from a fake/seeded store; `Lookup` hit/miss; `Add`/`Remove` mutate the map **and** call a fake `SubscriptionManager`'s Subscribe/Unsubscribe; `Reconcile` computes correct sub/unsub deltas; no-manager (MQTT-off) path is a subscription no-op.
- Storage integration test for `list_active_scan_topics()` (returns active mqtt rows across orgs; excludes deleted / web_ble / null-topic).
- CRUD integration tests: create with conforming prefix → ok + registry.Add called; create with wrong/missing prefix → 422; create for NULL-identifier org → 422; update unchanged topic on a grandfathered row → ok (no prefix check); update to non-conforming topic → 422; delete → registry.Remove called.
- `ingest` test: `handleMessage` routes via registry hit without touching the DB; registry miss falls back to `resolve_scan_topic`.
- `config_test.go`: `MQTT_TOPIC` retired (no static topic in subscribe path).
- Frontend: `readerKeyFromTopic` extracts key for `{org_slug}/{key}/reads` and grandfathered `trakrf.id/{key}/reads`; `ScanDeviceForm` prefills `{org_slug}/` on create.

## Out of scope

- Per-topic ACLs / per-device broker creds → TRA-857 (this sets up the `{org_slug}/#` namespace).
- Multi-replica ingest → TRA-907 (would use `$share/grp/{topic}` shared subscriptions; the registry/reconcile model composes with it).
- Data migration / re-provisioning of existing rows (grandfathered, D3).
- GL-S10/reader command channels (separate follow-up).

## Risks / notes

- **Subscribe-before-map race:** mitigated by ordering (map update precedes Subscribe in `Add`) and the `resolve_scan_topic` fallback on `Lookup` miss.
- **Disconnected Subscribe:** best-effort; `OnConnect` bulk-subscribe from `registry.Topics()` is the source of truth on every (re)connect.
- **Reconcile cost:** one `list_active_scan_topics` query every ~5 min; negligible.
- **No DB-level prefix enforcement:** a CHECK can't join to `organizations` and a trigger would break grandfathered edits, so the prefix is enforced in the app layer only (consistent with D3).
