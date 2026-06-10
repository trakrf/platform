# Subscriptions schema + lite entitlement — design

**Date:** 2026-06-10
**Tickets:** TRA-947 (closes), TRA-198 (foundation; remainder stays open), seeds TRA-135 / TRA-946
**Branch:** `feat/tra-947-subscriptions-schema-entitlement`
**Migration:** 000022 (latest on main is 000021)

## Goal

One Stripe-aware schema pass that satisfies both **Subscriptions lite** (TRA-947 — manual
org entitlement + server-side write-gating) and the **full plan/billing foundation**
(TRA-198 — plans, custom pricing, payment arrangements), so we migrate the data model once.
The lite enforcement ships live; Stripe integration, plan-limit enforcement, and the admin/UX
siblings are explicitly deferred.

## Decisions (locked during brainstorming)

1. **Hybrid entitlement** — the org keeps manual on/off booleans (the lite gate, used for
   superadmin comps of NADA / partners / internal orgs) AND can optionally carry a
   Stripe-managed subscription row. `is_entitled = manual override OR active subscription`.
   Manual control persists even after Stripe exists.
2. **Pin + supersede plans** — `subscription_plans` rows are immutable price-points (Stripe
   Price semantics). A subscription pins to a specific plan row; new pricing inserts a new row
   and existing pins stay put = automatic grandfathering. Standard tiers are shared rows
   (`owner_org_id` NULL); a custom deal is the only case that sets `owner_org_id`. Custom plans
   also unblock us from TRA-337 (we don't have to finalize the fixed tier list to ship).
3. **Payment = rail + bill-to, two fields** — `payment_rail` enum (`stripe` | `invoice`) is how
   we collect; `reseller_id` (nullable FK → partners) is who we bill (NULL = bill the customer
   org directly). CC/ACH instruments are fully externalized to Stripe (the point of using it).
4. **Dedicated `partners` table** — a billing entity we invoice, not necessarily a tenant.
   Also the forward home for whitelabel and future partner tenant-management, modeled after
   ThingsBoard multi-tenancy (their System-Admin → Tenant → Customer ≈ our
   trakrf-superadmin → partner → organization).
5. **Scope = full schema + lite enforcement** — all migrations PLUS the working `is_entitled`
   helper, 402 write-gating, and entitlement in the session payload. `subscriptions` / `partners`
   tables exist but stay dormant until Stripe (TRA-135). Plan-limit enforcement + get-my-plan
   endpoint deferred.
6. **Include `organizations.partner_id` dormant forward-hook** — one nullable FK, no behavior
   attached, so the tenant/whitelabel direction needs no future ALTER.

## Data model

### `organizations` — new columns

```sql
subscription_enabled      BOOLEAN NOT NULL DEFAULT true,  -- manual gate (lite)
subscription_expires_at   TIMESTAMPTZ,                    -- manual gate; NULL = perpetual
stripe_customer_id        VARCHAR(255),                   -- org-direct Stripe billing
default_payment_method_id VARCHAR(255),                   -- Stripe PaymentMethod ref
partner_id                BIGINT REFERENCES partners(id)  -- dormant forward-hook (tenancy)
```

The plan is **not** stored on the org — it lives on the subscription (single source of truth).
This intentionally supersedes TRA-198's literal `organizations.subscription_plan_id`.

### `subscription_plans` — immutable price-points

```sql
id              BIGINT PRIMARY KEY,                       -- Feistel id trigger
name            VARCHAR(100) NOT NULL,
owner_org_id    BIGINT REFERENCES organizations(id),      -- NULL = standard/shared; set = custom
stripe_price_id VARCHAR(255),                             -- NULL until Stripe (TRA-135)
max_users       INT,                                      -- NULL = unlimited (data-only this pass)
max_assets      INT,                                      -- NULL = unlimited
max_locations   INT,                                      -- NULL = unlimited
features        JSONB NOT NULL DEFAULT '{}',
is_active       BOOLEAN NOT NULL DEFAULT true,            -- false = retired but still pinnable
created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
```

- Immutability is a **convention enforced in code** (we never edit price/limits after a row is
  pinned; we supersede with a new row). Metadata-only edits (e.g. display `name`) are allowed,
  hence `updated_at`.
- Indexes: `owner_org_id`; partial index on standard active plans
  (`WHERE owner_org_id IS NULL AND is_active`).

### `subscriptions` — optional Stripe-managed deal (dormant until TRA-135)

```sql
id                     BIGINT PRIMARY KEY,                -- Feistel id trigger
org_id                 BIGINT NOT NULL REFERENCES organizations(id),
plan_id                BIGINT NOT NULL REFERENCES subscription_plans(id),  -- the pin
status                 subscription_status NOT NULL,      -- active|trialing|past_due|canceled|incomplete
current_period_end     TIMESTAMPTZ,
payment_rail           payment_rail NOT NULL DEFAULT 'stripe',  -- stripe | invoice
reseller_id            BIGINT REFERENCES partners(id),    -- NULL = bill org directly
stripe_subscription_id VARCHAR(255),
external_billing_ref   TEXT,                              -- PO/invoice ref for manual AR
created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
canceled_at            TIMESTAMPTZ
```

- Unique partial index: at most one `status = 'active'` subscription per org.
- Index on `org_id`.

### `partners` — reseller / whitelabel / future-tenant

```sql
id                 BIGINT PRIMARY KEY,                    -- Feistel id trigger
name               VARCHAR(255) NOT NULL,
identifier         VARCHAR(255) UNIQUE,                   -- slug; future whitelabel routing
billing_email      VARCHAR(255),
stripe_customer_id VARCHAR(255),                          -- if reseller pays via Stripe
metadata           JSONB NOT NULL DEFAULT '{}',
is_active          BOOLEAN NOT NULL DEFAULT true,
created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
deleted_at         TIMESTAMPTZ
```

### Enums

```sql
CREATE TYPE subscription_status AS ENUM ('active','trialing','past_due','canceled','incomplete');
CREATE TYPE payment_rail        AS ENUM ('stripe','invoice');
```

### Triggers / RLS

- New tables get the `generate_obfuscated_id()` BEFORE-INSERT trigger and the
  `update_updated_at_column()` trigger, matching existing table conventions.
- **RLS:** org-scoped tables (`subscriptions`, and custom plans by `owner_org_id`) get RLS on
  the org column mirroring the `asset_scans` pattern (TRA-875). `partners` and standard
  (`owner_org_id IS NULL`) plans are internal/global — no per-org RLS. The entitlement read in
  middleware queries `organizations` directly (already reachable in the existing org-context
  path). RLS specifics to be finalized in the implementation plan against the TRA-874/875 role
  model.

## Entitlement (Go) — the live behavior

```
is_entitled = (subscription_enabled AND (subscription_expires_at IS NULL OR now() < subscription_expires_at))
           OR (active subscription: status = 'active' AND (current_period_end IS NULL OR now() < current_period_end))
```

- Computed **server-side** in an `EntitlementService`; never trusted from the client.
- The subscription branch is wired but **dormant** now (no subscription rows until Stripe);
  manual booleans drive everything for lite.

### Enforcement

- New `SubscriptionRequired` middleware inserted **after `EitherAuth`** in the public write
  group (`backend/internal/cmd/serve/router.go` ~line 222) and the session-auth internal groups
  (scan-devices, output-devices).
- Gates **POST/PUT/PATCH/DELETE only**. GET stays open regardless of entitlement so lapsed orgs
  retain read-only visibility.
- Paid mutations covered: assets create/update/delete/rename/tags; locations
  create/update/delete/rename/tags; inventory save (asset_scans writes); scan-device &
  scan-point config writes; output-device config writes.
- Rejection: **402 Payment Required** via a new `ErrPaymentRequired` error type +
  `Respond402PaymentRequired` helper, emitted through the existing RFC-7807 envelope
  (`httputil.WriteJSONError`, TRA-538 lineage). Body is distinct from 401 (auth) and 403 (RBAC)
  so the frontend branches to the right prompt.

## Session payload

Expand `UserOrgWithRole` (`backend/internal/models/organization/organization.go`) and populate
in `GetMe` (`backend/internal/handlers/orgs/me.go`) with:

- `is_entitled` (bool, computed server-side)
- `subscription_enabled` (bool, raw)
- `subscription_expires_at` (timestamptz or null, raw)

## Migration 000022 + signup default

- One up/down migration pair (`000022_subscriptions_schema.{up,down}.sql`):
  - Create the two enums, then `partners`, `subscription_plans`, `subscriptions` (with triggers).
  - `ALTER TABLE organizations` add the five new columns.
  - **Backfill all existing orgs entitled** (`subscription_enabled = true`,
    `subscription_expires_at = NULL`) — NADA, partner/API/internal orgs are never surprise-locked
    at deploy.
  - **Seed standard tiers** (Free / Starter / Professional / Enterprise) as shared plan rows
    (`owner_org_id` NULL, `stripe_price_id` NULL until pricing/Stripe land). Limits left as
    placeholder/NULL pending TRA-337; they are data-only this pass.
  - `down.sql` drops in reverse (columns, tables, enums).
- **Column defaults:** `subscription_enabled` defaults `true`; `subscription_expires_at` defaults
  **NULL** (perpetual). There is intentionally **no blanket trial default** — a column default of
  `now() + 1 month` would silently expire the customer orgs trakrf creates internally
  (current workflow = trakrf-creates-org-then-invites-contacts). The trial is a property of
  self-service signup, not of org-creation in general.

### Entitlement at creation

- **Self-service signup** — `auth.Signup`, standard personal-org branch (a new user, no
  invitation, no human in the loop) **explicitly** sets `subscription_expires_at = now() + 1 month`.
  This is the **only** path that hardcodes a trial.
- **Invitation-based signup** (`auth.signupWithInvitation`) — joins an existing org, creates no
  trial org, sets nothing.
- **Internal / explicit org create** (`POST /api/v1/orgs` → `orgs.CreateOrgWithAdmin`) — lands
  **perpetual** (`expires_at` NULL) by default. A superadmin then either enters committed-paid
  details or applies a trial **via the TRA-949 controls** (which extend from org-edit to cover
  the moment of create). That trial-vs-paid judgment is a human decision, not a hardcoded path
  behavior — so this pass adds no trial logic to the explicit-create path.
- **Superadmin notification** on a self-service trial signup is split out to **TRA-967** (4th
  child of TRA-946) — out of scope here; it only has a live trigger once self-service signup is
  surfaced (TRA-948).

## Out of scope (explicitly deferred)

- Stripe webhooks / checkout / customer + subscription sync → **TRA-135**.
- Plan-limit enforcement (`max_users` / `max_assets` / `max_locations`) + the "get current-org
  plan + limits" endpoint → **TRA-198 remainder** (ticket stays open).
- Superadmin entitlement controls in org edit (and the create-time trial-vs-paid choice) → **TRA-949**.
- Three-state gating UX (logged-out / entitled / lapsed) → **TRA-948**.
- Superadmin notification on self-service trial signup → **TRA-967**.
- Full ThingsBoard 3-tier hierarchy, partner tenant-management, whitelabel → future
  (the `partners` table + `organizations.partner_id` hook are the seeds).

## Testing

- **Migration:** applies clean; all existing orgs entitled; down reverses; Feistel ids generate
  on new tables.
- **Entitlement helper (unit):** truth table over the formula — enabled+no-expiry, enabled+future,
  enabled+past (lapsed), disabled; plus the active-subscription branch.
- **Enforcement (integration):** a lapsed org's GETs succeed; paid POST/PUT/PATCH/DELETE return
  the distinct **402** envelope on both UI and public API routes; an entitled org passes.
- **Signup:** a self-service signup org has `subscription_expires_at` ~one month out; an
  invitation-based signup and an explicit `POST /api/v1/orgs` create leave it NULL (perpetual).
- **Session payload:** `is_entitled` + raw fields present and correct for entitled vs lapsed orgs.

## Ticket / PR mapping

One branch / PR. **Closes TRA-947** fully. Lands the schema foundation for **TRA-198** (which
stays open, descoped to its plan-limit enforcement + endpoint remainder). Seeds **TRA-135**
(Stripe) and the **TRA-946** parent.
