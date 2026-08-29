# ADR 0003 — One thing runs migrations, and it owns the schema, its ledger, and every object in them

Date: 2026-07-30, amended 2026-08-05
Status: Accepted
Tracking: TRA-1069 (shipped), TRA-1075 (local + edge non-superuser roles), TRA-1077 (migration checksum guard), TRA-1104 (ownership asserted — amendment), TRA-1105 (closing the drift generator, infra), TRA-1190 (extension members scoped out — amendment)

> **2026-08-05 amendment — an extension, not a reversal.** Everything decided on
> 2026-07-30 stands unchanged. This record already made the migrating role the
> owner of the schema and its ledger; TRA-1104 showed that ownership was modelled
> but never *asserted*, and a deployed environment had quietly drifted from it.
> The amendment adds that assertion and extends the same scope from "the schema
> and its ledger" to "every object in them". It lives inline rather than in a
> separate ADR because it is this decision carried through, not a new one.

## Context

Application objects live in a `trakrf` schema. golang-migrate's postgres driver
locates its `schema_migrations` ledger with `CURRENT_SCHEMA()` when
`Config.SchemaName` is unset — and on a fresh database `trakrf` does not exist
yet, because migration `000001` is what creates it. So `CURRENT_SCHEMA()` resolves
to `public` on the first run and `trakrf` on every run afterwards. The ledger
relocates, the new one starts at version 0, the stack replays onto a populated
schema and dies on "already exists". Forcing a version past that leaves a ledger
reporting clean over a schema that does not match it.

TRA-1069 is that: `trakrf.refresh_tokens` missing while the ledger read a clean
version 38, signup 500ing, nothing naming the cause.

It had been found four times and fixed zero times, because every fix addressed the
symptom — steering `CURRENT_SCHEMA()` — rather than the cause:

| Where | Mitigation | Steered `CURRENT_SCHEMA()` to |
|---|---|---|
| TRA-278 (2026-01-14, canceled) | Diagnosed it; specified `Config{SchemaName}` | — |
| infra `f52dd9d` / TRA-383 | Inverted the DSN to `public,trakrf` | `public` |
| `test-contract` | Pre-created the schema | `trakrf` |
| `deploy/edge` | Pre-created the schema | `trakrf` |
| a plain local database | nothing | drifts — TRA-1069 |

Two steered opposite ways, which is why the ledger's location differed by
environment. The deeper problem was that there were **two implementations** of
"run the migrations" — the `./server migrate` subcommand and a bare `migrate` CLI
invocation in the test harness — so anything learned had to be taught twice, and
wasn't.

### Ownership was modelled but never asserted (TRA-1104)

Six days after this record was accepted, `trakrf.normalize_tag_value(text)` on
preview turned out to be owned by `postgres`. Every other object in the schema was
owned by `trakrf-migrate` — the model above held everywhere else, and nothing had
noticed the one exception, because no migration had ever tried to *replace* that
function until `000039` did.

The failure chain matters, because none of it looks like a migration failure from
outside:

1. `ERROR: must be owner of function normalize_tag_value`
2. The migration aborts partway; golang-migrate leaves `version=39, dirty=true`
3. A dirty ledger makes every later migrate run refuse to start
4. The migrate Job is an ArgoCD **PreSync hook**, so the Deployment is never
   updated — no new ReplicaSet, and the old pod keeps serving
5. ArgoCD reports **OutOfSync / Healthy**, not a crashloop

Preview served a stale build for over an hour with green CI on every affected
commit, found only because someone checked `/version.json` by hand.

**The repair is not available to the thing that needs it.** `CREATE OR REPLACE`,
`DROP`, and `ALTER … OWNER TO` all require ownership — exactly what is missing — so
the migrating role is locked out of every verb that could fix it. Measured rather
than reasoned about:

```sql
BEGIN;
CREATE FUNCTION trakrf._probe(v text) ... ;   -- as postgres, so postgres owns it
SET ROLE "trakrf-migrate";
ALTER FUNCTION trakrf._probe(text) OWNER TO "trakrf-migrate";
-- ERROR:  must be owner of function _probe
ROLLBACK;
```

Only a superuser or the current owner can break the loop. Any design expecting the
migration or the runner to *fix* ownership expects something Postgres will not
permit.

**The drift comes from the ops path.** `just ops psql <env>` connects as
`postgres`, a superuser, so any hand-run DDL creates an object the migrating role
can never replace. It works perfectly until the first migration touches it, which
may be months later and will look like that migration's fault. Same family as
infra#118, where a schema rebuild wiped default privileges: the deployed schema
drifting from what the role model assumes, with the consequence deferred to
whoever next deploys.

## Decision

**There is one implementation.** `internal/cmd/migrate` exposes `Run` (reads
`PG_URL`) and `RunURL` (explicit URL). The integration harness calls `RunURL`
instead of shelling out to the `migrate` CLI, which deleted ~110 lines of
harness — `getMigrationsPath`, `findMigrateBinary`, and the "migrate binary not
found in PATH" failure mode along with them. Anything that migrates goes through
here and inherits everything below for free.

**It creates the schema itself**, `CREATE SCHEMA IF NOT EXISTS trakrf`, before
golang-migrate looks for its ledger. This is the actual fix: it is the one thing
that cannot be left to a migration, because the driver resolves the ledger
location and creates that table *before* migration `000001` runs. Migration
`000001` still declares the schema, for a hand-applied run.

**It sets its own `search_path`** via `ConnConfig.RuntimeParams`, so a migration's
unqualified DDL resolves to the application schema regardless of the caller's DSN
or role default. Nothing about placement depends on deployment config.

**The ledger lives in `trakrf`**, pinned with `Config{SchemaName}`, so the schema
and its bookkeeping are one unit. That matters for the documented rebuild path:
`DROP SCHEMA trakrf CASCADE` now takes the ledger with it, leaving a genuinely
empty database. With the ledger in `public` it would survive the drop still
claiming version 38, and the next migrate would report "no pending migrations"
against an empty schema — TRA-1069 reproduced by the reset procedure itself.

**Migrating is refused when a `schema_migrations` exists in any other schema**,
naming each with its version. A split history needs a human to decide which is
real; reporting success over one is how TRA-1069 stayed invisible. This also makes
the one-time ledger relocation below safe to forget: the Job fails loudly instead
of replaying.

**Every object in `trakrf` is owned by the migrating role, and the runner asserts
it before writing anything** (TRA-1104). Three parts, and the order matters:

* **Assert, do not repair.** The runner cannot fix ownership and must not pretend
  to. It reports every offending object with the exact `ALTER` that repairs it,
  says plainly that the repair needs superuser or owner rights, and stops.
* **Refuse before the first write** — before the `CREATE SCHEMA` above, before
  golang-migrate resolves its ledger. A preflight running after the ledger exists
  has already lost the property worth having: on refusal nothing is written, the
  ledger stays clean, and the old pod keeps serving.
* **Ownership means `pg_has_role`, not equality.** Postgres accepts an ownership
  check from a member of the owning role, so a role hierarchy that would have
  migrated fine must not be reported as drift. The same predicate covers
  superusers, which are implicit members of every role.

Hand-running DDL as a superuser against a deployed database is what creates this
condition. It is a mutation of the schema's role model, not a read-only
convenience.

## Consequences

* **Preview and prod need a one-time relocation** — their ledgers are in `public`
  (at 38 and 10). `ALTER TABLE public.schema_migrations SET SCHEMA trakrf;` before
  or with the deploy. If forgotten, migrate refuses and nothing is damaged.
* `deploy/edge` needs no change: its pre-create already steered to `trakrf`, which
  is now where the ledger belongs. The workaround is now redundant rather than
  wrong, and can be dropped whenever that file is next touched.
* **Already-split local databases must be rebuilt** (`just db reset` then
  migrate). Intended — a reconcile cannot be trusted when the foundation came from
  a pre-fold migration.
* infra's `public,trakrf` DSN inversion is no longer load-bearing, so the ordering
  is free to go back to `trakrf, public` whenever convenient. Not urgent.
* golang-migrate keeps no checksums — its ledger is one `(version, dirty)` row and
  `Up()` never opens files at or below the current version — so editing an applied
  migration is undetectable. That is how three migrations folded into `000009`
  silently never ran. TRA-1077 added the guard Flyway would have given us:
  `backend/migrations/checksums.txt` plus `TestMigrationChecksums`, which fails the
  build when a recorded migration is edited or deleted. It is a source-level guard,
  not a runtime one — it catches the edit in review, it cannot reconcile a database
  that already diverged.
* **The wedge becomes a refusal.** The costly part of TRA-1104 was never the
  failing statement — it was the dirty ledger, the silent hour, and needing
  superuser access to recover. A refusal before the first write costs a failed
  deploy and a one-line repair.
* **The ownership preflight detects; it does not prevent.** While the ops path
  connects as a superuser, new drift can appear at any time, and the preflight
  fires at the *next* deploy — possibly long after the session that caused it.
  Closing the generator is infra's half (TRA-1105) and is not done here.
* **A false positive would fail a deploy that would have succeeded.** That is why
  the role-membership case is tested rather than assumed. A guard that cries wolf
  on a correctly-owned schema gets disabled, and then guards nothing.
* **Local development is unaffected** by the ownership half: local and the
  integration harness migrate as `postgres`, and a superuser sees no drift by
  construction.

## Notes on scope

The `trakrf` schema stays. Its original justifications (plugins, schema-per-tenant)
were never implemented, but what it earns now is operational:
`DROP SCHEMA trakrf CASCADE` as the rebuild primitive, and a clean boundary for
the two-role least-privilege posture (TRA-85). Flattening it into `public` would
mean ~810 `trakrf.` references across 44 Go files, 198 more in migration SQL, and
a live 28-table migration on prod and preview — to forfeit both of those. It would
not even achieve single-schema: TimescaleDB occupies nine schemas with 213 tables.

Schema-per-tenant is abandoned. Plugin or custom-data schemas stay plausible, and
explicit placement is what makes them possible rather than merely compatible: if
core resolved names through the path, a plugin schema could shadow a core table,
and a shadowed table means the real table's RLS policies are never consulted.
Terms if one arrives — its own ledger (a single `(version, dirty)` row cannot
represent two histories), explicit DDL placement, and no unpinned
`SECURITY DEFINER`. Nothing to build now.

TRA-278 (configurable schema name) stays canceled. Its driver — Timescale Cloud
blocking `CREATE DATABASE`, forcing schema-per-environment — died when preview
moved to CNPG. It also cannot cover `SECURITY DEFINER` functions, which must pin a
literal `SET search_path` or a caller who can create objects earlier on the path
can shadow a table and execute as the definer. So the schema name can never fully
leave the SQL, and treating it as an identifier rather than a configuration point
is the honest position. Its migration-bootstrapping half shipped as TRA-1069.

TRA-1075 (non-superuser roles for local dev and edge) survives this record. It is
not justified by `search_path` — nothing about placement depends on the role — and
it stands on its own argument: a superuser bypasses RLS, so every policy goes
untested locally until it reaches a deployed environment. The integration harness
already proved the posture with `trakrf_test_app` (TRA-874).

The 2026-08-05 amendment gives it a second, independent justification. Placement
still does not depend on the role, exactly as recorded above; ownership does, and
ownership is new scope rather than a revision. The migrating role's identity is
load-bearing for it: the role decides which objects the runner can replace, and a
superuser session is precisely what quietly creates objects it cannot. That is the
same shape as the RLS argument — a superuser hides a constraint a deployed
environment will later enforce.

**"Every object" excludes an extension's own objects (2026-08-29, TRA-1190).**
The title's scope is otherwise read wider than the guard implements, which is the
worse kind of stale record — right conclusion, reason that has quietly expired.

`pgcrypto` is a *trusted* extension, so `CREATE EXTENSION pgcrypto` succeeds for a
non-superuser; Postgres nonetheless assigns every resulting object to the
bootstrap superuser, whoever ran the statement. Migration `000001` creates it
inside `trakrf`, so every database migrated by `trakrf-migrate` permanently holds
36 `postgres`-owned functions there. The migrating role cannot own them, and the
repair statement this record's amendment prints cannot make it so.

Left in scope, the preflight fired on every local database forever:
`just backend migrate` worked once on a fresh database and refused every run after
— including as a no-op, since the preflight precedes golang-migrate deciding there
is nothing to do — so the *second* `just dev` failed. It went unseen only because
`PG_URL_MIGRATE_LOCAL` was unset and the command had never run at all.

The exclusion concedes nothing the guard protects. A migration never
`CREATE OR REPLACE`s an extension member — the extension owns its definitions — so
such an object cannot produce the half-applied migration and dirty ledger the
check exists to prevent. The boundary is ownership the migrating role could
*plausibly hold*, not ownership it holds. Objects it could hold and does not are
still drift, asserted in the same test as the exclusion.

Ownership is not privileges. This record governs who owns objects in `trakrf`;
`GRANT`s drift independently and by a different mechanism (infra#118), and
asserting ownership is not a substitute for asserting privileges.

An earlier draft of the ownership check carried an explicit superuser exemption
alongside the `pg_has_role` test. Mutation testing could not kill it: each clause
masked defects in the other, and the test asserting the superuser exemption could
not be made to fail. It was removed. Redundant belt-and-braces in a guard is not
free — it hides which clause is load-bearing, and takes a test down with it.
