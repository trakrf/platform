# ADR 0003 — `search_path` gets exactly one load-bearing owner: the migration runner. Everything else names its schema

Date: 2026-07-30
Status: Accepted
Tracking: TRA-1069 (ledger pin + runner-owned DDL path — shipped), TRA-1076 (hermetic stored procedures — gates the rest), TRA-1074 (delete the DSN parameter), TRA-1075 (local + edge roles)

## Context

All application objects live in a `trakrf` schema rather than `public`. The
original motivations were forward-looking: plugins or microservices with their
own schemas alongside the core tables, and schema-per-tenant or
schema-per-environment segmentation inside one database. **Neither was ever
implemented.** Schema-per-tenant is now explicitly abandoned; plugin schemas stay
plausible on the terms set out below. What the schema earns today is operational:
`DROP SCHEMA trakrf CASCADE` as the rebuild primitive, and a clean boundary for
the two-role least-privilege posture (TRA-85), where `trakrf-migrate` owns the
schema and `trakrf-app` holds `USAGE` and CRUD with no `CREATE`.

Because the objects are not in `public`, something has to put `trakrf` on the
session `search_path`. That "something" was never decided once, and the
consequences compounded for six months.

### The failure this record exists to prevent

golang-migrate's postgres driver locates its `schema_migrations` bookkeeping
table with `CURRENT_SCHEMA()` when `Config.SchemaName` is unset. With
`search_path=trakrf,public`, `CURRENT_SCHEMA()` returns `public` on a fresh
database — `trakrf` does not exist until migration `000001` creates it — and
`trakrf` on every run afterwards. The ledger silently relocates, the new location
starts empty at version 0, and the stack replays against an already-populated
schema, dying on an "already exists" error. Forcing a version to unstick that
leaves a ledger reporting a clean version over a schema that does not match it.

TRA-1069 is what that looks like from outside: `trakrf.refresh_tokens` absent
while the ledger reported a clean version 38, signup returning 500, and nothing
pointing at the cause. (It was that table because a later commit folded three
incremental migrations *into* `000009` — editing an applied migration silently
skips its new DDL on every database that already recorded it. Separate lesson,
recorded on TRA-1069.)

### The same bug was found five times and fixed zero times

| Where | Mitigation | Steers `CURRENT_SCHEMA()` to |
|---|---|---|
| TRA-278 (2026-01-14, **canceled**) | Diagnosed it correctly; specified `postgres.Config{SchemaName}` | — |
| infra `f52dd9d` / TRA-383 (2026-04-16) | Inverted the DSN to `search_path=public,trakrf` | `public` |
| `backend/justfile test-contract` | Pre-created the `trakrf` schema | `trakrf` |
| `deploy/edge/db-init.sh` + quadlet | Pre-created the `trakrf` schema | `trakrf` |
| a plain local database | nothing | drifts — TRA-1069 |

Four mitigations, two steering in opposite directions, none addressing the cause.
That is why the ledger's schema differed by environment: `trakrf,public`
(Railway, Timescale Cloud, local, edge) put it in `trakrf`; `public,trakrf` (GKE
preview and prod) put it in `public`. Each was locally reasonable; together they
made the invariant unknowable.

The root mistake was not choosing the wrong carrier. It was letting a
*structural* fact — where bookkeeping lives — be derived from *session state*. A
per-role default would have made the drift consistent and **hidden** this bug
rather than removed it.

### The trilemma

Three properties are desirable and only two are simultaneously achievable:

1. **One authority** for the schema configuration point.
2. **No schema names hardcoded** in migrations or SQL.
3. **DDL placement never ambient** — not dependent on who connected or how.

Role-only config gets (1) and (2) and loses (3): a `migrate` run from a laptop
under a role without the default silently puts tables somewhere else.
Schema-qualifying every `CREATE TABLE trakrf.x` gets (1) and (3) and maximally
violates (2). This record chooses **(1) and (3)**, and accepts that a small,
principled set of places name the schema.

The tiebreaker is that (2) is unreachable anyway. `SECURITY DEFINER` functions
*must* pin their own `search_path`, or a caller who can create objects in an
earlier schema on the path can shadow a table and execute code with the definer's
privileges. Three such functions exist (`resolve_scan_topic`,
`list_active_scan_topics`, `org_is_entitled`) and all three already do it
correctly. Since the schema name can never fully leave the SQL, "one authority"
is the property worth protecting.

## Decision

### The schema name is an identifier, not a configuration point

Counting the places `trakrf` appears gives an alarming number — a role default, a
pool setting, 48 migration headers, and ~810 SQL literals in Go. But those are two
categories, and only one of them is redundancy:

* **Configuring name resolution** — the role default, the runner's pool setting,
  the per-migration headers. These all answer the same question ("what does an
  unqualified name mean here?"), so having three is genuine duplication with three
  ways to disagree. This record collapses them to one.
* **Naming a thing** — `trakrf.assets` in a Go query, `SET search_path` inside a
  `SECURITY DEFINER` function. These do not participate in resolution; they bypass
  it. They are not competing authorities, they are the table's full name used
  where tables are named.

The second category is only objectionable if the schema name is a *variable*. It
is not. TRA-278 tried to make it configurable, its motivation is gone, and this
record rejects reviving it. Treating `trakrf` as a fixed part of each object's
identity — as permanent as `assets` — means ~810 literals cost nothing ongoing.
They cost exactly once, if the schema is ever renamed, and nothing has ever wanted
to.

The alternative is worse on its own terms. Dropping qualification from Go to
reduce the literal count would make *runtime reads* resolve through ambient
`search_path`. Best case that yields a loud "relation does not exist"; worst case
it reads a shadowed table, which is the same privilege-escalation shape that
forces `SECURITY DEFINER` functions to pin their path — and here it would sit
directly on the RLS boundary. Explicit qualification is what makes the application
immune to resolution config entirely.

So: **one configuration point, and the schema name used wherever objects are
named.** That is the shape being chosen, and "four places" is an artifact of
counting two different things together.

### `search_path` may resolve references; it must never determine placement

Reading an existing object through the path is convenience — the object's schema
is already fixed. Creating an object through the path, or locating bookkeeping
through it, lets ambient session state decide where something permanently lives.
The first is fine. The second is the defect class above.

### There is exactly one load-bearing setter: the migration runner

`internal/cmd/migrate` imposes `search_path = trakrf, public` on every connection
it opens, via `ConnConfig.RuntimeParams`, overriding the DSN and any role
default. Unqualified DDL in a migration resolves against that, so the *runner*
decides where objects are created — not whoever invoked it.

This is the only place a session-level `search_path` is required for correctness.
Everything else either names its schema directly or does not depend on the path:

| Mechanism | Status | Why |
|---|---|---|
| Migration runner pool config | **Load-bearing** | Sole authority for DDL placement |
| `SECURITY DEFINER` functions pinning their own | **Required by Postgres** | Shadowing is a privilege-escalation vector; not optional |
| Go storage queries | Fully qualified | `trakrf.<table>` throughout; the only unqualified names are CTEs |
| Per-migration `SET search_path` headers | **Legacy, decaying** | `000001`-`000038` keep theirs (rewriting applied migrations buys nothing); new migrations do not get one |
| Role default `search_path` | **To stop being correctness** | See below |
| DSN `options=-c search_path=…` | **To be deleted** | TRA-1074 |

### Nothing at runtime should need `search_path`, and once that is true the role default stops being load-bearing

Audited at acceptance:

* Go storage queries qualify every table. The apparent exceptions are CTE names
  (`ancestors`, `chain`, `page`, `subtree`, `latest_scans`), not relations.
* No dynamic or string-built SQL anywhere in the migration stack.
* RLS policy expressions, continuous aggregates and generated-column expressions
  are resolved at creation time and stored as OIDs — the runtime path is
  irrelevant to them.
* The id trigger calls the qualified `nextval('trakrf.id_seq')` (TRA-886), which
  retired the original reason TRA-383 put `search_path` on the DSN.

**One real runtime dependency remains**: the `000010` stored procedures
(`process_tag_scans`, `create_asset_with_tags`, `create_location_with_tags`)
reference tables unqualified with no `SET search_path` of their own, so they
resolve through the caller's session path. TRA-1076 makes them hermetic.

Sequencing follows. Until TRA-1076 lands, the runtime `search_path` setting is
genuinely load-bearing and must not be deleted — only relocated, if desired.
After TRA-1076, no runtime consumer needs it, so **TRA-1074 becomes "delete the
DSN parameter" rather than "move it to the role."** A role default may be kept
purely as `psql` convenience, explicitly labelled non-load-bearing, or not set at
all.

That is the answer to the sprawl. The end state is one load-bearing setting, one
Postgres-mandated pattern, and a legacy header set that stops growing — not three
mechanisms restating one value.

### If a role default is kept, it must be re-applied idempotently

Per-role settings live in `pg_db_role_setting`, keyed to the role. If CNPG
recreates a managed role the setting silently disappears — the same failure mode
as `DROP SCHEMA … CASCADE` wiping `pg_default_acl`, which is why the init-grants
Job exists (infra#118). Anything kept therefore belongs in that Job, beside the
grants, re-applied on every `post-install,post-upgrade` run. A convenience
setting that silently vanishes is tolerable; a correctness one is not — another
reason not to make it correctness.

### The migration ledger's schema is pinned in code

`internal/cmd/migrate` passes an explicit `postgres.Config{SchemaName: "public"}`.
Ledger location is structural and does not depend on `search_path`, role
defaults, or anything ambient.

`public` rather than `trakrf` because that is where preview's and prod's ledgers
already are — relocating live migration bookkeeping buys nothing — and where a
fresh database's first run lands. Not for privilege reasons: `serve` stopped
running migrations at TRA-367, so the app role never touches the ledger.

Migrating is refused outright when a `schema_migrations` table exists in any
other schema, naming each with its version. A split history cannot be resolved
without a human deciding which is real, and reporting success over it is how
TRA-1069 stayed invisible.

Because the ledger is pinned, TRA-383's `public,trakrf` inversion is no longer
load-bearing and the ordering is free to return to `trakrf, public`.

### Local dev and edge get real roles

Not for `search_path` reasons — that justification dissolves once runtime does
not depend on the path — but because **a superuser bypasses RLS**. Connecting as
`postgres` means every row-level security policy goes untested until it reaches a
deployed environment, which is the TRA-900 class of bug. The integration harness
already proved the posture with the non-superuser `trakrf_test_app` role
(TRA-874); TRA-1075 extends it to the dev stack, keeping the DDL/DML split.

### Future plugin schemas are explicit too, and this decision is what enables them

Schema-per-tenant is off the table. Additional schemas for plugins or
customer-specific data stay plausible, and if they arrive they follow the same
rule: **explicit, never path-reliant.**

This is not merely consistent with plugins, it is the precondition for them. If
core code resolved names through `search_path`, any new schema would be a
potential regression — a plugin schema earlier on the path containing a table
named `assets` silently intercepts core queries, and a shadowed table means the
RLS policies attached to the real table are never consulted. Path-reliance would
hand plugin authors a way to break tenant isolation. Because core names
`trakrf.assets`, a plugin schema cannot interfere; the core is immune by
construction.

Conditions on any such schema, each following from TRA-1069:

* **Its own migration ledger.** golang-migrate's ledger is a single
  `(version, dirty)` row and structurally cannot represent two independent
  histories, so a plugin pins its own via `SchemaName` / `x-migrations-table` and
  never shares core's. The configuration point that caused TRA-1069 is the right
  tool here — used deliberately rather than by accident.
* **Explicit DDL placement.** A plugin's migrations name their target schema
  rather than inheriting an ambient path. Plugin authors are more likely to run
  migrations ad hoc than the platform is, and an inherited path would quietly
  place their objects in `trakrf`.
* **No unpinned `SECURITY DEFINER`.** Any plugin-defined `SECURITY DEFINER`
  function must pin its own `search_path`. This is the escalation vector and is
  not relaxable.

Keeping this door open costs nothing now, precisely because explicitness was
chosen. No configurability machinery is required today, and none should be built
speculatively — the `trakrf` schema itself was originally justified by plugin and
multi-tenant plans that never materialized, and TRA-278 was the same instinct one
level up.

## Consequences

* Ledger location is knowable from code, in every environment, permanently.
* A split history fails loudly instead of reporting success. This is a
  **breaking change for already-split local databases**, which must be rebuilt
  (`just db reset` then migrate). Intended: a reconcile cannot be trusted when
  the foundation came from a pre-fold migration.
* New migrations are shorter — no `SET search_path` header — and their DDL target
  is guaranteed by the runner regardless of who invokes it.
* The transitional overlap (runner + role/DSN + legacy headers) is real and
  temporary. TRA-1076 then TRA-1074 remove two of the three.
* Local dev gains RLS enforcement it never had, which will likely surface latent
  RLS bugs. A benefit that will feel like a cost.
* The `trakrf` schema stays. Not free, but the alternative is worse (below).

## Open at time of acceptance

* Whether to keep a role-level `search_path` at all after TRA-1076, as `psql`
  convenience. Leaning toward not setting it, so there is nothing to drift.
* Whether the edge box should share deployed role names or keep its own.

## Alternatives considered

**Flatten the schema — move everything to `public`.** Tempting, since the plugin
and multi-tenant motivations never materialized. The bill: ~810 `trakrf.`
references across 44 non-test Go files, 198 more in migration SQL, a live data
migration of 28 tables on prod and preview, and rework of a grant posture keyed
per-schema by design. It forfeits `DROP SCHEMA trakrf CASCADE` as the rebuild
primitive and the least-privilege boundary, both in active use. And it does not
even achieve single-schema: TimescaleDB occupies nine schemas with 213 tables, so
multi-schema is unavoidable. On preview, `public` currently holds exactly one
table — the ledger. Rejected: high cost, negative value.

**Make the schema name configurable (TRA-278).** Its motivation — Timescale Cloud
blocking `CREATE DATABASE`, forcing schema-per-environment inside one database —
disappeared when preview moved to CNPG with its own database. It also requires
removing all qualification from Go, which this record treats as a regression, and
cannot cover `SECURITY DEFINER` functions, which must pin a literal.
Independently, schema-per-environment in one database shares WAL, autovacuum and
Timescale's background worker pool — poor isolation for a time-series workload.
Stays canceled; its migration-bootstrapping half shipped as TRA-1069.

**Role as the single carrier, runner sets nothing.** Achieves (1) and (2) of the
trilemma, and is the tidiest on paper. Rejected because DDL placement becomes
ambient: a `migrate` run under a role without the default, or an interactive
session, silently creates objects in the wrong schema — the TRA-1069 failure mode
with a different object. The runner is the one actor that can guarantee its own
correctness without trusting deployment config.

**Runner verifies the role's `search_path` instead of setting it.** Keeps one
authority and fails fast on misconfiguration. Rejected: it still needs the
expected value in code, so nothing is deduplicated, and it converts a
self-healing situation into an outage whenever role config lags a deploy.

**Schema-qualify all DDL (`CREATE TABLE trakrf.x`).** Achieves (1) and (3) with no
session dependency at all. Rejected as the worse trade against (2): it puts the
schema name in every DDL statement, which is the duplication this record is trying
to reduce, and makes migrations noisier to read.

**Keep steering `CURRENT_SCHEMA()`, but consistently.** Pick one ordering, enforce
it everywhere. Cheaper, and would have prevented TRA-1069's split. Rejected
because it preserves the defect — placement still derived from session state,
waiting for the first environment that differs. Four mitigations already
demonstrated how well "enforce it everywhere" holds up.
