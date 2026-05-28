# TRA-853 Runtime-Driven Environment Banner — Design

**Date:** 2026-05-28
**Ticket:** TRA-853 (High)
**Status:** Approved (design locked in conversation 2026-05-28), pending implementation plan
**Related:** TRA-852 (canceled — build-time gke-preprod target this supersedes), TRA-375 (prod cutover — consumes this), TRA-485 (version-derive, adjacent build metadata)
**Infra counterpart:** trakrf/infra #126 (adds `config.environmentLabel` passthrough → ConfigMap `ENVIRONMENT_LABEL`)

## Problem

The environment banner is baked into the frontend bundle at **build time** via `import.meta.env.VITE_ENVIRONMENT` (`frontend/src/components/EnvironmentBanner.tsx:4`). Vite does a literal string substitution at build, so the value is frozen into the hashed `assets/index-*.js`. This is incompatible with the deployment model Mike confirmed: immutable `:sha-<hash>` images promoted across environments by floating tags. The *same* artifact lands in preview and prod, so a baked-in `"preview"` would show in prod.

Decision (Mike, 2026-05-28): the banner must be **100% runtime-driven** from a backend env var. No rebuild to change it.

## Architecture

The backend already serves the SPA's `index.html` fresh on every request (`backend/internal/handlers/frontend/frontend.go:37` `ServeSPA`, with `no-cache` headers). That is the single injection seam.

**Data flow:**
1. Operator sets `ENVIRONMENT_LABEL` on the backend pod (via infra ConfigMap). Empty/unset for prod.
2. On each `index.html` request, `ServeSPA` replaces a placeholder in the HTML with an inline script carrying the label:
   ```html
   <script>window.__APP_CONFIG__ = {"environmentLabel":"GKE pre-prod"};</script>
   ```
3. The inline classic `<script>` executes at parse time, before the deferred `<script type="module">` bundle — so `window.__APP_CONFIG__` is set before any React code runs. No flash, no fetch round-trip.
4. Frontend reads `window.__APP_CONFIG__.environmentLabel` at runtime. The hashed bundle contains no environment string and is byte-identical across environments.

Because index.html is templated per-request from the pod's env var, one built artifact renders the correct banner in any environment.

## Components

### Backend

- **`index.html` placeholder.** Add `<!--__APP_CONFIG__-->` in `<head>` of `frontend/index.html`. In dev (Vite serves index.html directly) it stays an inert HTML comment; `window.__APP_CONFIG__` is then undefined, which the frontend treats as "no label."
- **`ServeSPA` injection** (`frontend.go`). After `fs.ReadFile`, replace the placeholder with the inline script. The label value comes from `os.Getenv("ENVIRONMENT_LABEL")` read once at handler construction (`NewHandler`) and stored on the struct — not per-request — since it can't change without a pod restart.
- **Safe encoding.** The label is operator-controlled (low risk) but still injected into an inline script, so encode defensively: `json.Marshal` the config object, then in the marshaled bytes replace every `<` with the JS string escape `<` (so a label like `</script>` becomes `</script>` and cannot break out of the inline `<script>`). Never string-concatenate the raw value into the HTML.
- **Config struct.** `type appConfig struct { EnvironmentLabel string \`json:"environmentLabel"\` }` — a single field today, but a struct so future runtime config (feature flags, etc.) has a home without re-plumbing.

### Frontend

- **`window.__APP_CONFIG__` accessor** — new `frontend/src/lib/appConfig.ts`: typed reader returning `{ environmentLabel: string }`, defaulting `environmentLabel` to `''` when `window.__APP_CONFIG__` is absent (dev / placeholder-not-replaced).
- **Shared `isNonProd(label)` predicate** in the same module: `label !== '' && label !== 'prod' && label !== 'production'`. Both the banner and the test-hook gate use it. (Today `EnvironmentBanner.tsx` inlines this logic and `main.tsx` uses a narrower `=== 'preview'` — see nuance below.)
- **`EnvironmentBanner.tsx`** — read `appConfig.environmentLabel` instead of `import.meta.env.VITE_ENVIRONMENT`. Visibility and label text logic otherwise unchanged (still hidden when `isNonProd` is false; still `"<Label> Environment"`; title prefix from first 3 chars).
- **`main.tsx:47` test-hook gate** — currently `import.meta.env.DEV || import.meta.env.VITE_ENVIRONMENT === 'preview'`. Repoint to `import.meta.env.DEV || isNonProd(appConfig.environmentLabel)`. **This is a behavior fix, not just a port:** the old `=== 'preview'` would have *withheld* test hooks from the GKE dry-run (label `"GKE pre-prod"`). Using `isNonProd` exposes test hooks in every non-prod deployed env (preview + GKE dry-run) and withholds them in prod — which is the intent the comment already states ("Mirrors backend testhandler's APP_ENV != production gate"). `import.meta.env.DEV` stays build-time (legitimately a local-dev concept).

### Build cleanup

- Remove `VITE_ENVIRONMENT` from `.github/workflows/docker-build.yml` build-args (the entire `Resolve`/ternary path) and from `Dockerfile` (`ARG VITE_ENVIRONMENT` / `ENV VITE_ENVIRONMENT`, lines 33/35). No build-time environment baking remains.
- `version.ts` (`VITE_APP_VERSION`) and `VITE_SENTRY_DSN` stay build-time — they are legitimately build-scoped and out of scope here.

## Error handling

- Placeholder absent (e.g. a future index.html edit drops it): `ServeSPA` injection is a no-op `strings.Replace` (replaces nothing), HTML served unchanged, `window.__APP_CONFIG__` undefined, frontend defaults to no banner. Fail-safe: the worst case is "no banner," never a broken page.
- `ENVIRONMENT_LABEL` unset: empty label → `isNonProd` false → no banner, test hooks withheld. Correct prod behavior by default.

## Testing

- **Backend** (`frontend_test.go`): `ServeSPA` with `ENVIRONMENT_LABEL` set injects a well-formed inline script with the JSON-encoded label; with it unset injects `environmentLabel:""`; a label containing `</script>`/`<` is escaped to `<` (breakout-prevention assertion); placeholder-absent input is served unchanged.
- **Frontend** (`appConfig.test.ts`): reader defaults to `''` when global absent; `isNonProd` truth table (empty/prod/production → false; preview/GKE pre-prod → true). Update `EnvironmentBanner.test.tsx` to stub `window.__APP_CONFIG__` instead of `VITE_ENVIRONMENT`.
- **Manual:** run backend with `ENVIRONMENT_LABEL="GKE pre-prod"` → banner renders; unset → no banner; same built bundle both times.

## Out of scope

- The image-promotion mechanism (floating `:prod` tag vs main-sha pin) — TRA-375 decision; this design is agnostic to it.
- Migrating `VITE_SENTRY_DSN` / `VITE_APP_VERSION` to runtime.
- Any per-env config beyond the environment label (the struct leaves room, but no new fields ship here).
