# Releasing to production

The procedure for cutting a platform release and promoting it to prod.

This exists because the v1.3.0 release (TRA-1046) ran off a checklist that lived
in a single Linear ticket description, so the next release would have started
from a blank page. At a weeks-apart cadence that is tolerable; the plan is to
move to days, where it guarantees drift (TRA-1085).

The mistakes those releases hit are now enforced rather than documented — see
[What the guards enforce](#what-the-guards-enforce). The rest is here.

The version itself is declared in the root `VERSION` file rather than derived
from `git describe`, which is what removed the ordering hazard that broke
v1.3.0 and v1.4.0 (TRA-1126, `docs/adr/0004-declared-platform-version.md`).

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
      -- via: just psql prod  (interactive session; paste both statements)
      SELECT schemaname FROM pg_tables WHERE tablename = 'schema_migrations';
      SELECT version, dirty FROM trakrf.schema_migrations;
      ```

      Expect exactly one row from the first query. If it says `public`, you are
      on a pre-TRA-1069 database and step 4 applies in full.

      > ### ⚠️ TEMPORARY WORKAROUND — DELETE THIS BLOCK WHEN TRA-1105 LANDS
      >
      > `just psql ENV` takes no query argument — it only opens an interactive
      > shell, so the statements above cannot be run non-interactively as
      > written (`just psql prod -c "…"` fails with ``Justfile does not contain
      > recipe `-c` ``). **TRA-1105 adds `just psql ENV [QUERY]` and drops the
      > superuser default.** Until it lands, a scripted read has to go through
      > the pod directly, which connects as **superuser**:
      >
      > ```bash
      > kubectl -n trakrf-prod exec trakrf-db-prod-1 -- \
      >   psql -U postgres -d trakrf -c "SELECT version, dirty FROM trakrf.schema_migrations;"
      > ```
      >
      > Read-only checks only. **Never run DDL through it** — superuser-owned
      > objects are permanently un-replaceable by the migrate role and wedge a
      > later deploy. That is the whole reason TRA-1105 exists, so leaving this
      > block in place after the fix ships actively undermines it.
      >
      > **On TRA-1105 landing:** delete this block and change the `-- via:`
      > comment above to the working `just psql prod "<query>"` form.

- [ ] **Draft the changelog section.** `CHANGELOG.md` gets a real `[X.Y.Z]`
      section, and anything under `[Unreleased]` that is actually shipping moves
      into it. This is **enforced** as of TRA-1126 — `lint-test` fails a PR that
      flips `VERSION` to a clean `X.Y.Z` without the matching `## [X.Y.Z]`
      heading (TRA-1085 item 4). Run it locally with `just check-changelog`.

---

## 1. Open the release PR

**Releasing is a diff.** The platform version is declared in the root `VERSION`
file, and a release is the one-line change that flips it from `X.Y.Z-dev` to
`X.Y.Z`. There is no tag to push and no ordering to get right: **the merge build
IS the release build**, and CI produces the git tag as an *output* of it.

Substitute the version you are cutting throughout — `cat VERSION` tells you
which one that is, minus the `-dev`.

```bash
git checkout main && git pull
cat VERSION                             # e.g. X.Y.Z-dev -> you are cutting X.Y.Z
git checkout -b release/X.Y.Z
printf 'X.Y.Z\n' > VERSION
# move the shipping items from [Unreleased] into a new ## [X.Y.Z] - YYYY-MM-DD
$EDITOR CHANGELOG.md
just check-changelog                    # the gate CI will run
git commit -am "release: X.Y.Z"
gh pr create --base main --title "release: X.Y.Z"
```

Pasting the block verbatim fails loudly rather than quietly: `X.Y.Z` is not
semver, so `just check-changelog` and the image build both refuse it.

The PR runs the four required checks — `build`, `lint-test`, `api-spec` and
`main contract-tests must be green`. `lint-test` carries the changelog gate.

**On merge**, the `release` job in `docker-build.yml`:

1. reads `VERSION`, sees a clean `X.Y.Z`,
2. creates the git tag `vX.Y.Z` at the merge commit,
3. publishes `ghcr.io/trakrf/backend:vX.Y.Z` from the manifest that build just
   pushed,
4. opens a follow-up PR bumping `VERSION` to the next minor `-dev`.

All of it is idempotent. A re-run finds the tag already at that commit and
republishes the image tag only, which is also how a run that minted the tag and
then died repairs itself.

Merging deploys nothing. Step 2 is still a separate, manual act.

> **Why this replaces "tag first, then let the build run".** `APP_VERSION` used
> to be `git describe` evaluated at build time, so merging and *then* tagging
> baked a dev-shaped version into the image permanently (v1.3.0), and tagging
> *first* produced two builds of one commit racing on the shared `sha-<short>`
> tag (v1.4.0, TRA-1125). Both failure modes were properties of deriving the
> version from ref topology. It is now declared in the commit, so two builds of
> one commit produce byte-identical version metadata and nothing has to
> serialize. See `docs/adr/0004-declared-platform-version.md`.

### Merge the bump-back promptly

Until the bump-back PR lands, `main` still declares a clean `X.Y.Z`, so every
ordinary merge builds an image carrying a clean `vX.Y.Z` label. Those images are
**not** promotable — `assert-release-commit.sh` requires the promoted image to
be the exact commit `vX.Y.Z` names — but the window is untidy and the release
job logs a `stale` warning on every merge inside it. Merge the bump-back.

### Hotfixes

A patch on a shipped release branches from its tag rather than from main, and
sets `VERSION` to the next patch of that line:

```bash
git checkout -b release/X.Y.x vX.Y.Z
printf 'X.Y.<Z+1>\n' > VERSION
```

`resolve-promote-source.sh` currently requires the source be an ancestor of
`origin/main`, which a `release/*` head is not; the rule wants restating as
*promotable = ancestor of main or of a `release/*` head*, and the auto-tag
trigger wants extending to `release/*`. Decided, not built — this is the shape
to build at the first real hotfix, not a thing to improvise under pressure.

### If a release is reverted

A revert does not remove the release commit from main's ancestry, so the tag
and the image stay valid and promote still accepts them — correctly; the
artifact is immutable history. **A reverted release is never promoted.** The fix
ships as the next patch version.

---

## 2. Promote the image

Run the **Promote to prod** workflow, with `source` set to the release tag:

```bash
gh workflow run promote-prod.yml -f source=vX.Y.Z
```

`source` resolves a release tag, a branch, a full SHA or a `sha-xxxxxxx` image
tag to an image, but only the release commit itself passes the guards. You no
longer need to know that `git rev-parse --short` returns 8 characters while the
registry publishes 7 — the mismatch that failed the first v1.3.0 promotion
outright.

The workflow refuses, before touching `:prod`, if:

| Refusal | Meaning |
|---|---|
| `Cannot resolve '<ref>' to a commit` | Typo, or a ref that does not exist |
| `not an ancestor of origin/main` | Not a main-derived commit. `preview` is a force-rewritten PR composition and must never reach prod. As of TRA-1126 a raw `sha-xxxxxxx` input gets this check too |
| `<image>: not found` | The image for that commit was never built, or the build has not finished yet |
| `Refusing to promote an image whose version is '<x>'` | Not a release build. `VERSION` carries `-dev` outside a release, so this is a preview or in-development image |
| `there is no git tag <vX.Y.Z>` | The image was built while `VERSION` was clean but is not the release commit — see the bump-back window in step 1 |
| `it is not the <vX.Y.Z> release commit` | Same window, tag present. Promote the release tag itself rather than a later main commit |
| `Cannot bind 'latest' to a commit` | `latest` names an image and no commit, so it cannot be checked against the release tag. Name the tag |

An empty version in the "whose version is" message means the image predates the
`id.trakrf.app-version` label — rebuild it from a commit that declares a release
`VERSION`.

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

- [ ] **All three version surfaces report the new version** — a clean `vX.Y.Z`,
      matching the tag. They share one source (`Dockerfile` stage 0 writes
      `/version` once; the Go `-ldflags` and Vite's `VITE_APP_VERSION` both read
      it), so they cannot disagree *within* an image — but a **stale cached
      frontend bundle** can still serve the old UI against a new backend, and
      `/health` alone cannot see that.

      ```bash
      curl -s https://app.trakrf.id/health       | jq '{version, commit, built}'
      curl -s https://app.trakrf.id/version.json
      ```

      Third surface is the UI itself: the version sits in the **sidebar header**
      under "Handheld Tag Reader" (`TabNavigation.tsx:174`) and on Settings
      (`SettingsScreen.tsx:352`, `:440`). The sidebar is collapsed by default —
      open the menu or you will find no version string and think it is missing.
      A UI/`version.json` mismatch means a cached bundle, not a build problem.

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
gh workflow run promote-prod.yml -f source=v<previous>
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

Every mistake that has bitten a release is now unshippable rather than
documented (TRA-1085, TRA-1126):

- **The version cannot be derived, injected, or disagree with itself.** The
  Dockerfile `COPY`s `VERSION` and derives the version from it. A build that
  passes an `APP_VERSION` disagreeing with the file **fails**. Both arches of a
  multi-arch build read the same file, so they cannot diverge.
- **`promote-prod` refuses an image whose version is not a clean `vX.Y.Z`.** The
  version is read back off the registry from the `id.trakrf.app-version` OCI
  label. `VERSION` carries `-dev` outside a release, so no preview and no
  in-development main build can pass.
- **`promote-prod` refuses a clean image that is not the release commit.** The
  git tag is minted once, at one commit; the promoted image must be that commit.
  This is what makes the bump-back window safe rather than merely short.
- **`promote-prod` resolves its source from a git ref**, so the 7-vs-8 character
  short-SHA convention is no longer knowledge the operator has to carry — and a
  raw `sha-xxxxxxx` input is ancestry-checked like any other ref rather than
  waved through.
- **A release cannot be cut without its `CHANGELOG.md` section.** `lint-test`
  fails a PR that flips `VERSION` clean without the matching `## [X.Y.Z]`
  heading (TRA-1085 item 4, built under TRA-1126).
- **A version cannot be released twice.** The release job refuses to re-point a
  tag; a `VERSION` that names an already-released version mints nothing.

All of it lives in `scripts/` with tests (`just test-release-guards`) rather
than inline in workflow YAML, because this is exactly the class of logic that
was wrong in production.

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
