# ADR 0002 — Use-case surfaces are gated by per-org capability grants; a customer one-off is a capability with one grant

Date: 2026-07-16
Status: Accepted
Tracking: TRA-1024 (schema + SQL function), TRA-1025 (enforcement + error type + payload), TRA-1026 (frontend gating), TRA-1027 (superadmin grant management)

## Context

TrakRF serves multiple use-case surfaces — asset management, inventory,
geofence, mustering, and future customer-driven workflows — to different
organizations from one platform. The platform is **one core plus gated
surfaces**, not a set of peer modules: the core is identity + observations
(assets, locations, tags, the scan-event ledger), and use cases are workflows,
rule engines, and projections over it.

* **Asset management** — identity, tag binding, current-location view. The
  always-on base; never gated.
* **Inventory** — expected-vs-observed reconciliation and count sessions.
  A separate capability, not a view mode of asset, because it is a different
  *unit of identity* (pets vs. cattle): assets track per-serial individuals
  (a rack of 1U servers, each with its own serial, RAM, service history);
  inventory tracks quantity-of-class-at-location (1,000 widgets on a shelf,
  where the class has metadata and the individual does not). Reconciliation
  — expected 1,000, counted 987 — is only meaningful in the cattle world.
  The same physical tag read carries different semantics under each
  capability (increment a class count vs. update one asset's position), so
  the count-session is its own workflow primitive, never a reskinned asset
  scan handler with a flag. The two capabilities coexist in one org.
* **Geofence** — zones, enter/exit/dwell rules, alarm engine. A rule engine
  plus a projection. The engine is also the substrate mustering runs on —
  core-as-infrastructure even where the UI surface is gated.
* **Mustering** — roster, muster mode, presence rollup. A use-case surface on
  the geofence engine, with a distinct buyer persona and data sensitivity.

Two questions drove this record: how is per-org feature entitlement modeled,
stored, enforced, and rendered — and how do customer-specific one-off use
cases fit in without a separate service or endpoint per customer.

The subscription-entitlement work (TRA-947, PR #479) established the
structural template this ADR generalizes: org-row SQL function
(`org_is_entitled`) → middleware gate (`SubscriptionRequired`) → distinct
top-level error type (`payment_required`) → field on the `/users/me` org
payload → frontend hook (`useEntitlement`). Capabilities are the same five
points with the boolean generalized to a set.

Repo facts this design was verified against (2026-07-16): neither auth path
loads the org row (session auth carries `org_id` in the JWT; API-key auth
queries `api_keys` by JTI); chi v5 with auth-mode-shaped route groups and
flat literal path registration (no `r.Route()` sub-mounts, per the chi 405
behavior noted in TRA-604); error-envelope contract is branch-on-`type`/
`title`-only; frontend org context is Zustand
(`useAuthStore.profile` → `useOrgStore.currentOrg`), refreshed by
`switchOrg`'s profile refetch.

## Decision

### A capability is the unit of entitlement

A **capability** is a named grant that unlocks a surface (routes + UI) for an
org. **A customer one-off is not an architectural category** — it is a
capability granted to one org; a standard use case is a capability granted to
many. Nothing about code structure, routing, deployment, or enforcement
differs between them; the difference lives entirely in grant data. This is
the property that eliminates per-customer services and endpoints.

The term is **capability**, everywhere — column, API field, code, nav.
"Entitlement" already means subscription status in live code (`is_entitled`,
`org_is_entitled`, `useEntitlement`); no word gets two meanings.

### Vocabulary is code-owned; names describe workflows, never customers

A single Go registry defines which capability names exist. Initial
vocabulary: `inventory`, `geofence`, `mustering`. The `asset` base is core,
never gated, never a grant. A test asserts registry equals the seeded lookup
table so code and DDL cannot drift.

Naming rule: `wip_tracking`, not `acmecorp`. Promotion from one-off to
standard is then a grant-data change, not a rename through flat surfaces
(DB values, JSON fields, logs, error envelopes); customer names and
workflows must not leak through package or capability names — nor through
architecture documents in this repo, since the repo is source-available;
and workflow naming forces the generic shape of a request to be identified
at build time, the main defense against unpromotable customer-specific code.

### Storage: join table plus name-only lookup

```sql
CREATE TABLE capabilities (
    name text PRIMARY KEY
);

CREATE TABLE org_capabilities (
    org_id      bigint      NOT NULL REFERENCES organizations(id),
    capability  text        NOT NULL REFERENCES capabilities(name),
    granted_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, capability)
);
```

Grant = insert; revoke = delete; "which orgs have X" is a WHERE clause.
Per-grant metadata (`expires_at`, `config jsonb`) has an obvious home but is
added only at first real need. The lookup table carries the name and nothing
else — no policy column (see next section).

Loading: the capability set costs one indexed lookup per request via a
set-returning SQL function (`org_capability_set(org_id) RETURNS text[]`),
sibling to `org_is_entitled`, stashed on the request context and shared with
the subscription gate on mutations. Baking capabilities into the session JWT
was rejected: grants must take effect without waiting out token lifetime.
An in-process cache keyed by `org_id` is the escape hatch if profiling ever
demands it — not built until then.

### Backend enforcement: uniform 403 `capability_required`

All routes mount unconditionally at startup; middleware decides per-request.
Authorization layers, distinct and ordered:

1. **Authentication** — session or API key; resolves the org (401).
2. **Org capability** — `RequireCap(cap)`, attached **per-route via
   `r.With`**, adjacent to `RequireScope` where both apply, so a route
   registration reads as its complete authorization story. This is the
   entitlement check and it is mandatory; frontend gating is UX, not
   enforcement.
3. **Subscription entitlement** (TRA-947, mutations only) — 402. Capability
   precedes subscription: an org cannot be past-due on a surface it never
   bought; a lapsed org sees `capability_required` for ungranted surfaces
   and `payment_required` for granted ones.
4. **Principal permission** — user role or API-key scope (403 `forbidden`).

Denial is **uniform**: every ungated access, on every route, for every
principal type, returns 403 with new top-level `ErrorType` enum value
`capability_required` — additive under `x-extensible-enum: true`, following
the `payment_required` precedent, distinguishable by contract from role/scope
`forbidden` and from 402.

There is deliberately **no hide-404 at the backend.** An earlier draft
assigned mustering a 404-on-ungated-access policy; that was reversed on the
observation that the platform is source-available under BSL — route existence
is confirmable from the repository by anyone, so a 404 conceals nothing a 403
reveals, and "your own org has not licensed this" is not sensitive to the
org's own authenticated principals. One denial contract for integrators and
test cycles, and a `RequireCap` with no policy branch. The pre-existing 401
that API-key principals receive on session-only routes is moot for the same
reason.

Gated capabilities' routes begin life internal-only (absent from the
published OpenAPI spec); adding them to the public spec is a deliberate,
per-capability act via the existing paired-PR spec workflow. Spec absence is
documentation curation, not concealment.

**Data-level gating, standing rule:** per-route middleware cannot protect
shared endpoints that aggregate across capabilities (events, notifications,
reports, search). Any endpoint returning rows *produced by* a gated
capability must filter those rows on the same capability set. Required
question at endpoint design time: does this endpoint return data a gated
capability produced?

### Frontend: presentation policy lives here, not in the backend

How an ungated capability *presents* — `absent` (no trace) vs. `locked`
(visible upsell affordance) — is declared in a frontend nav/route registry
(`{ capability, label, route, icon, presentation }`) and in the OpenAPI spec
partition. Both are build-time artifacts, which is why a DB policy column was
rejected: it would advertise runtime flexibility that shipped bundles and
published specs cannot honor. Initial assignment: `mustering` = `absent`.
`inventory` and `geofence` are a pending commercial decision with zero
backend implication.

**Amended.** The pending decision resolved on 2026-07-16 (Mike + Tim) to
`geofence` = `locked` and `inventory` = `locked` when its surface ships. On
2026-07-27 Tim additionally moved `mustering` from `absent` to `locked`, so
every capability now presents as a visible upsell teaser and no shipped
registry entry uses `absent`. That presentation remains fully implemented and
tested — the assignment is a commercial call that can change again, and it
carries no backend implication either way (TRA-1026).

**Gate precedence (TRA-1057, 2026-07-28).** Four gates filter the frontend and
they compose in one order: **auth → entitlement → capability → role.**

A lock means money can unlock this; hiding means money cannot. Capability and
entitlement are commercial, so they lock and carry a CTA. Role is
organizational, so it hides — showing a viewer a lock on "Geofence defaults"
invites a sales conversation that does not exist.

Auth is its own gate, ahead of the rest, rather than folded into any of them. A
capability is a property of an org, so asking it of a visitor with no org is
meaningless; answering it produces copy addressed to "your organization" for
someone who has none. A signed-out visitor on an org-scoped route therefore gets
the auth answer and never reaches the capability question — including by direct
URL to a gated route, which renders the generic signed-out card and makes no
capability claim.

The auth answer lives in `frontend/src/lib/routing/routePolicy.ts`, evaluated in
`App`'s route switch ahead of the capability gate, and it renders
`<SignedOutUpsell>`: what the surface does, plus a free-trial path and a log-in
path. A third state covers the moment a persisted auth token has survived
rehydration but `initialize()` has not yet run: the gate answers `pending` and
renders the route's loading screen rather than a verdict, so a signed-in
visitor reloading a bookmarked org-scoped route never sees the signed-out card
flash before the route corrects to allowed. It replaced `ProtectedRoute`, which
redirected to `#login` with no explanation and had been applied to some
screens and not others. The signed-out nav is the ungated core — Scan, Locate,
Assets, Locations, Reports, Settings, Help — with capability-gated entries
hidden via `no-org`, so a first-time visitor is not opened with advanced
modules.

Delivery: the capability set is one more field on the `/users/me` org payload
(the same shape carrying `is_entitled`), flowing through the existing
Zustand path; `switchOrg`'s profile refetch refreshes it for free.
`useCapability(cap)` mirrors `useEntitlement()` with one deliberate
inversion: `useEntitlement` fails open when org is null; `useCapability`
fails **closed** — gated surfaces never flash on during load. Gated surfaces
are lazy-loaded (`lazy(() => import(...))`) so non-granted orgs never
download the chunk; guards live at the route definition, not inside the
component.

No plugin system: capabilities and presentation as data are in scope; a
lifecycle interface that modules implement is not.

### One-off capabilities

A one-off is a capability with a grant count of one, plus rules that keep it
quarantined and promotable:

1. **Backend:** a package under the same feature layout
   (`internal/feature/<workflow>/`), mounted behind `RequireCap` like any
   other, consuming core services and repositories.
2. **Schema:** a one-off may own its own tables via namespaced migrations
   that FK **into** core entities. It must never alter core tables or core
   semantics. A request that requires changing core is not a one-off — it is
   a product decision or a no.
3. **Config before code:** many "one-offs" are a standard capability plus
   parameters. The first intake question is whether org-level config on an
   existing capability expresses the request; a custom package is the
   fallback, not the default.
4. **Promotion** to standard is a commercial event (grant it to a second
   org), not an engineering event — a direct consequence of the naming rule.

The accretion failure mode is core service signatures growing optional
parameters to accommodate one-offs. The FK-in/never-alter-core rule plus the
intake question (projection over core, or change to core?) is the defense.

### Pricing stays decoupled

Capabilities are the gate-able unit. A commercial "plan" is a named bundle
that writes a capability set onto an org — no plan layer, plans table, or
billing coupling now; Stripe-era packaging composes on top of grants without
touching enforcement.

## Consequences

* **Backend work is unblocked by commercial decisions.** The pending
  `absent`-vs-`locked` call for inventory/geofence is frontend-only.
* **There is no backfill, and no signup default** (revised 2026-07-25,
  superseding an earlier decision to grant all three capabilities to all
  existing and new orgs). Existing orgs and new orgs alike receive **zero**
  `org_capabilities` rows: everyone gets the always-on asset-management base,
  nobody gets inventory, geofence, or mustering without an explicit grant.
  Segmentation therefore begins at deploy rather than only for accounts
  created after it. The costs are accepted deliberately — existing orgs lose
  reachability of the geofence and mustering surfaces (assessed safe:
  fixed-reader/geofence is pre-launch and neither live customer uses either),
  geofence becomes a locked upsell tile where an open tab used to be, and demo
  and internal orgs must be re-granted by hand. Consistent with "grants are
  explicit acts": no allowlist in the migration for known exceptions, and no
  default-grants config knob.
* **Grant management (TRA-1027) is a near-term need, not a nicety.** Deferring
  it was justified by the universal backfill; with an empty default, every
  grant — starting with the first customer exception on day one — requires
  either that UI or hand-written SQL.
* **Deploy sequencing is load-bearing.** Because the empty default is
  retroactive, shipping the schema ahead of the frontend nav gating means
  every org, live customers included, sees geofence UI that 403s underneath.
  The migration must land with, or after, the frontend gating.
* **Integrators get one new machine-readable denial class**
  (`capability_required`) and an unchanged contract otherwise; black-box API
  test cycles verify a single denial shape instead of two.
* **Revocation is immediate** — next request, no restart, no token reissue —
  because grants are read per-request, not baked into tokens.
* **One additional indexed query per request** (shared with the subscription
  gate on mutations). Accepted; cache only if profiling demands.
* **Adding a capability** = registry constant + seeded lookup row + feature
  package + `r.With(RequireCap(...))` on its routes + a nav registry entry.
  No topology change for standard capabilities or one-offs alike.

## Open at time of acceptance

* Presentation (`absent` vs. `locked`) for `inventory` and `geofence` —
  commercial decision, frontend-only.
* A pending customer-driven use case (tracked in Linear) enters as one
  registry entry + one feature package behind the same middleware when
  designed; expected to generalize into a standard capability.

## Alternatives considered

* **Postgres enum for capability names:** rejected. The vocabulary is
  code-owned; an enum duplicates it in DDL with `ALTER TYPE` friction and no
  value removal, to enforce integrity the validated write path and the
  lookup-table FK already provide.
* **`text[]` column on `organizations`:** rejected. Forfeits per-grant
  metadata, grant timestamps, and clean reverse queries for a marginal fetch
  simplification — and the fetch it would ride does not exist (auth never
  loads the org row).
* **Policy column on the lookup table / runtime-configurable presentation:**
  rejected. Presentation is enforced by shipped bundles and published specs —
  build-time artifacts — so DB-resident policy is false flexibility. If
  per-org presentation variance ever becomes real, it is a column on
  `org_capabilities`, not the lookup table.
* **Backend hide-404 per capability:** adopted in an earlier draft, reversed.
  BSL source-availability makes route existence public; concealment can only
  ever be presentation-layer curation, and a policy branch in `RequireCap`
  buys a second denial contract with no secrecy in return.
* **Capabilities baked into the session JWT:** rejected; grant changes must
  not wait out token lifetime.
* **Conditional route mounting per tenant:** rejected; per-tenant router
  state or restarts on grant changes, for zero benefit over per-request
  middleware.
* **Microservices / per-customer services and endpoints:** rejected; the
  monolith with data-driven gating is the design, and one-offs are grant
  data, not topology.
* **Plugin/module lifecycle framework:** rejected as
  framework-for-installability-we-don't-have.
* **Frontend-only gating:** rejected; the public API is live and keyed — the
  backend check is the entitlement system.
