# TRA-525 Frontend Tag Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the `Identifier` → `Tag` rename arc on the frontend by renaming concept-#2 (physical-tag entity) types, components, props, state, and labels. Concept-#1 (`identifier` natural-key) surfaces stay unchanged.

**Architecture:** Pure mechanical rename, no behavior change. Single PR off `main` in worktree `.worktrees/tra-525-frontend-tag-rename`, branch `feat/tra-525-frontend-tag-rename`. Each task isolates one rename axis (one type, one component, one prop family) so each commit type-checks and tests pass — no half-broken intermediate states.

**Tech Stack:** React 18 + TypeScript (strict), Vitest unit tests, Playwright e2e, pnpm, Just task runner. Project root: `/home/mike/platform/.worktrees/tra-525-frontend-tag-rename`. Frontend root: `frontend/`.

**Spec:** `docs/superpowers/specs/2026-04-27-tra-525-frontend-tag-rename-design.md`

**Verification convention:** Pure renames have no new behavior to test. Each task's "verification" is `just frontend typecheck` (must pass) + relevant `pnpm test` invocation showing existing tests stay green. The full `just validate` runs only at the end.

---

## Task 1: Rename type `TagIdentifier` → `Tag`

**Files:**
- Modify: `frontend/src/types/shared/tag.ts`
- Modify: `frontend/src/types/index.ts`
- Modify (import + annotation update only): `frontend/src/types/assets/index.ts`, `frontend/src/types/locations/index.ts`, `frontend/src/components/assets/AssetDetailsModal.tsx`, `frontend/src/components/assets/LocateTagPopover.tsx`, `frontend/src/components/locations/LocationDetailsPanel.tsx`, `frontend/src/components/assets/TagIdentifierList.tsx`, `frontend/src/components/assets/TagCountBadge.tsx`, `frontend/src/components/locations/LocationCard.tsx`, `frontend/src/components/assets/TagIdentifiersModal.tsx`, `frontend/src/components/assets/TagIdentifiersModal.test.tsx`, `frontend/src/components/assets/AssetCard.tsx`, `frontend/src/components/locations/LocationDetailsModal.tsx`

- [ ] **Step 1: Rename the interface in the type source file**

Edit `frontend/src/types/shared/tag.ts`. Replace the entire file contents with:

```typescript
/**
 * Tag Types
 *
 * Types for physical-tag entities (RFID, BLE, NFC, barcode) linked to assets and locations.
 * Matches backend: backend/internal/models/shared/tag.go
 */

/**
 * Tag type — supported physical-tag technologies
 */
export type TagType = 'rfid' | 'ble' | 'nfc' | 'barcode';

/**
 * Tag entity — returned from API
 * Reference: backend/internal/models/shared/tag.go
 */
export interface Tag {
  id: number;
  type: TagType;
  value: string;
  is_active: boolean;
}
```

- [ ] **Step 2: Update the re-export barrel**

Edit `frontend/src/types/index.ts`. Find:

```typescript
// Re-export shared types
export type { TagIdentifier, TagType } from './shared';
```

Replace with:

```typescript
// Re-export shared types
export type { Tag, TagType } from './shared';
```

- [ ] **Step 3: Sweep all remaining `TagIdentifier` references with one find/replace**

The remaining files only use `TagIdentifier` as an imported type name or annotation. Replace `TagIdentifier` (word boundary) with `Tag` everywhere except `TagIdentifierInput` (handled in Task 2) and the component file/symbol names `TagIdentifierList` / `TagIdentifiersModal` / `TagIdentifierInputRow` / `TagIdentifierRow` / `TagIdentifierHeader` (handled in Tasks 3–5).

Run from project root:

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
# Replace `TagIdentifier[` → `Tag[`, `TagIdentifier ` → `Tag `, `TagIdentifier>` → `Tag>`, `TagIdentifier,` → `Tag,`, `TagIdentifier;` → `Tag;`, and bare `TagIdentifier\n` (end-of-line in import lists)
# but NOT `TagIdentifierInput`, `TagIdentifierList`, `TagIdentifiersModal`, `TagIdentifierInputRow`, `TagIdentifierRow`, `TagIdentifierHeader`
git grep -l '\bTagIdentifier\b' frontend/src | xargs perl -i -pe 's/\bTagIdentifier\b(?!Input|List|sModal|InputRow|Row|Header)/Tag/g'
```

After running, sanity-check no concept-#2 callers were missed:

```bash
git grep -n '\bTagIdentifier\b' frontend/src
```

Expected output — only matches that should remain:
- File comments mentioning `TagIdentifier` historically (none expected — Step 1 already cleaned tag.ts)
- The component symbols: `TagIdentifierList`, `TagIdentifiersModal`, `TagIdentifierInputRow`, `TagIdentifierRow`, `TagIdentifierHeader` (and Props variants)
- `TagIdentifierInput` (Task 2)

If any other naked `TagIdentifier` remains, edit those files manually.

- [ ] **Step 4: Verify typecheck passes**

```bash
just frontend typecheck
```

Expected: PASS, zero errors.

- [ ] **Step 5: Run unit tests**

```bash
just frontend test
```

Expected: 1128 passing, 26 skipped, 0 failures (matches baseline).

- [ ] **Step 6: Commit**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
git add frontend/src
git commit -m "$(cat <<'EOF'
refactor(frontend): rename TagIdentifier type → Tag (TRA-525)

Concept-#2 type rename. JSON wire format unchanged (already `tags`).
Component file/symbol renames follow in subsequent commits.
EOF
)"
```

---

## Task 2: Rename type `TagIdentifierInput` → `TagInput`

**Files:**
- Modify: `frontend/src/types/assets/index.ts`, `frontend/src/types/locations/index.ts`, `frontend/src/components/locations/LocationFormModal.tsx`, `frontend/src/components/assets/AssetFormModal.tsx`, `frontend/src/components/locations/LocationForm.tsx`, `frontend/src/components/assets/AssetForm.tsx`

- [ ] **Step 1: Rename interface in `frontend/src/types/assets/index.ts`**

Find:

```typescript
/**
 * Tag identifier input for forms - may not have an id if new
 */
export interface TagIdentifierInput {
  id?: number; // Present if existing identifier, undefined if new
  type: 'rfid';
  value: string;
}
```

Replace with:

```typescript
/**
 * Tag input for forms — may not have an id if new
 */
export interface TagInput {
  id?: number; // Present if existing tag, undefined if new
  type: 'rfid';
  value: string;
}
```

- [ ] **Step 2: Rename interface in `frontend/src/types/locations/index.ts`**

Find the `TagIdentifierInput` interface (analogous shape) and rename it to `TagInput`. Update the comment if it says "Tag identifier input for forms" → "Tag input for forms".

- [ ] **Step 3: Sweep all remaining `TagIdentifierInput` references**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
git grep -l '\bTagIdentifierInput\b' frontend/src | xargs perl -i -pe 's/\bTagIdentifierInput\b/TagInput/g'
```

Verify nothing remains:

```bash
git grep -n '\bTagIdentifierInput\b' frontend/src
```

Expected: zero matches.

- [ ] **Step 4: Verify typecheck**

```bash
just frontend typecheck
```

Expected: PASS, zero errors.

- [ ] **Step 5: Run unit tests**

```bash
just frontend test
```

Expected: 1128 passing, 26 skipped, 0 failures.

- [ ] **Step 6: Commit**

```bash
git add frontend/src
git commit -m "$(cat <<'EOF'
refactor(frontend): rename TagIdentifierInput type → TagInput (TRA-525)

Concept-#2 form-input type rename. No JSON wire change.
EOF
)"
```

---

## Task 3: Rename component `TagIdentifiersModal` → `TagsModal` (file, symbol, props, callers)

**Files:**
- Rename: `frontend/src/components/assets/TagIdentifiersModal.tsx` → `frontend/src/components/assets/TagsModal.tsx`
- Rename: `frontend/src/components/assets/TagIdentifiersModal.test.tsx` → `frontend/src/components/assets/TagsModal.test.tsx`
- Modify: `frontend/src/components/assets/index.ts` (barrel)
- Modify (import + JSX usage): `frontend/src/components/assets/AssetCard.tsx`, `frontend/src/components/locations/LocationCard.tsx`

- [ ] **Step 1: Rename the source file with `git mv`**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
git mv frontend/src/components/assets/TagIdentifiersModal.tsx frontend/src/components/assets/TagsModal.tsx
git mv frontend/src/components/assets/TagIdentifiersModal.test.tsx frontend/src/components/assets/TagsModal.test.tsx
```

- [ ] **Step 2: Rename symbols, props, and concept-#2 names inside the moved source file**

Edit `frontend/src/components/assets/TagsModal.tsx`. Apply these substitutions in order:

- `TagIdentifiersModalProps` → `TagsModalProps`
- `TagIdentifiersModal` → `TagsModal`
- Prop name `identifiers` → `tags` in the props interface and destructured params (lines ~14, ~28, plus the JSX block currently `{identifiers.map((identifier) => …)}` becomes `{tags.map((tag) => …)}` and `identifiers.length === 0` → `tags.length === 0`)
- Loop variable: rename `identifier` → `tag` inside the `.map(...)` body and replace `identifier.id` / `identifier.type` / `identifier.value` / `identifier.is_active` with `tag.id` / `tag.type` / `tag.value` / `tag.is_active`
- Prop `onIdentifierRemoved` → `onTagRemoved` (interface and destructured)
- Inside `onIdentifierRemoved?.(confirmingId)` → `onTagRemoved?.(confirmingId)`
- `canRemove = entityId !== undefined && onIdentifierRemoved !== undefined` → `canRemove = entityId !== undefined && onTagRemoved !== undefined`
- Param `identifierId: number` → `tagId: number` in `handleRemoveClick`, `setConfirmingId(identifierId)` → `setConfirmingId(tagId)`
- aria-label and copy: `"No RFID tags linked to this {entityType}."` stays. `aria-label={\`Locate tag ${identifier.value}\`}` becomes `aria-label={\`Locate tag ${tag.value}\`}`. `aria-label="Remove tag"` stays.

Run a final sanity grep — there should be zero matches:

```bash
git grep -n 'identifier' frontend/src/components/assets/TagsModal.tsx
```

Expected: zero matches.

- [ ] **Step 3: Update the test file (rename + props/symbols)**

Edit `frontend/src/components/assets/TagsModal.test.tsx`. Apply:

- Import: `import { TagIdentifiersModal } from './TagIdentifiersModal'` → `import { TagsModal } from './TagsModal'` (or whatever path-style is used — verify exact line)
- All JSX `<TagIdentifiersModal …>` → `<TagsModal …>`
- All prop usages `identifiers={…}` → `tags={…}`, `onIdentifierRemoved={…}` → `onTagRemoved={…}`
- Variable `mockIdentifiers: TagIdentifier[]` is already `mockIdentifiers: Tag[]` after Task 1; rename the variable to `mockTags` for consistency
- Describe/it strings: `'TagIdentifiersModal'` → `'TagsModal'`. `'does not show remove buttons when onIdentifierRemoved is not provided'` → `'does not show remove buttons when onTagRemoved is not provided'`. `'shows remove buttons when both entityId and onIdentifierRemoved are provided'` → `'shows remove buttons when both entityId and onTagRemoved are provided'`. Any other prose referencing "identifier" as the tag entity → "tag".

Verify no stale `identifier` left in the test file (that refers to the tag entity):

```bash
git grep -n 'identifier\|Identifier' frontend/src/components/assets/TagsModal.test.tsx
```

Expected: zero matches (or only matches inside string fixtures that legitimately mean concept #1, none expected here).

- [ ] **Step 4: Update the barrel export `frontend/src/components/assets/index.ts`**

Find:

```typescript
export { TagIdentifiersModal } from './TagIdentifiersModal';
```

Replace with:

```typescript
export { TagsModal } from './TagsModal';
```

- [ ] **Step 5: Update callers — `AssetCard.tsx` and `LocationCard.tsx`**

Edit `frontend/src/components/assets/AssetCard.tsx`:
- Replace `TagIdentifiersModal` with `TagsModal` (import line + JSX usage, two JSX sites)
- Replace prop `identifiers={localTags}` with `tags={localTags}` on each `<TagsModal>` usage
- Replace prop `onIdentifierRemoved={handleIdentifierRemoved}` with `onTagRemoved={handleIdentifierRemoved}` (handler rename comes in Task 6 — for now only the prop name changes)
- Replace JSX comment `{/* Tag Identifiers Modal */}` with `{/* Tags Modal */}`

Edit `frontend/src/components/locations/LocationCard.tsx`:
- Same set of replacements as AssetCard (`TagIdentifiersModal` → `TagsModal`, prop renames, comment update)
- Two JSX sites for the modal in this file too

- [ ] **Step 6: Verify typecheck**

```bash
just frontend typecheck
```

Expected: PASS, zero errors.

- [ ] **Step 7: Run unit tests**

```bash
just frontend test
```

Expected: 1128 passing, 26 skipped, 0 failures.

- [ ] **Step 8: Commit**

```bash
git add frontend/src
git commit -m "$(cat <<'EOF'
refactor(frontend): rename TagIdentifiersModal component → TagsModal (TRA-525)

File, symbol, props (identifiers→tags, onIdentifierRemoved→onTagRemoved),
test file, barrel, callers in AssetCard/LocationCard.
EOF
)"
```

---

## Task 4: Rename component `TagIdentifierList` → `TagList` (plus internal `TagIdentifierHeader`/`TagIdentifierRow`)

**Files:**
- Rename: `frontend/src/components/assets/TagIdentifierList.tsx` → `frontend/src/components/assets/TagList.tsx`
- Modify: `frontend/src/components/assets/index.ts` (barrel)
- Modify (import + JSX): `frontend/src/components/assets/AssetDetailsModal.tsx`, `frontend/src/components/assets/AssetCard.tsx`, `frontend/src/components/locations/LocationDetailsPanel.tsx`, `frontend/src/components/locations/LocationDetailsPanel.test.tsx`, `frontend/src/components/locations/LocationDetailsModal.tsx`

- [ ] **Step 1: Rename the source file**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
git mv frontend/src/components/assets/TagIdentifierList.tsx frontend/src/components/assets/TagList.tsx
```

- [ ] **Step 2: Rename symbols, props, and concept-#2 names inside the moved file**

Edit `frontend/src/components/assets/TagList.tsx`. Apply these substitutions:

- `TagIdentifierListProps` → `TagListProps`
- `TagIdentifierList` → `TagList` (export + function decl)
- `TagIdentifierHeader` → `TagHeader` (internal helper, both decl and call sites)
- `TagIdentifierRowProps` → `TagRowProps`
- `TagIdentifierRow` → `TagRow` (export + function decl + JSX usage)
- Prop `identifiers: Tag[]` → `tags: Tag[]` and destructured `identifiers` → `tags` (lines ~9, ~20, ~29, ~49, ~51)
- `identifiers.length === 0` → `tags.length === 0`
- `identifiers.map((identifier) => …)` → `tags.map((tag) => …)`. Inside the body the prop passed to `<TagRow>` is `identifier={identifier}` → `tag={tag}`. Same for the `key={identifier.id}` → `key={tag.id}`.
- The TagRow component's prop interface: `identifier: Tag` → `tag: Tag`. Inside its body, every `identifier.id`/`identifier.type`/`identifier.value`/`identifier.is_active` → `tag.id`/`tag.type`/`tag.value`/`tag.is_active`.
- Prop `onIdentifierRemoved?: (identifierId: number) => void` → `onTagRemoved?: (tagId: number) => void` in `TagListProps`
- Destructured `onIdentifierRemoved` → `onTagRemoved` and `canDelete = … && onIdentifierRemoved !== undefined` → `canDelete = … && onTagRemoved !== undefined`
- The `onDelete={canDelete ? onIdentifierRemoved : undefined}` → `onDelete={canDelete ? onTagRemoved : undefined}`
- Inside `TagRow`, prop `onDelete?: (identifierId: number) => void` → `onDelete?: (tagId: number) => void`
- aria-labels: `\`Locate tag ${identifier.value}\`` → `\`Locate tag ${tag.value}\``, `\`Remove tag ${identifier.value}\`` → `\`Remove tag ${tag.value}\``
- API call sites: `assetsApi.removeTag(entityId, identifier.id)` → `assetsApi.removeTag(entityId, tag.id)`, `locationsApi.removeTag(entityId, identifier.id)` → `locationsApi.removeTag(entityId, tag.id)`, `onDelete(identifier.id)` → `onDelete(tag.id)`

Sanity grep:

```bash
git grep -n 'identifier\|Identifier' frontend/src/components/assets/TagList.tsx
```

Expected: zero matches.

- [ ] **Step 3: Update the barrel export**

Edit `frontend/src/components/assets/index.ts`. Find:

```typescript
export { TagIdentifierList } from './TagIdentifierList';
```

Replace with:

```typescript
export { TagList, TagRow } from './TagList';
```

(Verify whether `TagIdentifierRow` is currently re-exported from the barrel; if it isn't but is imported elsewhere, this re-export is fine. If it isn't imported anywhere outside the file, drop the `, TagRow` part.)

Quick check:

```bash
git grep -n 'TagIdentifierRow\|TagRow' frontend/src
```

If `TagRow` is only used inside `TagList.tsx`, the barrel line is just `export { TagList } from './TagList';`.

- [ ] **Step 4: Update callers**

For each caller, replace the import name `TagIdentifierList` with `TagList`, and rename the prop `identifiers={…}` → `tags={…}` and `onIdentifierRemoved={…}` → `onTagRemoved={…}` on each JSX usage. Also update any JSX comments `{/* Tag Identifiers */}` → `{/* Tags */}`.

Files (verify line numbers with `git grep -n 'TagIdentifierList' <file>` if needed):

- `frontend/src/components/assets/AssetDetailsModal.tsx`
- `frontend/src/components/assets/AssetCard.tsx` — note this file uses both TagsModal (Task 3) and TagList; only the TagList portions are touched here
- `frontend/src/components/locations/LocationDetailsPanel.tsx`
- `frontend/src/components/locations/LocationDetailsModal.tsx`
- `frontend/src/components/locations/LocationDetailsPanel.test.tsx` (test file using the symbol)

- [ ] **Step 5: Verify typecheck**

```bash
just frontend typecheck
```

Expected: PASS.

- [ ] **Step 6: Run unit tests**

```bash
just frontend test
```

Expected: 1128 passing, 26 skipped, 0 failures.

- [ ] **Step 7: Commit**

```bash
git add frontend/src
git commit -m "$(cat <<'EOF'
refactor(frontend): rename TagIdentifierList component → TagList (TRA-525)

Includes internal TagIdentifierHeader→TagHeader, TagIdentifierRow→TagRow.
Props renamed identifiers→tags, onIdentifierRemoved→onTagRemoved.
EOF
)"
```

---

## Task 5: Rename component `TagIdentifierInputRow` → `TagInputRow`

**Files:**
- Rename: `frontend/src/components/assets/TagIdentifierInputRow.tsx` → `frontend/src/components/assets/TagInputRow.tsx`
- Rename: `frontend/src/components/assets/TagIdentifierInputRow.test.tsx` → `frontend/src/components/assets/TagInputRow.test.tsx`
- Modify: `frontend/src/components/assets/index.ts` (barrel)
- Modify (import + JSX): `frontend/src/components/assets/AssetForm.tsx`, `frontend/src/components/locations/LocationForm.tsx`

- [ ] **Step 1: Rename source + test files**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
git mv frontend/src/components/assets/TagIdentifierInputRow.tsx frontend/src/components/assets/TagInputRow.tsx
git mv frontend/src/components/assets/TagIdentifierInputRow.test.tsx frontend/src/components/assets/TagInputRow.test.tsx
```

- [ ] **Step 2: Rename symbol and props in the moved source file**

Edit `frontend/src/components/assets/TagInputRow.tsx`. Apply:

- `TagIdentifierInputRowProps` → `TagInputRowProps`
- `TagIdentifierInputRow` → `TagInputRow`

The internal field names (type, value, etc.) come from `TagInput` already; nothing else to rename in this file.

Sanity grep:

```bash
git grep -n 'TagIdentifier\|Identifier' frontend/src/components/assets/TagInputRow.tsx
```

Expected: zero matches.

- [ ] **Step 3: Update the test file**

Edit `frontend/src/components/assets/TagInputRow.test.tsx`. Replace all `TagIdentifierInputRow` → `TagInputRow` (import, JSX, describe strings). Replace describe/it text references.

Sanity:

```bash
git grep -n 'TagIdentifier' frontend/src/components/assets/TagInputRow.test.tsx
```

Expected: zero matches.

- [ ] **Step 4: Update the barrel**

Edit `frontend/src/components/assets/index.ts`. Find:

```typescript
export { TagIdentifierInputRow } from './TagIdentifierInputRow';
```

Replace with:

```typescript
export { TagInputRow } from './TagInputRow';
```

- [ ] **Step 5: Update callers**

Edit `frontend/src/components/assets/AssetForm.tsx`:
- Import `TagIdentifierInputRow` → `TagInputRow`
- JSX usage `<TagIdentifierInputRow …>` → `<TagInputRow …>`

Edit `frontend/src/components/locations/LocationForm.tsx`:
- Same: import + JSX

- [ ] **Step 6: Verify typecheck**

```bash
just frontend typecheck
```

Expected: PASS.

- [ ] **Step 7: Run unit tests**

```bash
just frontend test
```

Expected: 1128 passing, 26 skipped, 0 failures.

- [ ] **Step 8: Commit**

```bash
git add frontend/src
git commit -m "$(cat <<'EOF'
refactor(frontend): rename TagIdentifierInputRow component → TagInputRow (TRA-525)

File + symbol + props interface + test file + barrel + callers.
EOF
)"
```

---

## Task 6: Rename component-internal state and handlers (concept #2)

**Goal:** Sweep remaining concept-#2 internal names — handler functions, form state, helper variables — across components that already have new prop/component names.

**Files:**
- Modify: `frontend/src/components/assets/AssetForm.tsx`, `frontend/src/components/locations/LocationForm.tsx`, `frontend/src/components/assets/AssetCard.tsx`, `frontend/src/components/locations/LocationCard.tsx`, `frontend/src/components/assets/AssetDetailsModal.tsx`, `frontend/src/components/locations/LocationDetailsPanel.tsx`, `frontend/src/components/locations/LocationDetailsModal.tsx`

- [ ] **Step 1: Rename `handleIdentifierRemoved` → `handleTagRemoved` everywhere**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
git grep -l 'handleIdentifierRemoved' frontend/src | xargs perl -i -pe 's/\bhandleIdentifierRemoved\b/handleTagRemoved/g'
```

Verify the prop wiring is consistent — every `<TagsModal …>` and `<TagList …>` usage now passes `onTagRemoved={handleTagRemoved}`:

```bash
git grep -n 'onTagRemoved=\|handleTagRemoved' frontend/src
```

Expected: matched pairs (each `handleTagRemoved` defined in a parent passes through to a child via `onTagRemoved={…}`).

Also check there are no stale references:

```bash
git grep -n 'onIdentifierRemoved\|handleIdentifierRemoved' frontend/src
```

Expected: zero matches.

- [ ] **Step 2: Rename `identifierId` parameter and `identifierId` local var → `tagId` inside callbacks**

Inside the renamed `handleTagRemoved` callbacks (in AssetCard, LocationCard, AssetDetailsModal, LocationDetailsPanel, LocationDetailsModal), the param is `(identifierId: number)` and the body uses `identifierId`. Rename:

```bash
git grep -l 'handleTagRemoved' frontend/src | xargs perl -i -pe 's/\bidentifierId\b/tagId/g'
```

Verify:

```bash
git grep -n '\bidentifierId\b' frontend/src
```

Expected: zero matches.

- [ ] **Step 3: Rename form state `tagIdentifiers` → `tagInputs` in AssetForm and LocationForm**

Edit `frontend/src/components/assets/AssetForm.tsx`. Apply replacement (word-boundary):

```bash
perl -i -pe 's/\btagIdentifiers\b/tagInputs/g; s/\bsetTagIdentifiers\b/setTagInputs/g' frontend/src/components/assets/AssetForm.tsx
```

Then sanity-check the loop variable inside the `.map((identifier, index) => …)` block — rename loop variable `identifier` → `tagInput` to keep the file consistent. Find this block (around line 500):

```typescript
{tagInputs.map((identifier, index) => (
  <TagInputRow
    key={identifier.id ?? `new-${index}`}
    type={identifier.type}
    value={identifier.value}
    ...
```

Replace inside that block: `identifier` → `tagInput`. Other `identifier` symbols in this file (`formData.identifier`, `htmlFor="identifier"`, `id="identifier"`, `fieldErrors.identifier`, `errors.identifier`) are concept #1 — leave alone.

Repeat for `frontend/src/components/locations/LocationForm.tsx`:

```bash
perl -i -pe 's/\btagIdentifiers\b/tagInputs/g; s/\bsetTagIdentifiers\b/setTagInputs/g' frontend/src/components/locations/LocationForm.tsx
```

Then manually rename the loop variable inside the `.map((identifier, index) => …)` block (around line 530) the same way: `identifier` → `tagInput`. The line `getLocationByIdentifier(...)` and `validateIdentifier(...)` and `formData.identifier` references are concept #1, leave alone.

Verify no stale form-state names remain:

```bash
git grep -n '\btagIdentifiers\b\|\bsetTagIdentifiers\b' frontend/src
```

Expected: zero matches.

- [ ] **Step 4: Rename pre-submit local `validIdentifiers` → `validTags` in AssetForm and LocationForm**

```bash
git grep -l '\bvalidIdentifiers\b' frontend/src | xargs perl -i -pe 's/\bvalidIdentifiers\b/validTags/g'
```

Verify:

```bash
git grep -n '\bvalidIdentifiers\b' frontend/src
```

Expected: zero matches.

- [ ] **Step 5: Sweep `LocationFormModal.tsx` and `AssetFormModal.tsx` for any inner `identifiers` / `identifier` concept-#2 vars left over**

Open each file and audit. The earlier grep showed lines like:

```typescript
const identifiers = (data as CreateAssetRequest & { tags?: TagInput[] }).tags || [];
const validIdentifiers = identifiers.filter(id => id.value.trim() !== '');
for (const identifier of validIdentifiers) {
```

Rename inside these blocks: `identifiers` → `tagsToCreate` (or just `newTags` — `tags` would shadow the destructured field), `validIdentifiers` was already covered in Step 4 and renamed to `validTags`, the `for (const identifier of validTags)` loop variable becomes `for (const tag of validTags)` and the body's `identifier.type` / `identifier.value` references update to `tag.type` / `tag.value`.

Same for the `newIdentifiers = identifiers.filter(id => !id.id)` line in the update branch — rename `newIdentifiers` → `newTags`.

After: sanity-grep both files for any remaining concept-#2 `identifier` or `identifiers` variable names. Don't touch `Asset.identifier` / `Location.identifier` field reads (e.g., `normalized.identifier`) — those are concept #1.

```bash
git grep -n 'identifier' frontend/src/components/assets/AssetFormModal.tsx
git grep -n 'identifier' frontend/src/components/locations/LocationFormModal.tsx
```

Acceptable matches (concept #1, leave alone): `normalized.identifier`, `location?.identifier`, anything reading the natural-key field. Anything else gets fixed.

- [ ] **Step 6: Verify typecheck**

```bash
just frontend typecheck
```

Expected: PASS.

- [ ] **Step 7: Run unit tests**

```bash
just frontend test
```

Expected: 1128 passing, 26 skipped, 0 failures.

- [ ] **Step 8: Commit**

```bash
git add frontend/src
git commit -m "$(cat <<'EOF'
refactor(frontend): rename concept-#2 internal state/handlers to tag (TRA-525)

handleIdentifierRemoved→handleTagRemoved, identifierId→tagId,
tagIdentifiers→tagInputs, validIdentifiers→validTags, plus
form-modal local var sweep. Concept-#1 natural-key vars untouched.
EOF
)"
```

---

## Task 7: UI label updates (concept #2)

**Files:**
- Modify: `frontend/src/components/locations/LocationForm.tsx`

- [ ] **Step 1: Rename the section heading**

Edit `frontend/src/components/locations/LocationForm.tsx`. Find (around line 462):

```typescript
            Tag Identifiers
```

Replace with:

```typescript
            Tags
```

(The full surrounding line should be inside a heading element — verify the exact JSX shape and replace just the visible text.)

- [ ] **Step 2: Update the empty-state copy**

Find (around line 526):

```typescript
            No tag identifiers added. Click &quot;Add Tag&quot; to link RFID tags.
```

Replace with:

```typescript
            No tags added. Click &quot;Add Tag&quot; to link RFID tags.
```

- [ ] **Step 3: Verify no concept-#2 "Tag Identifier(s)" copy remains in JSX**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
git grep -n 'Tag Identifier\|tag identifier' frontend/src
```

Expected: zero matches. (Note: AssetForm already says "Tags" / "Add Tag" — no copy change there.)

- [ ] **Step 4: Verify typecheck + tests**

```bash
just frontend typecheck && just frontend test
```

Expected: PASS, 1128 passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "$(cat <<'EOF'
refactor(frontend): rename concept-#2 UI labels to Tags (TRA-525)

LocationForm section heading + empty-state copy.
Concept-#1 "Identifier" labels (Asset/Location natural key) unchanged.
EOF
)"
```

---

## Task 8: E2E test sweep (B-lite)

**Files:**
- Audit: `frontend/tests/e2e/`

- [ ] **Step 1: Confirm no concept-#2 "Tag Identifier" wording exists**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
grep -rn 'Tag Identifier\|tag identifier\|TagIdentifier' frontend/tests/e2e/
```

Expected: zero matches (verified during planning). If anything appears, fix it inline.

- [ ] **Step 2: Confirm no breakable selectors target the renamed UI**

```bash
grep -rn 'getByText.*Tag Identifier\|getByRole.*Tag Identifier\|locator.*Tag Identifier' frontend/tests/e2e/
```

Expected: zero matches. If any appear, update them to "Tags".

- [ ] **Step 3: Skim describe/it strings for any obvious concept-#2 phrasing**

Open these files and read describe/it lines (they should already use concept-#1 wording for `location.identifier` etc., which is fine):

```bash
ls frontend/tests/e2e/*.spec.ts
```

If a describe/it explicitly references "tag identifier" or "TagIdentifier", reword. Otherwise no change.

- [ ] **Step 4: Note no commit if zero changes**

If steps 1–3 produced no edits, skip the commit and proceed. Otherwise:

```bash
git add frontend/tests/e2e
git commit -m "$(cat <<'EOF'
test(e2e): align concept-#2 wording to Tag (TRA-525)

B-lite sweep — fix obvious references; no comprehensive audit.
EOF
)"
```

---

## Task 9: Final validation, push, PR

- [ ] **Step 1: Run full validate**

```bash
cd /home/mike/platform/.worktrees/tra-525-frontend-tag-rename
just frontend validate
```

Expected: typecheck PASS, lint PASS, 1128 unit tests passing, build PASS.

- [ ] **Step 2: Sanity-grep — no concept-#2 stragglers in src**

```bash
git grep -nE '\b(TagIdentifier|tagIdentifier|TagIdentifiers)\b' frontend/src
```

Expected: zero matches.

```bash
git grep -nE '\b(handleIdentifierRemoved|onIdentifierRemoved|identifierId)\b' frontend/src
```

Expected: zero matches.

- [ ] **Step 3: Verify concept-#1 surfaces are intact (spot-check)**

These should ALL still be present (proves we didn't accidentally rename concept #1):

```bash
git grep -n "label: 'Identifier'" frontend/src/components/assets/AssetSearchSort.tsx
git grep -n "label: 'Identifier'" frontend/src/components/locations/LocationSearchSort.tsx
git grep -n "label: 'Identifier'" frontend/src/components/locations/LocationTable.tsx
git grep -n 'Identifier <span' frontend/src/components/locations/LocationForm.tsx
git grep -n 'htmlFor="identifier"' frontend/src/components/locations/LocationForm.tsx
git grep -n 'Tag EPC Identifier' frontend/src/components/LocateScreen.tsx
git grep -n 'getLocationByIdentifier\|validateIdentifier' frontend/src
```

Each command should produce at least one match — if any return empty, the corresponding concept-#1 surface was wrongly renamed; revert.

- [ ] **Step 4: Push the branch**

```bash
git push -u origin feat/tra-525-frontend-tag-rename
```

- [ ] **Step 5: Open PR**

```bash
gh pr create --title "feat(frontend): TRA-525 complete Identifier→Tag rename for concept-#2 surfaces" --body "$(cat <<'EOF'
## Summary

Finishes the `Identifier` → `Tag` rename arc on the frontend after TRA-524 shipped the API/DB cutover. Concept-#2 (physical-tag entity) types, components, props, internal state, and labels rename to `Tag` / `Tags`. Concept-#1 (`identifier` natural-key) surfaces stay unchanged.

- Type renames: `TagIdentifier` → `Tag`, `TagIdentifierInput` → `TagInput`
- Component renames: `TagIdentifiersModal` → `TagsModal`, `TagIdentifierList` → `TagList` (with internal `TagHeader`/`TagRow`), `TagIdentifierInputRow` → `TagInputRow`
- Internal renames: `tagIdentifiers` → `tagInputs`, `handleIdentifierRemoved` → `handleTagRemoved`, `identifierId` → `tagId`, prop `identifiers` → `tags`, prop `onIdentifierRemoved` → `onTagRemoved`
- Label updates: `LocationForm` "Tag Identifiers" → "Tags" (heading + empty-state)

Backend Go type `TagIdentifier` deliberately left as-is (TRA-524 marked that as a future call). JSON wire format unchanged.

Spec: `docs/superpowers/specs/2026-04-27-tra-525-frontend-tag-rename-design.md`
Plan: `docs/superpowers/plans/2026-04-27-tra-525-frontend-tag-rename.md`

## Test plan

- [x] `just frontend validate` — typecheck + lint + 1128 unit tests passing
- [ ] Preview e2e (Playwright) green after deploy
- [ ] Manual smoke: open AssetCard tag list, open LocationCard tag list, edit AssetForm tag rows, edit LocationForm tag rows, remove a tag

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 6: Move TRA-525 to In Review in Linear**

After PR opens, update the Linear ticket status to "In Review" and link the PR.

---

## Out of scope (not in this plan)

- Backend Go type `TagIdentifier` (TRA-524 left as future)
- Renaming `Asset.identifier` / `Location.identifier` / `parent_identifier` / `validateIdentifier` / `getLocationByIdentifier` (concept #1)
- Restructuring `TagList.tsx` to split header/row into separate files
- Behavior changes
