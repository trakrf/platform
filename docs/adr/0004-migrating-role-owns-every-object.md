# ADR 0004 — The migrating role owns every object in the schema, and that is asserted rather than assumed

Date: 2026-08-05
Status: Proposed
Tracking: TRA-1104 (preview wedge), TRA-1076 (the migration that exposed it), infra#118 (privileges lost the same way)

## Context

[ADR 0003](0003-explicit-schema-placement.md) settled that one thing runs
migrations and "owns the schema and its ledger". That was a claim about intent.
Nothing enforced it, and on 2026-08-05 it turned out not to be true.

`trakrf.normalize_tag_value(text)` on preview was owned by `postgres`. Every
other object in the schema was owned by `trakrf-migrate`. Nothing had ever
noticed, because no migration had tried to *replace* that function — until
migration `000039` did.

The failure chain is worth spelling out, because none of it looks like a
migration failure from the outside:

1. `ERROR: must be owner of function normalize_tag_value`
2. The migration aborts partway; golang-migrate leaves the ledger `version=39,
   dirty=true`
3. A dirty ledger makes every subsequent migrate run refuse to start
4. The migrate Job is an ArgoCD **PreSync hook**, so the Deployment is never
   updated — no new ReplicaSet, and the old pod keeps serving
5. ArgoCD reports **OutOfSync / Healthy**, not a crashloop

Preview served a stale build for over an hour with green CI on every affected
commit. It was found only because someone checked `/version.json` by hand while
verifying an unrelated ticket.

### The repair is not available to the thing that needs it

The obvious fix — have the migration correct the ownership — does not exist.
`CREATE OR REPLACE`, `DROP`, and `ALTER … OWNER TO` all require ownership, which
is exactly what is missing. The migrating role is locked out of every verb that
could repair the condition. Measured on preview rather than reasoned about:

```sql
BEGIN;
CREATE FUNCTION trakrf._probe(v text) ... ;   -- as postgres, so postgres owns it
SET ROLE "trakrf-migrate";
ALTER FUNCTION trakrf._probe(text) OWNER TO "trakrf-migrate";
-- ERROR:  must be owner of function _probe
ROLLBACK;
```

Only a superuser, or the current owner, can break the loop. Any design that
expects the migration or the runner to *fix* ownership is expecting something
Postgres will not permit.

### Where the drift comes from

The ops path hands out superuser sessions: `just ops psql <env>` connects as
`postgres`. Any DDL run by hand through it — a hotfix, an experiment, a
half-finished investigation — creates an object the migrating role can never
replace. The object works perfectly until the first migration touches it, which
may be months later and will look like that migration's fault.

This is the same family as infra#118, where a schema rebuild wiped default
privileges: a deployed schema drifting away from what the role model assumes,
silently, with the consequence deferred to whoever next deploys.

## Decision

**Every object in `trakrf` is owned by the migrating role. The runner asserts
this before it writes anything, and refuses if it does not hold.**

Three parts, and the order matters:

1. **Assert, do not repair.** The runner cannot fix ownership and must not
   pretend to. It reports every offending object with the exact `ALTER` that
   repairs it, and stops.
2. **Refuse before the first write** — before schema creation, before
   golang-migrate resolves its ledger. A preflight that runs after the ledger
   exists has already lost the property that makes it worth having: on refusal
   nothing is written, the ledger stays clean, and the old pod keeps serving.
3. **Ownership means `pg_has_role`, not equality.** Postgres accepts an
   ownership check from a member of the owning role, so a role hierarchy that
   would have migrated fine must not be reported as drift. The same predicate
   covers superusers, which are implicit members of every role.

Hand-running DDL as a superuser against a deployed database is the thing that
creates this condition. Treat it as a mutation of the schema's role model, not a
read-only convenience.

## Consequences

**The wedge becomes a refusal.** The costly part of TRA-1104 was never the
failing statement — it was the dirty ledger, the silent hour, and needing
superuser access to recover. A refusal before the first write costs a failed
deploy and a one-line repair.

**The preflight detects; it does not prevent.** As long as the ops path connects
as a superuser, new drift can be introduced at any time. Detection turns a silent
wedge into a loud failure at the next deploy, which may still be much later than
the hand-run session that caused it. Closing the generator is infra's half and is
tracked separately; this ADR does not claim it is done.

**Local development is unaffected.** Local and the integration harness migrate as
`postgres`, and a superuser sees no drift by construction.

**A false positive would fail a deploy that would have succeeded.** That is why
the membership case is tested rather than assumed. The preflight is only worth
having if a correctly-owned schema never trips it — a guard that cries wolf gets
disabled, and then guards nothing.

## Notes on scope

This governs the `trakrf` schema and the role that migrates it. It says nothing
about privileges (`GRANT`), which drift independently and by a different
mechanism — infra#118 is that problem, and ownership is not a substitute for it.

An earlier draft carried an explicit superuser exemption alongside the
`pg_has_role` test. Mutation testing could not kill it: each clause masked
defects in the other, and the test asserting the superuser exemption could not be
made to fail. It was removed. Redundant belt-and-braces in a guard is not free —
it hides which clause is actually load-bearing, and takes a test down with it.
