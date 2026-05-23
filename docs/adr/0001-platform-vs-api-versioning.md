# ADR 0001 — Platform version, API contract version, and OpenAPI spec version are three independent numbers

Date: 2026-05-23
Status: Accepted
Tracking: TRA-485 (mechanism), TRA-672 (`info.version` semantics), TRA-481 (`/health` SHA exposure)

## Context

The TrakRF monolith historically carried three different version numbers, none of
them synchronised:

* `frontend/package.json` `version` — `1.0.18`, climbed for legacy reasons
  inherited from the standalone `trakrf-handheld` repo, surfaced as `v1.0.18`
  in the nav header.
* Backend `main.version` — hardcoded `0.1.0-preview` in the root Dockerfile and
  `0.1.0-dev` in `backend/justfile`, surfaced at `/health`.
* OpenAPI `info.version` in the published spec — set to `1.0.0` per the Zalando
  must-use-semantic-versioning convention (TRA-672), bumped manually when the
  spec ships a breaking change.

The first two were nominally describing the same artifact — the monolith ships
the frontend embedded in the backend binary, so any commit produces exactly one
deployable. Two numbers a whole major apart was incoherent. The third was
describing something else entirely and was correctly decoupled, but the prior
state made that easy to forget.

The URL path version `/api/v1/` is a fourth axis but is not a number — it is a
long-lived URL contract that flips to `/api/v2/` only on a real breaking change
to the customer-facing API surface. It is mentioned here only to clarify it is
not the same as `info.version`.

## Decision

The monolith has **one platform version**. It is the output of
`git describe --tags --always --dirty` at image-build time, injected once and
fed into every surface that needs to name the running build:

* Go binary — via `-X main.version` ldflags, reported at `/health`.
* Frontend bundle — via `VITE_APP_VERSION`, rendered in the nav header and the
  Settings screen.
* `dist/version.json` — emitted by the Vite plugin so `curl host/version.json`
  matches `/health`.

The CI workflow (`.github/workflows/docker-build.yml`) computes
`git describe` once and passes it as the `APP_VERSION` build-arg to the
Dockerfile. The Dockerfile's build-meta stage applies a precedence chain
(`APP_VERSION` > `BUILD_TAG` > `RAILWAY_GIT_BRANCH` > `"dev"`) so a Railway
build — which cannot run `git describe` in its build sandbox — still produces
a meaningful, if less specific, value (the ref name) instead of a stale
hardcoded literal.

`frontend/package.json` `version` is demoted to `0.0.0` and is no longer the
source of any user-visible string. It exists only because pnpm requires a
SemVer there; the package is `private: true` and never publishes.

**OpenAPI `info.version` keeps its existing semantics** (TRA-672): it is the
spec version, bumped manually on breaking spec changes, currently `1.0.0`.
It is intentionally *not* coupled to the platform version. Conflating the two
would couple every backend release to a spec-consumer breaking-change signal
and is rejected. This ADR records that separation explicitly so future
contributors do not "fix" the apparent inconsistency by re-coupling them.

**API contract version** (`/api/v1/`) is similarly unchanged and similarly
decoupled. Platform can ship v1.x → v2.x → v3.x without touching `/api/v1/`;
`/api/v1/` flips to `/api/v2/` only on a customer-visible breaking API change.

## Consequences

* **Trip-test for AI ingestion partners:** `/health` and `info.version` are
  now both honest. `/health` reports a non-stale, machine-readable platform
  version. `info.version` reports a spec semver, which is what generated
  clients expect.
* **No release-time file edits.** A release is `git tag vX.Y.Z && git push --tags`.
  CI rebuilds the image, the new version flows everywhere. No "did someone
  remember to bump?" failure mode.
* **`pnpm-lock.yaml` may re-resolve** when the demoted `0.0.0` is first
  installed; harmless for a private package.
* **Local `pnpm dev`** has no build-time env, so the nav header reads `dev`.
  This is correct — there is no released artifact to name.
* **Railway preview** cannot run `git describe`, so its UI shows the branch
  name (e.g. `worktree-tra-485-...`) instead of a full describe string. This
  is an acceptable degradation for non-prod surfaces. Once prod images come
  from CI (GHCR), prod always has a full describe.
* **GKE move (TRA-351) is unaffected.** The image is built once by CI with
  the platform version baked in; the deploy target (Railway vs GKE) only
  decides who pulls the image, not what is in it.

## Alternatives considered

* **Runtime fetch (`/api/v1/version`):** frontend asks backend for the version
  on render. Single-source, but adds a network round-trip for a value that is
  static for the lifetime of the bundle, needs a loading state, and depends
  on `/api/v1/version` being reachable and unauthenticated when the header
  first renders (pre-auth, Device Status = Disconnected). Build-time injection
  avoids all of that.
* **release-please / standard-version / semantic-release:** automates tag
  cuts from Conventional Commits. Solves a coordination problem (multiple
  contributors, divergent bump-level judgment) that this team does not have
  at 1.5 FTE. Manual `git tag` + CHANGELOG.md edit is more honest about the
  team's actual coordination cost. Revisit at 5+ engineers.
* **Couple `info.version` to platform version:** rejected. `info.version` is
  a spec-consumer signal — it changes when generated SDKs need to regenerate.
  Coupling it to the platform release cadence would emit false-positive
  breaking-change signals on every backend bump.
