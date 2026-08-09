# ADR 0004 — The platform version is declared in the commit; the git tag is an output, not a driver

Date: 2026-08-09
Status: Proposed
Tracking: TRA-1126 (this change), TRA-485 (the describe-based mechanism this replaces), TRA-1085 (the guards it builds on), TRA-1125 (the race it dissolves), TRA-1046 / TRA-1114 (the two releases that hit it)

Supersedes in part [ADR 0001](0001-platform-vs-api-versioning.md). The
three-axis separation 0001 records — platform version, API contract version,
OpenAPI `info.version` — stands unchanged. Only the *source* of the platform
version changes.

## Context

ADR 0001 made the platform version the output of
`git describe --tags --always --dirty`, evaluated at image-build time. That was
a reasonable choice at the time and it removed a real problem (three unrelated
version numbers, one of them a hardcoded literal). It also had one property
nothing else offered: a clean `vX.Y.Z` was **unforgeable**, because producing
one required a real tag at that commit. You could not get there by editing
files.

It broke three releases in a row, each time in a different place:

* **v1.3.0** — merged, then tagged. `describe` had already run, so
  `v1.2.0-559-gaa9822bb` was baked into the image permanently and `/health`
  would have reported it for the life of the release. Recovered by tagging and
  re-running the build. Became TRA-1085.
* **v1.4.0** — followed the corrected runbook exactly (tag first) and the
  promote *still* refused. `main` and `refs/tags/v1.4.0` both built commit
  `85e46284`, both wrote the shared `sha-<short>` image tag, and the
  concurrency group is keyed on `github.ref` so they did not serialize. Main
  finished 52 seconds later and overwrote the clean version. Became TRA-1125.
* **The display need** — preview should visibly read a different release line
  from prod, so a non-technical viewer can tell them apart. Two attempts to
  express that with a `v1.5.0-pre` tag failed under test: two tags on one commit
  resolved to the wrong one, and a hotfix tag beats a pre tag on commit distance
  (`v1.4.1-1-g…` wins over `v1.5.0-pre`).

These look like three bugs. They are one.

`git describe` answers **"what release is this build descended from"** — a
statement about topology. What both the release path and the display need is
**"which release line is this"** — a declaration. Topology cannot express a
declaration, so every attempt to make it do so produced a new edge case, and
each fix moved the failure rather than removing it.

The concrete symptom is that `APP_VERSION` was a property of **the ref that
built the image**, not of the commit. Two builds of one commit could therefore
disagree, and `promote-prod` never rebuilds, so whichever build finished last
decided what prod would report.

## Decision

**The platform version is declared in a root `VERSION` file.** It holds bare
semver — `1.5.0-dev` during development, flipped to `1.5.0` by a release PR.
`git describe` is removed from the image entirely.

1. **The Dockerfile `COPY`s `VERSION`** into the `build-meta` stage and derives
   the version there. It is not computed by the workflow and injected. This is
   the load-bearing detail: an injected value is a property of the build
   invocation, and any caller — a re-run, Railway, a local `docker build` —
   could supply a different one. A `COPY`'d file is a property of the commit by
   construction, which also makes both arches of a multi-arch build agree
   without a cross-check. The `APP_VERSION` build-arg survives only because an
   OCI `LABEL` cannot read a file; `build-meta` **fails the build** if the
   passed value disagrees with the declared one.

2. **The merge build is the release build.** No second build, no ordering, no
   re-run.

3. **The git tag is an output.** On push to `main`, if `VERSION` is a clean
   semver and no tag exists for it, CI creates `vX.Y.Z` at that commit *and*
   publishes `ghcr.io/trakrf/backend:vX.Y.Z` — written once, by one job. That is
   the immutability `sha-<short>` was wrongly documented as having.

4. **`docker-build.yml` no longer triggers on tags.** Tag builds existed only to
   re-resolve `describe`; keeping the trigger would reintroduce the second build
   that broke v1.4.0.

5. **`git describe` does not survive as a provenance field.** It is tempting to
   keep it as a `build` field alongside the version, but describe output for a
   fixed commit *changes when tags are created* — a re-run after CI mints
   `v1.5.0` describes differently from the merge build. That would quietly break
   the byte-identical property this ADR exists to establish. The commit SHA
   carries the same information and is stable; anyone can run `git describe`
   after the fact.

### Replacing the unforgeability property

Moving the version into a file moves the hardness that made a clean `vX.Y.Z`
unforgeable, and that had to be replaced deliberately rather than assumed away.

**This is an accident-prevention problem, not a security one.** Push access is
Mike and Nick, with Peter joining — all trusted, so there is no adversary to
model. The guard exists to stop a *mistake* becoming a bad prod image, which is
exactly what v1.3.0 and v1.4.0 were. The accident surface grows with headcount
even though the trust level does not: a third contributor merging ordinary work
should not need to carry the release machinery in their head.

Three things replace it:

* **The `-dev` suffix does most of the work.** During development nothing
  matches `^v[0-9]+\.[0-9]+\.[0-9]+$`, so no preview and no in-development main
  build is promotable. Releasing becomes a distinct, legible act — one line in a
  reviewed diff — rather than something adjacent to an ordinary merge.
* **A release now goes through review and CI.** Verified 2026-08-09: the
  `main-branch-protection` ruleset requires a PR and the checks `build`,
  `lint-test`, `api-spec` and `main contract-tests must be green` — and does
  **not** cover tag pushes at all. So the old release act (`git push origin
  vX.Y.Z`) got no PR, no diff and no checks, while a `VERSION` bump gets all
  four. Same two hands; strictly more chances for CI to catch a slip.
* **`assert-release-commit.sh` restores the exact-commit binding.** A clean
  `vX.Y.Z` label is only promotable from the commit its git tag names. This is
  what the tag used to provide implicitly.

That last guard closes a hole the design opens. Between the release merge and
the bump-back landing, `main` still declares a clean `1.5.0`, so **every**
ordinary merge in that window builds an image carrying a clean `v1.5.0` label —
including whatever `promote-prod` resolves from an empty `source`. A follow-up
bump-back PR narrows that window; it cannot close it. The commit binding closes
it, and makes the bump-back purely cosmetic.

A second hole: while a release PR is *open*, the `preview` composition contains
`VERSION = 1.5.0`, so the preview image would carry a clean label on a commit
that is not and never will be the release commit. Two fixes, both applied — a
`-preview+419+420` suffix keeps preview builds unclean (which also delivers the
display need directly), and the `sha-<hex>` promote input is now
ancestry-checked instead of being waved through. `sha-<hex>` *is* the git short
SHA; the old script's claim that it "cannot be ancestry-checked" was simply
wrong.

## Consequences

* **The race stops being observable rather than being serialized.** Two builds
  of one commit now produce byte-identical version metadata, so overwriting
  `sha-<short>` is idempotent. Nothing has to coordinate. TRA-1125's narrow fix
  (serialize the two builds) is unnecessary and that ticket is superseded.
* **Releasing is a diff.** `docs/releasing.md` loses roughly 60 lines of
  ordering folklore — steps 1 and 1.5 are deleted rather than rewritten.
* **The changelog gate becomes possible.** TRA-1085 item 4 had no natural place
  to hang while the release act produced no diff. It now runs inside `lint-test`
  as a condition on the release PR, needing no new required-check context.
* **A local or Railway build gets an empty OCI label** and is therefore
  unpromotable. Correct and fail-closed: promotable images come from CI. Such
  builds still report the declared version at `/health`, which is an improvement
  — Railway used to show a branch name.
* **Two `-dev` numbers now exist transiently**: main's, and any release branch's.
  This is the cost of a declaration and it is visible in the diff, which is the
  point.
* **The bump-back PR needs a non-default token.** PRs opened with the default
  `GITHUB_TOKEN` do not trigger workflows, so the four required checks would
  never report and the PR would sit permanently blocked. It uses the existing
  `trakrf-preview-sync` GitHub App token, the same anti-recursion workaround
  `sync-preview.yml` already documents.
* **Hotfixes need one rule restated.** `resolve-promote-source.sh` requires the
  source be an ancestor of `origin/main`, which a `release/1.4.x` head is not.
  The rule wants restating by its actual purpose — *promotable = ancestor of
  main or of a `release/*` head* — and the auto-tag trigger extending to
  `release/*`. Decided and documented; built at the first real hotfix rather
  than speculatively.
* **`openapi.yaml` `info.version` and `frontend/package.json` `version` stay
  exactly where ADR 0001 and TRA-672 / TRA-485 left them.** Nothing here
  re-couples them.

## Alternatives considered

* **Serialize the two builds (TRA-1125's original proposal).** Share a
  concurrency group across the branch and tag refs so the tag build always wins.
  Fixes the v1.4.0 symptom and nothing else: the version remains a property of
  the ref, the v1.3.0 ordering hazard remains, and the display need remains
  unexpressible. It is a patch on one of three faces of the same fault.
* **Keep `git describe` as a `build` provenance field.** Rejected in decision 5
  above: describe output for a fixed commit changes when tags are created, so
  the field would violate the byte-identical property.
* **Use `frontend/package.json` `version` as the source.** Explicitly demoted by
  TRA-485 after it drifted to `1.0.18` while the backend reported
  `0.1.0-preview`. Re-adopting it would repeat that.
* **A deliberate second act to mint the artifact** (manual workflow dispatch
  after the merge) rather than auto-tag-on-merge. Rejected: merging the release
  PR is already the deliberate act — a reviewed diff behind four required
  checks — and a second manual step reintroduces exactly the forgotten-step and
  ordering error class this ADR removes. Note the stakes are lower than they
  sound: merging to main deploys nothing. `promote-prod` remains a separate,
  manual, human-triggered step and this ADR does not move it. Auto-tag decides
  only when a *promotable artifact* is minted, not when prod changes.
* **`-rc.N` or `-pre` instead of `-dev`.** Nothing in the system *orders*
  versions — both guards are pure regex matches — so ordering semantics buy
  nothing today. `-rc.N` becomes worth having if a staged-release environment
  ever exists.
* **release-please / semantic-release.** Same rejection as ADR 0001: it solves a
  multi-contributor bump-judgment coordination problem this team does not have.
  Revisit at 5+ engineers.
