# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.4.0] - 2026-08-09

Ten days of `main`, one migration (`000039`). Prod moves from `v1.3.0` (schema
version 38) to schema version 39.

The headline is a data-completeness bug rather than a feature: the Assets
screen had been fetching only the first 25 assets, and the list, the result
count, the stat tiles and the CSV/XLSX/PDF exports were all silently truncated
to that page. Any export taken before this release is incomplete for an
organization with more than 25 assets.

This is also the first release cut through the guards and runbook added in
1.3.0's wake (TRA-1085): `promote-prod` now refuses an image whose version is
not a clean `vX.Y.Z`, and resolves its image tag from a git ref.

### Upgrading

**`POST /api/v1/users` is removed from the public API** (TRA-1103). It was one
half of an endpoint group that had no authorization at all; see *Security*.
The path still serves `GET` (superadmin-gated), so a `POST` to it now returns
`405 Method Not Allowed`. No known integrator uses it. Editing your own
profile goes through `PATCH /api/v1/users/me` instead (TRA-958).

Migration `000039` is metadata-only — it re-declares the `000010` stored
functions with an explicit `search_path`. It does not touch data and the
previous release runs against it unchanged.

### Security

- **`/api/v1/users/{id}` had no authorization whatsoever** (TRA-1103). Any
  signed-in user could read, edit or delete any user in any organization,
  across org boundaries. The endpoints are now superadmin-gated, and
  `POST /api/v1/users` is removed outright rather than gated.

### Added

- **Users can edit their own profile** (TRA-958) — display name and email, via
  `PATCH /users/me`. Previously there was no UI for this at all.
- **Windows first-time Bluetooth pairing guidance** (TRA-1100). On Windows the
  browser's device chooser labels the CS108 "Unknown or unsupported device"
  next to a bare hex address, and connecting fails. Help now explains that the
  hex string is the reader's Bluetooth address — printed on the antenna label
  — and that pairing it once in Settings → Bluetooth & devices makes it list
  by name.
- **One consolidated Web Bluetooth support check** (TRA-1078) with
  platform-specific guidance, including iOS and Bluefy.

### Changed

- **Stored functions are hermetic** (TRA-1076, `000039`) — the `000010`
  functions now carry an explicit `SET search_path` instead of resolving
  against the caller's.
- **Local dev and edge run as non-superuser roles** (TRA-1075), so RLS is
  actually enforced outside of production.
- **The migrate preflight catches superuser-owned functions** (TRA-1104)
  rather than failing mid-migration and leaving the ledger dirty. This is what
  wedged preview on `000039`.
- **Reader mode follows the active tab** rather than being forced to Idle on
  connect (TRA-1101).

### Fixed

- **The Assets screen only ever fetched 25 assets** (TRA-1098) — list, result
  count, stat tiles, and CSV/XLSX/PDF export all truncated silently to the
  first page.
- **React StrictMode duplicated every row on the Assets page** (TRA-1070) via a
  double-invoked non-idempotent effect.
- **A saved scan took up to ~2 minutes to appear in Reports** (TRA-1117). The
  asset-locations report reads the `asset_scan_latest` continuous aggregate,
  which is `materialized_only` because TimescaleDB will not enable real-time
  aggregation over an RLS-guarded hypertable. The report now unions a bounded
  5-minute tail of raw `asset_scans` into the aggregate — idempotent, so the
  overlap cannot double-count — and saving invalidates the reports query
  client-side.
- **Locate could not find a 128-bit EPC, and could report the wrong tag**
  (TRA-1108). The Scan-tab deep link carried a leading-zero-stripped EPC that
  re-padded to the wrong width, and the tag mask covered only the leading 96
  bits, so two tags off one reel produced byte-identical mask sequences.
  Hardware-confirmed on two 128-bit bench tags.
- **Locate's first Start click silently failed** with "Cannot start scanning
  from state Busy" and then reported "No signal" (TRA-1080).
- **The Locate deep link raced the command mutex** (TRA-1091), logging a
  spurious hardware ERROR on the primary Locate path.
- **Blank header titles on Readers, Live Reads and Outputs** (TRA-1082), plus a
  reintroduced "inventory" in the nav vocabulary.

### Internal

No runtime behaviour: release guards and `docs/releasing.md` (TRA-1085);
preview composition tests, retiring the up-to-date branch rule (TRA-1094);
frontend vitest cross-file contamination (TRA-1093); rotted Locate e2e specs
(TRA-1088); `CLAUDE.md` refresh (TRA-1092); removal of the CSW spec tree.

## [1.3.0] - 2026-07-30

Eight weeks of `main` promoted to production in one release: 27 migrations
(`000011` … `000038`), capability gating, webhooks, kits, asset dwell, and the
Scan-tab rework. Prod moves from `v1.2.0` (schema version 10) to schema
version 38.

Nothing in this release is customer-visibly breaking on production. The
capability surfaces introduced here are new — no existing org loses access to
anything it had (see *Capability gating* below).

### Upgrading

**Databases created before this release hold their migration ledger in
`public` and must move it before migrating** (TRA-1069). `./server migrate`
detects this and refuses rather than replaying onto a populated schema, so the
failure is safe — the old process keeps serving and the database is untouched.
The fix is metadata-only and preserves both version and dirty flag:

```sql
ALTER TABLE public.schema_migrations SET SCHEMA trakrf;
```

Run it only *after* the new image is deployed and its migrate step has failed
the preflight; relocating ahead of the new binary leaves the old code unable to
find its ledger.

Two post-migration steps are not automatic:

- **Backfill the continuous aggregate.** `000027` creates `asset_scan_latest`
  `WITH NO DATA`, so the asset-locations report reads empty until
  `CALL refresh_continuous_aggregate('trakrf.asset_scan_latest', NULL, NULL);`
  has run once.
- **Grant capabilities.** There is no backfill — every organization, existing
  and new, starts with zero grants.

### Added

- **Capability gating** (TRA-1024 → TRA-1025 → TRA-1026, TRA-1027, TRA-1065).
  A `capabilities` vocabulary with per-org grants and an `org_capability_set()`
  set-returning function (`000036`); `RequireCap` middleware and a
  `capability_required` error type, with the caller's capability set on
  `/users/me`; a frontend registry that drives nav and route gating from that
  set and fails closed; and superadmin grant management via a whole-set
  `PUT /orgs/{id}/capabilities`. `kitting` joins the vocabulary in `000038`.
  Grants are opt-in per organization with **no backfill**. The gated surfaces
  (`geofence` → Outputs and Geofence defaults, `mustering`, `kitting`) all ship
  in this release, so a locked tile marks a feature arriving, not one removed.
- **Webhooks v1** (TRA-1043, `000035`) — delta-only `asset.moved` delivery, one
  webhook per organization, with signature verification.
- **Kits** (TRA-1032, TRA-1033, `000029`/`000031`) — `kits`, `kit_members` and
  `kit_verifications`, plus internal create/verify/lookup endpoints and client.
  Gated behind `kitting`, which is granted to nobody by default, so Kits ships
  switched off.
- **Asset dwell on the asset-locations report** (TRA-1023, `000037`) —
  `dwell_started_at` and `dwell_seconds`, measured as the observed span of a
  stay rather than wall-clock age.
- **`asset_scan_latest` continuous aggregate** (TRA-1022, `000026`–`000028`) —
  materializes latest-scan-per-asset, replacing the `DISTINCT ON` query that
  could crash under RLS. Ships with a refresh policy and, from `000037`, an
  index for the dwell columns.
- **Geofence and output devices** (TRA-901, TRA-903, TRA-906, TRA-991,
  TRA-1028) — a real-time boundary geofence engine, alarm event log
  (`000013`), output/actuator devices bound to logical locations
  (`000014`/`000016`/`000032`/`000033`), Shelly Gen4 firing over MQTT for a
  firewall-friendly path, and `Gpo.Set` on the reader mqtt-rpc contract.
- **`trakrf` CLI** (TRA-307) — generated from the public OpenAPI spec.
- **Scan tab** (TRA-1029, TRA-1031, TRA-1036, TRA-1038) — Inventory becomes
  Scan, with a session-local barcode/RFID mode toggle, a persisted status
  filter, and WYSIWYG save semantics: Save persists exactly the rows the view
  shows, with reconciliation-only "missing" rows explicitly excluded.
- **Signup hardening** (TRA-970, TRA-971, `000025`) — non-production signup is
  gated, and organization contact details plus an owner are required.
- **`readerd`** (TRA-1002, TRA-1015) — `/API` entity transport, golden CS463
  device definitions, and self-bootstrap of the EventID when none is enabled.
- **BLE ingestion** (TRA-926) — BLE advertisement model and iBeacon hex decoder.
- **`tag_scans` compression and retention** (TRA-921, `000030`) — compress
  after 2 days, drop after 7. `asset_scans` is deliberately left uncompressed:
  TimescaleDB does not enforce RLS on compressed chunks.
- **Migration integrity guard** (TRA-1077) — a checksum manifest fails the
  build when an already-applied migration is edited. Adding a migration now
  requires `just backend migrate-checksums`.
- **Infra ops passthrough** (TRA-1053) — `just ops`, `just psql` and
  `just logs` forward to the infra repo's justfile rather than reimplementing
  kubectl.
- Subscriptions schema (`000022`), muster events (`000024`), normalized tag
  values for leading-zero/case-insensitive matching (`000017`), and
  `list_active_scan_topics` (`000021`).

### Changed

- **Signed-out navigation is decided in one place** (TRA-1057). `ProtectedRoute`
  is gone; `routePolicy.ts` resolves auth → entitlement → capability → role.
- **One organization admin surface** (TRA-1058). `OrgModal`'s manage mode is
  retired in favour of a single org-settings surface; superadmins edit, plain
  admins see a read-only badge.
- **The migration ledger is pinned to `trakrf.schema_migrations`** (TRA-1069)
  rather than resolved through `CURRENT_SCHEMA()`, and the runner creates the
  schema before golang-migrate looks for its ledger. See *Upgrading*.
- `alarm_devices` is renamed to `output_devices` (`000016`), reflecting that the
  devices are general-purpose outputs rather than alarms specifically.
- Live Reads moves stats below the tag list and consolidates the header
  (TRA-1010).
- In-app Help names the surfaces the app actually has (TRA-1071), following the
  tab renames.

### Fixed

- **Split migration histories are detected and refused** (TRA-1069) instead of
  silently replaying onto a populated schema and reporting a clean version that
  does not describe the schema on disk.
- **Stale chunk 404s after every deploy** (TRA-1054) — nested lazy imports
  bypassed `lazyWithRetry`, crashing Scan and Locate.
- **`DISTINCT ON` over `asset_scans` 500ing under RLS** (TRA-1021) — the
  TimescaleDB SkipScan optimization could abort the query; superseded by the
  continuous aggregate above.
- OAuth2 `client_id` and `client_secret` are shown in the API-key create modal
  (TRA-1019); they are unrecoverable afterwards.
- Geofence alarms no longer re-fire for tags already inside the zone when the
  engine restarts (TRA-991).
- Frontend test isolation and a vitest deadlock (TRA-1050, TRA-1052) — unit
  tests no longer reach the network, and test files no longer leak state into
  each other.

### Removed

- The Home and Barcode tabs (TRA-1029, TRA-1031). Legacy hash routes
  (`#home`, `#inventory`, `#barcode`) redirect to `#scan`, so bookmarks survive.
- `scan_points.is_boundary` (`000018`) and `scans.external_key` (`000020`).
- The `tag_scan` trigger superseded by the ingestion path (`000012`).

## [1.2.0] - 2026-05-30

Entries below accumulated under `[Unreleased]` without ever being cut into
per-release sections, and are grouped here under the release that carried them
to production. Some of them shipped earlier, in [1.1.0] or [1.1.1] — that
boundary is left unreconstructed rather than guessed at.

### Fixed
- TRA-708 (BB32 follow-up, no spec shape change): two stale test fails on `main` that the TRA-707 cycle flagged as pre-existing were each broken by a different prior PR rather than always-pre-existing. (a) `TestPutAsset_MetadataNonObject_Returns400` broke when TRA-678 tightened `UpdateAssetRequest.Metadata` from `*any` to `*map[string]any` — `encoding/json` now intercepts a non-object body value as `*json.UnmarshalTypeError` before the TRA-619 runtime type-assert can run, and `RespondDecodeError` was routing non-time-target mismatches through the generic `bad_request` fallback (no `fields[]`). The public spec declares `metadata` as `type: object`, so a non-object is a schema violation, not a parse error — same shape as date-format mismatches. `RespondDecodeError` now has a map-target branch (mirroring the time-target branch) that emits `validation_error / invalid_value` keyed on the JSON-leaf field with message `"{field} must be a JSON object"`. (b) `TestPostAsset_MissingNameEmitsTooShort` and `TestPostLocation_MissingNameEmitsTooShort` broke when TRA-692 §1.2 introduced the presence overlay that promotes a collapsed `too_short` back to `required` for omitted length-bearing required fields; the tests still asserted the pre-TRA-692 `too_short` contract. Renamed to `MissingNameEmitsRequired` and updated to assert `code=required` with nil params. Docs (`errors.md`) already match the post-TRA-692 contract.

### Changed
- TRA-717 (BB34 F2 + F3 + F4 + F5, breaking wire shape on F2 and F3):
  - **F2 — scan-event timestamp field harmonization (breaking).** History endpoint response field `timestamp` → `event_observed_at`; report endpoint response field `last_seen` → `asset_last_seen`. Sort allowlists updated on both: `?sort=event_observed_at` on `/assets/{asset_id}/history`, `?sort=asset_last_seen` on `/reports/asset-locations`. Both new names match the qualifier-prefix precedent already set by `asset_deleted_at` on the same report shape and preserve the event-row vs asset-most-recent semantic distinction. Storage column names are unchanged.
  - **F3 — outbound fractional precision pinned to milliseconds (breaking).** Every public response timestamp now emits RFC 3339 with fixed three-digit fractional precision (`.000Z` shape) via the new `shared.PublicTime` wrapper type. Replaces Go-stdlib `time.RFC3339Nano` trailing-zero trimming, which produced variable-shape outputs (`.123Z` / `.752440Z` / no-fractional) and broke hand-rolled regex parsers in generated SDK consumers. Postgres `timestamptz` storage is unchanged (still microsecond); the bottom three digits are server-receipt-time jitter relative to what reader clients can use, so millisecond is the meaningful wire resolution. `shared.FlexibleDate.MarshalJSON` is aligned to the same formatter so request-echo paths emit the canonical shape too. Paired input-side change: the `from` / `to` query parameters on `/assets/{asset_id}/history` switch from `time.Parse(time.RFC3339, …)` to `time.Parse(time.RFC3339Nano, …)` so a client can copy any emitted `event_observed_at` value verbatim into a filter without parse rejection (round-trip symmetry). Reverses the original BB34 F3 direction (docs-only, leave service variable) — the service-side fix avoids per-consumer regex workarounds and pins one canonical wire shape across the surface. Spec example constant bumped to `2025-04-29T12:34:56.000Z` so docs match what the server actually emits.
  - **F4 — `info.contact.url` environment awareness.** The committed spec pins the production canonical URL (single artifact for every environment). The backend swaps to the preview equivalent at serve time when `APP_ENV=preview` via `swaggerspec.resolvePublicSpec`, so a spec pulled from `app.preview.trakrf.id/api/v1/openapi.yaml` reads preview. Targeted byte-replace on `info.contact.url` only; the bare-hostname `servers[]` entries are untouched.
  - **F5 — filter parameter pattern tightening (carry-over from BB33).** Five `*_external_key` query-parameter `items.pattern` declarations tightened from the loose tag-value pattern `^[^\x00-\x08\x0B\x0C\x0E-\x1F\x7F]*$` to the strict field pattern `^[A-Za-z0-9-]+$`: `assets.external_key`, `assets.location_external_key`, `locations.external_key`, `locations.parent_external_key`, `reports.location_external_key` (5th caught by same-surface audit). Mirrors the service-side validator path enforced in TRA-713 so a generated client that validates input against the spec catches `abc/def` at the client layer instead of round-tripping to a 400.

- TRA-702 (BB32 D2+D3, no spec shape change): every `validation_error` emit-site now routes through a single helper that derives `detail` from `fields[0].message` and appends `(and N more validation errors)` when more than one field rejected. Pre-TRA-702 four handler-emitted call sites (PATCH read-only echo on assets and locations, PATCH explicit-null on non-nullable fields on both) wrote the literal "validation failed" as detail, burying the redirect-to-/rename message inside `fields[0]` where AI integrators were less likely to read it (BB32 D2). The PATCH explicit-null loops also short-circuited at the first match, so a body that nulled multiple non-nullable fields surfaced only the first one per round trip; the loops now accumulate every violation before responding. Strict-decode helpers (`DecodeJSONStrict`, `DecodeJSONStrictWithPresence`, `DecodeJSONStrictWithNullsTolerant{,AndPresence}`) pre-detect every unknown top-level key via reflection on the destination struct and return a new `*JSONUnknownFieldsError`, so a body with multiple typo'd keys surfaces one `fields[]` entry per unknown rather than just the first one `encoding/json`'s `DisallowUnknownFields` happens to report (BB32 D3). No wire-level breaking change for clients that branched on `error.type` + `fields[].code` per the documented contract.
- TRA-699 (BB31 §2, breaking — pre-launch relaxation + tightening): uniform "accept-if-matches, reject-if-differs" semantics for the five natural-key reference fields on PATCH. For each of `external_key` (PATCH /assets, PATCH /locations), `parent_external_key` (PATCH /locations), `location_id` and `location_external_key` (PATCH /assets): a body value matching the current resource state is stripped from the update and the request succeeds (200, other fields apply); a differing value returns 400 `validation_error` with `fields[].code: "read_only"` and a detail naming the proper write path (`POST /…/rename` for the rename-managed natural keys; "record a scan event" for the derived asset-location fields). Pre-TRA-699 the same fields had three different behaviors: `external_key` and `parent_external_key` were pre-decode rejected unconditionally (TRA-686 / BB29 F7+F8); `location_external_key` was silently stripped (TRA-681); `location_id` was a writable field with null-clear semantics (TRA-614). The new shape is the same envelope for every member of the category, idempotency-friendly (verbatim GET → PATCH round-trips without a strip step), and reinforces the record-of-origin posture for asset location (the rejection detail points at scan events; medium-term the `current_location_id` denormalization moves to a continuous aggregate on `asset_scans` per TRA-411). On the storage side, `current_location_id` is no longer writable via PATCH, and `UpdateAssetRequest.ClearLocationID` is removed. The `parent_id` surrogate on PATCH /locations remains fully writable (re-parenting via surrogate is unchanged); only the natural-key form is locked down.

### Added
- TRA-698 (BB31 §1.4): new Spectral rule `trakrf-no-pattern-on-date-time-format` fails any schema property or parameter that declares both `format: date-time` and a `pattern`. RFC 3339 is already implied by the format, and the redundant pattern broke `openapi-generator-cli -g python` clients at runtime — its template applies the regex via a `@field_validator` that runs after Pydantic has already parsed the string into a `datetime`, then stringifies that datetime (space separator, not `T`) before matching, so every read path returning a timestamp threw a `ValidationError` on deserialize. The paired postprocess change (below) drops the existing pattern across all 23 date-time fields in the published spec; this rule prevents recurrence. Distinct from `trakrf-no-control-bytes-in-pattern` (BB29 F6 / TRA-687) — different root cause, different category.
- TRA-692: contract-test coverage gate that asserts every value in `components.schemas.FieldErrorCode.enum` was observed at least once in a real `validation_error` response during `just backend test-contract`. Catches "declared in the enum but never wired to emission" drift across the whole enum, not just the single case that surfaces in any given BB cycle. Deterministic supplementary case runner (`backend/contract-tests/explicit_error_cases.py`) probes each code; gate (`check_field_error_coverage.py`) fails CI with the missing list.
- Initial project documentation structure
- Business Source License 1.1 with Additional Use Grant
- Code of Conduct (Contributor Covenant 2.1)
- Security policy with vulnerability reporting procedures
- Contributing guidelines with code examples and testing requirements
- CLAUDE.md for AI assistant guidance

### Changed
- TRA-698 (BB31 §1.4, spec-shape change): `pattern` removed from every `format: date-time` field in the published OpenAPI spec — `AssetView.created_at` / `.updated_at` / `.valid_from` / `.valid_to` / `.deleted_at`, `LocationView.created_at` / `.updated_at` / `.deleted_at`, `Tag.created_at` / `.updated_at` / `.deleted_at`, `AssetLocationItem.last_seen` / `.asset_deleted_at`, `AssetHistoryItem.timestamp`, plus the inline `from` / `to` query parameters on `/api/v1/assets/{asset_id}/history`. The pattern was added to the postprocess in TRA-678 to keep Schemathesis's positive-data-acceptance check away from year-0001 dates (which `shared.FlexibleDate` rejects); positive-data-acceptance is excluded from the contract-test gate as of TRA-692, and the new `trakrf-no-pattern-on-date-time-format` Spectral rule guards against re-introduction. No behavior change on the request path — `FlexibleDate` still rejects out-of-range years with the documented `bad_request` envelope. The postprocess pass `markDateTimePatterns` is deleted; the corresponding `dateTimePattern` constant is gone.
- TRA-692 (BB30 §1.2, behavior change): omitted or explicit-null required fields on public POST/PATCH now emit `validation_error` with `fields[].code: "required"` instead of the prior TRA-675 collapse to `too_short`. Empty string on a `min_length:1` field still emits `too_short`. Affects every public POST/PATCH that carries a length-bearing required body field — assets POST/PATCH/rename/tags, locations POST/PATCH/rename/tags. Integrators branching on `code === "required"` per the errors docs page (rather than parsing `params.min_length`) now see the documented behavior; integrators that were relying on the §1.2-buggy `too_short` for omitted fields must switch to `code === "required"` for that case (`too_short` remains correct for "value supplied but shorter than allowed").
- Migrating handheld React app as frontend component
- TRA-684 (BB29 F9 / C3, breaking): `tree_path` and `depth` removed from the `LocationView` response shape, from the locations sort enum (`?sort=tree_path` / `-tree_path` no longer recognised — fall back to `external_key`, `name`, or `created_at`), and from the strip-on-PATCH allow list. The underlying `locations.path` ltree column, the generated `depth` column, the `update_location_path` BEFORE trigger and the `cascade_location_path_change` AFTER trigger are dropped in migration `000042_drop_location_path_and_depth`. Hierarchy queries (`/locations/{id}/ancestors`, `/locations/{id}/descendants`, `GET /locations` filtered by parent, internal subtree counts) now walk `parent_id` via recursive CTE; ancestors are still root-first and descendants are still depth-first by lowercased `external_key` segments (parity with the prior ltree order at typical scale). Default list sort is now `external_key ASC, id ASC` (was `path ASC`). `POST /locations/{id}/rename` still mutates this row's `external_key` and still returns `descendant_count_affected` (the live count reachable through `parent_id`) so integrators can refresh derived natural-key joins, but no descendant row is rewritten on the server. Frontend `LocationBar` derives hierarchy depth/order from the cached locations list (parent_id walk) instead of consuming `tree_path`/`depth`. Closes BB29 F9 (case-collision footgun: `LOSSY-CASE` and `lossy-case` no longer fold to the same materialized path — they now coexist as case-distinct siblings); BB29 C3 reduces to docs-only (the misleading "join key" field is gone).
- TRA-682 BB28 fix wave (consolidated; pre-launch breaking changes):
  - **Scope rename:** `history:read` → `tracking:read`. The scope gates both `/assets/{asset_id}/history` (time-series) and `/reports/asset-locations` (current-state snapshot); the new name better describes "where things are and have been" and pairs with `assets:read` / `locations:read`. Regenerate keys with the new scope name; existing preview keys are migrated by `000041_rename_history_read_to_tracking_read`. SPA scope picker label updated from "History" to "Tracking".
  - **Breaking change for generated clients:** PATCH operation IDs renamed from `patchAsset` / `patchLocation` to `updateAsset` / `updateLocation`. Regenerate clients from the updated spec.
  - **PATCH content-type tightened (RFC 7396 strict):** the two PATCH endpoints (`/api/v1/assets/{asset_id}`, `/api/v1/locations/{location_id}`) now reject `application/json` with 415 `unsupported_media_type`. Only `application/merge-patch+json` is accepted on PATCH; POST and PUT keep `application/json`. The 415 detail string is method-aware and names the correct content type per method. Enforcement is per-route so PATCH probes against POST-only paths (`/tags`, `/rename`) keep returning 405 from chi.
  - **FieldError enum cleanup:** `immutable_field` removed (retired in TRA-674 read-only-strip work); `unknown_field` added so integrators can branch on a wrong-field-name vs wrong-field-value without parsing detail strings.
  - **Internal references stripped from spec descriptions:** four leaks of `TRA-###` / `BB##` references from swag annotations into generated SDK docstrings cleaned up. New Spectral rule `trakrf-no-internal-references-in-descriptions` guards regression.
  - **New Spectral rule** `trakrf-patch-merge-patch-ct-only` asserts every PATCH `requestBody.content` declares only `application/merge-patch+json` so the spec cannot drift back to also declaring `application/json`.
- TRA-660 BB25 C1 public-spec schema namespace restructure (breaking for SDK consumers; no published SDK yet):
  - Schema components no longer carry Go-package prefixes. `asset.PublicAssetView` → `AssetView`, `location.UpdateLocationRequest` → `UpdateLocationRequest`, `errors.ErrorResponse` → `ErrorResponse`, `shared.Tag` → `Tag`, etc. Codegen tools that flatten `.` to a legal identifier (most do) no longer emit doubled-prefix model classes (`AssetPublicAssetView`).
  - The redundant `Public` qualifier is dropped — the spec is the public surface; the Go-side distinction is invisible to SDK consumers.
  - Where the clean name would collide across resources, the rename keeps a resource prefix in verb-noun form: `asset.AddTagResponse` → `AddAssetTagResponse`, `location.AddTagResponse` → `AddLocationTagResponse`.
  - Report-package wrappers fold onto resource-shaped names matching what the operation returns: `report.ListCurrentLocationsResponse` → `AssetLocationsResponse`, `report.PublicCurrentLocationItem` → `AssetLocationItem`.
  - operationIds adopt camelCase `verbResource` form: `assets.create` → `createAsset`, `locations.tags.add` → `addLocationTag`, `reports.asset-locations` → `getAssetLocations`. Generated SDK calls read `client.createAsset()` rather than `client.assets_create()`.
  - Top-level `tags` array now carries descriptions for each resource grouping (assets, locations, orgs, reports) — used by docs renderers.
  - Internal spec is unchanged. Go source is unchanged (rename happens entirely in the apispec transformer). No wire-level behavior changes.
- TRA-602 BB17 S2 schema namespace consolidation (breaking for SDK consumers; no published SDK yet):
  - Asset, location, and report schema components are now under a single (singular) namespace: `asset.*`, `location.*`, `report.*`. Response wrappers that previously lived under `assets.*` / `locations.*` / `reports.*` (e.g. `assets.CreateAssetResponse`, `locations.ListLocationsResponse`, `reports.AssetHistoryResponse`) are renamed to the singular form (`asset.CreateAssetResponse`, `location.ListLocationsResponse`, `report.AssetHistoryResponse`).
  - Org schemas are now under `org.*` (matches the `/api/v1/orgs/...` URL prefix). Public spec: `orgs.GetOrgMeResponse` → `org.GetOrgMeResponse`, `orgs.OrgMeView` → `org.OrgMeView`. Internal spec also folds the model package `organization.*` (full word) onto `org.*` for consistency.
  - Internal-spec audit extension: `users.ListResponse` → `user.ListResponse`; the swag-emission long-form `github_com_trakrf_platform_backend_internal_models_user.User` → `user.User`. `errors.*`, `shared.*`, `apikey.*`, and the remaining single-package families (`auth`, `bulkimport`, `health`, `inventory`, `lookup`, `storage`) are unchanged.
  - SDK regen required for downstream consumers; pre-launch with no published SDKs, the break has no current cost.
- TRA-603 BB17 S1 request body content-type alignment:
  - `POST /api/v1/locations/{location_id}/tags` now declares `application/json` for its request body (previously `*/*`), matching the sibling `POST /api/v1/assets/{asset_id}/tags` endpoint. No wire-level behavior change — the server already required `application/json` — but strict generators (Java/Go) no longer drop the typed body.
- TRA-586 BB16 S7 path-param naming sweep (breaking for SDK consumers; no published SDK yet):
  - Public API path parameters are now consistently qualified across all asset and location operations: `{id}` is renamed to `{asset_id}` on `/api/v1/assets/{asset_id}{,/history,/tags}` and to `{location_id}` on `/api/v1/locations/{location_id}{,/ancestors,/children,/descendants,/tags}`. The actual URL paths are unchanged — only the OpenAPI parameter names.
  - Generated `typescript-fetch` SDK now uses consistent parameter names per resource: `assetsTagsAdd({ assetId, ... })` and `assetsTagsRemove({ assetId, tagId })` — same `assetId` field across both calls. Same on `locationsTagsAdd` / `locationsTagsRemove` (`locationId`).
- TRA-579 BB15 D-4/D-6/D-10 platform-side fixes:
  - `error.title` is now a fixed string per `error.type` (e.g. `validation_error` → "Validation failed", `bad_request` → "Bad request"). Per-call specifics live in `error.detail` and `error.fields[]`. Generated clients should branch on `error.type` and `error.fields[].code`.
  - `GET /api/v1/assets/lookup` and `GET /api/v1/locations/lookup` now reject duplicate `external_key` query parameters with `400 bad_request` (previously: silent first-wins).
  - `GET /api/v1/locations` now accepts `parent_id` (canonical) as a filter, mutually exclusive with `parent_external_key`.
  - Wrong-resource title bug on tags POST conflict ("Failed to create asset" emitted on `/assets/{id}/tags`) is fixed; the conflict still returns 409 with the underlying duplicate-tag detail.
- TRA-580 BB15 spec naming hygiene (S-2/C-1/C-2/C-3, breaking renames):
  - `location.path` is now `tree_path` on the wire (request and response), and `tree_path` replaces `path` in the locations sort enum. The underlying ltree column is unchanged.
  - `asset.current_location_id` and `asset.current_location_external_key` are now `location_id` and `location_external_key` on the wire (request and response), aligning with the report row shape returned by `GET /locations/current`. The underlying SQL column `current_location_id` is unchanged.
  - `POST /api/v1/orgs/{id}/api-keys` response field `data.key` is now `data.token`. Avoids confusion with the human-readable `name` of an API key (and the LLM-leak risk of a "key" field). Endpoint is internal; SPA only.
  - `error.type` is annotated `x-extensible-enum: true` (existing behavior; no client-visible change). The codegen-limitation caveat on `x-extensible-enum` will land in the docs PR.
  - Frontend updated to match the new wire fields. SDK regen required for downstream consumers.
- TRA-578 Public API surface cleanup:
  - `POST/GET/DELETE /api/v1/orgs/{id}/api-keys*` removed from the public OpenAPI spec. Key minting remains browser-mediated by design (see Authentication docs). The endpoints are still implemented and used by the SPA's avatar menu.
  - Renamed scope `scans:read` → `history:read` to align with the `/assets/{id}/history` and `/locations/current` endpoint vocabulary. Existing keys are migrated by `000039_rename_scans_read_scope`. JWTs minted before the migration with a literal `scans:read` claim will return 403 — pre-launch hard cut, no production keys exist.

    > **Correction (2026-08-09):** this did not ship as written and does not
    > describe the system today. The TRA-720 migration re-baseline dropped 44
    > legacy migrations including `000039_rename_scans_read_scope` (commit
    > `8cfa3949`), and `671f0779` restored the scope name. The live scope is
    > **`tracking:read`** — see `models/apikey/apikey.go:17`. The number
    > `000039` was later reused by `000039_hermetic_stored_functions` in 1.4.0,
    > which is unrelated.

  - SPA "Scans" row in the new-key form is renamed to "History" to match the new scope name. *(Also superseded — see the correction above.)*

## [0.1.0] - 2025-10-11

### Added
- Initial project structure and licensing
- Core documentation for open source project
- .gitignore with Go backend and Node.js frontend support
