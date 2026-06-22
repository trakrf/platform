# trakrf CLI

A scriptable command-line client for the [TrakRF](https://trakrf.id) public REST
API. The typed API client is **generated from the published OpenAPI spec** with
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen); the hand-written
code is limited to flag parsing, auth/config, and output formatting.

```
trakrf auth login
trakrf assets list --search forklift
trakrf assets get 42 --json | jq .data.name
trakrf locations list --format csv > locations.csv
```

## Install

```bash
# from a checkout of the monorepo
just cli install        # go install .
# or
go install github.com/trakrf/platform/cli@latest
```

Release binaries (macOS/Linux/Windows) and a Homebrew tap are produced by
GoReleaser — see [Distribution](#distribution).

## Authentication

The public API uses OAuth2 **client_credentials**. Create an API key in the
TrakRF web UI to get a `{client_id, client_secret}` pair, then:

```bash
trakrf auth login                 # interactive prompt (env, client id, secret)
trakrf auth login --client-id ID --client-secret SECRET --env preview
trakrf auth status                # show the active profile + live org check
trakrf auth logout                # remove stored credentials for the profile
```

`login` verifies the credentials by minting a token before saving anything. The
short-lived access token is cached in the config file and re-minted from the
stored credentials when it expires. (The refresh-token rotation grant is
intentionally not used — re-minting is simpler and avoids the
reuse-revokes-the-chain failure mode for a multi-process CLI.)

### Config, profiles, and environment variables

Configuration lives at `~/.trakrf/config.yaml` (override the directory with
`TRAKRF_CONFIG_HOME`). Each **profile** binds an environment to a credential
pair:

```yaml
current_profile: default
profiles:
  default:
    env: prod        # prod -> https://app.trakrf.id
    client_id: ...
    client_secret: ...
  staging:
    env: preview     # preview -> https://app.preview.trakrf.id
    client_id: ...
    client_secret: ...
```

Environment overrides:

| Variable             | Effect                                                        |
| -------------------- | ------------------------------------------------------------ |
| `TRAKRF_API_KEY`     | `client_id:client_secret` — ephemeral creds for this run     |
| `TRAKRF_ORG`         | selects a profile by name when `--profile` is absent         |
| `TRAKRF_API_URL`     | override the base URL (self-hosted deployments, testing)     |
| `TRAKRF_CONFIG_HOME` | directory holding `config.yaml`                              |

Global flags: `--profile`, `--env prod|preview`, `--format table|json|csv`,
`--json` (shorthand for `--format json`), `--config <path>`.

## Output formats

* **table** (default) — human-readable, lipgloss-styled.
* **json** — the raw API response, for piping to `jq`.
* **csv** — for spreadsheet workflows.

Errors go to stderr; the exit code is non-zero on failure.

## Commands (v1)

```
trakrf auth login | logout | status
trakrf assets list [--limit --offset --external-key --active --search --sort --include-deleted]
trakrf assets get <id>
trakrf locations list [--limit --offset --external-key --parent-id --parent-external-key --active --search --sort]
trakrf locations get <id>
trakrf orgs list                 # the org bound to the active API key (GET /orgs/me)
trakrf orgs switch <profile>     # select a local profile (not an API call)
```

`<id>` is the canonical numeric `id`. To look something up by its natural key,
filter the list: `trakrf assets list --external-key ABC123`.

## Regenerating the client

The OpenAPI spec is vendored at `api/openapi.public.yaml` and the client is
committed (`api/api.gen.go`) for deterministic builds. After the API surface
changes:

```bash
just cli vendor-spec    # re-copy spec from ../docs/api + regenerate
# or
just cli regen          # regenerate from the already-vendored spec
```

## Divergences from the original ticket scope

This v1 wraps only what the **shipped** public API exposes. Known gaps versus the
TRA-307 wishlist (captured for the D2A-165 write-up):

1. **`scans` commands** — dropped. Scan data was pulled to internal-only on the
   2026-04-29 API pivot; it is not in the public spec.
2. **`orgs list` / `orgs switch`** — the API binds one org per key, so there is
   no list/switch endpoint. `list` shows the caller's org; `switch` selects a
   local credential profile.
3. **`assets list --location`** — `listAssets` has no location filter;
   location-scoped reads belong to `reports/asset-locations` (not yet wrapped).
4. **Primary key** — the wire key is the numeric `id`; `external_key` is the
   alternate lookup. The ticket's "identifier as primary key" predates the pivot.
5. **Write paths** (`assets create`, `assets import`, `assets export`) — deferred
   to a follow-up PR per the vertical-slice scope.

### oapi-codegen note

The spec names its response schemas `GetAssetResponse`, `ListAssetsResponse`,
etc. — which collide head-on with oapi-codegen's own per-operation response
wrapper types. Single-package generation fails to compile until the wrappers are
suffixed (`response-type-suffix: Resp` in `api/cfg.yaml`). See that file for the
full note.

## Distribution

`.goreleaser.yaml` builds single static binaries for linux/darwin/windows on
amd64/arm64 and a Homebrew formula for `trakrf/homebrew-tap`. Wiring the tagged
release workflow + tap token is a follow-up; `just cli release-snapshot` builds
locally today.
