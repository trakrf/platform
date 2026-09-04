# ADR 0018 — A check that cannot run says so, and never shares an encoding with "all clear"

Date: 2026-09-04
Status: Proposed
Tracking: TRA-1218 (this record), TRA-1190 / ADR 0010 (the check this was found in, and the corollary it extends), TRA-1069 / TRA-1104 (ADR 0003, the ledger's placement)

## Context

ADR 0010 closed with a corollary: **a component that knows a precondition is
unmet must say so itself, and name it.** The backend duly compares the migration
set embedded in its binary against the version the database has applied, and
returns 503 with the unapplied migrations named.

It did not work anywhere it mattered, and nothing noticed for a month.

The backend connects as `trakrf-app`. In preview and prod that role had no
SELECT on `trakrf.schema_migrations`, so the read errored on every request. The
handler treated an unreadable ledger as *unknown* and — correctly — declined to
report drift, because a database blip must not present as "run your migrations"
and send an operator to do the wrong thing.

The mistake was in how it declined. It **omitted the `schema` block**. A healthy
backend also omits nothing else; the payload was:

```json
{"status":"ok","version":"v1.5.0-dev-preview+595+618","database":"connected"}
```

which is, byte for byte, what a fully current backend returns. The boot-time
variant did the same thing in the log: `if err != nil { return }`. Two
independent reporting paths, both of which encoded "I could not evaluate this"
as *silence*, and silence was already spoken for.

So the check that exists to stop a stale schema hiding behind a green /health
was itself hiding behind a green /health. The verified state on both clusters:
`trakrf.schema_migrations` carried a NULL `relacl` while every other table in
the schema carried `"trakrf-app"=arwd/"trakrf-migrate"`.

Two mechanisms each suffice to produce that, and both are ordinary:

* `ALTER DEFAULT PRIVILEGES` applies at CREATE time in the schema it names. A
  ledger predating the ADR 0003 pin was created in `public`, where no default
  privileges were set, and relocated with `ALTER TABLE … SET SCHEMA` (TRA-1084).
  That preserves the ACL it had in `public` — none.
* `GRANT … ON ALL TABLES` only covers what exists when it runs. In the cluster
  that grant lives in the `trakrf-db` chart's init-grants Job while the ledger is
  created by the `trakrf-backend` chart's migrate Job — separate Helm releases on
  quite different cadences, so a ledger arriving between two db-chart upgrades is
  never picked up.

Neither is exotic, which is the point: the grant was always going to go missing
somewhere. What made it cost a month was that its absence was unobservable.

## Decision

**A check reports three states, not two: met, unmet, and could-not-evaluate. The
third is reported positively, and never shares an encoding with the first.**

Concretely, and in that order of importance:

1. **"Unknown" may not be spelled the same way as "fine".** If healthy is
   encoded by the absence of a field, unknown may not also be encoded by the
   absence of that field. `/health` now always carries the `schema` block when
   there is a database to ask, with `readable: false` and a `reason` when the
   ledger could not be read. `applied` and `expected` are absent rather than
   zero — a zero is a claim about the database, where the honest statement is an
   admission about the read.

2. **The verdict may stay lenient; the silence may not.** Not being able to read
   the ledger is still not evidence of drift, so it is still a 200 and still not
   `schema_behind`. Nothing about this record argues for failing closed. It
   argues only that the leniency be *audible* — the same distinction as a test
   that is skipped versus a test that passed.

3. **Carry the error text, not just a boolean.** `permission denied for table
   schema_migrations` and `relation … does not exist` are the same
   `readable: false` and different repairs — a grant versus a migration. A
   boolean sends the reader to find out which; the string is the answer.

4. **Every reporting path gets the same treatment.** The boot-time drift check
   had the identical `return` on the identical error and was fixed in the same
   change. One path that speaks and one that does not is a coin toss over which
   one the next person happens to be reading.

5. **A precondition a check depends on is asserted where it can fail.** The
   grant is now issued by name — not left to a blanket `ON ALL TABLES` — in
   `database/sql/03-grants.sql` and in the init-grants Job in `trakrf/infra`, and
   `scripts/test-db-init.sh` fails if the ledger stops being named. SELECT only:
   the default privileges would otherwise hand the app role write access to its
   own bookkeeping, which a locally bootstrapped database really did have.

## Consequences

`/health.json` gains a `readable` field that is always present in the block, and
`applied` / `expected` become optional. Consumers must read `readable` before
the versions; the e2e preflight does, and warns rather than failing when the
precondition could not be evaluated.

The boot log gains a warning naming the ledger, the effect ("the schema drift
check is inert"), and the fix. It is a `warn`, not an `error`: the schema may
well be current. What is certain is only that nothing can tell.

This record does not say what a check should conclude — that is each check's
business, and lenient defaults remain right in most cases including this one. It
says only that a check must never be able to decline silently in a channel where
silence already means success.

Rejected: failing closed — treating an unreadable ledger as `schema_behind` and
503ing. It reintroduces precisely the fault the omission was guarding against, a
database blip presenting as "run your migrations", and would have taken preview
and prod down for a missing grant. The defect was never the verdict.

Rejected: leaving the payload alone and fixing only the grant. It repairs the
two databases that are known to be wrong and nothing else. Grants drift by a
mechanism independent of ownership (ADR 0003) and there are at least two routine
ways to lose this one, so the next occurrence was a question of when. A repair
that leaves the state unobservable buys one incident.

Rejected: having the migration runner issue the grant itself, since it is the
component that creates the ledger and knows exactly when it appears. It is the
tidiest place and it would fix the cluster with no infra change — but the runner
does not know the app role's name and would need a new environment variable to
learn it, whose absence would silently skip the grant. Introducing a new silent
skip to fix a silent skip is the wrong trade. Reconsider if that role name ever
becomes available to the runner for another reason.
