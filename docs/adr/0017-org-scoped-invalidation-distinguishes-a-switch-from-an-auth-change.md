# ADR 0017 — Org-scoped invalidation distinguishes an org switch from an auth change

Date: 2026-09-04
Status: Proposed
Tracking: TRA-1191 (this change), TRA-318 (the cross-org guarantee it must not break), TRA-527 (the earlier "injected tags lost" of the same family)

## Context

`invalidateAllOrgScopedData()` in `frontend/src/lib/cache/orgScopedCache.ts` drops
everything belonging to the org being left. A registry, `ORG_SCOPED_STORES`,
names each store and the method to call on it:

```ts
{ name: 'tags', getStore: () => …useTagStore, clearFn: 'clearTags' },
```

`clearTags` empties the store. That is right for the caches either side of it —
`assetStore` and `locationStore` hold nothing but a copy of one org's server
state — and it reads as obviously right by symmetry.

It is wrong for tags, because a tag row welds together two things with different
lifetimes:

- **an observation** — this EPC was in front of the antenna, at this RSSI, at
  this time. A fact about the physical world. It belongs to no org.
- **a resolution** — this EPC is asset 11, "Pump A". Org-scoped, wrong the
  moment the org changes, and it must not survive.

### The failure

The invalidation runs on **login**, not only on org switch: `authStore`'s
`setOrgContext` calls it after `setCurrentOrg` returns. So logging in deleted
every tag scanned while logged out.

`tagStore` subscribes to the auth store for the express purpose of enriching
exactly those tags:

```ts
// Re-enrich tags when user logs in (for tags scanned while anonymous)
useAuthStore.subscribe((state, prevState) => {
  if (state.isAuthenticated && !prevState.isAuthenticated) { … }
```

Both features worked as written and cancelled each other out. The subscription
fired, found an empty list, and looked up nothing. The
scan-anonymously-then-log-in-to-enrich flow — a documented workflow with code
dedicated to it on both sides — could not have worked at any point.

Measured, tags before login / after login: **3 / 0** before, **3 / 3** after.

### Why the obvious fix was wrong

The first attempt made the lenient behaviour unconditional. That turned
`inventory-save.spec.ts` test 3 red, and rightly:

```
test('3. switching orgs clears tag store entirely')
// Per TRA-318, central invalidation clears all org-scoped data on org switch.
// We never carry tag context across orgs — verify the store goes to zero.
```

TRA-318 is a deliberate decision with a test asserting it. A fix for TRA-1191
that silently overturns it is not a fix, and "the EPC is not really org data" is
an argument for re-opening TRA-318 on its own merits, not for quietly changing
what it guaranteed.

The two events are simply not the same:

| | previous org? | what must not survive |
|---|---|---|
| org switch (A → B) | yes | everything org A gave you |
| login / logout | no | nothing — there is no prior org |

Clearing on login protected against a threat that does not exist there, and the
cost was the user's scan.

### Why it stayed hidden

The e2e console forwarder is a case-sensitive substring allowlist, and every
line on the enrichment path failed it **except** `_flushLookupQueue: API error`,
which survives only because it is a `console.error` and that type is kept
unconditionally. The one observable line was the failure line. A lookup that ran
and matched nothing, and a lookup that never happened, both printed exactly
nothing — so the report was filed as a timeout, with "wait longer" as the
obvious next move.

`invalidateAllOrgScopedData` then logged `[OrgCache] Cleared tags store`
regardless of what the registered method actually did.

## Decision

**`invalidateAllOrgScopedData` takes a reason, and the registry may name a
different method per reason.**

```ts
invalidateAllOrgScopedData(queryClient, reason: 'org-switch' | 'auth-change')
```

- `'org-switch'` — everything goes. TRA-318 is preserved exactly, including its
  test.
- `'auth-change'` — tags keep the observation and lose the resolution, via
  `clearEnrichment()`. Tags land back on `unknown`, which is what
  `refreshAssetEnrichment()` selects on, so the next lookup re-resolves them
  unaided. This covers **login and logout alike**.

**Clearing on an org switch is a simplicity choice, not a safety requirement.**
The machinery to keep the bare scan and re-enrich it against the new org now
exists — it is the same `clearEnrichment()` the auth path uses, and it would
leak nothing, since the resolution is what gets dropped. Switching orgs is
simply a coarser context change than logging in, the scan rarely means anything
in the new org, and an empty list is easier to reason about than one that
re-populates with different names. Recorded so that a future reader does not
mistake the strict branch for a constraint and try to justify one that isn't
there. Revisiting it is a product decision, not a bug fix.

**The parameter defaults to `'org-switch'`.** A call site that has not thought
about this can only over-clear; it can never leak one org's data into another.
Only `authStore`'s `setOrgContext` opts in, and that is a local closure
reachable solely from Login and Signup — the header org selector goes
`OrgSwitcher → useOrgSwitch → orgStore.switchOrg`, which passes no reason.

Two supporting rules, because this was as much an observability failure:

1. **A log line naming a generic action must name the action performed**, not
   the one the loop is named after: `[OrgCache] tags: clearEnrichment()`, and
   the reason on the opening line.
2. **Where a path's only forwarded line is its failure line, silence is not
   evidence.** Widen the e2e forwarder by adding a specific prefix — never by
   loosening an existing limb, which sweeps in unrelated chatter and shifts
   every count computed from the captured log.

That first rule earned its place immediately. Logout does two things to the tag
store: the auth subscription fires **synchronously** and strips enrichment,
while the invalidation runs **later**, from a floating promise behind two
dynamic imports. A test that polled for "the asset data is gone" was satisfied
by the first and returned before the second had landed — so it passed against
the unfixed code, where the store was about to be emptied a moment later. The
test now waits for the invalidation's own line and asserts the method named in
it. Without a log line that names what ran, there was nothing to synchronise on.

## Consequences

- Anonymous scan → log in → enrich works. Hardware-verified over 83 real EPCs:
  `inventory-save.spec.ts` test 2 enriches in 2.2 s where it previously consumed
  the whole 90 s budget.
- Org switch is unchanged, and test 3 still measures 2 tags before and 0 after.
- **Logout now keeps the bare scan too.** `tagStore`'s logout subscription was
  already written to strip enrichment and keep the scan, and had never once had
  an effect: the strict clear ran on the same transition and overrode it. That
  handler is now real. A logged-out user holds bare EPCs and no asset data
  whatsoever — the same state an anonymous scan would have left, which is why
  the alternative was the surprising one: a user who never logged in kept their
  scan while one who logged out lost it.
- Anyone adding a store to `ORG_SCOPED_STORES` now has a second question to
  answer: does this store care why? Most will not, and `authChangeFn` is
  optional so they need do nothing.

## Alternatives rejected

**Make `clearEnrichment` unconditional.** Overturns TRA-318 without saying so.
This was the first attempt and its test failure is what produced this ADR.

**Leave `clearTags` and re-trigger a lookup after login.** The shape the original
report suggested. It cannot work: re-running a lookup for tags that have been
deleted resolves nothing. It treats the lookup rather than the deletion upstream
of it.

**Persist `_lookupQueue` so the pending EPCs survive.** Keeps the EPCs in a
second place while the store they belong to is emptied, leaving two sources of
truth to disagree. The queue is a debounce buffer, not a home for the scan.

**Take tags out of `ORG_SCOPED_STORES`.** Fixes the deletion and reintroduces the
leak the entry exists to prevent. The entry is right; the method it named was
unconditional when it should not have been.
