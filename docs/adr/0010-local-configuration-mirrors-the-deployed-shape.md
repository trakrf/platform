# ADR 0010 — Local configuration mirrors the deployed shape, and states each fact once

Date: 2026-08-29
Status: Accepted
Tracking: TRA-1190 (this record), TRA-1075 (the role split that drifted), TRA-1069 / TRA-1104 (ADR 0003, the migration half)

## Context

Local dev and the cluster are supposed to be the same stack. When they are not,
the difference does not announce itself — it waits, and then presents as a defect
in whatever ran next.

Two changes landed in the cluster and never came back to local dev:

* the **`trakrf-migrate` / `trakrf-app` split** (TRA-1075), which local dev
  received in its *template* but not in any developer's actual `.env.local`
* **composing connection URLs from parts**, which arrived with the move from
  Timescale Cloud to GKE/CNPG. `helm/trakrf-backend/templates/migrate-job.yaml`
  interpolates `postgresql://$(PGUSER):$(PGPASSWORD)@host:port/db?sslmode=…` from
  a role name and a Secret, and stores no URL anywhere. Local dev kept four
  hand-written DSNs.

Neither omission was noticed, because local dev was "more or less working."

What it cost (TRA-1190): `PG_URL_MIGRATE_LOCAL` was added to
`.env.local.example` by the role split and reached no actual `.env.local`, so
`just backend migrate` could not run on any developer machine. It failed as
`PG_URL environment variable not set` — a message about the environment in
general, two layers from the one missing variable — because just's
`env(…, "")` turns a missing variable into a *successful* lookup of an empty
string. A stale database therefore went unmigrated while a correct one sat
beside it, created by `just database up` and used by nothing.

The result was a backend serving a schema two migrations behind. `/health`
returned 200 and signup returned 201, because neither touched the new column.
Only login failed. Every cheap check passed and the expensive one failed in a way
that looked like the test's fault: **89 e2e failures, every one at exactly
11.3s**, triaged as fixture rot and carried into a commit message as fact.

The database was not really the problem. The same afternoon produced two more
unmet preconditions with the same signature — no backend running, and a dev
server started without bridge mode — each failing inside an assertion that named
something else. A uniform failure duration was the only tell, and it only reads
as one if you already suspect a single shared cause.

The deeper fault was arithmetic: **the same fact was written down in five places
and nothing compared them.** `.env.example`, `.env.local.example`,
`database/justfile`, `backend/justfile` and the README each stated which database
local dev uses, and three of them disagreed. Each role password was written
twice, raw and URL-encoded, with a comment asking the reader to keep the copies
in step.

## Decision

**Local configuration takes the deployed shape, states each fact once, and every
remaining copy is derived from that statement or checked against it.**

1. **Parts, not DSNs.** `.env.local` declares `PG_HOST`, `PG_PORT`,
   `PG_DATABASE`, `PG_APP_USER`, `PG_MIGRATE_USER` and their passwords.
   `docker-compose.yaml` and the justfiles interpolate the connection strings, as
   the Helm templates do. A DSN that exists as editable text is a DSN that can
   drift; there is now no such text. Each password appears once.

2. **One env file.** `.env.local` is it. `.env` is a symlink to it, created by
   `just bootstrap` and asserted by a check — docker compose reads `.env` and
   direnv reads `.env.local`, and as two real files they can name two different
   databases, with the shell environment silently outranking `.env` so the
   disagreement does not even present the same way twice. `.env.example` is
   deleted; a second template is a second truth.

3. **`database/justfile` declares the database and role names.** The drift check
   *parses* them out of it rather than restating them, so the check cannot become
   a third opinion that itself needs maintaining.

4. **Defaults are the canonical local values, never empty.** A default that is
   the documented shape lets a fresh clone come up correctly; a default of `""`
   converts a missing variable into a successful lookup of nothing, which is the
   mechanism that hid this for months. Where a value genuinely must be supplied,
   fail naming it — `${VAR:?…}` in compose, an explicit guard in the justfiles.

5. **Deliberate divergences are stated where they diverge.** Deployed carries
   `search_path` on the connection URL; local sets it on the database
   (ADR 0003). Edge quadlets keep literal DSNs because `EnvironmentFile` cannot
   interpolate. Both are checked against the same declaration rather than
   exempted from it.

**A corollary, for the class of bug rather than this instance: a component that
knows a precondition is unmet must say so itself, and name it.** The backend now
compares the migration set embedded in its binary against the version the
database has applied. Behind, it refuses to look healthy. The suite asserts the
same before it runs a spec.

## Consequences

`just backend migrate` works, and says which database, host and role it is
migrating — so migrating the wrong one is no longer silent success.

`just dev` migrates before the backend serves, matching the
`pre-install,pre-upgrade` migrate Job hook. The old order — backend first, then
migrations — is the window this ticket lived in.

**`/health` returns 503 when the schema is behind, naming the unapplied
migrations. `/healthz` and `/readyz` deliberately do not follow it.** They are the
k8s liveness and readiness probes, and the repair for a behind schema is a
migration: a pod that has been killed or pulled from the load balancer cannot
serve while that runs. A pod *ahead* of its schema is a normal rolling deploy, not
a fault, and must stay healthy or every deploy becomes an outage. Whoever adds a
schema condition to a probe should read this paragraph first — nothing else will
stop them.

A `.env.local` written before this record now fails loudly rather than being
quietly ignored: a retired `PG_URL*` is refused by name, because a customised
value that silently stops applying is the same defect in a new costume.

The alignment is a standing obligation, not a one-off repair. It reaches into
`trakrf/infra`, so no check in this repository can express the whole of it —
which is why it is recorded here rather than left to the checks. **When the
cluster's shape changes, local dev is part of that change**, and the interval
between the two is measured in undetectable failures rather than in days.

## Notes on scope

This record does not govern *what* the values are — `database/justfile` and the
Helm values do. It governs where a fact may be stated and what must be true of
every copy.

It does not extend to secrets. Local passwords are deliberately well-known so a
fresh clone works; deployed credentials come from Secrets and CNPG-managed roles
and are not in scope here.

Rejected: keeping `.env` and `.env.example` and requiring them to agree, enforced
by a check. It was the smaller diff and the ticket's literal wording. It leaves
two files both defining `PG_URL` with the shell silently outranking one of them,
so it preserves the exact ambiguity that made the original failure so hard to
see — a check over a structure that should not exist, rather than removing the
structure.

Rejected: pointing docker compose at `.env.local` with `--env-file` instead of a
symlink. It only covers compose invoked through `just`; a bare `docker compose`
in a non-direnv shell gets nothing.
