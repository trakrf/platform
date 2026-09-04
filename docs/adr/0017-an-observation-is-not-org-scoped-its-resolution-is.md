# ADR 0017 — An observation is not org-scoped; what it resolves to is

Date: 2026-09-04
Status: Proposed
Tracking: TRA-1191 (this change), TRA-527 (the earlier "injected tags lost" of the same family)

## Context

`invalidateAllOrgScopedData()` in `frontend/src/lib/cache/orgScopedCache.ts` drops
everything that belongs to the org being left. It is driven by a registry,
`ORG_SCOPED_STORES`, naming each store and the method to call on it:

```ts
{ name: 'tags', getStore: () => …useTagStore, clearFn: 'clearTags' },
```

`clearTags` empties the store. That is the right shape for the caches either
side of it — `assetStore` and `locationStore` hold nothing but a copy of one
org's server state — and it reads as obviously right by symmetry.

It is wrong for tags, because a tag row is two different things welded together:

- **an observation** — this EPC was in front of the antenna, with this RSSI, at
  this time. That is a fact about the physical world. It belongs to no org, and
  no org can invalidate it.
- **a resolution** — this EPC is asset 11, "Pump A". That is org-scoped, it is
  wrong the moment the org changes, and it must not survive.

`clearTags` collapsed those two lifetimes into one and threw both away.

### Why that was expensive rather than merely untidy

The invalidation runs on **login**, not only on org switch — `authStore`'s
`setOrgContext` calls it after `setCurrentOrg` returns. So logging in deleted
every tag scanned while logged out.

`tagStore` subscribes to the auth store for the express purpose of enriching
those tags:

```ts
// Re-enrich tags when user logs in (for tags scanned while anonymous)
useAuthStore.subscribe((state, prevState) => {
  if (state.isAuthenticated && !prevState.isAuthenticated) { … }
```

Both features worked exactly as written and cancelled each other out. The
subscription fired, found an empty tag list, and issued a lookup for nothing.
The scan-anonymously-then-log-in-to-enrich flow — a documented workflow with
code dedicated to it on both sides — could not have worked at any point.

Measured on this branch, tags before login / after login: **3 / 0** before the
change, **3 / 3** after.

### Why it stayed hidden

Nothing on the path said what it had done.

The e2e console forwarder is a case-sensitive substring allowlist, and every
line on the enrichment path failed it except `_flushLookupQueue: API error`,
which survives only because it is a `console.error` and that type is kept
unconditionally. **The one observable line was the failure line.** A lookup that
ran and matched nothing and a lookup that never happened both printed exactly
nothing, so the two were indistinguishable from the outside — and the report was
filed as a timeout, with "wait longer" as the obvious next move.

`invalidateAllOrgScopedData` then logged `[OrgCache] Cleared tags store`
regardless of what the registered method actually did, so even a reader watching
the right line would have been told the wrong thing once the method changed.

## Decision

**A store that holds a physical observation is registered with a method that
strips the org-scoped fields and keeps the observation.** Only stores that are
purely a cache of one org's server state may be emptied outright.

Concretely: `tagStore` is registered with `clearEnrichment`, which resets
`type` to `unknown` and clears `assetId` / `assetName` / `assetIdentifier` /
`locationId` / `locationName`, leaving `epc`, `rssi`, `count` and the timestamps
untouched. Tags land back in exactly the state `refreshAssetEnrichment()`
selects on, so the next lookup re-resolves them against the new org with no
further wiring.

Two supporting rules, because the defect was as much about observability:

1. **A log line naming a generic action must name the action it performed**, not
   the one the loop is called after. `[OrgCache] tags: clearEnrichment()`.
2. **Where a path's only forwarded line is its failure line, silence is not
   evidence.** Widen the e2e forwarder by adding a specific prefix — never by
   loosening an existing limb, which sweeps in unrelated chatter and changes
   every count computed from the captured log.

## Consequences

- Anonymous scan → log in → enrich works, and so does org switch: the resolution
  is dropped, the scan survives, and re-resolution happens against the new org.
  No EPC is ever shown carrying another org's asset name.
- A scanned EPC now outlives a login and an org switch. That is deliberate. The
  EPC is not a secret the app is holding on the org's behalf — the person
  holding the reader just pointed it at the tag. What is org-scoped, and is
  still discarded, is the mapping to an asset.
- Anyone adding a store to `ORG_SCOPED_STORES` has to decide which kind it is.
  Getting it wrong deletes user data silently, which is why this is written down
  rather than left to the next reader's symmetry instinct.
- `clearTags` remains, and is still correct where the user asked for it: the
  Clear button, kit workspace reset, and post-save auto-clear.

## Alternatives rejected

**Leave `clearTags` and re-trigger a lookup after login.** This is the shape the
original report suggested, and it cannot work: re-running a lookup for tags that
have been deleted resolves nothing. It also treats the symptom — the lookup —
rather than the deletion upstream of it.

**Persist `_lookupQueue` so the pending EPCs survive.** Preserves the EPCs in a
second place while the store they belong to is emptied, leaving two sources of
truth to disagree. The queue is a debounce buffer; it is not a home for the
user's scan.

**Take tags out of `ORG_SCOPED_STORES` entirely.** Fixes the deletion and
reintroduces the leak it was added for: a tag enriched under org A would keep
showing org A's asset name after switching to org B. The registry entry is
right; the method it named was not.
