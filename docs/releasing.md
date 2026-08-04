# Releasing to production

The procedure for cutting a platform release and promoting it to prod.

This exists because the v1.3.0 release (TRA-1046) ran off a checklist that lived
in a single Linear ticket description, so the next release would have started
from a blank page. At a weeks-apart cadence that is tolerable; the plan is to
move to days, where it guarantees drift (TRA-1085).

Two of the mistakes that release hit are now enforced rather than documented —
see [What the guards enforce](#what-the-guards-enforce). The rest is here.

---

## 0. Before you start

- [ ] **Know what you are shipping.** `git log --oneline v<previous>..main` and
      the set of migrations added since the last release:
      `ls backend/migrations/ | tail -20`.
- [ ] **Confirm the CNPG backup to GCS is current for prod, and take a fresh
      verified one.** This is the only real undo. `just ops db-status prod`.
- [ ] **Read prod's current state rather than trusting a recent note:**

      ```bash
      curl -s https://app.trakrf.id/health | jq '{version, commit, built}'
      ```

- [ ] **Read prod's migration ledger, immediately before you migrate** — not
      days earlier:

      ```sql
      -- via: just psql prod
      SELECT schemaname FROM pg_tables WHERE tablename = 'schema_migrations';
      SELECT version, dirty FROM trakrf.schema_migrations;
      ```

      Expect exactly one row from the first query. If it says `public`, you are
      on a pre-TRA-1069 database and step 4 applies in full.

- [ ] **Cut the changelog section.** `CHANGELOG.md` gets a real `[X.Y.Z]`
      section, and anything under `[Unreleased]` that is actually shipping moves
      into it. Nothing enforces this yet (TRA-1085 item 4 is unbuilt), and it
      had drifted so far by v1.3.0 that no release before it had a section at
      all.

---

## 1. Tag first, then let the build run

**Order matters, and it is the opposite of the instinct.**

`APP_VERSION` is `git describe --tags` evaluated **at image build time**, and
promoting to prod is a pure manifest re-tag with no rebuild. So merging and
*then* tagging bakes a dev-shaped version — `v1.2.0-559-gaa9822bb` — into the
image permanently. Tagging afterwards changes nothing, because nothing rebuilds.
`/health` would report that string for the entire life of the release.

```bash
git checkout main && git pull
git tag v1.3.0
git push origin v1.3.0
```

Pushing the tag triggers **Docker Build and Push** on `refs/tags/v1.3.0`
(TRA-1085 added the `v*` trigger). That build re-resolves `git describe` against
the tag itself and republishes the same immutable `sha-<short>` tag with a clean
`v1.3.0` version.

Wait for it to finish before promoting. Neither floating tag is affected —
`latest` is gated on the default branch and `preview` on `refs/heads/preview`,
both false on a tag.

> Before TRA-1085 the recovery for getting this order wrong was to tag and then
> manually **re-run** the main build so `describe` re-resolved. That worked, but
> it was folklore. You should not need it now; if you do, it still works.

---

## 2. Promote the image

Run the **Promote to prod** workflow, with `source` set to the release tag:

```bash
gh workflow run promote-prod.yml -f source=v1.3.0
```

`source` accepts a release tag, a branch, or a full SHA, and resolves the image
tag itself. Leave it empty to promote current `main`. You no longer need to know
that `git rev-parse --short` returns 8 characters while the registry publishes
7 — the mismatch that failed the first v1.3.0 promotion outright.

The workflow refuses, before touching `:prod`, if:

| Refusal | Meaning |
|---|---|
| `Cannot resolve '<ref>' to a commit` | Typo, or a ref that does not exist |
| `not an ancestor of origin/main` | Not a main-derived commit. `preview` is a force-rewritten PR composition and must never reach prod |
| `<image>: not found` | The image for that commit was never built, or the build has not finished yet |
| `Refusing to promote an image whose version is '<x>'` | The image was built **before** the release tag existed. Go back to step 1 |

An empty version in that last message means the image predates the
`id.trakrf.app-version` label — rebuild it from a tagged commit.

---

## 3. Watch the promote land — and do not promote twice

ArgoCD Image Updater polls; it does not get pushed to. On v1.3.0 it picked up
the new digest **93 seconds** after the promote, because the promote landed 31
seconds after a poll cycle had completed.

**Expect up to ~2 minutes of nothing happening.** During that window a
successful promote looks exactly like one that did nothing, which invites a
second, unnecessary promote. Don't.

```bash
just ops argo-status
just ops rollout prod          # shows the image the deployment is actually on
```

---

## 4. Migrations — the ledger relocation, if it applies

**Only for a database whose ledger is still in `public`** (step 0's query). Prod
was in this state for v1.3.0. Once relocated, it stays relocated.

`./server migrate` pins its `schema_migrations` ledger to the `trakrf` schema
and **refuses to run** if it finds one anywhere else (TRA-1069).

### Do not relocate first

The instinct is to move the ledger ahead of the deploy. That is backwards.
Currently-running prod code resolves the ledger via `CURRENT_SCHEMA()` and finds
`public.schema_migrations`. Move it early and the old code sees no ledger,
creates an empty one in `public`, and replays from `000001`.

1. **Deploy the new image.** The Helm `pre-upgrade` hook
   `trakrf-backend-migrate` fails the preflight. **This is expected.** The
   upgrade stops there, the old backend pod keeps serving, and the database is
   untouched.

2. **Capture the Job log immediately.**

   ```bash
   kubectl logs job/trakrf-backend-migrate -n trakrf-prod
   ```

   The pod is deleted on both success and failure and is reaped within seconds.
   The refusal names both ledgers and their versions, and names the exact
   `ALTER TABLE` remedy (TRA-1084) — worth having in the release record. On
   preview it was reaped before anyone read it and the cause had to be inferred.

3. **Relocate the ledger.** Metadata-only, instant, reversible
   (`SET SCHEMA public` puts it back), and it preserves both the version and the
   dirty flag:

   ```sql
   ALTER TABLE public.schema_migrations SET SCHEMA trakrf;
   ```

4. **Delete the failed Job, then re-sync.** A plain re-sync will **not**
   recreate the hook while the failed Job object still exists.

   ```bash
   kubectl delete job trakrf-backend-migrate -n trakrf-prod
   just ops argo-sync trakrf-backend-prod
   ```

Failing safe is the design here: forgetting the relocation costs a failed Job,
not a corrupted schema.

---

## 5. Post-deploy

- [ ] **Ledger check** — exactly one `schema_migrations`, in `trakrf`, at the
      expected version and not dirty:

      ```sql
      SELECT schemaname FROM pg_tables WHERE tablename = 'schema_migrations';
      SELECT version, dirty FROM trakrf.schema_migrations;
      ```

- [ ] **Refresh the `asset_scan_latest` continuous aggregate.** Belt and
      braces, **not a gate** — the 30-second refresh policy from `000028`
      usually populates it before you get here (it had already done all 213 rows
      on v1.3.0). It will matter at larger volume. Until it is populated the
      asset-locations report reads an empty aggregate.

      ```sql
      CALL refresh_continuous_aggregate('trakrf.asset_scan_latest', NULL, NULL);
      ```

- [ ] **`/health` reports the new version** — a clean `vX.Y.Z`, matching the tag:

      ```bash
      curl -s https://app.trakrf.id/health | jq '{version, commit, built}'
      ```

- [ ] **Spot-check the real customers.** Confirm base asset management is
      untouched for each — that is the surface no release is allowed to disturb.

- [ ] **Capability grants, if this release adds a capability.** There is no
      backfill: every org starts at zero grants (ADR 0002), so a new capability
      ships dark until granted explicitly. Decide per-org and do it as part of
      the release, not after it.

- [ ] **Move the Linear tickets and publish the release notes.**

---

## 6. Rollback

Promotion is a manifest re-tag, so rolling back is the same operation aimed at
the previous release tag:

```bash
gh workflow run promote-prod.yml -f source=v1.2.0
```

**Migrations do not roll back with it.** A schema change already applied stays
applied, and the previous image has to be able to run against it. If a release
contains a migration the previous version cannot tolerate, the rollback path is
fix-forward, not re-promote — decide which of the two you are in *before* you
need it.

Never repair drift by editing an applied migration: fix forward in the next
numbered migration. Applied migrations are immutable and CI enforces it
(TRA-1077 — adding one requires `just backend migrate-checksums`).

---

## What the guards enforce

Two of the v1.3.0 mistakes are now unshippable rather than documented
(TRA-1085):

- **`promote-prod` refuses an image whose version is not a clean `vX.Y.Z`.** The
  version is read back off the registry from the `id.trakrf.app-version` OCI
  label stamped by the Dockerfile. This is what makes step 1's ordering
  self-enforcing.
- **`promote-prod` resolves its source from a git ref**, so the 7-vs-8 character
  short-SHA convention is no longer knowledge the operator has to carry.

Both live in `scripts/` with tests (`just test-release-guards`) rather than
inline in workflow YAML, because this is exactly the class of logic that was
wrong in production.

Still **not** enforced: nothing fails a release tag that has no matching
`CHANGELOG.md` section (TRA-1085 item 4).

## A note on prod-mutating ops recipes

Infra's `just ops` recipes that mutate prod prompt first — you type the
environment name to confirm. Non-interactive shells are refused outright rather
than prompted.

`YES=1` skips the prompt, and is the only way to run them from a script:

```bash
YES=1 just ops backend-restart prod
```

Use it when you mean it. It is not a general-purpose quiet flag; it is the
recorded decision that you intend to mutate production.

## Related

- `docs/adr/` — the decisions behind the version scheme and capability model
- `CHANGELOG.md` — per-release notes
- `.github/workflows/docker-build.yml`, `.github/workflows/promote-prod.yml`
- Cluster, namespace and CNPG specifics live in **trakrf/infra**, reachable from
  here via `just ops` — never reimplement a kubectl incantation in this repo
