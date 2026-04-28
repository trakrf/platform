# TRA-525 — Frontend cleanup: complete the `Identifier` → `Tag` rename for concept-#2 surfaces

## Problem

TRA-524 shipped the API/DB cutover for the `Identifier` entity → `Tag` rename: PostgreSQL tables, Go internals, URL paths, MQTT, frontend API client (`addTag`/`removeTag`/`/tags` URLs), `Asset.tags` / `Location.tags` JSON fields, and the `TagType` union widening to `'rfid' | 'ble' | 'nfc' | 'barcode'`. UI labels, component file/symbol names, TypeScript type names, and component-internal state were deliberately deferred so the cutover PR stayed reviewable and the e2e suite had a clean rollback boundary.

E2E passed against TRA-524 on preview, so this ticket finishes the rename in the parts of the frontend that didn't move with the API cutover. Two facts shape the design:

1. **Concept-#1 (`identifier` as customer natural key) is unchanged.** Field names like `Asset.identifier`, `Location.identifier`, `parent_identifier`, helpers like `validateIdentifier` / `getLocationByIdentifier`, and UI labels like the `LocationForm` "Identifier *" input stay. The natural-key column convention lives on five tables and is industry-standard.
2. **Backend Go type `TagIdentifier` stays.** TRA-524 left the Go struct name as a "minor call." Frontend can diverge — JSON keys are what cross the wire, and those are already `tags`.

## Approach

Single PR, mechanical rename, no behavior change. Concept-#2 ("physical tag entity") surfaces in the frontend rename to `Tag` / `Tags` consistently across types, components, props, state, labels, and tests.

Worktree on `feat/tra-525-frontend-tag-rename`. Sequenced solo (per "sequence worktree merges, never parallel" rule). Standard merge commit, no squash.

## Type renames

`src/types/shared/tag.ts`:
- `interface TagIdentifier` → `interface Tag`
- File comment updated to drop the "Go type name kept" note (it isn't kept on the frontend anymore)

`src/types/assets/index.ts`:
- `interface TagIdentifierInput` → `interface TagInput`
- `Asset.tags: TagIdentifier[]` → `Asset.tags: Tag[]`
- Imports of `TagIdentifier` → `Tag`

`src/types/locations/index.ts`:
- `interface TagIdentifierInput` → `interface TagInput`
- `Location.tags?: TagIdentifier[]` → `Location.tags?: Tag[]`
- Imports of `TagIdentifier` → `Tag`

`src/types/index.ts`:
- Re-export updated: `export type { Tag, TagType } from './shared'`

All call sites that import `TagIdentifier` / `TagIdentifierInput` (~25 files across `components/`, `hooks/`, `stores/`, `lib/`, `utils/`) update their imports and any explicit type annotations.

## Component renames

`src/components/assets/`:

| Old file/symbol | New file/symbol |
|---|---|
| `TagIdentifiersModal.tsx` / `TagIdentifiersModal` | `TagsModal.tsx` / `TagsModal` |
| `TagIdentifiersModal.test.tsx` | `TagsModal.test.tsx` |
| `TagIdentifierList.tsx` / `TagIdentifierList` | `TagList.tsx` / `TagList` |
| `TagIdentifierInputRow.tsx` / `TagIdentifierInputRow` | `TagInputRow.tsx` / `TagInputRow` |
| `TagIdentifierInputRow.test.tsx` | `TagInputRow.test.tsx` |
| `TagIdentifierHeader` (internal in TagList file) | `TagHeader` |
| `TagIdentifierRow` (exported from TagList file) | `TagRow` |
| `TagIdentifierListProps` / `TagIdentifierRowProps` | `TagListProps` / `TagRowProps` |
| `TagIdentifiersModalProps` | `TagsModalProps` |

Barrel `src/components/assets/index.ts` updated to export the renamed symbols.

External callers in `src/components/locations/` (`LocationDetailsPanel.tsx`, `LocationCard.tsx`, `LocationDetailsModal.tsx`, `LocationFormModal.tsx`, `LocationForm.tsx`) update their imports.

## Component-internal renames (concept #2 only)

Inside `AssetForm.tsx`, `LocationForm.tsx`, `AssetFormModal.tsx`, `LocationFormModal.tsx`, `TagsModal.tsx`, `TagList.tsx`:

- State: `tagIdentifiers` / `setTagIdentifiers` → `tagInputs` / `setTagInputs`
- Local pre-submit filter: `validIdentifiers` → `validTags`
- Props on `TagsModal` / `TagList`: `identifiers` → `tags`
- Prop `onIdentifierRemoved` → `onTagRemoved`
- Handler `handleIdentifierRemoved` → `handleTagRemoved`
- Param `identifierId` → `tagId`
- Loop variables (`identifiers.map((identifier) => …)`) update to `tags.map((tag) => …)` where the entity is concept #2

Concept-#1 variables (`getLocationByIdentifier`, `validateIdentifier`, `formData.identifier`, `parent_identifier`) are not touched.

## UI label updates (concept #2 only)

- `LocationForm.tsx:462` section heading: `"Tag Identifiers"` → `"Tags"`
- `LocationForm.tsx:526` empty-state copy: `"No tag identifiers added. Click \"Add Tag\" to link RFID tags."` → `"No tags added. Click \"Add Tag\" to link RFID tags."`
- JSX comments referencing the renamed components/sections: `{/* Tag Identifiers */}` → `{/* Tags */}`, `{/* Tag Identifiers Modal */}` → `{/* Tags Modal */}` (LocationCard, LocationDetailsPanel, LocationDetailsModal)
- `AssetForm.tsx` already uses "Tags" / "Add Tag" — no copy change

The modal's heading already reads "RFID Tags" — left as-is; the `TAG_TYPE_LABELS` map will continue to display per-tag-type badges, and the heading is intentionally narrower than the type union.

## UI label NON-changes (concept #1)

These are explicitly NOT renamed:

- `AssetSearchSort.tsx:14` — `{ value: 'identifier', label: 'Identifier' }`
- `LocationSearchSort.tsx:11` — same
- `LocationTable.tsx:24` — `{ key: 'identifier', label: 'Identifier', sortable: true }`
- `LocationForm.tsx:299-318` — `htmlFor="identifier"`, `id="identifier"`, label `"Identifier *"`, error keyed under `identifier`
- `LocationDetailsPanel.tsx:149` — `Identifier` detail label above `{location.identifier}`
- `LocateScreen.tsx:203` — `"Tag EPC Identifier"` (already disambiguated)

## Tests

**Unit tests:** Renamed alongside their source files (see Component renames table). `describe`/`it` strings that explicitly name the renamed component or use concept-#2 wording (e.g., `'TagIdentifiersModal renders identifiers'`) update to match (`'TagsModal renders tags'`). Tests in `assetStore.test.ts`, `locationStore.test.ts`, `tagStore.test.ts`, `assets.test.ts`, etc. that use `TagIdentifier` / `TagIdentifierInput` in type annotations or fixture comments update those references.

**E2E (`tests/e2e/`):** B-lite — fix what the rename breaks plus obvious concept-#2 wording I encounter, but no comprehensive sweep.

- Grep for selectors targeting renamed UI strings (`text=Tag Identifiers`, `getByRole('heading', { name: 'Tag Identifiers' })`, etc.) and update.
- Grep for `aria-label*='identifier'` or `aria-label*='Identifier'` targeting concept-#2 elements (e.g., the modal's "Remove tag" button — already says tag — likely no hits).
- While in each file, fix obvious describe/it strings that refer to the tag entity using "identifier" wording. Don't audit comprehensively.
- Concept-#1 e2e references (`location.identifier`, `parent_identifier`, `LOC-XXX`, `input#identifier` in location forms, `data: 'identifier'` sort fixtures) stay.

**Verification:** `pnpm validate` (typecheck + lint + unit) must pass before opening PR. E2E runs against preview after PR opens (preview-deploy workflow).

## Out of scope

- Backend Go type `TagIdentifier` (TRA-524 left as a future call)
- Renaming `Asset.identifier` / `Location.identifier` / `parent_identifier` / `validateIdentifier` / `getLocationByIdentifier` (concept #1)
- Restructuring `TagList.tsx` (currently houses `TagHeader` + `TagRow` + `TagList`) — file rename only, no split
- Behavior changes
- Re-litigating naming decisions made in TRA-524 / TRA-523

## PR shape

- Branch: `feat/tra-525-frontend-tag-rename`
- Worktree: `.worktrees/tra-525-frontend-tag-rename`
- Single PR
- Standard merge commit (no squash)
- Sequenced solo, not parallel with other worktree PRs
