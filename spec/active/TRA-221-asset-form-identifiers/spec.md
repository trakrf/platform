# Feature: Asset Form Tag Identifiers (TRA-221 Extension)

## Metadata
**Workspace**: frontend
**Type**: feature
**Parent**: TRA-221 (Frontend Asset View with Tag Identifiers)

## Outcome
Users can add, edit, and remove tag identifiers when creating or updating assets.

## User Story
As an asset manager
I want to link RFID tags to assets when creating or editing them
So that I can associate physical tags with asset records

## Technical Requirements

### 1. Update AssetForm Component

#### Location: `frontend/src/components/assets/AssetForm.tsx`

Add "Tag Identifiers" section after the main form fields:

```
+---------------------------------------------------+
| Create/Edit Asset                                  |
+---------------------------------------------------+
| Identifier *        | Name *                       |
| [_______________]   | [_______________]            |
|                                                    |
| Type *              | Location                     |
| [Asset ▼]           | [Select location ▼]         |
|                                                    |
| [✓] Active                                         |
|                                                    |
| Description                                        |
| [___________________________________]              |
|                                                    |
| Valid From          | Valid To                     |
| [_______________]   | [_______________]            |
+---------------------------------------------------+
| Tag Identifiers                              [+]   |
| +-----------------------------------------------+ |
| | Type: [RFID ▼]  Value: [____________] [🗑️]   | |
| | Type: [RFID ▼]  Value: [____________] [🗑️]   | |
| +-----------------------------------------------+ |
| [+ Add Tag Identifier]                             |
+---------------------------------------------------+
|                    [Cancel] [Create/Update Asset]  |
+---------------------------------------------------+
```

### 2. Tag Identifier Input Row

Each tag identifier row contains:
- **Type dropdown**: RFID, BLE, Barcode (default: RFID)
- **Value input**: Text field for tag ID
- **Remove button**: Delete icon to remove the row

### 3. Form State

```typescript
interface TagIdentifierInput {
  type: 'rfid' | 'ble' | 'barcode';
  value: string;
  id?: number; // Only for existing identifiers in edit mode
}

// Add to form state
const [tagIdentifiers, setTagIdentifiers] = useState<TagIdentifierInput[]>([]);
```

### 4. API Integration

#### Create Asset Request
```typescript
interface CreateAssetRequest {
  // ... existing fields ...
  identifiers?: Array<{
    type: string;
    value: string;
  }>;
}
```

#### Update Asset Request
```typescript
interface UpdateAssetRequest {
  // ... existing fields ...
  identifiers?: Array<{
    id?: number;  // Include for existing identifiers
    type: string;
    value: string;
  }>;
}
```

### 5. Validation

- Tag value must not be empty
- Tag value must be unique within the asset
- Type must be one of: rfid, ble, barcode

## Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `frontend/src/components/assets/AssetForm.tsx` | Modify | Add tag identifiers section |
| `frontend/src/types/assets/index.ts` | Modify | Update request types if needed |

## Validation Criteria

- [ ] Tag identifier section visible in create/edit form
- [ ] Can add new tag identifiers
- [ ] Can remove tag identifiers
- [ ] Type dropdown with RFID/BLE/Barcode options
- [ ] Value input field for tag ID
- [ ] Identifiers included in create/update API request
- [ ] Edit mode pre-populates existing identifiers
- [ ] Form validation for empty values

## UI/UX Notes

### Tag Type Options
- RFID (default)
- BLE
- Barcode

### Add Button
- Text: "+ Add Tag Identifier"
- Position: Below existing identifiers

### Remove Button
- Trash icon (Trash2 from lucide-react)
- Red color on hover
- Confirms removal (immediate, no modal)

### Styling
- Same card style as identifier list view
- Consistent with existing form field styling
