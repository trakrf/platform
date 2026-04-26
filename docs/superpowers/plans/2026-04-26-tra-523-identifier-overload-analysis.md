# TRA-523 — Terminology overload: `identifier` impact analysis

**Date:** 2026-04-26
**Status:** Read-only analysis. No code changed. Output for paste-in to TRA-523.
**Goal:** Let Mike + Tim choose between Options A / B / C without further code-spelunking.

---

## TL;DR

- Surface area is large: ~6,300 occurrences of `identifier` across 311 files in `backend/`, `frontend/`, `docs/`, and `spec/`. Cap was hit; reporting structurally rather than per-line.
- The "three concepts" framing in the prompt is partly inaccurate. **Concept #3 (the literal device value) is *already* cleanly disambiguated as `value`** in DB, Go, OpenAPI, and TS — see §1 evidence. There are really only **two** colliding meanings in code: #1 (natural key) and #2 (the physical-device entity).
- **Concept #1 is broader than the prompt described.** `identifier` is the universal natural-key column convention on **five** tables (`organizations`, `locations`, `scan_devices`, `scan_points`, `assets`), not just on assets. Renaming it ripples into ltree path triggers, partial unique indexes, multiple seed-data migrations, dozens of routes, and every public DTO.
- **Concept #2 is partly renamed already.** Public OpenAPI uses `shared.TagIdentifier` / `shared.TagIdentifierRequest` (`docs/api/openapi.public.yaml:557,579`); frontend types use `TagIdentifier` / `TagIdentifierInput`. What remains on concept #2 is the underlying DB table, hypertable, PL/pgSQL functions, file/handler/method names, and URL path segment.
- **Recommendation: Option B (rename #2).** Lower cost, lower blast radius, and finishes a renaming that's already half-done. See §6 for evidence and tradeoffs.

---

## 1. Surface area inventory

### Counts

Initial sweep (case-insensitive substring `identifier`, all variants), filtered to source extensions:

| Area | Files | Occurrences |
|---|---:|---:|
| `backend/` (Go + SQL + generated docs) | 105 | 2,739 |
| `frontend/` (TS/TSX) | 151 | 1,327 |
| `docs/` (markdown + OpenAPI) | 52 | 2,192 |
| `spec/` (active specs) | 3 | 53 |
| **Total** | **311** | **~6,311** |

Cap reached. The breakdown below is structural rather than line-by-line.

### Categorization (where structure makes the meaning unambiguous)

| Concept | Where it lives (canonical) | Evidence |
|---|---|---|
| **#1 — Customer natural key** | Column `identifier` on 5 entity tables; matching `Identifier` field on Go DTOs; matching `identifier` field/path-param in OpenAPI; matching `identifier` field on frontend TS types | `backend/migrations/000002_organizations.up.sql:10`, `000005_locations.up.sql:9`, `000006_scan_devices.up.sql:9`, `000007_scan_points.up.sql:11`, `000008_assets.up.sql:9` — all carry comment "Natural key/business identifier" |
| **#2 — Physical device entity** | Table `identifiers`; hypertable `identifier_scans`; PL/pgSQL `create_asset_with_identifiers`, `create_location_with_identifiers`, `process_identifier_scans`; Go file `storage/identifiers.go`; Go shared models `TagIdentifier` / `TagIdentifierRequest`; OpenAPI schemas `shared.TagIdentifier` / `shared.TagIdentifierRequest`; URL path segment `/identifiers`; frontend `TagIdentifier` / `TagIdentifierInput` | `backend/migrations/000009_identifiers.up.sql`, `000010_identifier_scans.up.sql`, `000024_identifier_functions.up.sql`, `000015_identifier_scans_trigger.up.sql`; `backend/internal/storage/identifiers.go`; `backend/internal/models/shared/identifier.go:5,12`; `frontend/src/types/shared/identifier.ts:11,17`; `docs/api/openapi.public.yaml:557,579` |
| **#3 — Literal device value** | Column `identifiers.value`; field `value` on `TagIdentifier` (Go + TS + OpenAPI). **Not actually overloaded with "identifier" anywhere.** | `backend/migrations/000009_identifiers.up.sql:10` ("the actual identifier (EPC, MAC, serial number, etc.)"); `backend/internal/models/shared/identifier.go:8`; `docs/api/openapi.public.yaml:571,589` |
| **(other)** | Generic prose ("a unique identifier for the resource"), `parent_identifier`, `location_identifier`, `asset_identifiers` (semantically all #1), and incidental MD/comments | `docs/logical-schema.md`, `docs/schema-naming-conventions.md`, request schemas in `openapi.public.yaml` |

**The actual collision is binary, not ternary.** #3 reads as "identifier" only in user mental models and prose; in code/schema it is `value`. That changes the rename problem materially — Options A and B can each completely fix the collision without touching the other.

### Top high-impact files (per-file counts)

Backend:
- `backend/internal/storage/assets.go` — 131 (concept #1 SQL + handlers)
- `backend/docs/docs.go` — 114 (auto-generated swagger; regenerated from struct annotations)
- `backend/internal/storage/locations.go` — 109 (concept #1)
- `backend/internal/handlers/locations/locations.go` — 103 (mix of #1 and #2)
- `backend/internal/storage/identifiers.go` — 91 (concept #2 — clean container)
- `backend/internal/handlers/assets/assets.go` — 77 (mix of #1 and #2)
- `backend/internal/cmd/serve/router.go` — 16 route lines (`router.go:138–196`)

Frontend:
- `frontend/src/components/locations/LocationForm.tsx` — 57 (concept #1 form)
- `frontend/src/components/assets/AssetForm.tsx` — 56 (concept #1 form)
- (test files dominate the rest; details in §4)

---

## 2. Database schema impact

### Columns named `identifier` (concept #1)

Found on **five** tables — all with comment "Natural key/business identifier":

| Table | Migration | Constraint / Indexes |
|---|---|---|
| `organizations` | `000002_organizations.up.sql:10` | `UNIQUE`, `idx_organizations_identifier` |
| `locations` | `000005_locations.up.sql:9` | partial unique `locations_org_id_identifier_unique WHERE deleted_at IS NULL` (000031); `idx_locations_identifier`; **trigger `maintain_location_path` fires on `BEFORE INSERT OR UPDATE OF parent_location_id, identifier`** (000018:64) |
| `scan_devices` | `000006_scan_devices.up.sql:9` | `UNIQUE(org_id, identifier, valid_from)`, `idx_scan_devices_identifier` |
| `scan_points` | `000007_scan_points.up.sql:11` | `UNIQUE(org_id, identifier, valid_from)`, `idx_scan_points_identifier` |
| `assets` | `000008_assets.up.sql:9` | partial unique `assets_org_id_identifier_unique WHERE deleted_at IS NULL` (000031); `idx_assets_identifier` |

### Tables / columns named `identifier` (concept #2)

| Object | Migration |
|---|---|
| Table `identifiers` (FK to `assets` and `locations`; check constraint requires exactly one non-null) | `000009_identifiers.up.sql` |
| Indexes `idx_identifiers_org/asset/location/value/valid/type/active`; partial unique `identifiers_org_id_type_value_unique WHERE deleted_at IS NULL` (000032) | `000009`, `000032` |
| Hypertable `identifier_scans` + retention policy + `idx_identifier_scans_topic` | `000010_identifier_scans.up.sql` |
| FK column `asset_scans.identifier_scan_id` (loose link — can't FK to hypertable) | `000011_asset_scans.up.sql:10` |
| Trigger `trigger_process_identifier_scans` and function `process_identifier_scans()` | `000015_identifier_scans_trigger.up.sql` |
| PL/pgSQL `create_asset_with_identifiers(...)`, `create_location_with_identifiers(...)` | `000024_identifier_functions.up.sql` |

### Migration sequencing

#### Renaming **concept #1** (e.g. `identifier` → `code` or `external_id` on each entity table)

```
Per table (organizations, locations, scan_devices, scan_points, assets):
1. ALTER TABLE … RENAME COLUMN identifier TO <new>;            -- atomic, fast
2. Recreate unique constraints / partial unique indexes        -- index name changes
3. Recreate non-unique indexes                                 -- name changes
4. ALTER trigger functions to read NEW.<new>:
     - update_location_path (000018:43,46,56)                  -- ltree path source
     - process_identifier_scans (000015:13,16,30,43,52,77)     -- topic→org lookup, denorm joins
5. Drop & recreate PL/pgSQL functions referencing the old col:
     - create_asset_with_identifiers (000024:5,23,...)
     - create_location_with_identifiers (000024:53,71,...)
6. Update seed data (000016) — only relevant if re-running
7. Update RLS policies — currently use org_id, no rename needed
```

**Complications:**
- `locations.path` (ltree, NOT NULL) and the GENERATED column `locations.depth` are *populated* from `identifier` via the `update_location_path` trigger. Column rename does not require backfill (path is materialized; trigger only fires on future writes), but the trigger function source must be updated atomically with the rename or new inserts will fail.
- `process_identifier_scans` parses the MQTT topic against `organizations.identifier` (`migrations/000015:13`) and joins on `locations.identifier` / `scan_devices.identifier` / `assets.identifier` for auto-creation. Every line of the trigger function would change.
- Full-text search uses `ILIKE` against the column (e.g. `backend/internal/storage/assets.go:867–868`) — Go SQL strings need updating in lockstep with the migration.
- No materialized views or continuous aggregates touch the column. Verified `grep -E "MATERIALIZED VIEW|CREATE VIEW|continuous_aggregate" migrations/*.sql` — only `000018_location_ltree.up.sql` matched, and only via the `GENERATED ALWAYS AS (nlevel(path)) STORED` clause (which is path-based, not identifier-based directly).

#### Renaming **concept #2** (e.g. `identifiers` → `tags`, `identifier_scans` → `tag_scans`)

```
1. ALTER TABLE identifiers RENAME TO tags;                     -- atomic
2. Drop & recreate FK constraint names + index names:
     - identifiers_org_id_type_value_unique → tags_org_id_type_value_unique
     - idx_identifiers_* → idx_tags_*
3. ALTER TABLE identifier_scans RENAME TO tag_scans;
   - hypertable rename: works in TimescaleDB; retention policy auto-tracks
4. ALTER TABLE asset_scans RENAME COLUMN identifier_scan_id TO tag_scan_id;
5. DROP FUNCTION process_identifier_scans CASCADE; recreate as process_tag_scans
   (or rename via ALTER FUNCTION) — trigger must be re-pointed
6. DROP FUNCTION create_asset_with_identifiers; recreate as create_asset_with_tags
   (parameters and signature change visibly — used by Go storage layer)
7. Update Go SQL strings to point at new table name (storage/identifiers.go)
```

**Complications:**
- `identifier_scans` is a TimescaleDB hypertable with a retention policy. `ALTER TABLE … RENAME` works on hypertables but is worth confirming on a preview environment first.
- Function rename: PL/pgSQL function names are referenced from Go (`storage/assets.go`, `storage/locations.go`) and from the trigger registration. Coordinated change.
- Composite request DTOs `CreateAssetWithIdentifiersRequest` / `CreateLocationWithIdentifiersRequest` (§3) name a parameter that holds concept #2; same DTOs also embed concept #1 (`identifier` field). The DTO rename is independent of the table rename but logically should land together.

---

## 3. API surface impact

### OpenAPI hits (`docs/api/openapi.public.yaml`, 103 occurrences)

#### Concept #1 (natural key) — request/response fields and path params

- Path params on every entity-by-natural-key endpoint:
  - `/api/v1/assets/{identifier}` (lines 796, 847, 990, 1090, 1161)
  - `/api/v1/locations/{identifier}` and sub-resources (1436, 1493, 1636, 1702, 1768, 1834, 1905)
- Body/response fields:
  - `asset.CreateAssetWithIdentifiersRequest.identifier` (line 78)
  - `asset.PublicAssetView.identifier` (122)
  - `asset.UpdateAssetRequest.identifier` (159)
  - `location.CreateLocationWithIdentifiersRequest.identifier` (289), `parent_identifier` (306)
  - `location.PublicLocationView.identifier` (332)
  - `location.UpdateLocationRequest.identifier` (364), `parent_identifier` (377)
  - `inventory.SaveRequest.asset_identifiers` (265 — array of #1), `location_identifier` (272)
- Query params: `assets.list location` filter accepts a natural key (650); search prose mentions "name, identifier" repeatedly (663, 1309, 1989).

#### Concept #2 (physical device entity) — already partially renamed

- Schemas already named `shared.TagIdentifier` (557), `shared.TagIdentifierRequest` (579) — semantically clear.
- But `identifiers` array fields embed those: `CreateAssetWithIdentifiersRequest.identifiers` (81), `PublicAssetView.identifiers` (124), `CreateLocationWithIdentifiersRequest.identifiers` (294), `PublicLocationView.identifiers` (334).
- URL path segment `/identifiers` and path param `{identifierId}`:
  - `/api/v1/assets/{identifier}/identifiers` (1090)
  - `/api/v1/assets/{identifier}/identifiers/{identifierId}` (1161, with body schema `shared.TagIdentifierRequest` at 1107)
  - `/api/v1/locations/{identifier}/identifiers` (1834)
  - `/api/v1/locations/{identifier}/identifiers/{identifierId}` (1905)
- Operation IDs: `assets.identifiers.add`, `assets.identifiers.remove`, `locations.identifiers.add`, `locations.identifiers.remove`.
- Description prose already calls them "tag identifier (RFID EPC, BLE beacon ID, barcode, etc.)" (1093, 1837).

#### Concept #3

- Field `value` on `shared.TagIdentifier` and `shared.TagIdentifierRequest` (571, 589). **Not named `identifier` anywhere in the spec.**

### Breaking-change calculus

Per the prompt, "breaking" here = breaking our own internal frontend, since there are no external integrators yet.

| Rename | OpenAPI surface change | Internal frontend impact |
|---|---|---|
| #1 → e.g. `code` | `{identifier}` path params on ~10 routes; field renames on every Public/Update DTO; `parent_identifier`, `location_identifier`, `asset_identifiers` → coordinated | Touches every form, every list view, every cache, every e2e selector that uses the field |
| #2 → e.g. `tag` | URL path segment `/identifiers` → `/tags`; operation IDs; the `identifiers` array field on 4 DTOs → `tags`; schema names already partially OK | Touches the "Tag identifiers" modal, hooks for add/remove, fixtures that build assets with tags |
| Both | Sum of above | Sum of above |

---

## 4. Frontend impact

### TypeScript types

- Already partly migrated:
  - `TagIdentifier` (`frontend/src/types/shared/identifier.ts:17`)
  - `TagIdentifierInput` (`frontend/src/types/assets/index.ts:75`, `frontend/src/types/locations/index.ts:102`)
  - `IdentifierType` literal union (`frontend/src/types/shared/identifier.ts:11`) — currently `'rfid'`
- Still using bare `identifier`:
  - `Asset.identifier` (concept #1)
  - `Asset.identifiers: TagIdentifier[]` (concept #2)
  - `Location.identifier`, `Location.parent` (#1)
  - `Location.identifiers` (#2)
  - `LocationFilters.identifier`, `LocationSort.field === 'identifier'` (#1)
  - `AssetCache.byIdentifier`, `LocationCache.byIdentifier` (#1)

**Types are hand-authored, not generated.** No `openapi-typescript`/`openapi-generator` step — verified in subagent search. Means renames are manual but not riskier than Go.

### UI labels (English-language strings)

Mixed usage — and crucially, **UI labels can stay as "Identifier" even if the API field is renamed**, because labels are semantic strings, not identifiers tied to JSON keys.

| Where | Text | Concept | Could keep label after rename? |
|---|---|---|---|
| `AssetTable.tsx:23`, `AssetDetailsModal.tsx:89`, `AssetForm.tsx:290` | "Asset ID" | #1 | Yes — already says "ID" |
| `AssetSearchSort.tsx:14`, `LocationSearchSort.tsx:11` | "Identifier" | #1 | Yes — English word, decoupled from API |
| `LocationTable.tsx:24`, `LocationForm.tsx:302` | "Identifier" | #1 | Yes |
| `LocateScreen.tsx` | "Tag EPC Identifier" | #3 (concept #3 in user-facing copy, even though API field is `value`) | n/a — no rename target |
| `TagIdentifiersModal.test.tsx` | "Tag identifier(s)" | #2 | Yes — already disambiguated |

### API client + form validation

- `frontend/src/lib/api/assets/index.ts:102–112` — `addIdentifier()` / `removeIdentifier()` hit the `/identifiers` sub-resource (concept #2)
- `frontend/src/lib/api/locations/index.ts:144–159` — same pattern
- `frontend/src/lib/location/validators.ts:~14` — `validateIdentifier()` enforces alphanumeric + `-_` (concept #1, regex on the natural key string)
- `AssetForm.tsx:214–216` — inline regex for #1
- No Zod/Yup; validation is procedural

### Tests

- 114 unit-test files; 56 e2e (Playwright) test files. Cumulative test occurrences ≈ 600 lines.
- Playwright selectors use `input#identifier` and label text "Identifier" — these break on UI rename only if the *label/HTML id* changes, not on API rename.
- e2e fixtures (`frontend/tests/e2e/fixtures/location.fixture.ts:28`) build payloads with `identifier`, `parent_identifier` — these break on API rename of #1.

---

## 5. Effort estimate per option

Effort buckets: **S** = 1–2 days, **M** = 3–7 days, **L** = 7+ days.

### Option A — Rename concept #1 (natural key) only

Candidate names: `code`, `external_id`, `slug`, `nat_key`, `name_key`. All have downsides; `external_id` is the clearest semantic for the "join key back to upstream system" framing in the prompt, though it implies external-only when in fact the value is also internal-facing.

- **Effort: L (7–10 days)**
- **Files touched: ~80–100** — 5 entity tables + every Go model + every storage query + every handler + router + 5 frontend type files + every form + cache stores + every e2e fixture
- **Migrations: ~10** — 5 column renames + index/constraint recreations + 4 PL/pgSQL function rewrites (`create_asset_with_identifiers`, `create_location_with_identifiers`, `process_identifier_scans`, `update_location_path`) + ltree trigger source update
- **Risk factors:**
  - `update_location_path` and `maintain_location_path` trigger must be updated atomically with the column rename (else inserts fail)
  - `process_identifier_scans` references the column on **four** tables (`organizations`, `locations`, `scan_devices`, `assets`) — single function, but a lot of sed-style changes that need to be exactly right
  - Generated swagger (`backend/docs/docs.go`, 114 hits) must be regenerated
  - Public OpenAPI schema field renames cascade into hand-written frontend types
  - Heavy Playwright e2e fixture rewrites — fixtures break silently if a stale field name is sent (server returns 400 but tests may report misleading errors)
- **Residual ambiguity:** **None on the rename target.** Concept #1 becomes `code` (or whatever); `identifier` keeps meaning #2 unambiguously. UI labels can keep saying "Identifier" because the English word is now free.
- **Surprise:** Concept #1 is on **five** tables — bigger than just the asset record framing in the prompt suggested. This is the dominant cost driver.

### Option B — Rename concept #2 (physical device entity) only

Candidate names from prompt: `tag`, `marker`, `token`. **`tag` is far ahead** because:
- Public OpenAPI schemas are already `TagIdentifier` / `TagIdentifierRequest`
- Frontend types are already `TagIdentifier` / `TagIdentifierInput`
- DB column comment in `identifiers.up.sql:9` calls them "tags" colloquially ("'rfid', 'ble', ...")
- "Tag" matches industry usage (RFID tag, BLE beacon tag, barcode tag)
- `marker` is generic and overloaded; `token` is auth-adjacent and confusable with bearer tokens.

- **Effort: M (3–5 days)**
- **Files touched: ~30–50** — 1 backend storage file, 2 handlers, ~5 model files, ~5 frontend type files, 1 modal component, fixtures, e2e tests, OpenAPI annotations on Go handler comments
- **Migrations: 1–2** — table renames (`identifiers` → `tags`; `identifier_scans` → `tag_scans`); FK column rename on `asset_scans.identifier_scan_id` → `tag_scan_id`; PL/pgSQL function renames + body updates (3 functions); index/constraint name updates
- **Risk factors:**
  - TimescaleDB hypertable rename: needs a preview-environment smoke test first to confirm rename + retention policy survive cleanly
  - Trigger `trigger_process_identifier_scans` must be dropped + recreated against new function name
  - `CreateAssetWithIdentifiersRequest` and `CreateLocationWithIdentifiersRequest` should rename to `CreateAssetWithTagsRequest` etc. — Go type rename + OpenAPI schema rename + frontend type rename
  - URL path segment `/identifiers` → `/tags` is a public-API breaking change to the spec — but per the prompt, no external integrators yet, so this is internal-frontend only. NADA already partially knows about both forms (`/by-id/{id}/identifiers` and `/by-identifier/{identifier}/identifiers`)
- **Residual ambiguity:** **None.** Concept #1 keeps being `identifier` (matches industry "natural key" naming convention); concept #2 becomes `tag`. Concept #3 stays `value`. The route `/api/v1/assets/{identifier}/tags/{tagId}` reads cleanly.
- **Bonus:** This option *finishes a migration that's already half-done.* `TagIdentifier` was clearly an interim step.

### Option C — Rename both

Pick distinct names for both. E.g. `identifier` (#1) → `code`, `identifier` (#2) → `tag`.

- **Effort: L+ (10–14 days)** — sum of A and B but with some economies (single coordinated cutover, single CHANGELOG entry, single docs sweep)
- **Files touched: ~120–150**
- **Migrations: ~12**
- **Risk factors:** All risks from A and B compound. Two simultaneous renames make code review harder and increase the chance of stale references slipping through.
- **Residual ambiguity:** None.
- **Verdict:** Maximum disambiguation, but pays for the win on concept #1 — which industry convention already calls `identifier` without confusion in most schemas.

---

## 6. Recommendation

**Option B looks like the lowest-cost path.** Evidence from §1–5:

1. **Half the work is already done.** OpenAPI schemas are `TagIdentifier` / `TagIdentifierRequest` (`docs/api/openapi.public.yaml:557,579`). Frontend types are `TagIdentifier` / `TagIdentifierInput` (`frontend/src/types/shared/identifier.ts:17`). The internal Go and DB layers haven't caught up. Option B finishes a migration; Options A and C start a new one.

2. **Concept #2's surface is contained.** The `identifiers` table, its functions, and its handlers are largely silo'd in `backend/internal/storage/identifiers.go` (91 hits in one file). Concept #1's surface is fragmented across five tables and dozens of route paths.

3. **Concept #1's "identifier" is consistent with industry convention.** "Natural key column called `identifier`" is a standard pattern (e.g., k8s `metadata.name`, many ERP systems). Renaming it to `code` or `external_id` invites its own ambiguity (`code` collides with status codes, `external_id` implies external-only when the value is also internal-facing). The status quo is not painful here.

4. **Concept #3 is a non-issue.** The third "concept" the prompt described — the literal device value — is already named `value` in DB, Go, OpenAPI, and TS. There is no rename work needed for it, regardless of which other option is chosen. This is the biggest framing surprise: the problem is binary, not ternary.

5. **The most user-facing collision is exactly what Option B fixes.** The URL `/api/v1/assets/{identifier}/identifiers/{identifierId}` (concept #1 then concept #2 then concept #2-row-id) becomes `/api/v1/assets/{identifier}/tags/{tagId}` — clean.

### Tradeoffs / dissenting evidence

- Option A would let UI strings like "Identifier" remain natural English without confusion against the entity #2 — but UI labels can already disambiguate independently (see §4 — labels are decoupled from API field names). So this is not a strong argument.
- Option B preserves the term `identifier` on the natural-key column, which means in casual conversation "the asset identifier" remains shorthand for #1. Some people may want #1 to *not* be `identifier` so the word is freed up. That's a taste call, not an evidence-driven one.
- Option C is justifiable only if Tim has strong product reasons to mint a public-facing "external id" / "customer code" surface independent of the rename question. If that's already a product line decision, fold it in here. Otherwise it's overkill.

### Surprises worth surfacing

- **The "three concepts" framing in TRA-523 doesn't match the code.** Concept #3 doesn't collide. The right way to describe the problem is: "We have one table (`identifiers`) whose name overlaps with a column (`identifier`) that appears on five other tables. The column's value is what users type/see; the table's rows hold device records that point at those entities."
- **The natural-key column convention is project-wide, not asset-specific.** Mike's prompt described concept #1 as "attribute on the asset record" — but the same column is on locations, scan devices, scan points, and orgs. This is the dominant cost driver for Option A.
- **`TagIdentifier` already exists.** This rename has already been chosen and partially landed in the public API and frontend. Option B is the path of completing what's already started, not picking a fresh direction. If there's a record of *why* the rename stopped at the public boundary, that history would inform whether resuming is straightforward or whether there was a deliberate reason to stop.

---

## Appendix: file-level pointers

Migrations (all under `backend/migrations/`):
- Concept #1 column definitions: `000002_organizations.up.sql:10`, `000005_locations.up.sql:9`, `000006_scan_devices.up.sql:9`, `000007_scan_points.up.sql:11`, `000008_assets.up.sql:9`
- Concept #2 table + hypertable + functions: `000009_identifiers.up.sql`, `000010_identifier_scans.up.sql`, `000015_identifier_scans_trigger.up.sql`, `000024_identifier_functions.up.sql`
- Cross-reference / FK: `000011_asset_scans.up.sql:10` (`identifier_scan_id BIGINT` — loose link to hypertable)
- Trigger source for ltree: `000018_location_ltree.up.sql:43,46,56,64`
- Partial unique indexes: `000031_unique_identifier_partial.up.sql` (#1), `000032_unique_identifiers_partial.up.sql` (#2)

Backend Go (`backend/internal/`):
- Models: `models/shared/identifier.go:5,12` (concept #2), `models/asset/asset.go:14,68`, `models/location/location.go:15,74`, `models/organization/organization.go:10`
- Storage: `storage/identifiers.go` (concept #2, 91 hits, single file)
- Handlers: `handlers/assets/assets.go`, `handlers/locations/locations.go` (mixed)
- Router: `cmd/serve/router.go:138–196`

Frontend (`frontend/src/`):
- Types: `types/shared/identifier.ts:11,17`, `types/assets/index.ts:75`, `types/locations/index.ts:102`
- Forms: `components/assets/AssetForm.tsx`, `components/locations/LocationForm.tsx`
- API client: `lib/api/assets/index.ts:102–112`, `lib/api/locations/index.ts:144–159`
- Validators: `lib/location/validators.ts`

OpenAPI:
- `docs/api/openapi.public.yaml` — concept #2 schemas at `:557,579`; concept #1 path params throughout; full grep mapping in §3

Schema docs:
- `docs/logical-schema.md`, `docs/schema-naming-conventions.md` — describe both concepts in prose; would need a section update under any option.

---

## Addendum — rename history

**Earliest TagIdentifier introduction:** `7022a67d` — 2025-12-16 — `backend/internal/models/shared/identifier.go` (also adds `migrations/000024_identifier_functions.up.sql`, leaves table `identifiers` unchanged).

**Commit message:**
> `feat(storage): add transactional asset/location with identifiers (TRA-214)`
> "Add PostgreSQL functions for atomic asset/location + identifiers creation. Add TagIdentifier and TagIdentifierRequest shared models. Add AssetView model with embedded identifiers."

**PR context:** PR #89 (TRA-214 Step 1, merged 2025-12-16), title `feat(storage): transactional asset/location with identifiers (TRA-214 Step 1)`. PR #90 (Step 2, 2025-12-19) added handlers using URL path `/identifiers` (NOT `/tags`) — see PR #90's "New API Endpoints" table. Neither PR description frames `TagIdentifier` as a rename — both use "identifiers" and "tag identifiers" as parallel terms, not as old-name/new-name.

**DB migration co-landed?:** **No.** PR #89 added `migrations/000024_identifier_functions.up.sql` (named `create_asset_with_identifiers`, `create_location_with_identifiers`); the `identifiers` table itself was untouched. Subsequent migrations (`000031`, `000032`) continue using `identifiers` table name through 2026-04.

**Later halt or reversal?:** No halt. Strong evidence of *active maintenance of the dual concept*:
- PR #101 (TRA-215, 2026-01-13), title `feat(inventory): match RFID tags via identifiers.value instead of assets.identifier`, body: *"Follow-up to TRA-193 (Asset CRUD - separate customer identifier from tag identifiers)"*.
- Commit `9983ce2d` (2026-04-19): `fix(tra-396): public asset field uses assets.identifier (natural key), not tag identifier`.
- PR #163 (TRA-396, 2026-04-20) Known-Deviation #3: *"PublicCurrentLocationItem.asset now sources from `assets.identifier` (natural key) rather than the legacy tag-identifier join... Consistent with TRA-193's asset-identifier separation."*

**Verdict: scenario (b) — deliberate boundary.** TRA-193 (referenced in two later PRs as "Asset CRUD — separate customer identifier from tag identifiers") established the two-concept design *as a settled architectural decision*. `TagIdentifier` was introduced to disambiguate the Go model from the natural-key field, not as the first step of a table rename. The team has cited "TRA-193's asset-identifier separation" in code review as recently as 2026-04-19. The Phase 1 framing of "half-done rename" was imprecise — what's actually shipped is "two concepts coexist; qualifier-prefixing ('customer identifier' / 'tag identifier') was the chosen disambiguation strategy."

**What this means for the recommendation:** Option B is still the cheapest option, but it should be reframed as *"finish the verbal disambiguation by collapsing 'tag identifier' to just 'tag' in DB + URL + internal Go"* — not as "complete a stalled rename." Worth confirming with whoever owned TRA-193 (likely Mike, given commit author patterns) that they're comfortable graduating from "qualifier-prefix" to "distinct names" before kicking off Option B execution. If TRA-193's design rationale was "we explicitly want both concepts called 'identifier' with qualifiers", Option B contradicts it; if the rationale was "we needed to disambiguate and chose prefixing as the cheapest path at the time," Option B is the natural next step.
