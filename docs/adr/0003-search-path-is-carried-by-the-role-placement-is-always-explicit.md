# ADR 0003 — `search_path` is carried by the database role and may only resolve references; anything that decides *placement* is explicit

Date: 2026-07-30
Status: Accepted
Tracking: TRA-1069 (pin the migration ledger schema — shipped), TRA-1074 (role-level carrier, infra), TRA-1075 (local + edge roles), TRA-1076 (hermetic stored procedures)

## Context

All application objects live in a `trakrf` schema rather than `public`. The
original motivations were forward-looking: plugins or microservices with their
own schemas coexisting with the core tables, and schema-per-tenant or
schema-per-environment segmentation inside one database. **Neither was ever
implemented.** What the schema actually earns today is operational:
`DROP SCHEMA trakrf CASCADE` as the rebuild primitive, and a clean boundary for
the two-role least-privilege posture (TRA-85) where `trakrf-migrate` owns the
schema and `trakrf-app` holds `USAGE` and CRUD with no `CREATE`.

Because the objects are not in `public`, something has to put `trakrf` on the
session `search_path`. That "something" was never decided once, and the
consequences compounded for six months.

### The failure this record exists to prevent

golang-migrate's postgres driver locates its `schema_migrations` bookkeeping
table with `CURRENT_SCHEMA()` when `Config.SchemaName` is unset. With
`search_path=trakrf,public`, `CURRENT_SCHEMA()` returns `public` on a fresh
database — the `trakrf` schema does not exist until migration `000001` creates
it — and `trakrf` on every run afterwards. The ledger silently relocates, the
new location starts empty at version 0, and the whole stack replays against an
already-populated schema, dying on an "already exists" error. Forcing a version
to unstick that leaves a ledger reporting a clean version over a schema that
does not match it.

TRA-1069 is what that looks like from the outside: `trakrf.refresh_tokens`
absent while the ledger reported a clean version 38, and signup returning 500
with nothing pointing at the cause. (It was that particular table because a
later commit folded three incremental migrations *into* `000009` — editing an
applied migration silently skips its new DDL on every database that already
recorded it. A separate lesson, recorded in TRA-1069.)

### The same bug was found five times and fixed zero times

| Where | Mitigation | Steers `CURRENT_SCHEMA()` to |
|---|---|---|
| TRA-278 (2026-01-14, **canceled**) | Correctly diagnosed it and specified `postgres.Config{SchemaName}` | — |
| infra `f52dd9d` / TRA-383 (2026-04-16) | Inverted the DSN to `search_path=public,trakrf` | `public` |
| `backend/justfile test-contract` | Pre-created the `trakrf` schema | `trakrf` |
| `deploy/edge/db-init.sh` + quadlet | Pre-created the `trakrf` schema | `trakrf` |
| a plain local database | nothing | drifts — TRA-1069 |

Four mitigations, two of them steering in opposite directions, none of them
addressing the cause. That is why the ledger's schema differed by environment:
`trakrf,public` (Railway, Timescale Cloud, local, edge) put it in `trakrf`;
`public,trakrf` (GKE preview and prod) put it in `public`. Each mitigation was
locally reasonable and collectively they made the invariant unknowable.

The root mistake was not choosing the wrong carrier. It was letting a
*structural* fact — where bookkeeping lives — be derived from *session state*.
A per-role default would have made the drift consistent and hidden this bug
rather than removing it.

## Decision

### `search_path` may resolve references; it must never determine placement

The operative distinction. Reading an existing object through the path is
convenience: the object's schema is already fixed, and the path only saves
typing. Creating an object through the path, or locating bookkeeping through it,
lets ambient session state decide where something permanently lives. The first
is fine. The second is the defect class above.

Everything below follows from that one line.

### The role is the carrier; connection strings are out of this business

`search_path` is set as a per-role default (`ALTER ROLE … IN DATABASE … SET
search_path = trakrf, public`), not appended to an assembled DSN.

A DSN setting has to be remembered at every connection site, and forgetting it
is silent. TRA-383 had to add it to multiple sites in lockstep, and every future
consumer — a debug pod, a backfill Job, a cron, an interactive `psql` — has to
remember independently. Server-side configuration cannot be forgotten by a new
client.

This applies uniformly, including local dev and the edge demo box, which is why
those get a real database and real roles instead of connecting as the `postgres`
superuser (TRA-1075). Setting `search_path` on a superuser role would be
cluster-wide, so without dedicated roles those environments need
`ALTER DATABASE` instead — a second carrier, and the policy stops being uniform.
Dedicated roles remove the exception rather than documenting it.

The local roles keep the DDL/DML split. Beyond consistency, a superuser bypasses
row-level security, so RLS policies go untested locally and only fail once
deployed. The integration harness already proved the posture with
`trakrf_test_app` (TRA-874).

### Per-role settings must be re-applied idempotently, not set by hand

Per-role settings live in `pg_db_role_setting`, keyed to the role. If CNPG
recreates a managed role, the setting silently disappears — the same failure
mode as `DROP SCHEMA … CASCADE` wiping `pg_default_acl`, which is why the
init-grants Job exists (infra#118). The role's `search_path` therefore belongs
in that Job, next to the grants, re-applied on every `post-install,post-upgrade`
run.

### The migration ledger's schema is pinned in code

`internal/cmd/migrate` passes an explicit `postgres.Config{SchemaName: "public"}`.
Ledger location is a structural fact and does not depend on `search_path`,
role defaults, or anything else ambient.

`public` rather than `trakrf` because that is where preview's and prod's ledgers
already are — relocating live migration bookkeeping buys nothing — and where a
fresh database's first run lands. Not for privilege reasons: `serve` stopped
running migrations at TRA-367, so the app role never touches the ledger.

Migrating is additionally refused outright when a `schema_migrations` table
exists in any other schema, naming each with its version. A split history cannot
be resolved without a human deciding which is real, and reporting success over
it is how TRA-1069 stayed invisible.

Because the ledger is pinned, the `public,trakrf` inversion from TRA-383 is no
longer load-bearing and the ordering can return to `trakrf, public`.

### Migrations keep their own `SET search_path` — permanently

Every migration begins with `SET search_path = trakrf, public`. These stay, in
existing files *and in new ones*.

A migration is a permanent, replayable artifact, run by whatever happens to
connect — the migrate Job, a `migrate` CLI from a laptop, a debug session, a
future tool. Its unqualified DDL decides *placement*, so it must not depend on
ambient state. One session with a different default would put tables in the
wrong schema, silently, in exactly the manner of TRA-1069.

This deliberately rejects TRA-278's step 4 ("remove explicit `SET search_path`
from migrations — it's now handled at connection level"). Role-level defaults
make that *possible*; the rule above makes it *wrong*. Self-contained is worth
more than terse here.

### Functions that reference objects unqualified carry their own `SET search_path`

A function without `SET search_path` resolves through the **caller's** session
path, which makes ambient state load-bearing at runtime. `trakrf.org_is_entitled`
already does this correctly and is the pattern. TRA-1076 brings the `000010`
stored procedures in line via a forward migration.

This is also why TRA-1074 must *replace* the DSN setting rather than delete it:
until those functions are hermetic, runtime `search_path` is genuinely required,
not merely convenient. Afterwards, the role default becomes a safety net for
ad-hoc use rather than a correctness dependency.

### Go continues to fully qualify

Storage queries name `trakrf.<table>` explicitly. This is not technical debt to
be swept away; it is the same principle applied at the application layer.

## Consequences

* Ledger location is knowable from the code, in every environment, forever.
  It no longer depends on `search_path`, on ordering, or on whether a schema
  happened to exist at first connect.
* A database with a split history fails loudly instead of reporting success.
  This is a **breaking change for already-split local databases**, which must be
  rebuilt (`just db reset` then migrate). That is intended: a reconcile cannot
  be trusted when the foundation came from a pre-fold migration.
* Three of the four historical mitigations are deleted; the fourth (infra's DSN
  inversion) becomes harmless and is removed by TRA-1074. No mitigation is
  load-bearing after that.
* One new connection path can no longer break schema resolution by forgetting a
  DSN parameter.
* Local dev gains RLS enforcement it never had, which will likely surface
  existing latent RLS bugs. That is a benefit that will feel like a cost.
* The `trakrf` schema stays. It is not free, but the alternative is worse (see
  below).

## Open at time of acceptance

* Whether `trakrf-app` should have its `search_path` set at all once TRA-1076
  lands, or whether fully-qualified access everywhere makes it unnecessary.
  Leaving it set is the safer default; removing it is a later cleanup, not a
  goal.
* Whether the edge demo box should share the deployed role names or keep its
  own. Depends on how much of the CNPG grant story is worth reproducing on a
  single-tenant box.

## Alternatives considered

**Flatten the schema — move everything to `public` and delete the concept.**
Tempting, since the plugin and multi-tenant motivations never materialized. The
bill is ~810 `trakrf.` references across 44 non-test Go files, 198 more in
migration SQL, a live data migration of 28 tables on prod and preview, and
rework of a grant posture that is keyed per-schema by design. It also forfeits
two things in active use: `DROP SCHEMA trakrf CASCADE` as the rebuild primitive,
and the least-privilege boundary. And it does not even achieve
single-schema — TimescaleDB occupies nine schemas with 213 tables, so multi-schema
is unavoidable regardless. `public` on preview currently holds exactly one
table: the ledger. Rejected: high cost, negative value.

**Make the schema name configurable (TRA-278).** Its motivation — Timescale
Cloud blocking `CREATE DATABASE`, forcing schema-per-environment inside one
database — disappeared when preview moved to CNPG with its own database. It also
requires removing all qualification from Go, which this record considers a
regression rather than a cleanup. Independently, schema-per-environment in one
database shares WAL, autovacuum and Timescale's background worker pool, which is
poor isolation for a time-series workload. Stays canceled. Its
migration-bootstrapping half was the valuable part and shipped as TRA-1069.

**Keep steering `CURRENT_SCHEMA()`, but consistently.** Pick one ordering and
enforce it everywhere. Cheaper, and it would have prevented TRA-1069's split.
Rejected because it preserves the actual defect: placement still derived from
session state, waiting for the first environment that differs. Four mitigations
already demonstrated how well "enforce it everywhere" holds up.

**Set `search_path` at the database level (`ALTER DATABASE`) instead of the
role.** Works, and is what edge and the contract-test database do today. Loses
the per-role distinction that makes least-privilege legible, and does not
generalize to a shared database serving multiple roles. Kept only as a
transitional state until TRA-1075.
