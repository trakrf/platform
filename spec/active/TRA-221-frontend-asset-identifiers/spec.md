# Feature: Frontend Asset View with Tag Identifiers (TRA-221)

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-221
**Parent**: TRA-193 (Asset CRUD - separate customer identifier from tag identifiers)
**Depends On**: TRA-214 (Backend transactional identifiers - COMPLETED)

## Outcome
Users can view RFID tag identifiers linked to assets in the asset list and details modal.

## User Story
As an asset manager
I want to see which RFID tags are linked to each asset
So that I can verify tag assignments and troubleshoot scanning issues

## Context

**Current State**:
- Backend (TRA-214) already returns `AssetView` with `identifiers[]` from all GET endpoints
- Frontend `Asset` type does NOT include identifiers
- Frontend details/list views do NOT show identifiers

**Desired State**:
- Frontend types match backend `AssetView` response (Asset + identifiers)
- Asset details modal shows all linked identifiers
- Asset list/card shows identifier count indicator

**Backend API Reference** (TRA-214 completed):
```go
// backend/internal/models/shared/identifier.go
type TagIdentifier struct {
    ID       int    `json:"id,omitempty"`
    Type     string `json:"type" validate:"required,oneof=rfid ble barcode"`
    Value    string `json:"value" validate:"required,min=1,max=255"`
    IsActive bool   `json:"is_active"`
}

// backend/internal/models/asset/asset.go
type AssetView struct {
    Asset
    Identifiers []shared.TagIdentifier `json:"identifiers"`
}
```

## Technical Requirements

### 1. Type Updates

#### New: `TagIdentifier` type
```typescript
// frontend/src/types/shared/identifier.ts
export type IdentifierType = 'rfid'; // Only RFID for this version

export interface TagIdentifier {
  id: number;
  type: IdentifierType;
  value: string;
  is_active: boolean;
}
```

#### Updated: `Asset` type (now includes identifiers)
```typescript
// frontend/src/types/assets/index.ts
export interface Asset {
  // ... existing fields ...
  identifiers: TagIdentifier[]; // NEW: Tag identifiers from backend
}
```

### 2. Asset Details Modal Changes

#### Location: `frontend/src/components/assets/AssetDetailsModal.tsx`

Add "Linked Identifiers" section after Location field:

```
+---------------------------------------------------+
| Asset Details                                      |
+---------------------------------------------------+
| Identifier: LAP-001  |  Name: Engineering Laptop  |
| Type: Asset          |  Status: [Active]          |
| Location: [icon] HQ Building                      |
+---------------------------------------------------+
| Linked Identifiers                                 |
| +-----------------------------------------------+ |
| | [RFID icon] E20000000000001234     [Active]  | |
| | [RFID icon] E20000000000005678     [Active]  | |
| +-----------------------------------------------+ |
| -- OR if empty --                                  |
| No tag identifiers linked                          |
+---------------------------------------------------+
| Valid From / Valid To / Created / Updated          |
+---------------------------------------------------+
```

**Requirements**:
1. Section title: "Linked Identifiers"
2. Show empty state if no identifiers: "No tag identifiers linked"
3. Show RFID icon (`<Radio />`) for each identifier
4. Show status badge: Active (green) / Inactive (gray)
5. Truncate long values with ellipsis, show full on hover/title

### 3. Asset List/Card Changes

#### Location: `frontend/src/components/assets/AssetCard.tsx`

Add identifier count badge:

```
+-----------------------------------------------+
| [Asset]  LAP-001            [2 tags] [Active] |
| Engineering Laptop          HQ Building       |
+-----------------------------------------------+
```

**Requirements**:
1. Show badge only if `identifiers.length > 0`
2. Badge text: `{count} tag` or `{count} tags` (pluralize)
3. Badge with RFID icon
4. Tooltip on hover: "{count} RFID tag(s) linked"

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `frontend/src/types/shared/identifier.ts` | New | TagIdentifier type |
| `frontend/src/types/shared/index.ts` | New | Export shared types |
| `frontend/src/types/assets/index.ts` | Modify | Add identifiers to Asset |
| `frontend/src/components/assets/AssetDetailsModal.tsx` | Modify | Add Linked Identifiers section |
| `frontend/src/components/assets/AssetCard.tsx` | Modify | Add tag count badge |

## Validation Criteria

- [ ] Asset type includes `identifiers: TagIdentifier[]`
- [ ] Asset details modal shows linked identifiers section
- [ ] Empty state displayed when no identifiers
- [ ] Each identifier shows: RFID icon, value, status badge
- [ ] Asset list/card shows identifier count badge
- [ ] Badge hidden when no identifiers
- [ ] No regression in existing asset views

## Success Metrics

- [ ] View asset with identifiers - all displayed correctly
- [ ] View asset without identifiers - empty state shown
- [ ] Asset list shows count badge for assets with identifiers
- [ ] No TypeScript errors
- [ ] All existing tests still pass

## Out of Scope (Future Work)

1. **Create identifiers** - Adding identifiers when creating asset
2. **Edit identifiers** - Modifying existing identifiers
3. **Delete identifiers** - Removing identifiers from asset
4. **BLE/Barcode types** - Only RFID for this version

## UI/UX Notes

### Identifier Icon
- RFID: `<Radio className="w-4 h-4" />` (lucide-react)

### Status Badge Colors
- Active: `bg-green-100 text-green-800` / dark: `bg-green-900/30 text-green-300`
- Inactive: `bg-gray-100 text-gray-800` / dark: `bg-gray-700 text-gray-300`

### Tag Count Badge
- Style: `bg-blue-100 text-blue-700` with RFID icon
- Position: After identifier, before status badge in card

## References

- [TRA-214 Spec](../TRA-214-transactional-identifiers/spec.md) - Backend implementation
- [Backend TagIdentifier model](backend/internal/models/shared/identifier.go)
- [Backend AssetView model](backend/internal/models/asset/asset.go)
- [Current AssetDetailsModal](frontend/src/components/assets/AssetDetailsModal.tsx)
- [Current AssetCard](frontend/src/components/assets/AssetCard.tsx)
